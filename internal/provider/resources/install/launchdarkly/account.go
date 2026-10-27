package installlaunchdarkly

import (
	"context"
	"fmt"
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
	installapp "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/app"
	installvaultedcredential "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/vaulted-credential"
)

var _ resource.Resource = &LaunchDarklyAccount{}
var _ resource.ResourceWithConfigure = &LaunchDarklyAccount{}
var _ resource.ResourceWithImportState = &LaunchDarklyAccount{}
var _ resource.ResourceWithModifyPlan = &LaunchDarklyAccount{}
var _ resource.ResourceWithValidateConfig = &LaunchDarklyAccount{}

// The LaunchDarkly `account` item: one LaunchDarkly account, the connector that
// reaches it, where the account is hosted, its plan, and where the connector finds
// the account's access token (packages/integrations/launchdarkly/shared/src/components.ts
// in the app). P0 fixes every field the form asks for when it adds the account, and
// reads the others from the connector each time it verifies it.
type LaunchDarklyAccount struct {
	installer *common.Install
}

const invalidAccountId = "Invalid LaunchDarkly account ID"

// The options of the item's `instance` and `plan` selects, which P0 stores by their
// type.
var (
	instances = []string{"commercial", "eu", "federal"}
	plans     = []string{"enterprise", "non-enterprise"}
)

type accountModel struct {
	Id                 types.String `tfsdk:"id"`
	Connector          types.String `tfsdk:"connector"`
	Instance           types.String `tfsdk:"instance"`
	Plan               types.String `tfsdk:"plan"`
	Vault              *vaultModel  `tfsdk:"vault"`
	TokenSecretName    types.String `tfsdk:"token_secret_name"`
	AccountId          types.String `tfsdk:"account_id"`
	TokenKind          types.String `tfsdk:"token_kind"`
	TokenOwnerMemberId types.String `tfsdk:"token_owner_member_id"`
	AllowedDomains     types.String `tfsdk:"allowed_domains"`
	State              types.String `tfsdk:"state"`
}

// A select, which P0 stores by its option's type.
type selectJson struct {
	Type string `json:"type"`
}

// The item, as P0 stores it.
type accountJson struct {
	Connector          *string     `json:"connector,omitempty"`
	Instance           *selectJson `json:"instance,omitempty"`
	Plan               *selectJson `json:"plan,omitempty"`
	Vault              *vaultJson  `json:"vault,omitempty"`
	TokenSecretName    *string     `json:"tokenSecretName,omitempty"`
	AccountId          *string     `json:"accountId,omitempty"`
	TokenKind          *string     `json:"tokenKind,omitempty"`
	TokenOwnerMemberId *string     `json:"tokenOwnerMemberId,omitempty"`
	AllowedDomains     *string     `json:"allowedDomains,omitempty"`
	State              *string     `json:"state,omitempty"`
}

type accountApi struct {
	Item *accountJson `json:"item"`
}

func NewLaunchDarklyAccount() resource.Resource {
	return &LaunchDarklyAccount{}
}

func (*LaunchDarklyAccount) Metadata(_ context.Context, req resource.MetadataRequest, res *resource.MetadataResponse) {
	res.TypeName = req.ProviderTypeName + "_launchdarkly_account"
}

// A select's attribute. P0 fixes the option when it adds the account, so a new one
// replaces it.
func selectAttribute(description string, options []string) schema.StringAttribute {
	return schema.StringAttribute{
		Required:            true,
		MarkdownDescription: description,
		Validators: []validator.String{
			stringvalidator.OneOf(options...),
		},
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.RequiresReplace(),
		},
	}
}

// A field that P0 reads from the connector each time it verifies the account.
func reportedAttribute(description string) schema.StringAttribute {
	return schema.StringAttribute{
		Computed:            true,
		MarkdownDescription: description,
	}
}

func (*LaunchDarklyAccount) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A LaunchDarkly account whose existing members P0 can grant base roles, teams, custom roles and role attributes to, through one of your LaunchDarkly connectors. Requests name the account by its ` + "`id`" + `, as in ` + "`acme-prod/writer`" + `.

P0 never holds the account's access token. You store it in your own secret manager, in the connector's cloud, and the connector reads it on every call. This resource tells P0 which connector reaches the account, where the account is hosted, its plan, and where the token's secret is.

**Important:** Install the connector first, store the token, and grant the connector's identity read access to the token's secret: in an earlier apply, or in the same one with this resource depending on the grant, as the examples do. Creating this resource has P0 check the account through the connector: the connector reads the secret, signs in to LaunchDarkly with the token, checks the account's plan, and reads its own allowed email domains. If a check fails, the apply fails with P0's message. IAM changes can take a few seconds to take effect, so if the first check is denied access, apply again.

P0 can hold each LaunchDarkly account only once. It refuses an account whose token is for a LaunchDarkly account it already has.

An account's attributes can't change once it is created. Changing ` + "`id`" + `, ` + "`connector`" + `, ` + "`instance`" + `, ` + "`plan`" + `, ` + "`vault`" + ` or ` + "`token_secret_name`" + ` replaces the account. To rotate the token, add a new version to the same secret, which needs no change here.

**Warning:** Replacing or destroying this resource removes the account from P0. P0 can't revoke access it granted on an account it no longer has, so that access stays in LaunchDarkly until you remove it there. Before you change an attribute or destroy this resource, revoke the account's open access in P0 or wait for it to expire.

An account that already exists in P0, such as one added in the P0 app, isn't created again: the apply fails, and says to import it with ` + "`terraform import`" + `. An imported account that P0 hasn't finished installing plans an update, and applying it finishes the install. If that install can't finish, fix what P0's message names and apply again, or start over: ` + "`terraform apply -replace=<address>`" + ` deletes the account and creates it again, and ` + "`terraform state rm <address>`" + ` stops managing it.

The email domains that may receive access are set on the connector, as its ` + "`DOMAIN_ALLOW_PATTERN`" + ` environment variable, and aren't an attribute of this resource. P0 reads them back as ` + "`allowed_domains`" + `.

**Prerequisites:**

- A LaunchDarkly connector, installed with ` + "`p0_launchdarkly_connector`" + `.

- An access token for the account. On Enterprise, a service token with an inline policy that allows what P0 grants. On any other plan, the personal access token of a LaunchDarkly admin with the Admin role: P0 then grants base roles only, and the token stops working when that admin leaves LaunchDarkly.

- A secret that holds the token alone, as plain text, in the secret manager that ` + "`vault`" + ` names. Store it outside Terraform, which keeps the token out of Terraform's state.

- For ` + "`aws-sm`" + `, ` + "`p0_aws_secrets_manager`" + ` installed for the account that holds the secret. Give the connector's role ` + "`secretsmanager:GetSecretValue`" + ` on the secret alone, and ` + "`kms:Decrypt`" + ` on its key if a customer-managed KMS key encrypts it. A secret in an account other than the connector's needs a customer-managed key, and the secret's resource policy and the key's policy must allow the connector's role.

- For ` + "`gcp-sm`" + `, Google Secret Manager installed in P0 for the project that holds the secret. This provider has no resource for it, so install it in the P0 app. Give the connector's service account ` + "`roles/secretmanager.secretAccessor`" + ` and ` + "`roles/secretmanager.viewer`" + ` on the secret alone.

` + betaNote + `

` + installDocs,
		Attributes: map[string]schema.Attribute{
			"id": itemIdAttribute(`A name you choose for the account, such as ` + "`acme-prod`" + `, which requests name it by: ` + itemIdRule),
			"connector": itemIdAttribute("The `id` of the `p0_launchdarkly_connector` that reaches this account. " +
				"The connector must be installed, and run in the same cloud as `vault`: on AWS Lambda for `aws-sm`, on Google Cloud Run for `gcp-sm`."),
			"instance": selectAttribute(
				"Where your LaunchDarkly account is hosted: `commercial` (app.launchdarkly.com), `eu` (app.eu.launchdarkly.com) or `federal` (app.launchdarkly.us).",
				instances,
			),
			"plan": selectAttribute(
				"Your LaunchDarkly plan: `enterprise`, or `non-enterprise` for any other plan, such as Developer, Foundation or a legacy plan like Starter. "+
					"On Enterprise, P0 grants base roles, teams, custom roles and role attributes with a service token. On any other plan, P0 grants base roles only, with an admin's personal token. "+
					"P0 checks the plan when it validates the account. Changing it replaces the account.",
				plans,
			),
			"vault": vaultAttribute(),
			// P0 fixes the token's secret when it creates the item, and refuses a change
			// to it, so a new secret replaces the account.
			"token_secret_name": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "The name of the secret that holds the account's LaunchDarkly access token, in `vault`: a service token on Enterprise, or an admin's personal token on any other plan. " +
					"Only your connector reads it, never P0. The name alone, never an ARN or a resource name. " +
					"For `aws-sm`, up to 512 letters, digits and `/_+=.@-`, and, when the vault's account isn't the connector's, not ending in a hyphen and six letters or digits, like `-AbCdEf`. " +
					"For `gcp-sm`, up to 255 letters, digits, underscores and hyphens.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"account_id": reportedAttribute("The ID of the LaunchDarkly account the access token belongs to, which P0 reads through the connector when it validates the account."),
			"token_kind": reportedAttribute("Whether the access token is a `service` token or a member's `personal` token, which P0 reads through the connector when it validates the account."),
			"token_owner_member_id": reportedAttribute("The ID of the LaunchDarkly member who owns a personal access token. P0 never grants to this member. " +
				"Empty for a service token."),
			"allowed_domains": reportedAttribute("The email domains the connector grants to: the `DOMAIN_ALLOW_PATTERN` it's deployed with, which P0 reads back from the connector when it validates the account. " +
				"To change it, change the connector's deployment."),
			"state": common.StateAttribute,
		},
	}
}

// Validating here rather than in Create surfaces mistakes at plan time, before P0 is
// called, in P0's words. A value that Terraform only knows at apply time is skipped
// while planning, and checked when Terraform validates the configuration again before
// it applies. That the vault is in the connector's cloud can't be checked here, since
// the configuration names the connector by its ID alone. P0 refuses an account whose
// vault isn't.
func (*LaunchDarklyAccount) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var secretName types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("token_secret_name"), &secretName)...)
	vault := vaultFromConfig(ctx, req.Config, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	validateVault(vault, &resp.Diagnostics)
	validateSecretName(vault, secretName, &resp.Diagnostics)
}

// The vault's type picks the rule. One that isn't known yet gets the rule that any
// vault's name meets.
func validateSecretName(vault *vaultModel, name types.String, diags *diag.Diagnostics) {
	vaultType := ""
	if vault != nil && installapp.IsSet(vault.Type) {
		vaultType = vault.Type.ValueString()
	}
	validateField(path.Root("token_secret_name"), name, secretNameRule(vaultType), diags)
}

// A rule for a value, and P0's message for a value that breaks it.
type rule struct {
	summary string
	message string
	test    func(value string) bool
}

// Rejects value, at attribute, with P0's message when it breaks rule. Like P0, it
// checks the value without the whitespace around it. That whitespace gets its own
// error: P0 trims the strings it stores, so the value would read back different from
// the configuration, which Terraform reports as an inconsistent result after apply. A null value is left to the checks that a field is
// set. The value is never repeated: a secret's name may be the access token, pasted
// into the wrong field.
func validateField(attribute path.Path, value types.String, rule rule, diags *diag.Diagnostics) {
	if !installapp.IsSet(value) {
		return
	}
	trimmed := strings.TrimSpace(value.ValueString())
	if !rule.test(trimmed) {
		diags.AddAttributeError(attribute, rule.summary, rule.message)
		return
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

func (r *LaunchDarklyAccount) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:        LaunchDarklyKey,
		Component:          installresources.Account,
		ProviderData:       data,
		GetId:              accountItemId,
		GetItemJson:        accountItemJson,
		FromJson:           accountFromJson,
		ToJson:             accountToJson,
		DescribeCheckError: describeAccountCheckError,
		// An account that P0 no longer has is dropped from state, as other resources
		// do, and the next apply creates it again.
	}
}

// P0's refusal, on verify or configure, of a request that would change a field it
// fixed when it created the account (configure.ts in the app's install framework).
const fixedFieldRefusal = "can only be altered on initial installation"

// What to do about an install that P0 won't finish. A create that fails leaves the
// resource tainted, so the next apply replaces it. An update, which finishes an
// imported account's install, fails again on every apply until its cause is fixed.
const installWayOut = "Fix what P0's message names and apply again. " + startOver

// How to start over with an account, for each refusal that names it.
const startOver = "To start over, `terraform apply -replace=<address>` deletes the account " +
	"and creates it again, and `terraform state rm <address>` stops managing it."

// Describes a verify or configure step that P0 refused. P0 answers alike when its
// check of the account fails, which usually points at the customer's setup (the
// connector, the secret, the access token or the plan), and when it refuses the
// request for another reason, so the summary blames neither and leaves the reason to
// P0's message. A refusal to change a fixed field is told apart by its text, since no
// setup causes it: the item P0 holds differs from the configuration.
func describeAccountCheckError(id string, err error) (string, string) {
	if strings.Contains(err.Error(), fixedFieldRefusal) {
		return "LaunchDarkly account differs from the one in P0",
			fmt.Sprintf("P0 holds a LaunchDarkly account %q whose attributes differ from this configuration, and they can't change once it is created:\n\n%s\n\n"+
				"Make the configuration match the account in P0. %s", id, err, startOver)
	}
	return "P0 didn't finish installing the LaunchDarkly account",
		fmt.Sprintf("P0 refused to install the LaunchDarkly account %q:\n\n%s\n\n%s", id, err, installWayOut)
}

func accountItemId(data any) *string {
	model, ok := data.(*accountModel)
	if !ok {
		return nil
	}
	str := model.Id.ValueString()
	return &str
}

func accountItemJson(json any) any {
	// An answer with no item leaves Item nil, which has to come back as an untyped
	// nil: a nil *accountJson in an any isn't nil, and would reach FromJson.
	inner, ok := json.(*accountApi)
	if !ok || inner.Item == nil {
		return nil
	}
	return inner.Item
}

// A select's option as P0 stores it, or null for none.
func selectValue(json *selectJson) types.String {
	if json == nil {
		return types.StringNull()
	}
	return types.StringValue(json.Type)
}

func accountFromJson(_ context.Context, diags *diag.Diagnostics, id string, json any) any {
	jsonv, ok := json.(*accountJson)
	if !ok {
		return nil
	}

	// vault is Required, so leaving it nil here would surface as Terraform core's
	// "provider produced inconsistent result after apply" rather than something the
	// reader can act on.
	if jsonv.Vault == nil {
		diags.AddError(
			"Bad API response",
			fmt.Sprintf("P0 returned the LaunchDarkly account %q without its vault. "+
				"Remove the account in the P0 app and apply again, or remove it from Terraform state with `terraform state rm <address>`.", id),
		)
		return nil
	}

	vault := vaultFromJson(jsonv.Vault)
	if vault == nil {
		diags.AddError(
			"Unsupported LaunchDarkly account vault",
			fmt.Sprintf("The LaunchDarkly account %s has a %q vault. This provider supports %q and %q. "+
				"Manage it in the P0 app, or remove it from Terraform state with `terraform state rm`.",
				id, jsonv.Vault.Type, awsSecretsManager, installvaultedcredential.GcpSecretManager),
		)
		return nil
	}

	return &accountModel{
		Id:                 types.StringValue(id),
		Connector:          types.StringPointerValue(jsonv.Connector),
		Instance:           selectValue(jsonv.Instance),
		Plan:               selectValue(jsonv.Plan),
		Vault:              vault,
		TokenSecretName:    types.StringPointerValue(jsonv.TokenSecretName),
		AccountId:          types.StringPointerValue(jsonv.AccountId),
		TokenKind:          types.StringPointerValue(jsonv.TokenKind),
		TokenOwnerMemberId: types.StringPointerValue(jsonv.TokenOwnerMemberId),
		AllowedDomains:     types.StringPointerValue(jsonv.AllowedDomains),
		State:              types.StringPointerValue(jsonv.State),
	}
}

// The body of every request: every field the form asks for. The PUT that stages the
// account needs each one. Verify and configure get them again, so that P0, which
// compares each with the item it stored, refuses to install an account that isn't the
// configured one. P0 reads the other fields from the connector, and sets the state.
func accountToJson(data any) any {
	datav, ok := data.(*accountModel)
	if !ok {
		return nil
	}

	json := accountJson{
		Connector:       datav.Connector.ValueStringPointer(),
		TokenSecretName: datav.TokenSecretName.ValueStringPointer(),
	}
	if installapp.IsSet(datav.Instance) {
		json.Instance = &selectJson{Type: datav.Instance.ValueString()}
	}
	if installapp.IsSet(datav.Plan) {
		json.Plan = &selectJson{Type: datav.Plan.ValueString()}
	}
	if datav.Vault != nil {
		json.Vault = datav.Vault.toJson()
	}
	return &json
}

func (r *LaunchDarklyAccount) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json accountApi
	var data accountModel

	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	if resp.Diagnostics.HasError() {
		return
	}

	// An account that P0 holds is never staged again: the PUT would return one that
	// isn't installed to "stage", and a failed check would then taint the resource, so
	// that the next apply deleted an account Terraform never created.
	refuseExistingItem(r.installer.ProviderData, installresources.Account, "account", data.Id.ValueString(), &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	// P0 checks each field before it creates the item, so a value it refuses leaves
	// nothing staged. A step that fails after that leaves the item where it got to,
	// at "stage" if verify failed and at "configure" if configure did, and the
	// resource tainted, so the next apply deletes the item and creates it again.
	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, accountToJson(&data))
	if resp.Diagnostics.HasError() {
		return
	}

	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data)
}

func (r *LaunchDarklyAccount) Read(ctx context.Context, _ resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &accountApi{}, &accountModel{})
}

// The fields that P0 reads from the connector when it verifies the account.
var reportedFields = []path.Path{
	path.Root("account_id"), path.Root("token_kind"), path.Root("token_owner_member_id"), path.Root("allowed_domains"),
}

// Refuses a secret name that Secrets Manager can't find from another account, and
// plans an update for an account that P0 hasn't installed.
//
// An account that P0 hasn't installed plans an update even when the configuration
// hasn't changed, so that the next apply finishes the install: P0 grants nothing on
// an account until its item is installed. That covers an item imported before its
// checks passed. An installed item keeps the framework's plan, which shows no
// difference while the configuration matches it. As in any update the framework
// plans, the fields that only P0 sets are unknown until apply: the state, and the
// fields P0 reads from the connector. Update takes them from P0's response.
func (r *LaunchDarklyAccount) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	// Nothing is left to finish on destroy.
	if req.Plan.Raw.IsNull() {
		return
	}
	r.checkSecretNameAcrossAccounts(ctx, req, resp)
	// Nothing is installed yet on create.
	if resp.Diagnostics.HasError() || req.State.Raw.IsNull() {
		return
	}

	var state types.String
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("state"), &state)...)
	if resp.Diagnostics.HasError() || state.ValueString() == common.StateInstalled {
		return
	}

	for _, attribute := range append([]path.Path{path.Root("state")}, reportedFields...) {
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, attribute, types.StringUnknown())...)
	}
}

// Refuses, for an account that the apply will create, a secret name that Secrets
// Manager can't find from another account: one ending in a hyphen and six letters or
// digits, when the vault's account isn't the connector's. P0 reads such a secret by
// its partial ARN, which Secrets Manager reads as a full one. The configuration names
// the connector by its ID alone, so its account is read from P0. A value that isn't
// known yet, a connector P0 doesn't have yet, and any error reading it skip the check:
// P0 makes it again when it stages the account.
func (r *LaunchDarklyAccount) checkSecretNameAcrossAccounts(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if r.installer == nil || r.installer.ProviderData == nil {
		return
	}
	var plan accountModel
	if resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...); resp.Diagnostics.HasError() {
		return
	}
	if !req.State.Raw.IsNull() {
		var state accountModel
		// An account that exists keeps its secret: P0 took it when it was created.
		if resp.Diagnostics.Append(req.State.Get(ctx, &state)...); resp.Diagnostics.HasError() || !accountWillBeCreated(&state, &plan) {
			return
		}
	}
	if plan.Vault == nil || !installapp.IsSet(plan.Vault.Type) || plan.Vault.Type.ValueString() != awsSecretsManager ||
		!installapp.IsSet(plan.Vault.AccountId) || !installapp.IsSet(plan.TokenSecretName) || !installapp.IsSet(plan.Connector) ||
		!awsArnSuffixRegex.MatchString(plan.TokenSecretName.ValueString()) {
		return
	}
	var connector connectorApi
	if _, err := r.installer.ProviderData.Get(itemPath(installresources.Connector, plan.Connector.ValueString()), &connector); err != nil ||
		connector.Item == nil || connector.Item.Hosting == nil || connector.Item.Hosting.AccountId == nil {
		return
	}
	if *connector.Item.Hosting.AccountId != plan.Vault.AccountId.ValueString() {
		resp.Diagnostics.AddAttributeError(path.Root("token_secret_name"), invalidSecretNameSummary, suffixedSecretName)
	}
}

// Whether the planned account is another item than the one in state, which the apply
// creates by replacing the resource. Each of these fields requires replacement.
func accountWillBeCreated(state *accountModel, plan *accountModel) bool {
	if state.Vault == nil || plan.Vault == nil {
		return true
	}
	for _, pair := range [][2]types.String{
		{state.Id, plan.Id}, {state.Connector, plan.Connector}, {state.Instance, plan.Instance}, {state.Plan, plan.Plan},
		{state.TokenSecretName, plan.TokenSecretName},
		{state.Vault.Type, plan.Vault.Type}, {state.Vault.AccountId, plan.Vault.AccountId}, {state.Vault.SecretsRegion, plan.Vault.SecretsRegion},
	} {
		if !pair[0].Equal(pair[1]) {
			return true
		}
	}
	return false
}

// Every attribute that can be set requires replacement, so Terraform plans an update
// only when ModifyPlan plans one, to finish the install of an item that isn't
// installed, with the item as it is. An item that P0 has verified, at configure or
// installed, gets the configure step alone. Any other, a staged one, is verified
// first, as Create does.
func (r *LaunchDarklyAccount) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var state types.String
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("state"), &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	switch state.ValueString() {
	case common.StateConfigure, common.StateInstalled:
		r.installer.UpsertFromConfigure(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &accountApi{}, &accountModel{})
	default:
		r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &accountApi{}, &accountModel{})
	}
}

func (r *LaunchDarklyAccount) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Delete(ctx, &resp.Diagnostics, &req.State, &accountModel{})
}

func (r *LaunchDarklyAccount) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importById(ctx, req, resp, invalidAccountId)
}
