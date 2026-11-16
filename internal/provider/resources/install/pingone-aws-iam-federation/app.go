package installpingoneawsiamfederation

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
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

// copy of internal/provider/resources/install/okta-aws-iam-federation/app.go, renamed for
// PingOne (AWS IAM Federation) (INT-1473), with the environment's region and ID and the
// role attribute in place of the Okta domain, and a client secret in place of a private
// key.

var _ resource.Resource = &PingOneAwsIamFederationApp{}
var _ resource.ResourceWithConfigure = &PingOneAwsIamFederationApp{}
var _ resource.ResourceWithImportState = &PingOneAwsIamFederationApp{}
var _ resource.ResourceWithModifyPlan = &PingOneAwsIamFederationApp{}
var _ resource.ResourceWithValidateConfig = &PingOneAwsIamFederationApp{}

type PingOneAwsIamFederationApp struct {
	installer *common.Install
}

type pingOneAwsIamFederationAppModel struct {
	Id               types.String `tfsdk:"id"`
	Connector        types.String `tfsdk:"connector"`
	Region           types.String `tfsdk:"region"`
	EnvironmentId    types.String `tfsdk:"environment_id"`
	RoleAttribute    types.String `tfsdk:"role_attribute"`
	ClientId         types.String `tfsdk:"client_id"`
	Vault            *vaultModel  `tfsdk:"vault"`
	ClientSecretName types.String `tfsdk:"client_secret_name"`
	SamlMetadataUrl  types.String `tfsdk:"saml_metadata_url"`
	State            types.String `tfsdk:"state"`
}

type regionJson struct {
	Type string `json:"type"`
}

type pingOneAwsIamFederationAppJson struct {
	Connector        *string     `json:"connector,omitempty"`
	Region           *regionJson `json:"region,omitempty"`
	EnvironmentId    *string     `json:"environmentId,omitempty"`
	RoleAttribute    *string     `json:"roleAttribute,omitempty"`
	ClientId         *string     `json:"clientId,omitempty"`
	Vault            *vaultJson  `json:"vault,omitempty"`
	ClientSecretName *string     `json:"clientSecretName,omitempty"`
	State            *string     `json:"state,omitempty"`
}

// What P0 computes for the item without storing it.
type pingOneAwsIamFederationAppMetadataJson struct {
	SamlMetadataUrl *string `json:"samlMetadataUrl,omitempty"`
}

type pingOneAwsIamFederationAppApi struct {
	Item     *pingOneAwsIamFederationAppJson         `json:"item"`
	Metadata *pingOneAwsIamFederationAppMetadataJson `json:"metadata,omitempty"`
}

func NewPingOneAwsIamFederationApp() resource.Resource {
	return &PingOneAwsIamFederationApp{}
}

func (*PingOneAwsIamFederationApp) Metadata(_ context.Context, req resource.MetadataRequest, res *resource.MetadataResponse) {
	res.TypeName = req.ProviderTypeName + "_pingone_aws_iam_federation_app"
}

func (*PingOneAwsIamFederationApp) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		MarkdownDescription: `A PingOne SAML app that signs users in to AWS, and that P0 assigns AWS roles in through one of your PingOne (AWS IAM Federation) connectors. A ` + "`p0_aws_iam_federation`" + ` resource has an AWS account trust it.

The connector acts on the environment as a PingOne worker app, which signs in with a client secret. P0 never holds the client secret. You store it in your own secret manager, in the connector's cloud, and the connector reads it on every call. This resource tells P0 which connector acts on the app, the environment, the role attribute, the worker app's client ID, and where the client secret is.

**Important:** Create the worker app, store its client secret and let the connector read it before Terraform creates this resource. Do that in an earlier apply, or in the same apply with this resource depending on the connector's read grant. Creating this resource has P0 validate the install through the connector: the connector reads the secret, signs in to PingOne as the worker app, and checks the SAML app and the role attribute. If a check fails, the apply fails with P0's message. IAM changes can take a few seconds to propagate, so if the connector is denied the secret right after you grant it, apply again.

P0 fixes every attribute when it adds the app, so changing any of them replaces the installation. To rotate the client secret, store the new one as a new version of the same secret, which needs no change here. If the app is already added in P0, import it rather than creating it: P0 refuses to add an app twice. If P0 hasn't finished installing the app, for example because you imported it before its checks passed, Terraform plans an update, and applying it finishes the install.

**Prerequisites:**

- A PingOne (AWS IAM Federation) connector, installed with ` + "`p0_pingone_aws_iam_federation_connector`" + `. The client secret must be in the connector's cloud: AWS Secrets Manager for a connector on AWS Lambda, or Google Secret Manager for one on Cloud Run. It can be in another account or project.

- A multi-valued PingOne user attribute, named by ` + "`role_attribute`" + `, that P0 adds each user's AWS roles to.

- The PingOne SAML app. It maps ` + "`role_attribute`" + ` to ` + "`https://aws.amazon.com/SAML/Attributes/Role`" + `, and also sends ` + "`https://aws.amazon.com/SAML/Attributes/RoleSessionName`" + ` and ` + "`https://aws.amazon.com/SAML/Attributes/SourceIdentity`" + `, each with the URI name format. Its subject uses the persistent NameID format.

- A PingOne worker app in the same environment, whose roles let it read applications and their attribute mappings, read the user schema, and read and update users. Store its client secret outside Terraform, as the secret's whole value, in the account and region or the project that ` + "`vault`" + ` names. Storing it outside Terraform keeps it out of Terraform's state.

- Read access to that one secret, and no other, for the connector's identity (its ` + "`connector_identity`" + `). P0 names the secret in each call, so a call could point the connector at any other secret it can read.

  On AWS, give the connector's role ` + "`secretsmanager:GetSecretValue`" + ` on ` + "`arn:aws:secretsmanager:<secrets_region>:<account_id>:secret:<name>-??????`" + `. Secrets Manager adds a hyphen and six random characters to the name in a secret's ARN, and ` + "`??????`" + ` matches those, so the grant covers this one secret. If a customer-managed KMS key encrypts the secret, also give the role ` + "`kms:Decrypt`" + ` on that key, with the condition that ` + "`kms:ViaService`" + ` is ` + "`secretsmanager.<secrets_region>.amazonaws.com`" + `. A secret outside the connector's account needs a customer-managed key, because no other account can use the default ` + "`aws/secretsmanager`" + ` key. In the secret's account, the secret's resource policy and the key's policy must also allow the connector's role.

  On Google Cloud, give the connector's service account ` + "`roles/secretmanager.secretAccessor`" + ` and ` + "`roles/secretmanager.viewer`" + ` on the secret itself, not on its project.

- P0's secret manager integration for where the secret is: AWS Secrets Manager (` + "`p0_aws_secrets_manager`" + `) for the AWS account, or Google Secret Manager for the Google Cloud project.

**Note:** This integration is currently in preview.

For the AWS install steps, see [Setting up AWS IAM management](https://docs.p0.dev/integrations/resource-integrations/aws/installation-methods).`,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The ID of the PingOne SAML app that signs users in to AWS, as its Configuration tab in the PingOne admin console shows it",
				PlanModifiers:       replace,
			},
			"connector": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The connector that acts on this app: the `id` of a `p0_pingone_aws_iam_federation_connector` resource. Requires that connector to be installed.",
				Validators:          connectorIdValidators(),
				PlanModifiers:       replace,
			},
			"region": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The region of the PingOne environment: `NA`, `EU`, `CA`, `AP` or `AU`",
				Validators:          []validator.String{stringvalidator.OneOf(pingOneRegions...)},
				PlanModifiers:       replace,
			},
			"environment_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The ID of the PingOne environment that holds the app",
				PlanModifiers:       replace,
			},
			"role_attribute": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The name of the multi-valued user attribute that holds each user's AWS roles, such as `awsRoles`",
				PlanModifiers:       replace,
			},
			"client_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The client ID of the PingOne worker app that the connector signs in as",
				PlanModifiers:       replace,
			},
			"vault": vaultAttribute(
				`The secret manager that holds the client secret. Your connector reads the secret, and P0 never does. It must be in the connector's cloud, and can be in another account or project there.`,
				secretPlace.what,
			),
			"client_secret_name": schema.StringAttribute{
				Required: true,
				MarkdownDescription: `The name of the secret that holds the worker app's client secret, such as ` + "`pingone/acme/p0`" + ` in AWS Secrets Manager or ` + "`pingone-acme-p0`" + ` in Google Secret Manager: the name alone, not the secret's ARN or its full resource name. ` +
					`In AWS Secrets Manager the name can't end in a hyphen and six letters or digits, like ` + "`-AbCdEf`" + `, because Secrets Manager can't find a secret by such a name. ` +
					`To rotate the client secret, store the new one as a new version of the same secret, which needs no change here.`,
				PlanModifiers: replace,
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
func (*PingOneAwsIamFederationApp) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var id, environmentId, roleAttribute, clientId, secretName types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("id"), &id)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("environment_id"), &environmentId)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("role_attribute"), &roleAttribute)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("client_id"), &clientId)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("client_secret_name"), &secretName)...)
	vault := vaultFromConfig(ctx, req.Config, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	// P0's checks (`validateAppId`, `validateEnvironmentId`, `validateRoleAttribute` and
	// `validateClientId` in
	// packages/integrations/pingone-aws-iam-federation/backend/src/install/validation.ts in
	// the app). None repeats the value, because it may be the pasted client secret.
	validateField(path.Root("id"), id, []rule{{summary: "Invalid PingOne app ID", message: invalidAppId, test: PingOneIdRegex.MatchString}}, &resp.Diagnostics)
	validateField(path.Root("environment_id"), environmentId, []rule{{summary: "Invalid PingOne environment ID", message: invalidEnvironmentId, test: PingOneIdRegex.MatchString}}, &resp.Diagnostics)
	validateField(path.Root("role_attribute"), roleAttribute, []rule{{summary: "Invalid role attribute", message: invalidRoleAttribute, test: RoleAttributeRegex.MatchString}}, &resp.Diagnostics)
	validateField(path.Root("client_id"), clientId, []rule{{summary: "Invalid client ID", message: invalidClientId, test: PingOneIdRegex.MatchString}}, &resp.Diagnostics)
	validateClientSecret(vault, secretName, &resp.Diagnostics)
}

func (r *PingOneAwsIamFederationApp) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:        PingOneAwsIamFederationKey,
		Component:          installresources.App,
		ProviderData:       data,
		GetId:              r.getId,
		GetItemJson:        r.getItemJson,
		FromJson:           r.fromJson,
		ToJson:             r.toJson,
		DescribeCheckError: describeCheckError("app"),
	}
}

func (r *PingOneAwsIamFederationApp) getId(data any) *string {
	model, ok := data.(*pingOneAwsIamFederationAppModel)
	if !ok {
		return nil
	}
	str := model.Id.ValueString()
	return &str
}

// Returns the whole response rather than the item alone, because the metadata URL is in
// the response's metadata, beside the item. A response with no item is a bad response,
// not an item to read.
func (r *PingOneAwsIamFederationApp) getItemJson(json any) any {
	inner, ok := json.(*pingOneAwsIamFederationAppApi)
	if !ok || inner.Item == nil {
		return nil
	}
	return inner
}

func (r *PingOneAwsIamFederationApp) fromJson(_ context.Context, diags *diag.Diagnostics, id string, json any) any {
	api, ok := json.(*pingOneAwsIamFederationAppApi)
	if !ok || api.Item == nil {
		return nil
	}
	jsonv := api.Item

	// vault is Required, so a nil vault would surface as Terraform core's "provider
	// produced inconsistent result after apply", which tells the reader nothing to do.
	if jsonv.Vault == nil {
		diags.AddError(
			"Bad API response",
			fmt.Sprintf("The PingOne (AWS IAM Federation) app %s carries no vault configuration. "+
				"Remove it in the P0 app and add it again, or remove it from Terraform state with `terraform state rm`.", id),
		)
		return nil
	}

	// A vault of a type this provider doesn't know can't be read into the attribute's
	// fields.
	if !slices.Contains(vaultTypes, jsonv.Vault.Type) {
		diags.AddError(
			"Unsupported PingOne (AWS IAM Federation) app",
			fmt.Sprintf("The PingOne (AWS IAM Federation) app %s has a %q vault. This provider supports only %s. "+
				"Upgrade the provider, manage the app in the P0 app, or remove it from Terraform state with `terraform state rm`.",
				id, jsonv.Vault.Type, strings.Join(vaultTypes, " and ")),
		)
		return nil
	}

	region := types.StringNull()
	if jsonv.Region != nil {
		region = types.StringValue(jsonv.Region.Type)
	}
	samlMetadataUrl := types.StringNull()
	if api.Metadata != nil {
		samlMetadataUrl = types.StringPointerValue(api.Metadata.SamlMetadataUrl)
	}

	return &pingOneAwsIamFederationAppModel{
		Id:               types.StringValue(id),
		Connector:        types.StringPointerValue(jsonv.Connector),
		Region:           region,
		EnvironmentId:    types.StringPointerValue(jsonv.EnvironmentId),
		RoleAttribute:    types.StringPointerValue(jsonv.RoleAttribute),
		ClientId:         types.StringPointerValue(jsonv.ClientId),
		Vault:            vaultFromJson(jsonv.Vault),
		ClientSecretName: types.StringPointerValue(jsonv.ClientSecretName),
		SamlMetadataUrl:  samlMetadataUrl,
		State:            types.StringPointerValue(jsonv.State),
	}
}

// The body of the PUT that stages the item, and of verify and configure: the fields
// that P0 fixes when it creates the item. Sending them on each step has P0 refuse a
// request that would change any of them, before it calls the connector.
func (r *PingOneAwsIamFederationApp) toJson(data any) any {
	datav, ok := data.(*pingOneAwsIamFederationAppModel)
	if !ok {
		return nil
	}

	json := pingOneAwsIamFederationAppJson{
		Connector:        datav.Connector.ValueStringPointer(),
		EnvironmentId:    datav.EnvironmentId.ValueStringPointer(),
		RoleAttribute:    datav.RoleAttribute.ValueStringPointer(),
		ClientId:         datav.ClientId.ValueStringPointer(),
		ClientSecretName: datav.ClientSecretName.ValueStringPointer(),
	}
	if !datav.Region.IsNull() && !datav.Region.IsUnknown() {
		json.Region = &regionJson{Type: datav.Region.ValueString()}
	}
	if datav.Vault != nil {
		json.Vault = datav.Vault.toJson()
	}
	return &json
}

// P0 checks every field before it creates the item, without repeating the value, which
// may be the client secret pasted in place of the secret's name or an ID. So a value it
// refuses leaves nothing staged. A check that fails after that leaves the staged item
// in state, tainted, and the next apply replaces it, because P0 refuses a second PUT of
// an app that's already added.
func (r *PingOneAwsIamFederationApp) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json pingOneAwsIamFederationAppApi
	var data pingOneAwsIamFederationAppModel

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

func (r *PingOneAwsIamFederationApp) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &pingOneAwsIamFederationAppApi{}, &pingOneAwsIamFederationAppModel{})
}

// Plans an update for an app that P0 hasn't installed, so that the next apply finishes
// the install.
func (*PingOneAwsIamFederationApp) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	planUnfinishedInstall(ctx, req, resp)
}

// Every configurable attribute requires replacement, so the only update Terraform
// plans is the one that finishes an install, which verifies and configures the item as
// Create does. P0 validates the install on verify alone, so there is no shorter way.
func (r *PingOneAwsIamFederationApp) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &pingOneAwsIamFederationAppApi{}, &pingOneAwsIamFederationAppModel{})
}

func (r *PingOneAwsIamFederationApp) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Delete(ctx, &resp.Diagnostics, &req.State, &pingOneAwsIamFederationAppModel{})
}

func (r *PingOneAwsIamFederationApp) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
