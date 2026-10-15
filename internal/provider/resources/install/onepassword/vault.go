package installonepassword

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

var _ resource.Resource = &OnePasswordVault{}
var _ resource.ResourceWithConfigure = &OnePasswordVault{}
var _ resource.ResourceWithImportState = &OnePasswordVault{}
var _ resource.ResourceWithModifyPlan = &OnePasswordVault{}
var _ resource.ResourceWithValidateConfig = &OnePasswordVault{}

type OnePasswordVault struct {
	installer *common.Install
}

type onePasswordVaultModel struct {
	Id              types.String        `tfsdk:"id"`
	Connector       types.String        `tfsdk:"connector"`
	SecretManager   *secretManagerModel `tfsdk:"secret_manager"`
	TokenSecretName types.String        `tfsdk:"token_secret_name"`
	SignInAddress   types.String        `tfsdk:"sign_in_address"`
	AllowedDomains  types.String        `tfsdk:"allowed_domains"`
	State           types.String        `tfsdk:"state"`
}

// The item, as P0 stores it.
type onePasswordVaultJson struct {
	Connector       *string            `json:"connector,omitempty"`
	SecretManager   *secretManagerJson `json:"secretManager,omitempty"`
	TokenSecretName *string            `json:"tokenSecretName,omitempty"`
	SignInAddress   *string            `json:"signInAddress,omitempty"`
	AllowedDomains  *string            `json:"allowedDomains,omitempty"`
	State           *string            `json:"state,omitempty"`
}

type onePasswordVaultApi struct {
	Item *onePasswordVaultJson `json:"item"`
}

func NewOnePasswordVault() resource.Resource {
	return &OnePasswordVault{}
}

func (*OnePasswordVault) Metadata(_ context.Context, req resource.MetadataRequest, res *resource.MetadataResponse) {
	res.TypeName = req.ProviderTypeName + "_1password_vault"
}

func (*OnePasswordVault) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A 1Password vault whose items P0 can share, through one of your 1Password connectors. When a request for an item is approved, P0 has the connector create a read-only share link to it that only the requestor's email can open.

P0 never holds the vault's service account token. You store it in your own secret manager, in the connector's cloud, and the connector reads it on every call. This resource tells P0 which connector shares the vault's items, where the token's secret is, and your account's sign-in address.

**Important:** Create the token, store it and let the connector read it before you apply this resource, in an earlier apply or in the same one with this resource depending on the connector's read grant, as the example does. Creating this resource has P0 validate the install through the connector: the connector reads the secret, signs in to 1Password with the token, checks that the service account can see the vault, checks that the account's sharing policy allows sharing an item with a specific person by email, and reads its own allowed domains. It creates no share link. If a check fails, the apply fails with P0's message. IAM changes can take a few seconds to take effect, so if the connector is denied the secret right after it is granted, apply again.

The connector, the secret manager, the secret and the sign-in address are fixed when the vault is added, so changing ` + "`id`" + `, ` + "`connector`" + `, ` + "`secret_manager`" + `, ` + "`token_secret_name`" + ` or ` + "`sign_in_address`" + ` replaces the installation. To rotate the token, store the new one as a new version of the same secret, which needs no change here. If the vault is already added in P0, import it rather than creating it: P0 refuses to add a vault twice. An installation that P0 hasn't finished, such as one imported before its checks passed, plans an update, and applying it finishes the install.

**Prerequisites:**

- A 1Password Business or Enterprise account.

- A 1Password connector, installed with ` + "`p0_1password_connector`" + `. The token's secret must be in the connector's cloud: AWS Secrets Manager for a connector on AWS Lambda, or Google Secret Manager for one on Cloud Run. It can be in another account or project.

- A 1Password service account for this vault alone, with the permissions **Read Items** and **Share Items** on this vault and no other vault. Its token starts with ` + "`ops_`" + `. Store the token as a secret in the account and region, or the project, that ` + "`secret_manager`" + ` names, outside Terraform, which keeps the token out of Terraform's state.

- A sharing policy that lets the account's members share items with a specific person by email, for at least as long as people may request access.

- Read access to that one secret for the connector's identity, the connector's ` + "`connector_identity`" + `, and to no other secret. P0 names the secret in each call, so any other secret the connector can read is one a call could point it at.

  On AWS, give the connector's role ` + "`secretsmanager:GetSecretValue`" + ` on ` + "`arn:aws:secretsmanager:<secrets_region>:<account_id>:secret:<name>-??????`" + `. Secrets Manager adds a hyphen and six random characters to the name in a secret's ARN, and ` + "`??????`" + ` matches those, so the grant covers this one secret. If a customer-managed KMS key encrypts the secret, also give the role ` + "`kms:Decrypt`" + ` on that key, with the condition that ` + "`kms:ViaService`" + ` is ` + "`secretsmanager.<secrets_region>.amazonaws.com`" + `. A secret in an account other than the connector's needs a customer-managed key, because the default ` + "`aws/secretsmanager`" + ` key can't be used from another account, and in that account the secret's resource policy and the key's policy must allow the connector's role too.

  On Google Cloud, give the connector's service account ` + "`roles/secretmanager.secretAccessor`" + ` and ` + "`roles/secretmanager.viewer`" + ` on the secret itself, not on its project.

- P0's secret manager integration for where the secret is: AWS Secrets Manager (` + "`p0_aws_secrets_manager`" + `) for the AWS account, or Google Secret Manager for the Google Cloud project.

**Note:** This integration is currently in beta.

For the full install steps, see [1Password](https://docs.p0.dev/integrations/resource-integrations/1password).`,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The vault's ID in 1Password: 26 lowercase letters and digits, such as `3fz7kq2xmw4vjn6rbt5hlpd8yc`. In 1Password, open the vault's menu and copy its ID, or run `op vault list`.",
				Validators: []validator.String{
					stringvalidator.RegexMatches(VaultIdRegex, "Enter the vault's ID as 1Password shows it: 26 lowercase letters and digits. In 1Password, open the vault's menu and copy its ID, or run op vault list"),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"connector": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The connector that shares this vault's items: the `id` of a `p0_1password_connector` resource. Requires that connector to be installed.",
				Validators:          connectorIdValidators(),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"secret_manager": secretManagerAttribute(
				`The secret manager that holds the service account token. P0 never reads the token. Your connector does. It must be in the connector's cloud, and can be in another account or project.`,
				secretPlace.what,
			),
			// P0 fixes the token's secret when it creates the item, and refuses a change
			// to it, so a new secret replaces the installation.
			"token_secret_name": schema.StringAttribute{
				Required: true,
				MarkdownDescription: `The name of the secret that holds the service account token, such as ` + "`1password/engineering/token`" + ` in AWS Secrets Manager or ` + "`1password-engineering-token`" + ` in Google Secret Manager: the name alone, not the secret's ARN or its full resource name. ` +
					`In AWS Secrets Manager the name can't end in a hyphen and six letters or digits, like ` + "`-AbCdEf`" + `, because Secrets Manager can't find a secret by such a name. ` +
					`Changing it replaces the installation. To rotate the token, store the new one as a new version of the same secret, which needs no change here.`,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			// P0 fixes the sign-in address when it creates the item, and refuses a change to
			// it.
			"sign_in_address": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Your 1Password account's sign-in address, such as `acme.1password.com`. Changing it replaces the installation.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"allowed_domains": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The email domains the connector shares items with: the `DOMAIN_ALLOW_PATTERN` it's deployed with, which P0 reads back from the connector when it checks the install. To change it, change the connector's deployment.",
			},
			"state": common.StateAttribute,
		},
	}
}

// Validating here rather than in Create surfaces mistakes at plan time, before P0 is
// called, in P0's words. A value that Terraform only knows at apply time is skipped
// while planning, and checked when Terraform validates the configuration again before
// it applies.
func (*OnePasswordVault) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var secretName, signInAddress types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("token_secret_name"), &secretName)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("sign_in_address"), &signInAddress)...)
	secretManager := secretManagerFromConfig(ctx, req.Config, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	validateTokenSecret(secretManager, secretName, &resp.Diagnostics)
	// P0 has no rule for the sign-in address, but trims it as it does every field.
	validateField(path.Root("sign_in_address"), signInAddress, nil, &resp.Diagnostics)
}

func (r *OnePasswordVault) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:        OnePasswordKey,
		Component:          installresources.Vault,
		ProviderData:       data,
		GetId:              r.getId,
		GetItemJson:        r.getItemJson,
		FromJson:           r.fromJson,
		ToJson:             r.toJson,
		DescribeCheckError: describeCheckError("vault"),
	}
}

// The item is keyed by the vault's ID.
func (r *OnePasswordVault) getId(data any) *string {
	model, ok := data.(*onePasswordVaultModel)
	if !ok {
		return nil
	}
	str := model.Id.ValueString()
	return &str
}

// A response with no item is a bad response, not an item to read. Its nil item,
// returned as it is, wouldn't compare equal to nil, and would be read as one.
func (r *OnePasswordVault) getItemJson(json any) any {
	inner, ok := json.(*onePasswordVaultApi)
	if !ok || inner.Item == nil {
		return nil
	}
	return inner.Item
}

func (r *OnePasswordVault) fromJson(_ context.Context, diags *diag.Diagnostics, id string, json any) any {
	jsonv, ok := json.(*onePasswordVaultJson)
	if !ok {
		return nil
	}

	// secret_manager is Required, so leaving it nil here would surface as Terraform core's
	// "provider produced inconsistent result after apply" rather than something the
	// reader can act on.
	if jsonv.SecretManager == nil {
		diags.AddError(
			"Bad API response",
			fmt.Sprintf("The 1Password vault %s carries no secret manager configuration. "+
				"Remove it in the P0 app and add it again, or remove it from Terraform state with `terraform state rm`.", id),
		)
		return nil
	}

	// A secret manager of a type this provider doesn't know can't be read into the
	// attribute's fields.
	if !slices.Contains(secretManagerTypes, jsonv.SecretManager.Type) {
		diags.AddError(
			"Unsupported 1Password vault",
			fmt.Sprintf("The 1Password vault %s has a %q secret manager. This provider supports only %s. "+
				"Upgrade the provider, manage the vault in the P0 app, or remove it from Terraform state with `terraform state rm`.",
				id, jsonv.SecretManager.Type, strings.Join(secretManagerTypes, " and ")),
		)
		return nil
	}

	return &onePasswordVaultModel{
		Id:              types.StringValue(id),
		Connector:       types.StringPointerValue(jsonv.Connector),
		SecretManager:   secretManagerFromJson(jsonv.SecretManager),
		TokenSecretName: types.StringPointerValue(jsonv.TokenSecretName),
		SignInAddress:   types.StringPointerValue(jsonv.SignInAddress),
		AllowedDomains:  types.StringPointerValue(jsonv.AllowedDomains),
		State:           types.StringPointerValue(jsonv.State),
	}
}

// The body of the PUT that stages the item, and of verify and configure: the
// connector, the secret manager, the token's secret and the sign-in address, which P0
// fixes when it creates the item. Sending them on each step has P0 refuse a request that would move any of
// them, before it calls the connector. P0 reads the allowed domains back from the
// connector, and sets the state.
func (r *OnePasswordVault) toJson(data any) any {
	datav, ok := data.(*onePasswordVaultModel)
	if !ok {
		return nil
	}

	json := onePasswordVaultJson{
		Connector:       datav.Connector.ValueStringPointer(),
		TokenSecretName: datav.TokenSecretName.ValueStringPointer(),
		SignInAddress:   datav.SignInAddress.ValueStringPointer(),
	}
	if datav.SecretManager != nil {
		json.SecretManager = datav.SecretManager.toJson()
	}
	return &json
}

// P0 checks every field before it creates the item, and its error for one doesn't
// repeat the value, which may be a token pasted in place of the secret's name. So a
// value it refuses leaves nothing staged. A check that fails after that leaves the
// staged item in state, tainted, and the next apply replaces it: P0 refuses a second
// PUT of a vault that's already added.
func (r *OnePasswordVault) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json onePasswordVaultApi
	var data onePasswordVaultModel

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

func (r *OnePasswordVault) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &onePasswordVaultApi{}, &onePasswordVaultModel{})
}

// Plans an update for a vault that P0 hasn't installed, so that the next apply
// finishes the install. That is when P0 reads the allowed domains back from the
// connector, so they are unknown until then.
func (*OnePasswordVault) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	planUnfinishedInstall(ctx, req, resp, path.Root("allowed_domains"))
}

// Every configurable attribute requires replacement, so the only update Terraform
// plans is the one that finishes an install, which verifies and configures the item as
// Create does. P0 validates the install on verify alone, so there is no shorter way.
func (r *OnePasswordVault) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &onePasswordVaultApi{}, &onePasswordVaultModel{})
}

func (r *OnePasswordVault) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Delete(ctx, &resp.Diagnostics, &req.State, &onePasswordVaultModel{})
}

func (r *OnePasswordVault) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
