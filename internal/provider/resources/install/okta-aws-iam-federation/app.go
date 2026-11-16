package installoktaawsiamfederation

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
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/p0-security/terraform-provider-p0/internal"
	"github.com/p0-security/terraform-provider-p0/internal/common"
	installresources "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install"
)

// copy of internal/provider/resources/install/auth0/tenant.go, renamed for Okta (AWS IAM
// Federation) (INT-1473), keyed by the Okta app's ID, with the org's domain, and with a
// private key in place of a client secret.

var _ resource.Resource = &OktaAwsIamFederationApp{}
var _ resource.ResourceWithConfigure = &OktaAwsIamFederationApp{}
var _ resource.ResourceWithImportState = &OktaAwsIamFederationApp{}
var _ resource.ResourceWithModifyPlan = &OktaAwsIamFederationApp{}
var _ resource.ResourceWithValidateConfig = &OktaAwsIamFederationApp{}

type OktaAwsIamFederationApp struct {
	installer *common.Install
}

type oktaAwsIamFederationAppModel struct {
	Id              types.String `tfsdk:"id"`
	Connector       types.String `tfsdk:"connector"`
	Domain          types.String `tfsdk:"domain"`
	ClientId        types.String `tfsdk:"client_id"`
	Vault           *vaultModel  `tfsdk:"vault"`
	PrivateKeyName  types.String `tfsdk:"private_key_name"`
	EntityId        types.String `tfsdk:"entity_id"`
	SamlMetadataUrl types.String `tfsdk:"saml_metadata_url"`
	State           types.String `tfsdk:"state"`
}

type oktaAwsIamFederationAppJson struct {
	Connector      *string    `json:"connector,omitempty"`
	Domain         *string    `json:"domain,omitempty"`
	ClientId       *string    `json:"clientId,omitempty"`
	Vault          *vaultJson `json:"vault,omitempty"`
	PrivateKeyName *string    `json:"privateKeyName,omitempty"`
	EntityId       *string    `json:"entityId,omitempty"`
	State          *string    `json:"state,omitempty"`
}

// What P0 computes for the item without storing it.
type oktaAwsIamFederationAppMetadataJson struct {
	SamlMetadataUrl *string `json:"samlMetadataUrl,omitempty"`
}

type oktaAwsIamFederationAppApi struct {
	Item     *oktaAwsIamFederationAppJson         `json:"item"`
	Metadata *oktaAwsIamFederationAppMetadataJson `json:"metadata,omitempty"`
}

func NewOktaAwsIamFederationApp() resource.Resource {
	return &OktaAwsIamFederationApp{}
}

func (*OktaAwsIamFederationApp) Metadata(_ context.Context, req resource.MetadataRequest, res *resource.MetadataResponse) {
	res.TypeName = req.ProviderTypeName + "_okta_aws_iam_federation_app"
}

func (*OktaAwsIamFederationApp) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		MarkdownDescription: `An Okta AWS Account Federation app that P0 assigns AWS roles in, through one of your Okta (AWS IAM Federation) connectors. Users sign in to AWS through the app, and a ` + "`p0_aws_iam_federation`" + ` resource has an AWS account trust it.

The connector acts on the org as an Okta API service app, which signs in with a private key. P0 never holds the private key. You store it in your own secret manager, in the connector's cloud, and the connector reads it on every call. This resource tells P0 which connector acts on the app, the org's domain, the service app's client ID, and where the private key is.

**Important:** Create the service app, store its private key and let the connector read it before Terraform creates this resource. Do that in an earlier apply, or in the same apply with this resource depending on the connector's read grant. Creating this resource has P0 validate the install through the connector: the connector reads the key, signs in to Okta as the service app, and reads the federation app. If a check fails, the apply fails with P0's message. IAM changes can take a few seconds to propagate, so if the connector is denied the secret right after you grant it, apply again.

P0 fixes the connector, the domain, the client ID, the vault and the secret when it adds the app, so changing any of them, or ` + "`id`" + `, replaces the installation. To rotate the private key, store the new one as a new version of the same secret, which needs no change here. If the app is already added in P0, import it rather than creating it: P0 refuses to add an app twice. If P0 hasn't finished installing the app, for example because you imported it before its checks passed, Terraform plans an update, and applying it finishes the install.

**Prerequisites:**

- An Okta (AWS IAM Federation) connector, installed with ` + "`p0_okta_aws_iam_federation_connector`" + `. The private key must be in the connector's cloud: AWS Secrets Manager for a connector on AWS Lambda, or Google Secret Manager for one on Cloud Run. It can be in another account or project.

- The Okta AWS Account Federation app, with the ` + "`https://aws.amazon.com/SAML/Attributes/SourceIdentity`" + ` attribute. An app that a legacy federated AWS login uses can't be used.

- An Okta API service app that signs in with a public and private key, with these Okta API scopes and no other: ` + oktaScopes + `. Give it a custom admin role with the permissions **Edit users' application assignments** and **Manage applications**, on a resource set that holds all users and the federation app. Store its private key, in JSON format, outside Terraform, as the secret's whole value, in the account and region or the project that ` + "`vault`" + ` names. Storing it outside Terraform keeps it out of Terraform's state.

- Read access to that one secret, and no other, for the connector's identity (its ` + "`connector_identity`" + `). P0 names the secret in each call, so a call could point the connector at any other secret it can read.

  On AWS, give the connector's role ` + "`secretsmanager:GetSecretValue`" + ` on ` + "`arn:aws:secretsmanager:<secrets_region>:<account_id>:secret:<name>-??????`" + `. Secrets Manager adds a hyphen and six random characters to the name in a secret's ARN, and ` + "`??????`" + ` matches those, so the grant covers this one secret. If a customer-managed KMS key encrypts the secret, also give the role ` + "`kms:Decrypt`" + ` on that key, with the condition that ` + "`kms:ViaService`" + ` is ` + "`secretsmanager.<secrets_region>.amazonaws.com`" + `. A secret outside the connector's account needs a customer-managed key, because no other account can use the default ` + "`aws/secretsmanager`" + ` key. In the secret's account, the secret's resource policy and the key's policy must also allow the connector's role.

  On Google Cloud, give the connector's service account ` + "`roles/secretmanager.secretAccessor`" + ` and ` + "`roles/secretmanager.viewer`" + ` on the secret itself, not on its project.

- P0's secret manager integration for where the secret is: AWS Secrets Manager (` + "`p0_aws_secrets_manager`" + `) for the AWS account, or Google Secret Manager for the Google Cloud project.

Okta reads each AWS account's roles as an IAM user with ` + "`iam:ListAccountAliases`" + ` and ` + "`iam:ListRoles`" + `. Create one in each account that trusts the app, give its access key to the app's provisioning settings, and refresh the app's data after the pool roles exist.

**Note:** This integration is currently in preview.

For the AWS install steps, see [Setting up AWS IAM management](https://docs.p0.dev/integrations/resource-integrations/aws/installation-methods).`,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The 20-character ID of the Okta AWS Account Federation app, as its URL in the Okta Admin Console shows it",
				PlanModifiers:       replace,
			},
			"connector": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The connector that acts on this app: the `id` of a `p0_okta_aws_iam_federation_connector` resource. Requires that connector to be installed.",
				Validators:          connectorIdValidators(),
				PlanModifiers:       replace,
			},
			"domain": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The org's Okta domain, such as `acme.okta.com`, in lowercase. A custom domain can't be used.",
				PlanModifiers:       replace,
			},
			"client_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The 20-character client ID of the Okta API service app that the connector signs in as",
				PlanModifiers:       replace,
			},
			"vault": vaultAttribute(
				`The secret manager that holds the private key. Your connector reads the secret, and P0 never does. It must be in the connector's cloud, and can be in another account or project there.`,
				secretPlace.what,
			),
			"private_key_name": schema.StringAttribute{
				Required: true,
				MarkdownDescription: `The name of the secret that holds the service app's private key as a JWK, such as ` + "`okta/acme/p0`" + ` in AWS Secrets Manager or ` + "`okta-acme-p0`" + ` in Google Secret Manager: the name alone, not the secret's ARN or its full resource name. ` +
					`In AWS Secrets Manager the name can't end in a hyphen and six letters or digits, like ` + "`-AbCdEf`" + `, because Secrets Manager can't find a secret by such a name. ` +
					`To rotate the private key, store the new one as a new version of the same secret, which needs no change here.`,
				PlanModifiers: replace,
			},
			"entity_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The app's SAML entity ID, which P0 reads from Okta when it checks the install",
			},
			"saml_metadata_url": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The URL of the app's SAML metadata. Create each AWS account's SAML provider from it.",
			},
			"state": common.StateAttribute,
		},
	}
}

// Checking here rather than in Create reports a mistake at plan time, before the
// provider calls P0, in P0's words. A value that Terraform only knows at apply time
// is skipped while planning, and checked when Terraform validates the configuration
// again before it applies.
func (*OktaAwsIamFederationApp) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var id, domain, clientId, keyName types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("id"), &id)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("domain"), &domain)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("client_id"), &clientId)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("private_key_name"), &keyName)...)
	vault := vaultFromConfig(ctx, req.Config, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	// P0's checks (`validateAppId`, `validateDomain` and `validateClientId` in
	// packages/integrations/okta-aws-iam-federation/backend/src/install/validation.ts in
	// the app). None repeats the value, because it may be the pasted private key.
	validateField(path.Root("id"), id, []rule{{summary: "Invalid Okta app ID", message: invalidAppId, test: OktaIdRegex.MatchString}}, &resp.Diagnostics)
	validateField(path.Root("domain"), domain, []rule{{summary: "Invalid Okta domain", message: invalidOktaDomain, test: OktaDomainRegex.MatchString}}, &resp.Diagnostics)
	validateField(path.Root("client_id"), clientId, []rule{{summary: "Invalid client ID", message: invalidClientId, test: OktaIdRegex.MatchString}}, &resp.Diagnostics)
	validatePrivateKey(vault, keyName, &resp.Diagnostics)
}

func (r *OktaAwsIamFederationApp) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:        OktaAwsIamFederationKey,
		Component:          installresources.App,
		ProviderData:       data,
		GetId:              r.getId,
		GetItemJson:        r.getItemJson,
		FromJson:           r.fromJson,
		ToJson:             r.toJson,
		DescribeCheckError: describeCheckError("app"),
	}
}

func (r *OktaAwsIamFederationApp) getId(data any) *string {
	model, ok := data.(*oktaAwsIamFederationAppModel)
	if !ok {
		return nil
	}
	str := model.Id.ValueString()
	return &str
}

// Returns the whole response rather than the item alone, because the metadata URL is in
// the response's metadata, beside the item. A response with no item is a bad response,
// not an item to read.
func (r *OktaAwsIamFederationApp) getItemJson(json any) any {
	inner, ok := json.(*oktaAwsIamFederationAppApi)
	if !ok || inner.Item == nil {
		return nil
	}
	return inner
}

func (r *OktaAwsIamFederationApp) fromJson(_ context.Context, diags *diag.Diagnostics, id string, json any) any {
	api, ok := json.(*oktaAwsIamFederationAppApi)
	if !ok || api.Item == nil {
		return nil
	}
	jsonv := api.Item

	// vault is Required, so a nil vault would surface as Terraform core's "provider
	// produced inconsistent result after apply", which tells the reader nothing to do.
	if jsonv.Vault == nil {
		diags.AddError(
			"Bad API response",
			fmt.Sprintf("The Okta (AWS IAM Federation) app %s carries no vault configuration. "+
				"Remove it in the P0 app and add it again, or remove it from Terraform state with `terraform state rm`.", id),
		)
		return nil
	}

	// A vault of a type this provider doesn't know can't be read into the attribute's
	// fields.
	if !slices.Contains(vaultTypes, jsonv.Vault.Type) {
		diags.AddError(
			"Unsupported Okta (AWS IAM Federation) app",
			fmt.Sprintf("The Okta (AWS IAM Federation) app %s has a %q vault. This provider supports only %s. "+
				"Upgrade the provider, manage the app in the P0 app, or remove it from Terraform state with `terraform state rm`.",
				id, jsonv.Vault.Type, strings.Join(vaultTypes, " and ")),
		)
		return nil
	}

	samlMetadataUrl := types.StringNull()
	if api.Metadata != nil {
		samlMetadataUrl = types.StringPointerValue(api.Metadata.SamlMetadataUrl)
	}

	return &oktaAwsIamFederationAppModel{
		Id:              types.StringValue(id),
		Connector:       types.StringPointerValue(jsonv.Connector),
		Domain:          types.StringPointerValue(jsonv.Domain),
		ClientId:        types.StringPointerValue(jsonv.ClientId),
		Vault:           vaultFromJson(jsonv.Vault),
		PrivateKeyName:  types.StringPointerValue(jsonv.PrivateKeyName),
		EntityId:        types.StringPointerValue(jsonv.EntityId),
		SamlMetadataUrl: samlMetadataUrl,
		State:           types.StringPointerValue(jsonv.State),
	}
}

// The body of the PUT that stages the item, and of verify and configure: the fields
// that P0 fixes when it creates the item. Sending them on each step has P0 refuse a
// request that would change any of them, before it calls the connector. P0 reads the
// entity ID from Okta, and sets the state.
func (r *OktaAwsIamFederationApp) toJson(data any) any {
	datav, ok := data.(*oktaAwsIamFederationAppModel)
	if !ok {
		return nil
	}

	json := oktaAwsIamFederationAppJson{
		Connector:      datav.Connector.ValueStringPointer(),
		Domain:         datav.Domain.ValueStringPointer(),
		ClientId:       datav.ClientId.ValueStringPointer(),
		PrivateKeyName: datav.PrivateKeyName.ValueStringPointer(),
	}
	if datav.Vault != nil {
		json.Vault = datav.Vault.toJson()
	}
	return &json
}

// P0 checks every field before it creates the item, without repeating the value, which
// may be the private key pasted in place of the secret's name or the client ID. So a
// value it refuses leaves nothing staged. A check that fails after that leaves the
// staged item in state, tainted, and the next apply replaces it, because P0 refuses a
// second PUT of an app that's already added.
func (r *OktaAwsIamFederationApp) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json oktaAwsIamFederationAppApi
	var data oktaAwsIamFederationAppModel

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

func (r *OktaAwsIamFederationApp) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &oktaAwsIamFederationAppApi{}, &oktaAwsIamFederationAppModel{})
}

// Plans an update for an app that P0 hasn't installed, so that the next apply finishes
// the install. P0 reads the entity ID from Okta during that apply, and the metadata URL
// follows from it, so both stay unknown until then.
func (*OktaAwsIamFederationApp) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	planUnfinishedInstall(ctx, req, resp, path.Root("entity_id"), path.Root("saml_metadata_url"))
}

// Every configurable attribute requires replacement, so the only update Terraform
// plans is the one that finishes an install, which verifies and configures the item as
// Create does. P0 validates the install on verify alone, so there is no shorter way.
func (r *OktaAwsIamFederationApp) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &oktaAwsIamFederationAppApi{}, &oktaAwsIamFederationAppModel{})
}

func (r *OktaAwsIamFederationApp) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Delete(ctx, &resp.Diagnostics, &req.State, &oktaAwsIamFederationAppModel{})
}

func (r *OktaAwsIamFederationApp) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// The Okta API scopes for the resource's description (OKTA_SCOPES in
// packages/integrations/okta-aws-iam-federation/frontend/src/steps.tsx in the app).
const oktaScopes = "`okta.apps.manage`, `okta.schemas.manage` and `okta.users.read`"
