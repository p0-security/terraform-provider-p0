package installgithubrepositories

import (
	"context"
	"fmt"
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
	installapp "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/app"
)

var _ resource.Resource = &RepositoryAccess{}
var _ resource.ResourceWithConfigure = &RepositoryAccess{}
var _ resource.ResourceWithImportState = &RepositoryAccess{}
var _ resource.ResourceWithModifyPlan = &RepositoryAccess{}
var _ resource.ResourceWithValidateConfig = &RepositoryAccess{}

type RepositoryAccess struct {
	installer *common.Install
}

type repositoryAccessModel struct {
	Org                  types.String                      `tfsdk:"org"`
	AppId                types.String                      `tfsdk:"app_id"`
	Vault                *vaultModel                       `tfsdk:"vault"`
	PrivateKeySecretName types.String                      `tfsdk:"private_key_secret_name"`
	Hosting              *installapp.ConnectorHostingModel `tfsdk:"hosting"`
	State                types.String                      `tfsdk:"state"`
}

// The item, as P0 stores it.
type repositoryAccessJson struct {
	AppId                *string                          `json:"appId,omitempty"`
	Vault                *vaultJson                       `json:"vault,omitempty"`
	PrivateKeySecretName *string                          `json:"privateKeySecretName,omitempty"`
	Hosting              *installapp.ConnectorHostingJson `json:"hosting,omitempty"`
	State                *string                          `json:"state,omitempty"`
}

type repositoryAccessApi struct {
	Item *repositoryAccessJson `json:"item"`
}

// The body of verify and configure: the GitHub App and the key's secret, which can
// change in place. Verify gets them too, so that its install check, which runs
// whenever both are set, checks these rather than the ones P0 stored for a staged
// item.
type repositoryAccessConfigureJson struct {
	AppId                string `json:"appId"`
	PrivateKeySecretName string `json:"privateKeySecretName"`
}

func NewRepositoryAccess() resource.Resource {
	return &RepositoryAccess{}
}

func (*RepositoryAccess) Metadata(_ context.Context, req resource.MetadataRequest, res *resource.MetadataResponse) {
	res.TypeName = req.ProviderTypeName + "_github_repositories"
}

func (*RepositoryAccess) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A GitHub Repositories installation for one GitHub organization.

Installing GitHub Repositories lets P0 grant your organization's members just-in-time roles on its repositories. P0 acts through a GitHub App that you create and install on the organization, and through a connector that you deploy in your own AWS or Google Cloud account. The App's private key stays in your secret manager, and only the connector reads it. Each organization needs its own App and its own connector.

**Important:** Create the App and store its private key before you apply this resource, and deploy the connector first: in an earlier apply, or in the same one with this resource depending on the connector's access grants, as the example does. Creating this resource has P0 check the install through the connector: that P0 can invoke the connector, that the connector can read the private key, and that the App is installed on the organization with the permissions it needs. If a check fails, the apply fails with P0's message.

The vault and the connector must be in the same cloud: ` + "`aws-sm`" + ` with ` + "`aws`" + ` hosting, or ` + "`gcp-sm`" + ` with ` + "`gcp`" + ` hosting. Changing ` + "`vault`" + ` or ` + "`hosting`" + ` replaces the installation. Changing ` + "`app_id`" + ` or ` + "`private_key_secret_name`" + ` updates it in place, and P0 checks the install again. If the check fails, the apply fails, and an installed organization keeps its current App and secret. An installation that P0 hasn't finished, such as one imported before its checks passed, plans an update, and applying it finishes the install.

P0 checks where the connector runs when it creates the installation, and so does a plan. ` + "`hosting.connector_name`" + ` has 2 to 64 characters on Lambda, or 2 to 49 on Cloud Run: lowercase letters, digits and single hyphens, starting with a letter and ending with a letter or digit. On AWS, the connector and the secret must be in commercial regions: GitHub Repositories doesn't support AWS GovCloud, China, ISO or European Sovereign Cloud regions yet.

**Prerequisites:**

- A GitHub App for the organization. Make it private, turn off its webhook, and give it these permissions:
  - Repository: Administration (read and write) and Metadata (read)
  - Organization: Members (read) and Custom repository roles (read)

  An owner of the organization installs the App. Generate a private key for it, and store the key as a secret in AWS Secrets Manager or Google Secret Manager.

  **Warning:** Administration: write lets the App change roles on, and administer, every repository it is installed on. Select only the repositories P0 should manage.

- P0's GitHub Repositories connector, from the image ` + "`p0security/p0-connector-github-repositories`" + ` on AWS Lambda or ` + "`p0security/p0-connector-github-repositories-gcloud`" + ` on Google Cloud Run. Lambda runs images only from Amazon ECR, so copy the Lambda image from Docker Hub into ECR first. P0's GitHub Repositories installer generates Terraform that deploys the connector with its image pinned, as in the example.

  Grant the connector's identity read access to the private key secret alone, never through a wildcard or at the account or project level. P0 names the secret in each call, so any other secret the connector can read is one a call could point it at.
  - On AWS, give its execution role ` + "`secretsmanager:GetSecretValue`" + ` on the secret. If a customer-managed KMS key encrypts the secret, also give the role ` + "`kms:Decrypt`" + ` on that key, with the condition that ` + "`kms:ViaService`" + ` is ` + "`secretsmanager.<region>.amazonaws.com`" + `. For its logs, give the role ` + "`logs:CreateLogStream`" + ` and ` + "`logs:PutLogEvents`" + ` on its own log group alone, ` + "`/aws/lambda/<function name>`" + `. The role can't create that log group, so create it before the function first runs.
  - On Google Cloud, give its service account ` + "`secretmanager.versions.access`" + ` and ` + "`secretmanager.versions.get`" + ` on the secret, for example by binding ` + "`roles/secretmanager.secretAccessor`" + ` and ` + "`roles/secretmanager.viewer`" + ` on it.

- For AWS Lambda hosting, ` + "`p0_aws_iam_write`" + ` installed for the account the connector runs in. Grant that installation's role ` + "`lambda:InvokeFunction`" + ` on the connector's function.

- For Google Cloud Run hosting, ` + "`p0_gcp`" + ` installed. Grant its service account ` + "`roles/run.invoker`" + ` and ` + "`roles/run.viewer`" + ` on the connector's service: P0 invokes the connector, and reads the service to find its URL. Set the connector's ` + "`INVOKER_SA_EMAIL`" + ` environment variable to the same service account.

**Note:** This integration is currently in preview.`,
		Attributes: map[string]schema.Attribute{
			"org": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The login of the GitHub organization to install on, as in `github.com/<login>`",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			// app_id and private_key_secret_name can change in place, for a new App or a
			// rotated key, so neither requires replacement. P0 checks the install again
			// when either changes.
			"app_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: `The ID of the GitHub App your connector authenticates as: the number on the App's settings page`,
			},
			"vault": vaultAttribute(),
			"private_key_secret_name": schema.StringAttribute{
				Required: true,
				MarkdownDescription: `The name or ARN of the secret that holds the GitHub App's private key. ` +
					`On AWS, a name is looked up in the connector's own account, in ` + "`vault.secrets_region`" + `. ` +
					`Give a secret in another account as its full ARN, and allow the connector's role in the secret's resource policy and in its KMS key's policy. ` +
					`On Google Cloud, this is the secret's ID or its full resource name, ` + "`projects/<project>/secrets/<id>`" + `.`,
			},
			// P0's rules for the connector's name, region, account and project depend on the
			// hosting's type, which a field's validator can't see, so ValidateConfig checks
			// them instead.
			"hosting": installapp.ConnectorHostingAttribute(installapp.ConnectorHostingValidators{}),
			"state":   common.StateAttribute,
		},
	}
}

// Validating here rather than in Create surfaces mistakes at plan time, before P0 is
// called, in P0's words. A value that Terraform only knows at apply time is skipped
// while planning, and checked when Terraform validates the configuration again before
// it applies.
func (*RepositoryAccess) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var org, appId, secretName types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("org"), &org)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("app_id"), &appId)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("private_key_secret_name"), &secretName)...)
	vault := vaultFromConfig(ctx, req.Config, &resp.Diagnostics)
	hosting := installapp.ConnectorHostingFromConfig(ctx, req.Config, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	installapp.ValidateConnectorHosting(hosting, &resp.Diagnostics)
	// The rest in the order P0 checks them when it creates the item.
	validateSameCloud(vault, hosting, &resp.Diagnostics)
	validateField(path.Root("private_key_secret_name"), secretName, secretNameRules(vault), &resp.Diagnostics)
	validateField(path.Root("app_id"), appId, appIdRules, &resp.Diagnostics)
	validateOrgLogin(org, &resp.Diagnostics)
	validateDeployable(vault, hosting, &resp.Diagnostics)
}

// P0's rule for the private key's secret name: a form the vault accepts, which a
// pasted key never is. While the vault's type isn't known, either vault's forms pass.
func secretNameRules(vault *vaultModel) []rule {
	vaultType := ""
	if vault != nil && installapp.IsSet(vault.Type) {
		vaultType = vault.Type.ValueString()
	}
	return []rule{{
		summary: "Invalid private key secret name",
		message: InvalidSecretName,
		test:    func(name string) bool { return isSecretName(name, vaultType) },
	}}
}

var appIdRules = []rule{{
	summary: "Invalid GitHub App ID",
	message: InvalidAppId,
	test:    githubAppIdRegex.MatchString,
}}

// P0 checks the item's ID as it is, without trimming it.
func validateOrgLogin(org types.String, diags *diag.Diagnostics) {
	if installapp.IsSet(org) && !GithubOrgRegex.MatchString(org.ValueString()) {
		diags.AddAttributeError(path.Root("org"), "Invalid GitHub organization login", InvalidOrgLogin)
	}
}

// Rejects value, at attribute, with P0's message for the first of rules that it
// breaks. Like P0, it checks the value without the whitespace around it. That
// whitespace is rejected on its own: P0 trims the strings it stores, so the value
// would read back different from the configuration, which Terraform reports as an
// inconsistent result after apply. A null value is left to the checks that a field is
// set. The value is never repeated, since a secret's name may be a pasted key.
func validateField(attribute path.Path, value types.String, rules []rule, diags *diag.Diagnostics) {
	if !installapp.IsSet(value) {
		return
	}
	trimmed := strings.TrimSpace(value.ValueString())
	for _, r := range rules {
		if !r.test(trimmed) {
			diags.AddAttributeError(attribute, r.summary, r.message)
			return
		}
	}
	if trimmed != value.ValueString() {
		diags.AddAttributeError(
			attribute,
			"Whitespace around a value",
			fmt.Sprintf("P0 removes the whitespace around '%s', so the value it stores wouldn't match this configuration. "+
				"Remove the whitespace, for example with trimspace().", attribute),
		)
	}
}

func (r *RepositoryAccess) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:        GithubRepositoriesKey,
		Component:          installresources.RepositoryAccess,
		ProviderData:       data,
		GetId:              r.getId,
		GetItemJson:        r.getItemJson,
		FromJson:           r.fromJson,
		ToJson:             r.toJson,
		DescribeCheckError: describeCheckError,
	}
}

// P0 runs the install checks through the connector, so a failed check usually means a
// problem in the customer's setup, which P0's message describes. The wording doesn't
// say where: P0 also fails a check, for one, on a Cloud Run service it can't read.
func describeCheckError(org string, err error) (string, string) {
	return "GitHub Repositories install check failed",
		fmt.Sprintf("P0 rejected the install check for the GitHub organization %q:\n\n%s", org, err)
}

func (r *RepositoryAccess) getId(data any) *string {
	model, ok := data.(*repositoryAccessModel)
	if !ok {
		return nil
	}
	str := model.Org.ValueString()
	return &str
}

func (r *RepositoryAccess) getItemJson(json any) any {
	inner, ok := json.(*repositoryAccessApi)
	if !ok {
		return nil
	}
	return inner.Item
}

func (r *RepositoryAccess) fromJson(_ context.Context, diags *diag.Diagnostics, id string, json any) any {
	jsonv, ok := json.(*repositoryAccessJson)
	if !ok {
		return nil
	}

	// vault and hosting are Required, so leaving either nil here would surface as
	// Terraform core's "provider produced inconsistent result after apply" rather than
	// something the reader can act on.
	if jsonv.Vault == nil || jsonv.Hosting == nil {
		diags.AddError(
			"Bad API response",
			fmt.Sprintf("The GitHub Repositories install %s carries no vault or hosting configuration. "+
				"Configure it in the P0 app, or remove it from Terraform state with `terraform state rm`.", id),
		)
		return nil
	}

	return &repositoryAccessModel{
		Org:                  types.StringValue(id),
		AppId:                types.StringPointerValue(jsonv.AppId),
		Vault:                vaultFromJson(jsonv.Vault),
		PrivateKeySecretName: types.StringPointerValue(jsonv.PrivateKeySecretName),
		Hosting:              installapp.ConnectorHostingFromJson(jsonv.Hosting),
		State:                types.StringPointerValue(jsonv.State),
	}
}

// The body of verify and configure, which UpsertFromStage and UpsertFromConfigure send.
func (r *RepositoryAccess) toJson(data any) any {
	datav, ok := data.(*repositoryAccessModel)
	if !ok {
		return nil
	}
	return &repositoryAccessConfigureJson{
		AppId:                datav.AppId.ValueString(),
		PrivateKeySecretName: datav.PrivateKeySecretName.ValueString(),
	}
}

// The body of the PUT that stages the item, which is the whole item. P0 checks each
// field before it creates the item, so a value it refuses leaves nothing staged, and
// its error for one doesn't repeat the value, which may be a key pasted in place of
// the secret's name.
func stageJson(data *repositoryAccessModel) *repositoryAccessJson {
	json := repositoryAccessJson{
		AppId:                data.AppId.ValueStringPointer(),
		PrivateKeySecretName: data.PrivateKeySecretName.ValueStringPointer(),
	}
	if data.Vault != nil {
		json.Vault = data.Vault.toJson()
	}
	if data.Hosting != nil {
		json.Hosting = data.Hosting.ToJson()
	}
	return &json
}

func (r *RepositoryAccess) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json repositoryAccessApi
	var data repositoryAccessModel

	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	if resp.Diagnostics.HasError() {
		return
	}

	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, stageJson(&data))
	if resp.Diagnostics.HasError() {
		return
	}

	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data)
}

func (r *RepositoryAccess) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &repositoryAccessApi{}, &repositoryAccessModel{})
}

// Plans an update for an item that P0 hasn't installed, even when the configuration
// hasn't changed, so that the next apply finishes the install: P0 lists an
// organization's repositories only once its item is installed. That covers an item
// imported before its checks passed, and one that a failed apply left at configure.
// An installed item keeps the framework's plan, which shows no difference while the
// configuration matches it.
func (*RepositoryAccess) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	// Nothing is installed yet on create, and nothing is left to finish on destroy.
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}

	var state types.String
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("state"), &state)...)
	if resp.Diagnostics.HasError() || state.ValueString() == common.StateInstalled {
		return
	}

	// As in any update the framework plans, the attributes that only P0 sets are
	// unknown until apply. Update takes both from P0's response, which has the item's
	// new state and, on Cloud Run, the URL that P0 looked the connector up at.
	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("state"), types.StringUnknown())...)
	var hosting types.Object
	resp.Diagnostics.Append(resp.Plan.GetAttribute(ctx, path.Root("hosting"), &hosting)...)
	if resp.Diagnostics.HasError() || hosting.IsNull() || hosting.IsUnknown() {
		return
	}
	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("hosting").AtName("connector_service_uri"), types.StringUnknown())...)
}

// Only app_id and private_key_secret_name update in place, and P0 checks the install
// with both. An item that P0 has verified, at configure or installed, gets the
// configure step alone: P0 checks the new values and saves nothing if the check fails,
// so the update is all-or-nothing, with one check. Any other item, such as a staged
// one, is verified first, as Create does.
func (r *RepositoryAccess) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var state types.String
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("state"), &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	switch state.ValueString() {
	case common.StateConfigure, common.StateInstalled:
		r.installer.UpsertFromConfigure(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &repositoryAccessApi{}, &repositoryAccessModel{})
	default:
		r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &repositoryAccessApi{}, &repositoryAccessModel{})
	}
}

func (r *RepositoryAccess) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Delete(ctx, &resp.Diagnostics, &req.State, &repositoryAccessModel{})
}

func (r *RepositoryAccess) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("org"), req, resp)
}
