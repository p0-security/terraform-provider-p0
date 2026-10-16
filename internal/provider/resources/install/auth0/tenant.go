package installauth0

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/p0-security/terraform-provider-p0/internal"
	"github.com/p0-security/terraform-provider-p0/internal/common"
	installresources "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install"
)

// copy of internal/provider/resources/install/docker/organization.go, renamed for Auth0
// (INT-590), keyed by the tenant's canonical domain and with the application's client
// ID.

var _ resource.Resource = &Auth0Tenant{}
var _ resource.ResourceWithConfigure = &Auth0Tenant{}
var _ resource.ResourceWithImportState = &Auth0Tenant{}
var _ resource.ResourceWithModifyPlan = &Auth0Tenant{}
var _ resource.ResourceWithValidateConfig = &Auth0Tenant{}

type Auth0Tenant struct {
	installer *common.Install
}

type auth0TenantModel struct {
	Id               types.String `tfsdk:"id"`
	Connector        types.String `tfsdk:"connector"`
	Vault            *vaultModel  `tfsdk:"vault"`
	ClientId         types.String `tfsdk:"client_id"`
	ClientSecretName types.String `tfsdk:"client_secret_name"`
	AllowedDomains   types.String `tfsdk:"allowed_domains"`
	State            types.String `tfsdk:"state"`
}

type auth0TenantJson struct {
	Connector        *string    `json:"connector,omitempty"`
	Vault            *vaultJson `json:"vault,omitempty"`
	ClientId         *string    `json:"clientId,omitempty"`
	ClientSecretName *string    `json:"clientSecretName,omitempty"`
	AllowedDomains   *string    `json:"allowedDomains,omitempty"`
	State            *string    `json:"state,omitempty"`
}

type auth0TenantApi struct {
	Item *auth0TenantJson `json:"item"`
}

func NewAuth0Tenant() resource.Resource {
	return &Auth0Tenant{}
}

func (*Auth0Tenant) Metadata(_ context.Context, req resource.MetadataRequest, res *resource.MetadataResponse) {
	res.TypeName = req.ProviderTypeName + "_auth0_tenant"
}

func (*Auth0Tenant) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `An Auth0 tenant that P0 can grant roles, permissions and organization memberships in, through one of your Auth0 connectors.

The connector acts on the tenant as a machine-to-machine application authorized for the Auth0 Management API. P0 never holds the application's client secret. You store it in your own secret manager, in the connector's cloud, and the connector reads it on every call. This resource tells P0 which connector acts on the tenant, the application's client ID, and where the client secret is.

**Important:** Create the application, store its client secret and let the connector read it before Terraform creates this resource. Do that in an earlier apply, or in the same apply with this resource depending on the connector's read grant, as the example does. Creating this resource has P0 validate the install through the connector: the connector reads the secret, exchanges the client ID and secret for a Management API token, checks that the token carries every scope the connector needs, and reads back its own ` + "`DOMAIN_ALLOW_PATTERN`" + `. If a check fails, the apply fails with P0's message. IAM changes can take a few seconds to propagate, so if the connector is denied the secret right after you grant it, apply again.

P0 fixes the connector, the vault, the client ID and the secret when it adds the tenant, so changing ` + "`id`" + `, ` + "`connector`" + `, ` + "`vault`" + `, ` + "`client_id`" + ` or ` + "`client_secret_name`" + ` replaces the installation. To rotate the client secret, store the new one as a new version of the same secret, which needs no change here. If the tenant is already added in P0, import it rather than creating it: P0 refuses to add a tenant twice. If P0 hasn't finished installing the tenant, for example because you imported it before its checks passed, Terraform plans an update, and applying it finishes the install.

**Prerequisites:**

- An Auth0 connector, installed with ` + "`p0_auth0_connector`" + `. The client secret must be in the connector's cloud: AWS Secrets Manager for a connector on AWS Lambda, or Google Secret Manager for one on Cloud Run. It can be in another account or project.

- A machine-to-machine application in the tenant, authorized for the Auth0 Management API with these scopes: ` + managementScopes + `. Store its client secret outside Terraform, as the secret's whole value, in the account and region or the project that ` + "`vault`" + ` names. Storing it outside Terraform keeps it out of Terraform's state.

- Read access to that one secret, and no other, for the connector's identity (its ` + "`connector_identity`" + `). P0 names the secret in each call, so a call could point the connector at any other secret it can read.

  On AWS, give the connector's role ` + "`secretsmanager:GetSecretValue`" + ` on ` + "`arn:aws:secretsmanager:<secrets_region>:<account_id>:secret:<name>-??????`" + `. Secrets Manager adds a hyphen and six random characters to the name in a secret's ARN, and ` + "`??????`" + ` matches those, so the grant covers this one secret. If a customer-managed KMS key encrypts the secret, also give the role ` + "`kms:Decrypt`" + ` on that key, with the condition that ` + "`kms:ViaService`" + ` is ` + "`secretsmanager.<secrets_region>.amazonaws.com`" + `. A secret outside the connector's account needs a customer-managed key, because no other account can use the default ` + "`aws/secretsmanager`" + ` key. In the secret's account, the secret's resource policy and the key's policy must also allow the connector's role.

  On Google Cloud, give the connector's service account ` + "`roles/secretmanager.secretAccessor`" + ` and ` + "`roles/secretmanager.viewer`" + ` on the secret itself, not on its project.

- P0's secret manager integration for where the secret is: AWS Secrets Manager (` + "`p0_aws_secrets_manager`" + `) for the AWS account, or Google Secret Manager for the Google Cloud project.

**Note:** This integration is currently in beta.

For the full install steps, see [Auth0](https://docs.p0.dev/integrations/resource-integrations/auth0).`,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "The tenant's canonical Auth0 domain, such as `acme.us.auth0.com` or `acme.auth0.com`, in lowercase. " +
					"A custom domain can't be used: the Auth0 Management API only answers on the canonical one.",
				Validators: []validator.String{tenantDomainValidator{}},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"connector": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The connector that acts on this tenant: the `id` of a `p0_auth0_connector` resource. Requires that connector to be installed.",
				Validators:          connectorIdValidators(),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"vault": vaultAttribute(
				`The secret manager that holds the client secret. Your connector reads the secret, and P0 never does. It must be in the connector's cloud, and can be in another account or project there.`,
				secretPlace.what,
			),
			"client_id": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "The client ID of the machine-to-machine application the connector acts as, authorized for the Auth0 Management API, as the application's settings show it. " +
					"Changing it replaces the installation.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"client_secret_name": schema.StringAttribute{
				Required: true,
				MarkdownDescription: `The name of the secret that holds the application's client secret, such as ` + "`auth0/acme/mgmt`" + ` in AWS Secrets Manager or ` + "`auth0-acme-mgmt`" + ` in Google Secret Manager: the name alone, not the secret's ARN or its full resource name. ` +
					`In AWS Secrets Manager the name can't end in a hyphen and six letters or digits, like ` + "`-AbCdEf`" + `, because Secrets Manager can't find a secret by such a name. ` +
					`Changing it replaces the installation. To rotate the client secret, store the new one as a new version of the same secret, which needs no change here.`,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"allowed_domains": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The email domains the connector acts for: the `DOMAIN_ALLOW_PATTERN` it's deployed with, which P0 reads back from the connector when it checks the install. To change it, change the connector's deployment.",
			},
			"state": common.StateAttribute,
		},
	}
}

// Checking here rather than in Create reports a mistake at plan time, before the
// provider calls P0, in P0's words. A value that Terraform only knows at apply time
// is skipped while planning, and checked when Terraform validates the configuration
// again before it applies.
func (*Auth0Tenant) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var clientId, secretName types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("client_id"), &clientId)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("client_secret_name"), &secretName)...)
	vault := vaultFromConfig(ctx, req.Config, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	validateClientId(clientId, &resp.Diagnostics)
	validateClientSecret(vault, secretName, &resp.Diagnostics)
}

// P0's check (`validateClientId` in
// packages/integrations/auth0/backend/src/install/validation.ts in the app). Like P0,
// it checks the ID without the whitespace around it, and never repeats it, because a
// customer may have pasted the client secret in its place.
func validateClientId(clientId types.String, diags *diag.Diagnostics) {
	validateField(path.Root("client_id"), clientId, []rule{{
		summary: "Invalid client ID",
		message: invalidClientId,
		test:    ClientIdRegex.MatchString,
	}}, diags)
}

// Refuses a tenant ID that isn't a canonical Auth0 domain, with P0's message
// (`validateTenantDomain` in
// packages/integrations/auth0/backend/src/install/validation.ts in the app). P0 checks
// the ID as it is, without trimming it. The framework's stringvalidator.RegexMatches
// would add the value after P0's message, which already ends in a full stop.
type tenantDomainValidator struct{}

var _ validator.String = tenantDomainValidator{}

func (tenantDomainValidator) Description(context.Context) string {
	return invalidTenantDomain
}

func (v tenantDomainValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (tenantDomainValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if !TenantRegex.MatchString(req.ConfigValue.ValueString()) {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid Auth0 tenant domain", invalidTenantDomain)
	}
}

func (r *Auth0Tenant) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:        Auth0Key,
		Component:          installresources.Tenant,
		ProviderData:       data,
		GetId:              r.getId,
		GetItemJson:        r.getItemJson,
		FromJson:           r.fromJson,
		ToJson:             r.toJson,
		DescribeCheckError: describeCheckError("tenant"),
	}
}

func (r *Auth0Tenant) getId(data any) *string {
	model, ok := data.(*auth0TenantModel)
	if !ok {
		return nil
	}
	str := model.Id.ValueString()
	return &str
}

// A response with no item is a bad response, not an item to read. Its nil item,
// returned as it is, wouldn't compare equal to nil, and would be read as one.
func (r *Auth0Tenant) getItemJson(json any) any {
	inner, ok := json.(*auth0TenantApi)
	if !ok || inner.Item == nil {
		return nil
	}
	return inner.Item
}

func (r *Auth0Tenant) fromJson(_ context.Context, diags *diag.Diagnostics, id string, json any) any {
	jsonv, ok := json.(*auth0TenantJson)
	if !ok {
		return nil
	}

	// vault is Required, so a nil vault would surface as Terraform core's "provider
	// produced inconsistent result after apply", which tells the reader nothing to do.
	if jsonv.Vault == nil {
		diags.AddError(
			"Bad API response",
			fmt.Sprintf("The Auth0 tenant %s carries no vault configuration. "+
				"Remove it in the P0 app and add it again, or remove it from Terraform state with `terraform state rm`.", id),
		)
		return nil
	}

	// A vault of a type this provider doesn't know can't be read into the attribute's
	// fields.
	if !slices.Contains(vaultTypes, jsonv.Vault.Type) {
		diags.AddError(
			"Unsupported Auth0 tenant",
			fmt.Sprintf("The Auth0 tenant %s has a %q vault. This provider supports only %s. "+
				"Upgrade the provider, manage the tenant in the P0 app, or remove it from Terraform state with `terraform state rm`.",
				id, jsonv.Vault.Type, strings.Join(vaultTypes, " and ")),
		)
		return nil
	}

	return &auth0TenantModel{
		Id:               types.StringValue(id),
		Connector:        types.StringPointerValue(jsonv.Connector),
		Vault:            vaultFromJson(jsonv.Vault),
		ClientId:         types.StringPointerValue(jsonv.ClientId),
		ClientSecretName: types.StringPointerValue(jsonv.ClientSecretName),
		AllowedDomains:   types.StringPointerValue(jsonv.AllowedDomains),
		State:            types.StringPointerValue(jsonv.State),
	}
}

// The body of the PUT that stages the item, and of verify and configure: the
// connector, the vault, the client ID and the secret's name, which P0 fixes when it
// creates the item. Sending them on each step has P0 refuse a request that would
// change any of them, before it calls the connector. P0 reads the allowed domains back
// from the connector, and sets the state.
func (r *Auth0Tenant) toJson(data any) any {
	datav, ok := data.(*auth0TenantModel)
	if !ok {
		return nil
	}

	json := auth0TenantJson{
		Connector:        datav.Connector.ValueStringPointer(),
		ClientId:         datav.ClientId.ValueStringPointer(),
		ClientSecretName: datav.ClientSecretName.ValueStringPointer(),
	}
	if datav.Vault != nil {
		json.Vault = datav.Vault.toJson()
	}
	return &json
}

// P0 checks every field before it creates the item, without repeating the value, which
// may be the client secret pasted in place of the secret's name or the client ID. So a
// value it refuses leaves nothing staged. A check that fails after that leaves the
// staged item in state, tainted, and the next apply replaces it, because P0 refuses a
// second PUT of a tenant that's already added.
func (r *Auth0Tenant) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json auth0TenantApi
	var data auth0TenantModel

	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	if resp.Diagnostics.HasError() {
		return
	}

	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, r.toJson(&data))
	if resp.Diagnostics.HasError() {
		return
	}

	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data)
}

func (r *Auth0Tenant) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &auth0TenantApi{}, &auth0TenantModel{})
}

// Plans an update for a tenant that P0 hasn't installed, so that the next apply
// finishes the install. P0 reads the allowed domains back from the connector during
// that apply, so they stay unknown until then.
func (*Auth0Tenant) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	planUnfinishedInstall(ctx, req, resp, path.Root("allowed_domains"))
}

// Every configurable attribute requires replacement, so the only update Terraform
// plans is the one that finishes an install, which verifies and configures the item as
// Create does. P0 validates the install on verify alone, so there is no shorter way.
func (r *Auth0Tenant) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &auth0TenantApi{}, &auth0TenantModel{})
}

func (r *Auth0Tenant) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Delete(ctx, &resp.Diagnostics, &req.State, &auth0TenantModel{})
}

func (r *Auth0Tenant) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// The Management API scopes for the resource's description, in the order P0's
// installer lists them (AUTH0_MANAGEMENT_SCOPES in
// packages/integrations/auth0/shared/src/scopes.ts in the app). The connector's
// install check requires every one.
const managementScopes = "`read:users`, `read:roles`, `read:role_members`, `update:users`, " +
	"`create:role_members`, `delete:role_members`, `read:resource_servers`, " +
	"`read:user_effective_permissions`, `read:organizations`, `read:organization_members`, " +
	"`read:organization_member_roles`, `create:organization_members`, " +
	"`delete:organization_members`, `create:organization_member_roles` and " +
	"`delete:organization_member_roles`"
