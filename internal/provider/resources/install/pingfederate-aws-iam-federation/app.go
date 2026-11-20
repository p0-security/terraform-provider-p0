package installpingfederateawsiamfederation

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
	Id               types.String `tfsdk:"id"`
	Connector        types.String `tfsdk:"connector"`
	PingFederateUrl  types.String `tfsdk:"pingfederate_url"`
	RoleAttribute    types.String `tfsdk:"role_attribute"`
	ScimUsername     types.String `tfsdk:"scim_username"`
	Vault            *vaultModel  `tfsdk:"vault"`
	ScimPasswordName types.String `tfsdk:"scim_password_name"`
	CaCertificate    types.String `tfsdk:"ca_certificate"`
	SamlMetadataUrl  types.String `tfsdk:"saml_metadata_url"`
	State            types.String `tfsdk:"state"`
}

type pingFederateAwsIamFederationAppJson struct {
	Connector        *string    `json:"connector,omitempty"`
	PingFederateUrl  *string    `json:"pingFederateUrl,omitempty"`
	RoleAttribute    *string    `json:"roleAttribute,omitempty"`
	ScimUsername     *string    `json:"scimUsername,omitempty"`
	Vault            *vaultJson `json:"vault,omitempty"`
	ScimPasswordName *string    `json:"scimPasswordName,omitempty"`
	CaCertificate    *string    `json:"caCertificate,omitempty"`
	State            *string    `json:"state,omitempty"`
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
	resp.Schema = schema.Schema{
		MarkdownDescription: `A PingFederate SP connection that signs users in to AWS, and the SCIM connection that P0 assigns AWS roles through, from one of your PingFederate (AWS IAM Federation) connectors. A ` + "`p0_aws_iam_federation`" + ` resource has an AWS account trust it.

The connector writes each user's AWS roles to Active Directory through PingFederate's SCIM inbound provisioning, and signs in to it with HTTP Basic credentials. P0 never holds the SCIM password. You store it in your own secret manager, in the connector's cloud, and the connector reads it on every call. This resource tells P0 which connector acts on the app, PingFederate's URL, the role attribute, the SCIM user name, and where the SCIM password is.

**Important:** Set up PingFederate, store the SCIM password and let the connector read it before Terraform creates this resource. Do that in an earlier apply, or in the same apply with this resource depending on the connector's read grant. Creating this resource has P0 validate the install through the connector: the connector reads the secret, searches PingFederate's SCIM users by email, checks the role attribute in the custom SCIM schema, and reads PingFederate's SAML metadata for AWS. If a check fails, the apply fails with P0's message. IAM changes can take a few seconds to propagate, so if the connector is denied the secret right after you grant it, apply again.

P0 fixes every attribute when it adds the app, so changing any of them replaces the installation. To rotate the SCIM password, store the new one as a new version of the same secret, which needs no change here. If the app is already added in P0, import it rather than creating it: P0 refuses to add an app twice. If P0 hasn't finished installing the app, for example because you imported it before its checks passed, Terraform plans an update, and applying it finishes the install.

**Prerequisites:**

- A PingFederate (AWS IAM Federation) connector, installed with ` + "`p0_pingfederate_aws_iam_federation_connector`" + `. The connector must reach PingFederate's runtime at ` + "`pingfederate_url`" + `, including ` + "`/pf-scim`" + `. The SCIM password must be in the connector's cloud: AWS Secrets Manager for a connector on AWS Lambda, or Google Secret Manager for one on Cloud Run. It can be in another account or project.

- A multi-valued string attribute on users in the Active Directory schema, named by ` + "`role_attribute`" + `, that P0 adds each user's AWS roles to. The account of PingFederate's Active Directory data store must be able to write it.

- A PingFederate IdP connection with only Inbound Provisioning, on SCIM 2.0, whose user repository is the Active Directory data store. Add ` + "`role_attribute`" + ` to its custom SCIM schema as a multi-valued, read-write string. In Write Users, map the Active Directory attribute to the SCIM attribute of the same name. In Read Users, map ` + "`emails.work.value`" + ` to ` + "`mail`" + `, and ` + "`role_attribute`" + ` to itself. A SCIM search must return each user's ` + "`id`" + `. The connection uses HTTP Basic authentication with the user name ` + "`scim_username`" + `. Store its password outside Terraform, as the secret's whole value, in the account and region or the project that ` + "`vault`" + ` names. Storing it outside Terraform keeps it out of Terraform's state.

- A PingFederate SP connection to the partner ` + "`urn:amazon:webservices`" + `. It maps ` + "`role_attribute`" + ` to ` + "`https://aws.amazon.com/SAML/Attributes/Role`" + `, and the user's ` + "`mail`" + ` to ` + "`https://aws.amazon.com/SAML/Attributes/RoleSessionName`" + ` and ` + "`https://aws.amazon.com/SAML/Attributes/SourceIdentity`" + `, with the URI name format. Its subject uses the persistent NameID format. In its signature policy, select **Always sign the SAML Assertion**: AWS refuses a response that signs only the SAML response.

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
			"connector": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The connector that acts on this app: the `id` of a `p0_pingfederate_aws_iam_federation_connector` resource. Requires that connector to be installed. It must reach PingFederate's runtime, including `/pf-scim`.",
				Validators:          connectorIdValidators(),
				PlanModifiers:       replace,
			},
			"pingfederate_url": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "PingFederate's runtime base URL, such as `https://sso.example.com:9031`. P0 calls its SCIM endpoint, and reads the SAML metadata of its SP connection to AWS, from it.",
				PlanModifiers:       replace,
			},
			"role_attribute": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The multi-valued attribute in the SCIM connection's custom schema that holds each user's AWS roles, such as `awsRole`. Active Directory has an attribute of the same name. The SP connection maps it to AWS's Role attribute.",
				PlanModifiers:       replace,
			},
			"scim_username": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The user name of the HTTP Basic credentials on PingFederate's SCIM IdP connection, which writes to Active Directory",
				PlanModifiers:       replace,
			},
			"vault": vaultAttribute(
				`The secret manager that holds the SCIM password. Your connector reads the secret, and P0 never does. It must be in the connector's cloud, and can be in another account or project there.`,
				secretPlace.what,
			),
			"scim_password_name": schema.StringAttribute{
				Required: true,
				MarkdownDescription: `The name of the secret that holds the SCIM connection's password, such as ` + "`pingfederate/acme/p0`" + ` in AWS Secrets Manager or ` + "`pingfederate-acme-p0`" + ` in Google Secret Manager: the name alone, not the secret's ARN or its full resource name. ` +
					`In AWS Secrets Manager the name can't end in a hyphen and six letters or digits, like ` + "`-AbCdEf`" + `, because Secrets Manager can't find a secret by such a name. ` +
					`To rotate the SCIM password, store the new one as a new version of the same secret, which needs no change here.`,
				PlanModifiers: replace,
			},
			"ca_certificate": schema.StringAttribute{
				Optional: true,
				MarkdownDescription: `PEM certificates of the CA that issued PingFederate's TLS certificate, if it isn't a public CA. ` +
					`P0 removes the whitespace around the value, so remove it here too, for example with ` + "`trimspace(file(\"ca.pem\"))`" + `.`,
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
func (*PingFederateAwsIamFederationApp) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var pingFederateUrl, roleAttribute, scimUsername, secretName, caCertificate types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("pingfederate_url"), &pingFederateUrl)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("role_attribute"), &roleAttribute)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("scim_username"), &scimUsername)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("scim_password_name"), &secretName)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("ca_certificate"), &caCertificate)...)
	vault := vaultFromConfig(ctx, req.Config, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	// P0's checks (`validatePingFederateUrl`, `validateRoleAttribute`,
	// `validateScimUsername` and `validateCaCertificate` in
	// packages/integrations/pingfederate-aws-iam-federation/backend/src/install/validation.ts in
	// the app). None repeats the value, because it may be the pasted SCIM password.
	validateField(path.Root("pingfederate_url"), pingFederateUrl, []rule{{summary: "Invalid PingFederate URL", message: invalidPingFederateUrl, test: PingFederateUrlRegex.MatchString}}, &resp.Diagnostics)
	validateField(path.Root("role_attribute"), roleAttribute, []rule{{summary: "Invalid role attribute", message: invalidRoleAttribute, test: RoleAttributeRegex.MatchString}}, &resp.Diagnostics)
	validateField(path.Root("scim_username"), scimUsername, []rule{{summary: "Invalid SCIM user name", message: invalidScimUsername, test: ScimUsernameRegex.MatchString}}, &resp.Diagnostics)
	// P0 accepts an empty certificate.
	validateField(path.Root("ca_certificate"), caCertificate, []rule{{summary: "Invalid CA certificate", message: invalidCaCertificate, test: func(pem string) bool {
		return pem == "" || strings.Contains(pem, pemCertificateBegin)
	}}}, &resp.Diagnostics)
	validateScimPassword(vault, secretName, &resp.Diagnostics)
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

	// vault is Required, so a nil vault would surface as Terraform core's "provider
	// produced inconsistent result after apply", which tells the reader nothing to do.
	if jsonv.Vault == nil {
		diags.AddError(
			"Bad API response",
			fmt.Sprintf("The PingFederate (AWS IAM Federation) app %s carries no vault configuration. "+
				"Remove it in the P0 app and add it again, or remove it from Terraform state with `terraform state rm`.", id),
		)
		return nil
	}

	// A vault of a type this provider doesn't know can't be read into the attribute's
	// fields.
	if !slices.Contains(vaultTypes, jsonv.Vault.Type) {
		diags.AddError(
			"Unsupported PingFederate (AWS IAM Federation) app",
			fmt.Sprintf("The PingFederate (AWS IAM Federation) app %s has a %q vault. This provider supports only %s. "+
				"Upgrade the provider, manage the app in the P0 app, or remove it from Terraform state with `terraform state rm`.",
				id, jsonv.Vault.Type, strings.Join(vaultTypes, " and ")),
		)
		return nil
	}

	samlMetadataUrl := types.StringNull()
	if api.Metadata != nil {
		samlMetadataUrl = types.StringPointerValue(api.Metadata.SamlMetadataUrl)
	}

	return &pingFederateAwsIamFederationAppModel{
		Id:               types.StringValue(id),
		Connector:        types.StringPointerValue(jsonv.Connector),
		PingFederateUrl:  types.StringPointerValue(jsonv.PingFederateUrl),
		RoleAttribute:    types.StringPointerValue(jsonv.RoleAttribute),
		ScimUsername:     types.StringPointerValue(jsonv.ScimUsername),
		Vault:            vaultFromJson(jsonv.Vault),
		ScimPasswordName: types.StringPointerValue(jsonv.ScimPasswordName),
		CaCertificate:    types.StringPointerValue(jsonv.CaCertificate),
		SamlMetadataUrl:  samlMetadataUrl,
		State:            types.StringPointerValue(jsonv.State),
	}
}

// The body of the PUT that stages the item, and of verify and configure: the fields
// that P0 fixes when it creates the item. Sending them on each step has P0 refuse a
// request that would change any of them, before it calls the connector.
func (r *PingFederateAwsIamFederationApp) toJson(data any) any {
	datav, ok := data.(*pingFederateAwsIamFederationAppModel)
	if !ok {
		return nil
	}

	json := pingFederateAwsIamFederationAppJson{
		Connector:        datav.Connector.ValueStringPointer(),
		PingFederateUrl:  datav.PingFederateUrl.ValueStringPointer(),
		RoleAttribute:    datav.RoleAttribute.ValueStringPointer(),
		ScimUsername:     datav.ScimUsername.ValueStringPointer(),
		ScimPasswordName: datav.ScimPasswordName.ValueStringPointer(),
		CaCertificate:    datav.CaCertificate.ValueStringPointer(),
	}
	if datav.Vault != nil {
		json.Vault = datav.Vault.toJson()
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

// Every configurable attribute requires replacement, so the only update Terraform
// plans is the one that finishes an install, which verifies and configures the item as
// Create does. P0 validates the install on verify alone, so there is no shorter way.
func (r *PingFederateAwsIamFederationApp) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &pingFederateAwsIamFederationAppApi{}, &pingFederateAwsIamFederationAppModel{})
}

func (r *PingFederateAwsIamFederationApp) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Delete(ctx, &resp.Diagnostics, &req.State, &pingFederateAwsIamFederationAppModel{})
}

func (r *PingFederateAwsIamFederationApp) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
