package installclickhouse

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

// copy of internal/provider/resources/install/auth0/tenant.go (#245), itself a copy
// of Docker Hub's organization.go, renamed for ClickHouse Cloud (INT-695): keyed by
// the organization's ID, with the API key's secret in place of the client ID and
// secret, and the four fields P0 reads when it validates the install.

var _ resource.Resource = &ClickHouseOrganization{}
var _ resource.ResourceWithConfigure = &ClickHouseOrganization{}
var _ resource.ResourceWithImportState = &ClickHouseOrganization{}
var _ resource.ResourceWithModifyPlan = &ClickHouseOrganization{}
var _ resource.ResourceWithValidateConfig = &ClickHouseOrganization{}

type ClickHouseOrganization struct {
	installer *common.Install
}

type clickhouseOrganizationModel struct {
	Id               types.String `tfsdk:"id"`
	Connector        types.String `tfsdk:"connector"`
	Vault            *vaultModel  `tfsdk:"vault"`
	ApiKeySecretName types.String `tfsdk:"api_key_secret_name"`
	OrganizationName types.String `tfsdk:"organization_name"`
	ApiKeyId         types.String `tfsdk:"api_key_id"`
	AllowedDomains   types.String `tfsdk:"allowed_domains"`
	ServiceStatus    types.String `tfsdk:"service_status"`
	State            types.String `tfsdk:"state"`
}

type clickhouseOrganizationJson struct {
	Connector        *string    `json:"connector,omitempty"`
	Vault            *vaultJson `json:"vault,omitempty"`
	ApiKeySecretName *string    `json:"apiKeySecretName,omitempty"`
	OrganizationName *string    `json:"organizationName,omitempty"`
	ApiKeyId         *string    `json:"apiKeyId,omitempty"`
	AllowedDomains   *string    `json:"allowedDomains,omitempty"`
	ServiceStatus    *string    `json:"serviceStatus,omitempty"`
	State            *string    `json:"state,omitempty"`
}

type clickhouseOrganizationApi struct {
	Item *clickhouseOrganizationJson `json:"item"`
}

// The attributes P0 sets when it validates the install, which stay unknown in a plan
// that finishes one.
var validatedAttributes = []path.Path{
	path.Root("organization_name"),
	path.Root("api_key_id"),
	path.Root("allowed_domains"),
	path.Root("service_status"),
}

func NewClickHouseOrganization() resource.Resource {
	return &ClickHouseOrganization{}
}

func (*ClickHouseOrganization) Metadata(_ context.Context, req resource.MetadataRequest, res *resource.MetadataResponse) {
	res.TypeName = req.ProviderTypeName + "_clickhouse_organization"
}

func (*ClickHouseOrganization) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A ClickHouse Cloud organization whose services P0 can grant access to, through one of your ClickHouse Cloud connectors.

The connector acts on the organization with an API key. P0 never holds the key. You store it in your own secret manager, in the connector's cloud, and the connector reads it on every call. This resource tells P0 which connector acts on the organization, and where the API key's secret is.

**Important:** Create the API key, store it and let the connector read it before Terraform creates this resource. Do that in an earlier apply, or in the same apply with this resource depending on the connector's read grant, as the example does. Creating this resource has P0 validate the install through the connector: the connector reads the secret, signs in to the organization with the API key, checks that the key's ` + "`p0-connector`" + ` role has every permission below except ` + "`control-plane:service:manage-query-api-endpoints`" + `, lists the organization's services, and reads back its own ` + "`DOMAIN_ALLOW_PATTERN`" + `. P0 sets up its read-only Query API endpoints, whose names start with ` + "`p0_`" + `, on a service the first time someone requests SQL or database role access to it or lists its objects. That's when P0 first uses ` + "`control-plane:service:manage-query-api-endpoints`" + `: if the role lacks it, that request or listing fails with a message that names it. If a check fails, or no service in the organization can take requests from P0, the apply fails with P0's message. A service can take requests when it's ready, or when it takes console role requests only (` + "`console-only`" + ` in ` + "`service_status`" + `). A service that isn't ready doesn't fail the apply while another one can take requests: ` + "`service_status`" + ` lists it. IAM changes can take a few seconds to propagate, so if the connector is denied the secret right after you grant it, apply again.

P0 fixes the connector, the vault and the secret when it adds the organization, so changing ` + "`id`" + `, ` + "`connector`" + `, ` + "`vault`" + ` or ` + "`api_key_secret_name`" + ` replaces the installation. If the organization is already added in P0, import it rather than creating it: P0 refuses to add an organization twice. Install a ClickHouse Cloud organization in only one P0 tenant. If P0 hasn't finished installing the organization, for example because you imported it before its checks passed, Terraform plans an update, and applying it finishes the install.

**Rotating the API key or adding a service needs nothing in P0 or Terraform.** The next SQL or database role request on each service moves P0's Query API endpoints there to the new key, or sets them up on the new service. To rotate the key, create a new key with the same two roles, store it as a new version of the same secret, then delete the old key. ` + "`api_key_id`" + ` and ` + "`service_status`" + ` keep what the connector read when P0 last validated the install, so they don't show a rotated key or a new service.

**Prerequisites:**

- A ClickHouse Cloud connector, installed with ` + "`p0_clickhouse_connector`" + `. The API key's secret must be in the connector's cloud: AWS Secrets Manager for a connector on AWS Lambda, or Google Secret Manager for one on Cloud Run. It can be in another account or project.

- In the ClickHouse Cloud console, a custom role named ` + "`p0-connector`" + ` with exactly these permissions, and no others: ` + requiredPermissions + `. ` + "`control-plane:organization:manage`" + ` is admin-equivalent: it lets the key create roles and add anyone, itself included, to any role. The connector only ever changes or deletes roles whose names start with ` + "`p0_`" + `.

- An API key with two roles: ` + "`p0-connector`" + `, and the system role Basic service API reader, which running P0's Query API endpoints needs. Store it outside Terraform, as the secret's whole value, as JSON: ` + "`{\"keyId\": \"...\", \"keySecret\": \"...\"}`" + `, in the account and region or the project that ` + "`vault`" + ` names. Storing it outside Terraform keeps it out of Terraform's state.

- Read access to that one secret, and no other, for the connector's identity (its ` + "`connector_identity`" + `). P0 names the secret in each call, so a call could point the connector at any other secret it can read.

  On AWS, give the connector's role ` + "`secretsmanager:GetSecretValue`" + ` on ` + "`arn:aws:secretsmanager:<secrets_region>:<account_id>:secret:<name>-??????`" + `. Secrets Manager adds a hyphen and six random characters to the name in a secret's ARN, and ` + "`??????`" + ` matches those, so the grant covers this one secret. If a customer-managed KMS key encrypts the secret, also give the role ` + "`kms:Decrypt`" + ` on that key, with the condition that ` + "`kms:ViaService`" + ` is ` + "`secretsmanager.<secrets_region>.amazonaws.com`" + `. A secret outside the connector's account needs a customer-managed key, because no other account can use the default ` + "`aws/secretsmanager`" + ` key. In the secret's account, the secret's resource policy and the key's policy must also allow the connector's role.

  On Google Cloud, give the connector's service account ` + "`roles/secretmanager.secretAccessor`" + ` and ` + "`roles/secretmanager.viewer`" + ` on the secret itself, not on its project.

- P0's secret manager integration for where the secret is: AWS Secrets Manager (` + "`p0_aws_secrets_manager`" + `) for the AWS account, or Google Secret Manager for the Google Cloud project.

**Note:** This integration is currently in beta.

For the full install steps, see [ClickHouse Cloud](https://docs.p0.dev/integrations/resource-integrations/clickhouse).`,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "The organization's ID in ClickHouse Cloud, a UUID such as `0b5a3c1e-6f2d-4e8a-9c7b-1d2e3f4a5b6c`, in lowercase. " +
					"The ClickHouse Cloud console shows it in the organization's details. " +
					"Add each organization to one P0 tenant only: if two tenants have it, each keeps putting P0's Query API endpoints back to its own API key, and requests in both tenants fail from time to time.",
				Validators: []validator.String{organizationIdValidator{}},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"connector": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The connector that acts on this organization: the `id` of a `p0_clickhouse_connector` resource. Requires that connector to be installed.",
				Validators:          connectorIdValidators(),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"vault": vaultAttribute(
				`The secret manager that holds the API key. Your connector reads the secret, and P0 never does. It must be in the connector's cloud, and can be in another account or project there.`,
				secretPlace.what,
			),
			"api_key_secret_name": schema.StringAttribute{
				Required: true,
				MarkdownDescription: `The name of the secret that holds the API key, such as ` + "`clickhouse/acme/api-key`" + ` in AWS Secrets Manager or ` + "`clickhouse-acme-api-key`" + ` in Google Secret Manager: the name alone, not the secret's ARN or its full resource name. ` +
					`The secret holds the key as JSON: ` + "`{\"keyId\": \"...\", \"keySecret\": \"...\"}`" + `. ` +
					`In AWS Secrets Manager the name can't end in a hyphen and six letters or digits, like ` + "`-AbCdEf`" + `, because Secrets Manager can't find a secret by such a name. ` +
					`Changing it replaces the installation. To rotate the API key, store the new one as a new version of the same secret, which needs no change here or in P0.`,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"organization_name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The organization's name in ClickHouse Cloud, which the connector reads when P0 validates the install.",
			},
			"api_key_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The ID of the API key in the secret, which the connector reads when P0 validates the install. P0 never stores the key's secret.",
			},
			"allowed_domains": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The email domains the connector acts for: the `DOMAIN_ALLOW_PATTERN` it's deployed with, as P0 read it back from the connector when it last validated the install.",
			},
			"service_status": schema.StringAttribute{
				Computed: true,
				MarkdownDescription: "Each service that isn't ready for every request, with why, as of the organization's last validation in the P0 app or by Terraform: `<name>: <status> (<reason>)` for each, joined with `; `, where the status is `console-only` or `not-ready`. " +
					"Empty when every service is ready. " +
					"It doesn't show services added or changed since then.",
			},
			"state": common.StateAttribute,
		},
	}
}

// Checking here rather than in Create reports a mistake at plan time, before the
// provider calls P0, in P0's words. A value that Terraform only knows at apply time
// is skipped while planning, and checked when Terraform validates the configuration
// again before it applies.
func (*ClickHouseOrganization) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var secretName types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("api_key_secret_name"), &secretName)...)
	vault := vaultFromConfig(ctx, req.Config, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	validateApiKeySecret(vault, secretName, &resp.Diagnostics)
}

// Refuses an organization ID that isn't a lowercase UUID, with P0's message
// (`validateOrganizationId` in
// packages/integrations/clickhouse/backend/src/install/validation.ts in the app). P0
// checks the ID as it is, without trimming it. The framework's
// stringvalidator.RegexMatches would add the value after P0's message, which already
// ends in a full stop.
type organizationIdValidator struct{}

var _ validator.String = organizationIdValidator{}

func (organizationIdValidator) Description(context.Context) string {
	return invalidOrganizationId
}

func (v organizationIdValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (organizationIdValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if !OrganizationIdRegex.MatchString(req.ConfigValue.ValueString()) {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid ClickHouse Cloud organization ID", invalidOrganizationId)
	}
}

func (r *ClickHouseOrganization) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:        ClickHouseKey,
		Component:          installresources.Organization,
		ProviderData:       data,
		GetId:              r.getId,
		GetItemJson:        r.getItemJson,
		FromJson:           r.fromJson,
		ToJson:             r.toJson,
		DescribeCheckError: describeCheckError("organization"),
	}
}

func (r *ClickHouseOrganization) getId(data any) *string {
	model, ok := data.(*clickhouseOrganizationModel)
	if !ok {
		return nil
	}
	str := model.Id.ValueString()
	return &str
}

// A response with no item is a bad response, not an item to read. Its nil item,
// returned as it is, wouldn't compare equal to nil, and would be read as one.
func (r *ClickHouseOrganization) getItemJson(json any) any {
	inner, ok := json.(*clickhouseOrganizationApi)
	if !ok || inner.Item == nil {
		return nil
	}
	return inner.Item
}

func (r *ClickHouseOrganization) fromJson(_ context.Context, diags *diag.Diagnostics, id string, json any) any {
	jsonv, ok := json.(*clickhouseOrganizationJson)
	if !ok {
		return nil
	}

	// vault is Required, so a nil vault would surface as Terraform core's "provider
	// produced inconsistent result after apply", which tells the reader nothing to do.
	if jsonv.Vault == nil {
		diags.AddError(
			"Bad API response",
			fmt.Sprintf("The ClickHouse Cloud organization %s carries no vault configuration. "+
				"Remove it in the P0 app and add it again, or remove it from Terraform state with `terraform state rm`.", id),
		)
		return nil
	}

	// A vault of a type this provider doesn't know can't be read into the attribute's
	// fields.
	if !slices.Contains(vaultTypes, jsonv.Vault.Type) {
		diags.AddError(
			"Unsupported ClickHouse Cloud organization",
			fmt.Sprintf("The ClickHouse Cloud organization %s has a %q vault. This provider supports only %s. "+
				"Upgrade the provider, manage the organization in the P0 app, or remove it from Terraform state with `terraform state rm`.",
				id, jsonv.Vault.Type, strings.Join(vaultTypes, " and ")),
		)
		return nil
	}

	return &clickhouseOrganizationModel{
		Id:               types.StringValue(id),
		Connector:        types.StringPointerValue(jsonv.Connector),
		Vault:            vaultFromJson(jsonv.Vault),
		ApiKeySecretName: types.StringPointerValue(jsonv.ApiKeySecretName),
		OrganizationName: types.StringPointerValue(jsonv.OrganizationName),
		ApiKeyId:         types.StringPointerValue(jsonv.ApiKeyId),
		AllowedDomains:   types.StringPointerValue(jsonv.AllowedDomains),
		ServiceStatus:    types.StringPointerValue(jsonv.ServiceStatus),
		State:            types.StringPointerValue(jsonv.State),
	}
}

// The body of the PUT that stages the item, and of verify and configure: the
// connector, the vault and the secret's name, which P0 fixes when it creates the item.
// Sending them on each step has P0 refuse a request that would change any of them,
// before it calls the connector. P0 reads the organization's name, the key's ID, the
// allowed domains and the services' status from the connector, and sets the state. P0
// would keep a value sent for one of those on a step that doesn't read it, so the body
// never carries them.
func (r *ClickHouseOrganization) toJson(data any) any {
	datav, ok := data.(*clickhouseOrganizationModel)
	if !ok {
		return nil
	}

	json := clickhouseOrganizationJson{
		Connector:        datav.Connector.ValueStringPointer(),
		ApiKeySecretName: datav.ApiKeySecretName.ValueStringPointer(),
	}
	if datav.Vault != nil {
		json.Vault = datav.Vault.toJson()
	}
	return &json
}

// P0 checks every field before it creates the item, without repeating the value, which
// may be the API key pasted in place of the secret's name. So a value it refuses
// leaves nothing staged. A check that fails after that leaves the staged item in
// state, tainted, and the next apply replaces it, because P0 refuses a second PUT of
// an organization that's already added.
func (r *ClickHouseOrganization) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json clickhouseOrganizationApi
	var data clickhouseOrganizationModel

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

func (r *ClickHouseOrganization) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &clickhouseOrganizationApi{}, &clickhouseOrganizationModel{})
}

// Plans an update for an organization that P0 hasn't installed, so that the next apply
// finishes the install. P0 reads the fields it validates from the connector during
// that apply, so they stay unknown until then.
func (*ClickHouseOrganization) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	planUnfinishedInstall(ctx, req, resp, validatedAttributes...)
}

// Every configurable attribute requires replacement, so the only update Terraform
// plans is the one that finishes an install, which verifies and configures the item as
// Create does. P0 validates the install on verify alone, so there is no shorter way.
func (r *ClickHouseOrganization) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &clickhouseOrganizationApi{}, &clickhouseOrganizationModel{})
}

func (r *ClickHouseOrganization) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Delete(ctx, &resp.Diagnostics, &req.State, &clickhouseOrganizationModel{})
}

func (r *ClickHouseOrganization) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// The permissions of the API key's `p0-connector` role, for the resource's
// description, in the order P0's installer lists them (CLICKHOUSE_REQUIRED_PERMISSIONS
// in packages/integrations/clickhouse/shared/src/permissions.ts in the app). The
// connector's install check requires every one.
const requiredPermissions = "`control-plane:organization:view`, `control-plane:organization:manage`, " +
	"`control-plane:service:view`, `control-plane:service:view-query-api-endpoints`, " +
	"`control-plane:service:manage-query-api-endpoints` and `control-plane:organization:view-api-keys`"
