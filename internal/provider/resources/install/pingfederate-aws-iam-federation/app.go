package installpingfederateawsiamfederation

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/objectvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/p0-security/terraform-provider-p0/internal"
	"github.com/p0-security/terraform-provider-p0/internal/common"
	installresources "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install"
)

// copy of internal/provider/resources/install/pingone-aws-iam-federation/app.go, renamed for
// PingFederate (AWS IAM Federation) (INT-1565), with an ID the admin picks in place of
// the PingOne app's ID, PingFederate's URL and the SCIM user name in place of the
// environment's region and ID and the worker app's client ID, a SCIM password in place
// of a client secret, and an optional CA certificate.

var _ resource.Resource = &PingFederateAwsIamFederationApp{}
var _ resource.ResourceWithConfigure = &PingFederateAwsIamFederationApp{}
var _ resource.ResourceWithImportState = &PingFederateAwsIamFederationApp{}
var _ resource.ResourceWithModifyPlan = &PingFederateAwsIamFederationApp{}
var _ resource.ResourceWithValidateConfig = &PingFederateAwsIamFederationApp{}

type PingFederateAwsIamFederationApp struct {
	installer *common.Install
}

type pingFederateAwsIamFederationAppModel struct {
	Id              types.String          `tfsdk:"id"`
	PingFederateUrl types.String          `tfsdk:"pingfederate_url"`
	Scim            *scimModel            `tfsdk:"scim"`
	AttributeSource *attributeSourceModel `tfsdk:"attribute_source"`
	SamlMetadataUrl types.String          `tfsdk:"saml_metadata_url"`
	State           types.String          `tfsdk:"state"`
}

type pingFederateAwsIamFederationAppJson struct {
	PingFederateUrl *string         `json:"pingFederateUrl,omitempty"`
	RoleSource      *roleSourceJson `json:"roleSource,omitempty"`
	State           *string         `json:"state,omitempty"`
}

// The app's `roleSource` element: a discriminated union on "type", whose fields are
// those of its option.
type roleSourceJson struct {
	Type string `json:"type"`

	Connector        *string    `json:"connector,omitempty"`
	RoleAttribute    *string    `json:"roleAttribute,omitempty"`
	ScimUsername     *string    `json:"scimUsername,omitempty"`
	Vault            *vaultJson `json:"vault,omitempty"`
	ScimPasswordName *string    `json:"scimPasswordName,omitempty"`
	CaCertificate    *string    `json:"caCertificate,omitempty"`

	EntityId *string   `json:"entityId,omitempty"`
	Issuer   *string   `json:"issuer,omitempty"`
	Jwks     *jwksJson `json:"jwks,omitempty"`
	Audience *string   `json:"audience,omitempty"`
}

// One option of the app's `roleSource` element, as a block of this resource.
type roleSource interface {
	attribute() schema.Attribute
	// Checks the role source's block, which may be null.
	validate(ctx context.Context, config tfsdk.Config, diags *diag.Diagnostics)
	// Returns nil when the model doesn't use this role source.
	toJson(model *pingFederateAwsIamFederationAppModel) *roleSourceJson
	// Sets the model's block, or reports why it can't.
	fromJson(installer *common.Install, diags *diag.Diagnostics, id string, json *roleSourceJson, model *pingFederateAwsIamFederationAppModel)
}

// Keyed by P0's role source type.
var roleSources = map[string]roleSource{
	scimRoleSourceType:            scimRoleSource{},
	attributeSourceRoleSourceType: attributeSourceRoleSource{},
}

// P0 fixes an app's role source when it adds the app, so a block that appears or goes
// away replaces it. A change inside the block is the block's own to plan.
func roleSourcePlanModifiers() []planmodifier.Object {
	return []planmodifier.Object{objectplanmodifier.RequiresReplaceIf(
		func(_ context.Context, req planmodifier.ObjectRequest, resp *objectplanmodifier.RequiresReplaceIfFuncResponse) {
			resp.RequiresReplace = req.StateValue.IsNull() != req.PlanValue.IsNull()
		},
		"Replaces the app when its role source changes.",
		"Replaces the app when its role source changes.",
	)}
}

// What P0 computes for the item without storing it.
type pingFederateAwsIamFederationAppMetadataJson struct {
	SamlMetadataUrl *string `json:"samlMetadataUrl,omitempty"`
}

type pingFederateAwsIamFederationAppApi struct {
	Item     *pingFederateAwsIamFederationAppJson         `json:"item"`
	Metadata *pingFederateAwsIamFederationAppMetadataJson `json:"metadata,omitempty"`
}

func NewPingFederateAwsIamFederationApp() resource.Resource {
	return &PingFederateAwsIamFederationApp{}
}

func (*PingFederateAwsIamFederationApp) Metadata(_ context.Context, req resource.MetadataRequest, res *resource.MetadataResponse) {
	res.TypeName = req.ProviderTypeName + "_pingfederate_aws_iam_federation_app"
}

func (*PingFederateAwsIamFederationApp) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	scim := scimRoleSource{}.attribute().(schema.SingleNestedAttribute)
	scim.Validators = []validator.Object{objectvalidator.ExactlyOneOf(path.MatchRoot("attribute_source"))}
	resp.Schema = schema.Schema{
		MarkdownDescription: `A PingFederate SP connection that signs users in to AWS. A ` + "`p0_aws_iam_federation`" + ` resource has an AWS account trust it.

Its role source sets how PingFederate gets each user's AWS roles. Set exactly one of ` + "`attribute_source`" + ` or ` + "`scim`" + `. P0 fixes ` + "`id`" + `, ` + "`pingfederate_url`" + `, the role source and every ` + "`scim`" + ` field when it adds the app, so changing one replaces the installation. An attribute source's ` + "`entity_id`" + `, ` + "`issuer`" + ` and ` + "`jwks`" + ` change in place. If the app is already added in P0, import it rather than creating it: P0 refuses to add an app twice. If P0 hasn't finished installing the app, for example because you imported it before its checks passed, Terraform plans an update, and applying it finishes the install.

## Attribute source

PingFederate reads each user's roles from P0 at each sign-in, through a REST API data store. P0 calls nothing in your network and holds no secret: PingFederate's own OAuth server issues the data store's access token, and P0 checks it against ` + "`issuer`" + `, the public keys, and the audience that P0 sets for the app. A grant or a revoke takes effect at the user's next sign-in.

Creating this resource doesn't call PingFederate, so a mistake in PingFederate's setup shows as a refused AWS sign-in. Set up PingFederate with the values this resource gives:

- Under **Access Token Management**, add a **JSON Web Tokens** instance that uses the centralized signing key. Set its **Issuer Claim Value** to ` + "`issuer`" + `, and its **Audience Claim Value** to ` + "`attribute_source.audience`" + `, which P0 sets when it adds the app.

- Map the instance for the ` + "`client_credentials`" + ` context, and add an OAuth client with the client credentials grant that issues only this instance's tokens.

- Add a **REST API** data store whose base URL is ` + "`attribute_source.roles_url`" + `. Its authentication method is **OAuth 2.0 Bearer Token** with **Client Credentials**, the client above, and PingFederate's own token endpoint, ` + "`<pingfederate_url>/as/token.oauth2`" + `. Map the local attribute ` + "`roles`" + ` to ` + "`/roles`" + `.

- Create an SP connection to the partner ` + "`urn:amazon:webservices`" + `. Add the data store as an attribute source with the resource path ` + "`/apps/<id>/roles?email=${mail}`" + `, and fulfill ` + "`https://aws.amazon.com/SAML/Attributes/Role`" + ` from its ` + "`roles`" + `. Map the user's ` + "`mail`" + ` to ` + "`https://aws.amazon.com/SAML/Attributes/RoleSessionName`" + ` and ` + "`https://aws.amazon.com/SAML/Attributes/SourceIdentity`" + `, with the URI name format. Its subject uses the persistent NameID format. In its signature policy, select **Always sign the SAML Assertion**: AWS refuses a response that signs only the SAML response.

## SCIM

P0's connector writes each user's AWS roles to Active Directory through PingFederate's SCIM inbound provisioning, and signs in to it with HTTP Basic credentials. P0 never holds the SCIM password. You store it in your own secret manager, in the connector's cloud, and the connector reads it on every call.

**Important:** Set up PingFederate, store the SCIM password and let the connector read it before Terraform creates this resource. Do that in an earlier apply, or in the same apply with this resource depending on the connector's read grant. Creating this resource has P0 validate the install through the connector: the connector reads the secret, searches PingFederate's SCIM users by email, checks the role attribute in the custom SCIM schema, and reads PingFederate's SAML metadata for AWS. If a check fails, the apply fails with P0's message. IAM changes can take a few seconds to propagate, so if the connector is denied the secret right after you grant it, apply again. To rotate the SCIM password, store the new one as a new version of the same secret, which needs no change here.

**Prerequisites:**

- A PingFederate (AWS IAM Federation) connector, installed with ` + "`p0_pingfederate_aws_iam_federation_connector`" + `. The connector must reach PingFederate's runtime at ` + "`pingfederate_url`" + `, including ` + "`/pf-scim`" + `. The SCIM password must be in the connector's cloud: AWS Secrets Manager for a connector on AWS Lambda, or Google Secret Manager for one on Cloud Run. It can be in another account or project.

- A multi-valued string attribute on users in the Active Directory schema, named by ` + "`scim.role_attribute`" + `, that P0 adds each user's AWS roles to. The account of PingFederate's Active Directory data store must be able to write it.

- A PingFederate IdP connection with only Inbound Provisioning, on SCIM 2.0, whose user repository is the Active Directory data store. Add ` + "`scim.role_attribute`" + ` to its custom SCIM schema as a multi-valued, read-write string. In Write Users, map the Active Directory attribute to the SCIM attribute of the same name. In Read Users, map ` + "`emails.work.value`" + ` to ` + "`mail`" + `, and ` + "`scim.role_attribute`" + ` to itself. A SCIM search must return each user's ` + "`id`" + `. The connection uses HTTP Basic authentication with the user name ` + "`scim.scim_username`" + `. Store its password outside Terraform, as the secret's whole value, in the account and region or the project that ` + "`scim.vault`" + ` names. Storing it outside Terraform keeps it out of Terraform's state.

- A PingFederate SP connection to the partner ` + "`urn:amazon:webservices`" + `. It maps ` + "`scim.role_attribute`" + ` to ` + "`https://aws.amazon.com/SAML/Attributes/Role`" + `, and the user's ` + "`mail`" + ` to ` + "`https://aws.amazon.com/SAML/Attributes/RoleSessionName`" + ` and ` + "`https://aws.amazon.com/SAML/Attributes/SourceIdentity`" + `, with the URI name format. Its subject uses the persistent NameID format. In its signature policy, select **Always sign the SAML Assertion**: AWS refuses a response that signs only the SAML response.

- Read access to that one secret, and no other, for the connector's identity (its ` + "`connector_identity`" + `). P0 names the secret in each call, so a call could point the connector at any other secret it can read.

  On AWS, give the connector's role ` + "`secretsmanager:GetSecretValue`" + ` on ` + "`arn:aws:secretsmanager:<secrets_region>:<account_id>:secret:<name>-??????`" + `. Secrets Manager adds a hyphen and six random characters to the name in a secret's ARN, and ` + "`??????`" + ` matches those, so the grant covers this one secret. If a customer-managed KMS key encrypts the secret, also give the role ` + "`kms:Decrypt`" + ` on that key, with the condition that ` + "`kms:ViaService`" + ` is ` + "`secretsmanager.<secrets_region>.amazonaws.com`" + `. A secret outside the connector's account needs a customer-managed key, because no other account can use the default ` + "`aws/secretsmanager`" + ` key. In the secret's account, the secret's resource policy and the key's policy must also allow the connector's role.

  On Google Cloud, give the connector's service account ` + "`roles/secretmanager.secretAccessor`" + ` and ` + "`roles/secretmanager.viewer`" + ` on the secret itself, not on its project.

- P0's secret manager integration for where the secret is: AWS Secrets Manager (` + "`p0_aws_secrets_manager`" + `) for the AWS account, or Google Secret Manager for the Google Cloud project.

**Note:** This integration is currently in preview.

For the AWS install steps, see [Setting up AWS IAM management](https://docs.p0.dev/integrations/resource-integrations/aws/installation-methods).`,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "A name you choose for the app, such as `acme-aws`. Use lowercase letters, digits and hyphens only, starting and ending with a letter or digit, up to 63 characters.",
				Validators:          connectorIdValidators(),
				PlanModifiers:       replace,
			},
			"pingfederate_url": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "PingFederate's runtime base URL, such as `https://sso.example.com:9031`. P0 reads the SAML metadata of its SP connection to AWS from it. With `scim`, P0 also calls its SCIM endpoint there.",
				PlanModifiers:       replace,
			},
			"scim":             scim,
			"attribute_source": attributeSourceRoleSource{}.attribute(),
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
func (*PingFederateAwsIamFederationApp) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var pingFederateUrl types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("pingfederate_url"), &pingFederateUrl)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// P0's `validatePingFederateUrl`.
	validateField(path.Root("pingfederate_url"), pingFederateUrl, []rule{{summary: "Invalid PingFederate URL", message: invalidPingFederateUrl, test: PingFederateUrlRegex.MatchString}}, &resp.Diagnostics)

	for _, source := range roleSources {
		source.validate(ctx, req.Config, &resp.Diagnostics)
	}
}

func (r *PingFederateAwsIamFederationApp) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:        PingFederateAwsIamFederationKey,
		Component:          installresources.App,
		ProviderData:       data,
		GetId:              r.getId,
		GetItemJson:        r.getItemJson,
		FromJson:           r.fromJson,
		ToJson:             r.toJson,
		DescribeCheckError: describeCheckError("app"),
	}
}

func (r *PingFederateAwsIamFederationApp) getId(data any) *string {
	model, ok := data.(*pingFederateAwsIamFederationAppModel)
	if !ok {
		return nil
	}
	str := model.Id.ValueString()
	return &str
}

// Returns the whole response rather than the item alone, because the metadata URL is in
// the response's metadata, beside the item. A response with no item is a bad response,
// not an item to read.
func (r *PingFederateAwsIamFederationApp) getItemJson(json any) any {
	inner, ok := json.(*pingFederateAwsIamFederationAppApi)
	if !ok || inner.Item == nil {
		return nil
	}
	return inner
}

func (r *PingFederateAwsIamFederationApp) fromJson(_ context.Context, diags *diag.Diagnostics, id string, json any) any {
	api, ok := json.(*pingFederateAwsIamFederationAppApi)
	if !ok || api.Item == nil {
		return nil
	}
	jsonv := api.Item

	// Every app has a role source, so one without is a bad response.
	if jsonv.RoleSource == nil {
		diags.AddError(
			"Bad API response",
			fmt.Sprintf("The PingFederate (AWS IAM Federation) app %s carries no role source. "+
				"Remove it in the P0 app and add it again, or remove it from Terraform state with `terraform state rm`.", id),
		)
		return nil
	}
	source, known := roleSources[jsonv.RoleSource.Type]
	if !known {
		diags.AddError(
			"Unsupported PingFederate (AWS IAM Federation) app",
			fmt.Sprintf("The PingFederate (AWS IAM Federation) app %s has a %q role source. This provider supports only %s and %s. "+
				"Upgrade the provider, manage the app in the P0 app, or remove it from Terraform state with `terraform state rm`.",
				id, jsonv.RoleSource.Type, attributeSourceRoleSourceType, scimRoleSourceType),
		)
		return nil
	}

	samlMetadataUrl := types.StringNull()
	if api.Metadata != nil {
		samlMetadataUrl = types.StringPointerValue(api.Metadata.SamlMetadataUrl)
	}

	model := pingFederateAwsIamFederationAppModel{
		Id:              types.StringValue(id),
		PingFederateUrl: types.StringPointerValue(jsonv.PingFederateUrl),
		SamlMetadataUrl: samlMetadataUrl,
		State:           types.StringPointerValue(jsonv.State),
	}
	source.fromJson(r.installer, diags, id, jsonv.RoleSource, &model)
	if diags.HasError() {
		return nil
	}
	return &model
}

// The body of the PUT that stages the item, and of verify and configure. Sending the
// fields that P0 fixes on each step has P0 refuse a request that would change any of
// them, before it calls the connector.
func (r *PingFederateAwsIamFederationApp) toJson(data any) any {
	datav, ok := data.(*pingFederateAwsIamFederationAppModel)
	if !ok {
		return nil
	}

	json := pingFederateAwsIamFederationAppJson{PingFederateUrl: datav.PingFederateUrl.ValueStringPointer()}
	for _, source := range roleSources {
		if roleSource := source.toJson(datav); roleSource != nil {
			json.RoleSource = roleSource
		}
	}
	return &json
}

// P0 checks every field before it creates the item, without repeating the value, which
// may be the SCIM password pasted in place of the secret's name or another value. So a value it
// refuses leaves nothing staged. A check that fails after that leaves the staged item
// in state, tainted, and the next apply replaces it, because P0 refuses a second PUT of
// an app that's already added.
func (r *PingFederateAwsIamFederationApp) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json pingFederateAwsIamFederationAppApi
	var data pingFederateAwsIamFederationAppModel

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

func (r *PingFederateAwsIamFederationApp) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &pingFederateAwsIamFederationAppApi{}, &pingFederateAwsIamFederationAppModel{})
}

// Plans an update for an app that P0 hasn't installed, so that the next apply finishes
// the install.
func (*PingFederateAwsIamFederationApp) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	planUnfinishedInstall(ctx, req, resp)
}

// An update finishes an install, or changes an attribute source's fields in place. An
// app at "stage" still needs verify, where P0 runs the SCIM checks, and then configure.
// One that P0 has verified needs configure alone, which saves nothing unless it
// accepts the planned values.
func (r *PingFederateAwsIamFederationApp) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var state types.String
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("state"), &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	upsert := r.installer.UpsertFromConfigure
	if state.ValueString() == common.StateStage {
		upsert = r.installer.UpsertFromStage
	}
	upsert(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &pingFederateAwsIamFederationAppApi{}, &pingFederateAwsIamFederationAppModel{})
}

func (r *PingFederateAwsIamFederationApp) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Delete(ctx, &resp.Diagnostics, &req.State, &pingFederateAwsIamFederationAppModel{})
}

func (r *PingFederateAwsIamFederationApp) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
