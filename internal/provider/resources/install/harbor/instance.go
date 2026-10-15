package installharbor

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
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/p0-security/terraform-provider-p0/internal"
	"github.com/p0-security/terraform-provider-p0/internal/common"
	installresources "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install"
	installapp "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/app"
	installvaultedcredential "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/vaulted-credential"
)

var _ resource.Resource = &HarborInstance{}
var _ resource.ResourceWithConfigure = &HarborInstance{}
var _ resource.ResourceWithImportState = &HarborInstance{}
var _ resource.ResourceWithModifyPlan = &HarborInstance{}
var _ resource.ResourceWithValidateConfig = &HarborInstance{}

// The Harbor `harbor-instance` item: one Harbor instance, the connector that reaches
// it, and where the connector finds the credentials of the instance's robot account
// (packages/integrations/harbor/shared/src/components.ts in the app).
type HarborInstance struct {
	installer *common.Install
}

const invalidInstanceId = "Invalid Harbor instance ID"

type instanceModel struct {
	Id         types.String `tfsdk:"id"`
	Connector  types.String `tfsdk:"connector"`
	Url        types.String `tfsdk:"url"`
	Vault      *vaultModel  `tfsdk:"vault"`
	SecretName types.String `tfsdk:"secret_name"`
	State      types.String `tfsdk:"state"`
}

// The item, as P0 stores it.
type instanceJson struct {
	Connector  *string    `json:"connector,omitempty"`
	Url        *string    `json:"url,omitempty"`
	Vault      *vaultJson `json:"vault,omitempty"`
	SecretName *string    `json:"secretName,omitempty"`
	State      *string    `json:"state,omitempty"`
}

type instanceApi struct {
	Item *instanceJson `json:"item"`
}

func NewHarborInstance() resource.Resource {
	return &HarborInstance{}
}

func (*HarborInstance) Metadata(_ context.Context, req resource.MetadataRequest, res *resource.MetadataResponse) {
	res.TypeName = req.ProviderTypeName + "_harbor_instance"
}

func (*HarborInstance) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A Harbor instance that P0 can grant project roles on.

P0 reaches the instance through a Harbor connector, which you install with ` + "`p0_harbor_connector`" + `. The connector calls the instance as a robot account whose credentials stay in your own secret manager: P0 never reads them, and only the connector does. One connector can serve several instances, and each instance has its own robot account and secret.

**Important:** Install the connector first, and grant its identity read access to the robot account's secret: in an earlier apply, or in the same one with this resource depending on the grant, as the examples do. Creating this resource has P0 check the instance through the connector. If the check fails, the apply fails with P0's message.

An instance's attributes can't change once it is created. Changing ` + "`id`" + `, ` + "`connector`" + `, ` + "`vault`" + ` or ` + "`secret_name`" + ` replaces the instance. A plan refuses a change to an installed instance's ` + "`url`" + `: a user's ID on one Harbor instance means nothing on another, so for another address, add a new ` + "`p0_harbor_instance`" + ` with its own ` + "`id`" + `, and remove this one once its open access has ended. To rotate the robot account's secret, add a new version to the same secret, which needs no change here.

**Warning:** Replacing or destroying this resource removes the instance from P0. P0 can't revoke access it granted on an instance it no longer has, so that access stays in Harbor until you remove it there. Before you change an attribute or destroy this resource, revoke the instance's open access in P0 or wait for it to expire.

An instance that already exists in P0, such as one added in the P0 app, isn't created again: the apply fails, and says to import it with ` + "`terraform import`" + `. An imported instance that P0 hasn't finished installing plans an update, and applying it finishes the install. If that install can't finish, fix what P0's message names and apply again, or start over: ` + "`terraform apply -replace=<address>`" + ` deletes the instance and creates it again, and ` + "`terraform state rm <address>`" + ` stops managing it.

The email domains that may receive access are set on the connector, as its ` + "`ALLOWED_EMAIL_DOMAIN_PATTERN`" + ` environment variable, and aren't an attribute of this resource.

**Prerequisites:**

- Harbor 2.12.0 or later, the first version whose robot accounts can hold the permissions below.

- A robot account for P0's connector on the instance, created in Harbor under Administration, Robot Accounts, such as ` + "`p0-connector`" + `. Give it the system permission User: List, and Member: List, Create, Update and Delete on all projects. Don't give it User: Read, which reads every OIDC user's CLI secret. Set it never to expire, and use it for nothing else: it can make anyone a member of any project at any role, Project Admin included.

- A secret that holds the file Harbor exports for the robot account, unchanged, in the secret manager that ` + "`vault`" + ` names. The connector reads the file's ` + "`name`" + ` and ` + "`secret`" + ` fields.

- For ` + "`aws-sm`" + `, ` + "`p0_aws_secrets_manager`" + ` installed for the account that holds the secret. Give the connector's role ` + "`secretsmanager:GetSecretValue`" + ` on the secret alone, and ` + "`kms:Decrypt`" + ` on its key if a customer-managed KMS key encrypts it. A secret in an account other than the connector's needs a customer-managed key, and the secret's resource policy and the key's policy must allow the connector's role.

- For ` + "`gcp-sm`" + `, Google Secret Manager installed in P0 for the project that holds the secret. This provider has no resource for it, so install it in the P0 app. Give the connector's service account ` + "`roles/secretmanager.secretAccessor`" + ` and ` + "`roles/secretmanager.viewer`" + ` on the secret alone.

` + betaNote + `

For the full install steps, see [Installing Harbor](https://docs.p0.dev/integrations/resource-integrations/harbor/installation).`,
		Attributes: map[string]schema.Attribute{
			"id": itemIdAttribute(`A name you choose for the instance, such as ` + "`prod`" + `: ` + itemIdRule),
			"connector": itemIdAttribute("The `id` of the `p0_harbor_connector` that reaches this instance. " +
				"The connector must be installed, and run in the same cloud as `vault`: on AWS Lambda for `aws-sm`, on Google Cloud Run for `gcp-sm`."),
			// P0 fixes the URL when it creates the instance: an open grant holds a
			// user's ID on the instance it was made for. Replacing the instance under
			// the same ID would point those grants at another Harbor's users, so url
			// doesn't require replacement: ModifyPlan refuses a change to it. A new
			// connector, vault or secret lands on the same Harbor, so those replace.
			"url": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "The address your Harbor instance is served at, such as `https://harbor.example.com`: `https://` and the host, " +
					"with a port if it isn't 443, in lowercase and with nothing after it. Leave out a trailing slash, any path, and any username or password. " +
					"P0 stores the address in this spelling alone, and the connector sends the robot account's credentials to it. " +
					"It can't change: for another address, add a new `p0_harbor_instance` with its own `id`.",
			},
			"vault": vaultAttribute(),
			"secret_name": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "The name of the secret that holds the robot account's credentials, in `vault`: the name alone, never an ARN or a resource name. " +
					"For `aws-sm`, up to 512 letters, digits and `/_+=.@-`, and, when the vault's account isn't the connector's, not ending in a hyphen and six letters or digits, like `-AbCdEf`. " +
					"For `gcp-sm`, up to 255 letters, digits, underscores and hyphens.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"state": common.StateAttribute,
		},
	}
}

// Validating here rather than in Create surfaces mistakes at plan time, before P0 is
// called, in P0's words. A value that Terraform only knows at apply time is skipped
// while planning, and checked when Terraform validates the configuration again before
// it applies. That the vault is in the connector's cloud can't be checked here, since
// the configuration names the connector by its ID alone. P0 refuses an instance whose
// vault isn't.
func (*HarborInstance) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var url, secretName types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("url"), &url)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("secret_name"), &secretName)...)
	vault := vaultFromConfig(ctx, req.Config, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	validateVault(vault, &resp.Diagnostics)
	validateInstanceUrl(url, &resp.Diagnostics)
	validateSecretName(vault, secretName, &resp.Diagnostics)
}

var instanceUrlRules = []rule{{
	summary: "Invalid Harbor instance URL",
	message: invalidInstanceUrl,
	test:    isInstanceUrl,
}}

func validateInstanceUrl(url types.String, diags *diag.Diagnostics) {
	validateField(path.Root("url"), url, instanceUrlRules, diags)
}

// The vault's type picks the rule. One that isn't known yet gets the rule that any
// vault's name meets.
func validateSecretName(vault *vaultModel, name types.String, diags *diag.Diagnostics) {
	vaultType := ""
	if vault != nil && installapp.IsSet(vault.Type) {
		vaultType = vault.Type.ValueString()
	}
	validateField(path.Root("secret_name"), name, secretNameRules(vaultType), diags)
}

// A rule for a value, and P0's message for a value that breaks it.
type rule struct {
	summary string
	message string
	test    func(value string) bool
}

// Rejects value, at attribute, with P0's message for the first of rules that it
// breaks. Like P0, it checks the value without the whitespace around it. That
// whitespace is rejected on its own: P0 trims the strings it stores, so the value
// would read back different from the configuration, which Terraform reports as an
// inconsistent result after apply. A null value is left to the checks that a field is
// set. The value is never repeated: a URL or a secret's name may be the robot
// account's credentials, pasted into the wrong field.
//
// TODO(INT-465): a copy of github-repositories' validateField, with its rule type and
// its whitespace check, kept close to it so that the two can merge into one.
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

func (r *HarborInstance) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:        HarborKey,
		Component:          installresources.HarborInstance,
		ProviderData:       data,
		GetId:              instanceId,
		GetItemJson:        instanceItemJson,
		FromJson:           instanceFromJson,
		ToJson:             instanceToJson,
		DescribeCheckError: describeInstanceCheckError,
		// An instance that P0 no longer has is dropped from state, as other resources
		// do, and the next apply creates it again.
	}
}

// P0's refusal, on verify or configure, of a request that would change a field it
// fixed when it created the instance (configure.ts in the app's install framework).
const fixedFieldRefusal = "can only be altered on initial installation"

// What to do about an install that P0 won't finish. A create that fails leaves the
// resource tainted, so the next apply replaces it. An update, which finishes an
// imported instance's install, fails again on every apply until its cause is fixed.
const installWayOut = "Fix what P0's message names and apply again. To start over, `terraform apply -replace=<address>` deletes the instance " +
	"and creates it again, and `terraform state rm <address>` stops managing it."

// Describes a verify or configure step that P0 refused. P0 answers alike when its
// check of the instance fails, which usually points at the customer's setup (the
// connector, the secret, the robot account or the instance itself), and when it
// refuses the request for another reason, so the summary blames neither and leaves
// the reason to P0's message. A refusal to change a fixed field is told apart by its
// text, since no setup causes it: the item P0 holds differs from the configuration.
func describeInstanceCheckError(id string, err error) (string, string) {
	if strings.Contains(err.Error(), fixedFieldRefusal) {
		return "Harbor instance differs from the one in P0",
			fmt.Sprintf("P0 holds a Harbor instance %q whose attributes differ from this configuration, and they can't change once it is created:\n\n%s\n\n"+
				"Make the configuration match the instance in P0. To start over, `terraform apply -replace=<address>` deletes the instance and creates it again, "+
				"and `terraform state rm <address>` stops managing it.", id, err)
	}
	return "P0 didn't finish installing the Harbor instance",
		fmt.Sprintf("P0 refused to install the Harbor instance %q:\n\n%s\n\n%s", id, err, installWayOut)
}

func instanceId(data any) *string {
	model, ok := data.(*instanceModel)
	if !ok {
		return nil
	}
	str := model.Id.ValueString()
	return &str
}

func instanceItemJson(json any) any {
	// An answer with no item leaves Item nil, which has to come back as an untyped
	// nil: a nil *instanceJson in an any isn't nil, and would reach FromJson.
	inner, ok := json.(*instanceApi)
	if !ok || inner.Item == nil {
		return nil
	}
	return inner.Item
}

func instanceFromJson(_ context.Context, diags *diag.Diagnostics, id string, json any) any {
	jsonv, ok := json.(*instanceJson)
	if !ok {
		return nil
	}

	// vault is Required, so leaving it nil here would surface as Terraform core's
	// "provider produced inconsistent result after apply" rather than something the
	// reader can act on.
	if jsonv.Vault == nil {
		diags.AddError(
			"Bad API response",
			fmt.Sprintf("P0 returned the Harbor instance %q without its vault. "+
				"Remove the instance in the P0 app and apply again, or remove it from Terraform state with `terraform state rm <address>`.", id),
		)
		return nil
	}

	vault := vaultFromJson(jsonv.Vault)
	if vault == nil {
		diags.AddError(
			"Unsupported Harbor instance vault",
			fmt.Sprintf("The Harbor instance %s has a %q vault. This provider supports %q and %q. "+
				"Manage it in the P0 app, or remove it from Terraform state with `terraform state rm`.",
				id, jsonv.Vault.Type, awsSecretsManager, installvaultedcredential.GcpSecretManager),
		)
		return nil
	}

	return &instanceModel{
		Id:         types.StringValue(id),
		Connector:  types.StringPointerValue(jsonv.Connector),
		Url:        types.StringPointerValue(jsonv.Url),
		Vault:      vault,
		SecretName: types.StringPointerValue(jsonv.SecretName),
		State:      types.StringPointerValue(jsonv.State),
	}
}

// The body of every request: the whole item. The PUT that stages the instance needs
// every field. Verify and configure get them again, so that P0, which compares each
// with the item it stored, refuses to install an instance that isn't the configured
// one.
func instanceToJson(data any) any {
	datav, ok := data.(*instanceModel)
	if !ok {
		return nil
	}

	json := instanceJson{
		Connector:  datav.Connector.ValueStringPointer(),
		Url:        datav.Url.ValueStringPointer(),
		SecretName: datav.SecretName.ValueStringPointer(),
	}
	if datav.Vault != nil {
		json.Vault = datav.Vault.toJson()
	}
	return &json
}

func (r *HarborInstance) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json instanceApi
	var data instanceModel

	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	if resp.Diagnostics.HasError() {
		return
	}

	// An instance that P0 holds is never staged again: the PUT would return an
	// installed one to "stage", and a failed check would then taint the resource, so
	// that the next apply deleted an instance Terraform never created.
	refuseExistingItem(r.installer.ProviderData, installresources.HarborInstance, "instance", data.Id.ValueString(), &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	// P0 checks each field before it creates the item, so a value it refuses leaves
	// nothing staged. A step that fails after that leaves the item where it got to,
	// at "stage" if verify failed and at "configure" if configure did, and the
	// resource tainted, so the next apply deletes the item and creates it again.
	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, instanceToJson(&data))
	if resp.Diagnostics.HasError() {
		return
	}

	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data)
}

func (r *HarborInstance) Read(ctx context.Context, _ resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &instanceApi{}, &instanceModel{})
}

const fixedUrlSummary = "A Harbor instance's address can't change"

const fixedUrlDetail = "A Harbor instance's address can't change. Add a new p0_harbor_instance with its own id, and remove this one once its open access has ended."

// Refuses a new URL for an installed instance, and reports whether it did. Only an
// installed instance can have open access. For one imported before it finished
// installing, P0's own refusal of the change says more. A URL that Terraform doesn't
// know yet passes: one unknown until apply is checked again in Update.
func refuseNewUrl(ctx context.Context, state tfsdk.State, plan tfsdk.Plan, diags *diag.Diagnostics) bool {
	var installState, before, after types.String
	diags.Append(state.GetAttribute(ctx, path.Root("state"), &installState)...)
	diags.Append(state.GetAttribute(ctx, path.Root("url"), &before)...)
	diags.Append(plan.GetAttribute(ctx, path.Root("url"), &after)...)
	if diags.HasError() || installState.ValueString() != common.StateInstalled ||
		!installapp.IsSet(before) || !installapp.IsSet(after) || before.ValueString() == after.ValueString() {
		return false
	}
	diags.AddAttributeError(path.Root("url"), fixedUrlSummary, fixedUrlDetail)
	return true
}

// Refuses a new URL for an installed instance, and plans an update for one that P0
// hasn't installed.
//
// Replacing an instance under its ID, which is what Terraform would do for a new URL,
// deletes the item and creates one with the same ID. P0 keys open access by that ID,
// and a user's ID on one Harbor is nobody's, or someone else's, on another. A new ID
// goes through, since it replaces the resource with a new item.
//
// An item that P0 hasn't installed plans an update even when the configuration hasn't
// changed, so that the next apply finishes the install: P0 grants nothing on an
// instance until its item is installed. That covers an item imported before its
// checks passed. An installed item keeps the framework's plan, which shows no
// difference while the configuration matches it.
//
// TODO(INT-465): the second half is a copy of github-repositories' ModifyPlan.
func (r *HarborInstance) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	// Nothing is left to finish on destroy.
	if req.Plan.Raw.IsNull() {
		return
	}
	r.checkSecretNameAcrossAccounts(ctx, req, resp)
	// Nothing is installed yet on create.
	if resp.Diagnostics.HasError() || req.State.Raw.IsNull() {
		return
	}

	var idBefore, idAfter types.String
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("id"), &idBefore)...)
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("id"), &idAfter)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if idBefore.Equal(idAfter) && refuseNewUrl(ctx, req.State, req.Plan, &resp.Diagnostics) {
		return
	}

	var state types.String
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("state"), &state)...)
	if resp.Diagnostics.HasError() || state.ValueString() == common.StateInstalled {
		return
	}

	// As in any update the framework plans, the state, which only P0 sets, is unknown
	// until apply. Update takes it from P0's response, which has the item's new state.
	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("state"), types.StringUnknown())...)
}

// Refuses, for an instance that the apply will create, a secret name that Secrets
// Manager can't find from another account: one ending in a hyphen and six letters or
// digits, when the vault's account isn't the connector's. P0 reads such a secret by
// its partial ARN, which Secrets Manager reads as a full one. The configuration names
// the connector by its ID alone, so its account is read from P0. A value that isn't
// known yet, a connector P0 doesn't have yet, and any error reading it skip the check:
// P0 makes it again when it stages the instance.
func (r *HarborInstance) checkSecretNameAcrossAccounts(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if r.installer == nil || r.installer.ProviderData == nil {
		return
	}
	var plan instanceModel
	if resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...); resp.Diagnostics.HasError() {
		return
	}
	if !req.State.Raw.IsNull() {
		var state instanceModel
		// An instance that exists keeps its secret: P0 took it when it was created.
		if resp.Diagnostics.Append(req.State.Get(ctx, &state)...); resp.Diagnostics.HasError() || !instanceWillBeCreated(&state, &plan) {
			return
		}
	}
	if plan.Vault == nil || !installapp.IsSet(plan.Vault.Type) || plan.Vault.Type.ValueString() != awsSecretsManager ||
		!installapp.IsSet(plan.Vault.AccountId) || !installapp.IsSet(plan.SecretName) || !installapp.IsSet(plan.Connector) ||
		!awsArnSuffixRegex.MatchString(plan.SecretName.ValueString()) {
		return
	}
	var connector connectorApi
	if _, err := r.installer.ProviderData.Get(itemPath(installresources.Connector, plan.Connector.ValueString()), &connector); err != nil ||
		connector.Item == nil || connector.Item.Hosting == nil || connector.Item.Hosting.AccountId == nil {
		return
	}
	if *connector.Item.Hosting.AccountId != plan.Vault.AccountId.ValueString() {
		resp.Diagnostics.AddAttributeError(path.Root("secret_name"), invalidSecretNameSummary, suffixedSecretName)
	}
}

// Whether the planned instance is another item than the one in state, which the
// apply creates by replacing the resource. Each of these fields requires replacement.
func instanceWillBeCreated(state *instanceModel, plan *instanceModel) bool {
	if state.Vault == nil || plan.Vault == nil {
		return true
	}
	for _, pair := range [][2]types.String{
		{state.Id, plan.Id}, {state.Connector, plan.Connector}, {state.SecretName, plan.SecretName},
		{state.Vault.Type, plan.Vault.Type}, {state.Vault.AccountId, plan.Vault.AccountId}, {state.Vault.SecretsRegion, plan.Vault.SecretsRegion},
	} {
		if !pair[0].Equal(pair[1]) {
			return true
		}
	}
	return false
}

// Terraform plans an update in two cases. ModifyPlan plans one to finish the install
// of an item that isn't installed, with the item as it is. And a URL that Terraform
// only knows at apply time plans one, which is refused here if the URL turns out to
// be a new one. An item that P0 has verified, at configure or installed, gets the
// configure step alone. Any other, a staged one, is verified first, as Create does.
func (r *HarborInstance) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	if refuseNewUrl(ctx, req.State, req.Plan, &resp.Diagnostics) {
		return
	}
	var state types.String
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("state"), &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	switch state.ValueString() {
	case common.StateConfigure, common.StateInstalled:
		r.installer.UpsertFromConfigure(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &instanceApi{}, &instanceModel{})
	default:
		r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &instanceApi{}, &instanceModel{})
	}
}

func (r *HarborInstance) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Delete(ctx, &resp.Diagnostics, &req.State, &instanceModel{})
}

func (r *HarborInstance) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importById(ctx, req, resp, invalidInstanceId)
}
