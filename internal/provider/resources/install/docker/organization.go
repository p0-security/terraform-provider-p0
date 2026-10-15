package installdocker

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

var _ resource.Resource = &DockerOrganization{}
var _ resource.ResourceWithConfigure = &DockerOrganization{}
var _ resource.ResourceWithImportState = &DockerOrganization{}
var _ resource.ResourceWithModifyPlan = &DockerOrganization{}
var _ resource.ResourceWithValidateConfig = &DockerOrganization{}

type DockerOrganization struct {
	installer *common.Install
}

type dockerOrganizationModel struct {
	Id              types.String `tfsdk:"id"`
	Connector       types.String `tfsdk:"connector"`
	Vault           *vaultModel  `tfsdk:"vault"`
	TokenSecretName types.String `tfsdk:"token_secret_name"`
	AllowedDomains  types.String `tfsdk:"allowed_domains"`
	State           types.String `tfsdk:"state"`
}

// The item, as P0 stores it.
type dockerOrganizationJson struct {
	Connector       *string    `json:"connector,omitempty"`
	Vault           *vaultJson `json:"vault,omitempty"`
	TokenSecretName *string    `json:"tokenSecretName,omitempty"`
	AllowedDomains  *string    `json:"allowedDomains,omitempty"`
	State           *string    `json:"state,omitempty"`
}

type dockerOrganizationApi struct {
	Item *dockerOrganizationJson `json:"item"`
}

func NewDockerOrganization() resource.Resource {
	return &DockerOrganization{}
}

func (*DockerOrganization) Metadata(_ context.Context, req resource.MetadataRequest, res *resource.MetadataResponse) {
	res.TypeName = req.ProviderTypeName + "_docker_organization"
}

func (*DockerOrganization) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A Docker Hub organization that P0 can grant roles in, through one of your Docker Hub connectors.

P0 never holds the organization's access token. You store it in your own secret manager, in the connector's cloud, and the connector reads it on every call. This resource tells P0 which connector acts on the organization and where the token's secret is.

**Important:** Create the token, store it and let the connector read it before you apply this resource, in an earlier apply or in the same one with this resource depending on the connector's read grant, as the example does. Creating this resource has P0 validate the install through the connector: the connector reads the secret, signs in to Docker Hub with the token, lists the organization's members and tries an invite as a dry run, which sends nothing and uses no seat. If a check fails, the apply fails with P0's message. IAM changes can take a few seconds to take effect, so if the connector is denied the secret right after it is granted, apply again.

The connector, the vault and the secret are fixed when the organization is added, so changing ` + "`id`" + `, ` + "`connector`" + `, ` + "`vault`" + ` or ` + "`token_secret_name`" + ` replaces the installation. To rotate the token, store the new one as a new version of the same secret, which needs no change here. If the organization is already added in P0, import it rather than creating it: P0 refuses to add an organization twice. An installation that P0 hasn't finished, such as one imported before its checks passed, plans an update, and applying it finishes the install.

**Prerequisites:**

- A Docker Hub connector, installed with ` + "`p0_docker_connector`" + `. The token's secret must be in the connector's cloud: AWS Secrets Manager for a connector on AWS Lambda, or Google Secret Manager for one on Cloud Run. It can be in another account or project.

- An organization access token, which needs a Docker Team or Business plan. In Docker Home, open the organization, go to **Identity & auth**, then **Access tokens**, and generate a token with the organization scopes **Member Edit** and **Invite Edit**, and no repository scopes. Store it as a secret in the account and region, or the project, that ` + "`vault`" + ` names, outside Terraform, which keeps the token out of Terraform's state.

- Read access to that one secret for the connector's identity, the connector's ` + "`connector_identity`" + `, and to no other secret. P0 names the secret in each call, so any other secret the connector can read is one a call could point it at.

  On AWS, give the connector's role ` + "`secretsmanager:GetSecretValue`" + ` on ` + "`arn:aws:secretsmanager:<secrets_region>:<account_id>:secret:<name>-??????`" + `. Secrets Manager adds a hyphen and six random characters to the name in a secret's ARN, and ` + "`??????`" + ` matches those, so the grant covers this one secret. If a customer-managed KMS key encrypts the secret, also give the role ` + "`kms:Decrypt`" + ` on that key, with the condition that ` + "`kms:ViaService`" + ` is ` + "`secretsmanager.<secrets_region>.amazonaws.com`" + `. A secret in an account other than the connector's needs a customer-managed key, because the default ` + "`aws/secretsmanager`" + ` key can't be used from another account, and in that account the secret's resource policy and the key's policy must allow the connector's role too.

  On Google Cloud, give the connector's service account ` + "`roles/secretmanager.secretAccessor`" + ` and ` + "`roles/secretmanager.viewer`" + ` on the secret itself, not on its project.

- P0's secret manager integration for where the secret is: AWS Secrets Manager (` + "`p0_aws_secrets_manager`" + `) for the AWS account, or Google Secret Manager for the Google Cloud project.

**Note:** This integration is currently in beta.

For the full install steps, see [Docker Hub](https://docs.p0.dev/integrations/resource-integrations/docker).`,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The organization's name on Docker Hub, as in its repositories' names: `<organization>/<repository>`",
				Validators: []validator.String{
					stringvalidator.RegexMatches(OrganizationRegex, "Enter the organization's name as Docker Hub shows it: lowercase letters, digits, hyphens and underscores, starting with a letter or digit"),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"connector": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The connector that acts on this organization: the `id` of a `p0_docker_connector` resource. Requires that connector to be installed.",
				Validators:          connectorIdValidators(),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"vault": vaultAttribute(
				`The secret manager that holds the organization access token. P0 never reads the token. Your connector does. It must be in the connector's cloud, and can be in another account or project.`,
				secretPlace.what,
			),
			// P0 fixes the token's secret when it creates the item, and refuses a change
			// to it, so a new secret replaces the installation.
			"token_secret_name": schema.StringAttribute{
				Required: true,
				MarkdownDescription: `The name of the secret that holds the organization access token, such as ` + "`docker/my-docker-org/token`" + ` in AWS Secrets Manager or ` + "`docker-my-docker-org-token`" + ` in Google Secret Manager: the name alone, not the secret's ARN or its full resource name. ` +
					`In AWS Secrets Manager the name can't end in a hyphen and six letters or digits, like ` + "`-AbCdEf`" + `, because Secrets Manager can't find a secret by such a name. ` +
					`Changing it replaces the installation. To rotate the token, store the new one as a new version of the same secret, which needs no change here.`,
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

// Validating here rather than in Create surfaces mistakes at plan time, before P0 is
// called, in P0's words. A value that Terraform only knows at apply time is skipped
// while planning, and checked when Terraform validates the configuration again before
// it applies.
func (*DockerOrganization) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var secretName types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("token_secret_name"), &secretName)...)
	vault := vaultFromConfig(ctx, req.Config, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	validateTokenSecret(vault, secretName, &resp.Diagnostics)
}

func (r *DockerOrganization) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:        DockerKey,
		Component:          installresources.Organization,
		ProviderData:       data,
		GetId:              r.getId,
		GetItemJson:        r.getItemJson,
		FromJson:           r.fromJson,
		ToJson:             r.toJson,
		DescribeCheckError: describeCheckError("organization"),
	}
}

// The item is keyed by the organization's name.
func (r *DockerOrganization) getId(data any) *string {
	model, ok := data.(*dockerOrganizationModel)
	if !ok {
		return nil
	}
	str := model.Id.ValueString()
	return &str
}

// A response with no item is a bad response, not an item to read. Its nil item,
// returned as it is, wouldn't compare equal to nil, and would be read as one.
func (r *DockerOrganization) getItemJson(json any) any {
	inner, ok := json.(*dockerOrganizationApi)
	if !ok || inner.Item == nil {
		return nil
	}
	return inner.Item
}

func (r *DockerOrganization) fromJson(_ context.Context, diags *diag.Diagnostics, id string, json any) any {
	jsonv, ok := json.(*dockerOrganizationJson)
	if !ok {
		return nil
	}

	// vault is Required, so leaving it nil here would surface as Terraform core's
	// "provider produced inconsistent result after apply" rather than something the
	// reader can act on.
	if jsonv.Vault == nil {
		diags.AddError(
			"Bad API response",
			fmt.Sprintf("The Docker Hub organization %s carries no vault configuration. "+
				"Remove it in the P0 app and add it again, or remove it from Terraform state with `terraform state rm`.", id),
		)
		return nil
	}

	// A vault of a type this provider doesn't know can't be read into the attribute's
	// fields.
	if !slices.Contains(vaultTypes, jsonv.Vault.Type) {
		diags.AddError(
			"Unsupported Docker Hub organization",
			fmt.Sprintf("The Docker Hub organization %s has a %q vault. This provider supports only %s. "+
				"Upgrade the provider, manage the organization in the P0 app, or remove it from Terraform state with `terraform state rm`.",
				id, jsonv.Vault.Type, strings.Join(vaultTypes, " and ")),
		)
		return nil
	}

	return &dockerOrganizationModel{
		Id:              types.StringValue(id),
		Connector:       types.StringPointerValue(jsonv.Connector),
		Vault:           vaultFromJson(jsonv.Vault),
		TokenSecretName: types.StringPointerValue(jsonv.TokenSecretName),
		AllowedDomains:  types.StringPointerValue(jsonv.AllowedDomains),
		State:           types.StringPointerValue(jsonv.State),
	}
}

// The body of the PUT that stages the item, and of verify and configure: the
// connector, the vault and the token's secret, which P0 fixes when it creates the
// item. Sending them on each step has P0 refuse a request that would move any of
// them, before it calls the connector. P0 reads the allowed domains back from the
// connector, and sets the state.
func (r *DockerOrganization) toJson(data any) any {
	datav, ok := data.(*dockerOrganizationModel)
	if !ok {
		return nil
	}

	json := dockerOrganizationJson{
		Connector:       datav.Connector.ValueStringPointer(),
		TokenSecretName: datav.TokenSecretName.ValueStringPointer(),
	}
	if datav.Vault != nil {
		json.Vault = datav.Vault.toJson()
	}
	return &json
}

// P0 checks every field before it creates the item, and its error for one doesn't
// repeat the value, which may be a token pasted in place of the secret's name. So a
// value it refuses leaves nothing staged. A check that fails after that leaves the
// staged item in state, tainted, and the next apply replaces it: P0 refuses a second
// PUT of an organization that's already added.
func (r *DockerOrganization) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json dockerOrganizationApi
	var data dockerOrganizationModel

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

func (r *DockerOrganization) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &dockerOrganizationApi{}, &dockerOrganizationModel{})
}

// Plans an update for an organization that P0 hasn't installed, so that the next apply
// finishes the install. That is when P0 reads the allowed domains back from the
// connector, so they are unknown until then.
func (*DockerOrganization) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	planUnfinishedInstall(ctx, req, resp, path.Root("allowed_domains"))
}

// Every configurable attribute requires replacement, so the only update Terraform
// plans is the one that finishes an install, which verifies and configures the item as
// Create does. P0 validates the install on verify alone, so there is no shorter way.
func (r *DockerOrganization) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &dockerOrganizationApi{}, &dockerOrganizationModel{})
}

func (r *DockerOrganization) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Delete(ctx, &resp.Diagnostics, &req.State, &dockerOrganizationModel{})
}

func (r *DockerOrganization) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
