package installargocd

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

// copy of internal/provider/resources/install/harbor/instance.go, kept close to it so
// that the copies merge into one (INT-576). It adds the instance's managing
// Application and namespace, which, like the URL, say where P0 writes grants.

var _ resource.Resource = &ArgocdInstance{}
var _ resource.ResourceWithConfigure = &ArgocdInstance{}
var _ resource.ResourceWithImportState = &ArgocdInstance{}
var _ resource.ResourceWithModifyPlan = &ArgocdInstance{}
var _ resource.ResourceWithValidateConfig = &ArgocdInstance{}

// The Argo CD `argocd-instance` item: one Argo CD instance, the connector that reaches
// it, the Application that manages its argocd-rbac-cm, and where the connector finds
// the p0-connector account's API token
// (packages/integrations/argocd/shared/src/components.ts in the app).
type ArgocdInstance struct {
	installer *common.Install
}

const invalidInstanceId = "Invalid Argo CD instance ID"

type instanceModel struct {
	Id          types.String `tfsdk:"id"`
	Connector   types.String `tfsdk:"connector"`
	Url         types.String `tfsdk:"url"`
	ManagingApp types.String `tfsdk:"managing_app"`
	Namespace   types.String `tfsdk:"namespace"`
	Vault       *vaultModel  `tfsdk:"vault"`
	SecretName  types.String `tfsdk:"secret_name"`
	State       types.String `tfsdk:"state"`
}

// The item, as P0 stores it. An instance without a namespace is in defaultNamespace.
type instanceJson struct {
	Connector   *string    `json:"connector,omitempty"`
	Url         *string    `json:"url,omitempty"`
	ManagingApp *string    `json:"managingApp,omitempty"`
	Namespace   *string    `json:"namespace,omitempty"`
	Vault       *vaultJson `json:"vault,omitempty"`
	SecretName  *string    `json:"secretName,omitempty"`
	State       *string    `json:"state,omitempty"`
}

type instanceApi struct {
	Item *instanceJson `json:"item"`
}

func NewArgocdInstance() resource.Resource {
	return &ArgocdInstance{}
}

func (*ArgocdInstance) Metadata(_ context.Context, req resource.MetadataRequest, res *resource.MetadataResponse) {
	res.TypeName = req.ProviderTypeName + "_argocd_instance"
}

func (*ArgocdInstance) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `An Argo CD instance that P0 can grant roles on.

P0 reaches the instance through an Argo CD connector, which you install with ` + "`p0_argocd_connector`" + `. The connector calls the instance's API as the Argo CD local account ` + "`p0-connector`" + `, whose API token stays in your own secret manager: P0 never reads it, and only the connector does. One connector can serve several instances, and each instance has its own token and secret.

P0 grants a role as a line in the ` + "`policy.p0.csv`" + ` key of the instance's ` + "`argocd-rbac-cm`" + `, which it writes through the Argo CD Application that ` + "`managing_app`" + ` names. P0 owns that key: leave it out of Git.

**Important:** Install the connector first, and grant its identity read access to the token's secret: in an earlier apply, or in the same one with this resource depending on the grant, as the examples do. Creating this resource has P0 check the instance through the connector. If the check fails, the apply fails with P0's message.

An instance's attributes can't change once it is created. Changing ` + "`id`" + `, ` + "`connector`" + `, ` + "`vault`" + ` or ` + "`secret_name`" + ` replaces the instance. A plan refuses a change to an installed instance's ` + "`url`" + `, ` + "`managing_app`" + ` or ` + "`namespace`" + `: P0 revokes access where it granted it, so for another instance, Application or namespace, add a new ` + "`p0_argocd_instance`" + ` with its own ` + "`id`" + `, and remove this one once its open access has ended. To rotate the API token, generate a new one and add it as a new version of the same secret, which needs no change here.

**Warning:** Replacing or destroying this resource removes the instance from P0. P0 can't revoke access it granted on an instance it no longer has, so those lines stay in ` + "`policy.p0.csv`" + ` until you remove them there. Before you change an attribute or destroy this resource, revoke the instance's open access in P0 or wait for it to expire.

An instance that already exists in P0, such as one added in the P0 app, isn't created again: the apply fails, and says to import it with ` + "`terraform import`" + `. An imported instance that P0 hasn't finished installing plans an update, and applying it finishes the install. If that install can't finish, fix what P0's message names and apply again, or start over: ` + "`terraform apply -replace=<address>`" + ` deletes the instance and creates it again, and ` + "`terraform state rm <address>`" + ` stops managing it.

The email domains that may receive access are set on the connector, as its ` + "`ALLOWED_EMAIL_DOMAIN_PATTERN`" + ` environment variable, and aren't an attribute of this resource.

**Prerequisites:**

- Argo CD 3.0 or later, served at the root of its address. An Argo CD served under a root path (` + "`server.rootpath`" + `) isn't supported.

- An Argo CD Application that manages ` + "`argocd-rbac-cm`" + `. Without one, P0 can't write grants.

- The local account ` + "`p0-connector`" + `, with ` + "`accounts.p0-connector: apiKey`" + ` in ` + "`argocd-cm`" + `, and these lines in ` + "`argocd-rbac-cm`" + `'s ` + "`policy.csv`" + `, with ` + "`<project>/<application>`" + ` the Application that manages it and ` + "`<namespace>`" + ` its namespace:

  ` + "```" + `
  p, role:p0-connector, applications, get, <project>/<application>, allow
  p, role:p0-connector, applications, update//ConfigMap/<namespace>/argocd-rbac-cm, <project>/<application>, allow
  p, role:p0-connector, projects, get, *, allow
  g, p0-connector, role:p0-connector
  ` + "```" + `

  ` + "`argocd-rbac-cm`" + `'s ` + "`scopes`" + ` must include ` + "`email`" + `, since P0 grants roles to users by their email.

- A secret that holds the account's API token as JSON, ` + "`{\"token\": \"<api token>\"}`" + `, in the secret manager that ` + "`vault`" + ` names. Generate the token with ` + "`argocd account generate-token --account p0-connector`" + `.

- For ` + "`aws-sm`" + `, ` + "`p0_aws_secrets_manager`" + ` installed for the account that holds the secret. Give the connector's role ` + "`secretsmanager:GetSecretValue`" + ` on the secret alone, and ` + "`kms:Decrypt`" + ` on its key if a customer-managed KMS key encrypts it. A secret in an account other than the connector's needs a customer-managed key, and the secret's resource policy and the key's policy must allow the connector's role.

- For ` + "`gcp-sm`" + `, Google Secret Manager installed in P0 for the project that holds the secret. This provider has no resource for it, so install it in the P0 app. Give the connector's service account ` + "`roles/secretmanager.secretAccessor`" + ` and ` + "`roles/secretmanager.viewer`" + ` on the secret alone.

` + betaNote + `

For the full install steps, see [Installing Argo CD](https://docs.p0.dev/integrations/resource-integrations/argocd/installation).`,
		Attributes: map[string]schema.Attribute{
			"id": itemIdAttribute(`A name you choose for the instance, such as ` + "`prod`" + `: ` + itemIdRule),
			"connector": itemIdAttribute("The `id` of the `p0_argocd_connector` that reaches this instance. " +
				"The connector must be installed, and run in the same cloud as `vault`: on AWS Lambda for `aws-sm`, on Google Cloud Run for `gcp-sm`."),
			// P0 fixes the URL, the managing Application and the namespace when it
			// creates the instance: an open grant is a line in that instance's
			// argocd-rbac-cm, and its revoke goes where the grant went. Replacing the
			// instance under the same ID would send those revokes elsewhere, so none of
			// the three requires replacement: ModifyPlan refuses a change to them. A new
			// connector, vault or secret reaches the same argocd-rbac-cm, so those replace.
			"url": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "The address your Argo CD instance is served at, such as `https://argocd.example.com`: `https://` and the host, " +
					"with a port if it isn't 443, in lowercase and with nothing after it. Leave out a trailing slash, any path, and any username or password. " +
					"P0 stores the address in this spelling alone, and the connector sends the API token to it. " +
					"It can't change: for another address, add a new `p0_argocd_instance` with its own `id`.",
			},
			"managing_app": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "The Argo CD Application that manages `argocd-rbac-cm`, as `<project>/<application>`, such as `default/argocd`: " +
					"each name lowercase letters, digits, hyphens and dots, up to 253 characters. " +
					"The connector writes grants through it, and the `p0-connector` account's policy names it. " +
					"It can't change: for another Application, add a new `p0_argocd_instance` with its own `id`.",
			},
			"namespace": schema.StringAttribute{
				Optional: true,
				MarkdownDescription: "The Kubernetes namespace `argocd-rbac-cm` is in: up to 63 lowercase letters, digits and hyphens, " +
					"starting and ending with a letter or digit. Leave it out, rather than empty, for `" + defaultNamespace + "`, Argo CD's default. " +
					"It can't change: for another namespace, add a new `p0_argocd_instance` with its own `id`.",
			},
			"vault": vaultAttribute(),
			"secret_name": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "The name of the secret that holds the API token, in `vault`: the name alone, never an ARN or a resource name. " +
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
func (*ArgocdInstance) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var url, managingApp, namespace, secretName types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("url"), &url)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("managing_app"), &managingApp)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("namespace"), &namespace)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("secret_name"), &secretName)...)
	vault := vaultFromConfig(ctx, req.Config, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	validateVault(vault, &resp.Diagnostics)
	validateInstanceUrl(url, &resp.Diagnostics)
	validateManagingApp(managingApp, &resp.Diagnostics)
	validateNamespace(namespace, &resp.Diagnostics)
	validateSecretName(vault, secretName, &resp.Diagnostics)
}

var instanceUrlRules = []rule{{
	summary: "Invalid Argo CD instance URL",
	message: invalidInstanceUrl,
	test:    isInstanceUrl,
}}

func validateInstanceUrl(url types.String, diags *diag.Diagnostics) {
	validateField(path.Root("url"), url, instanceUrlRules, diags)
}

var managingAppRules = []rule{{
	summary: "Invalid Argo CD managing application",
	message: invalidManagingApp,
	test:    isManagingApp,
}}

func validateManagingApp(app types.String, diags *diag.Diagnostics) {
	validateField(path.Root("managing_app"), app, managingAppRules, diags)
}

// P0 takes a blank namespace as defaultNamespace, as the app's form sends one. Here it
// is refused for a namespace left out, which P0 stores as no namespace: a blank one
// it stored could read back other than the configuration, and would count as a new
// namespace if the configuration then left it out.
var namespaceRules = []rule{{
	summary: "Invalid Argo CD namespace",
	message: blankNamespace,
	test:    func(namespace string) bool { return namespace != "" },
}, {
	summary: "Invalid Argo CD namespace",
	message: invalidNamespace,
	test:    namespaceRegex.MatchString,
}}

func validateNamespace(namespace types.String, diags *diag.Diagnostics) {
	validateField(path.Root("namespace"), namespace, namespaceRules, diags)
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
// set. The value is never repeated: a URL or a secret's name may be the API token,
// pasted into the wrong field.
//
// TODO(INT-576): a copy of Harbor's validateField, itself a copy of
// github-repositories', kept close to them so that the three can merge into one.
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

func (r *ArgocdInstance) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:        ArgocdKey,
		Component:          installresources.ArgocdInstance,
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
// connector, the secret, the p0-connector account or the instance itself), and when
// it refuses the request for another reason, so the summary blames neither and leaves
// the reason to P0's message. A refusal to change a fixed field is told apart by its
// text, since no setup causes it: the item P0 holds differs from the configuration.
func describeInstanceCheckError(id string, err error) (string, string) {
	if strings.Contains(err.Error(), fixedFieldRefusal) {
		return "Argo CD instance differs from the one in P0",
			fmt.Sprintf("P0 holds an Argo CD instance %q whose attributes differ from this configuration, and they can't change once it is created:\n\n%s\n\n"+
				"Make the configuration match the instance in P0. To start over, `terraform apply -replace=<address>` deletes the instance and creates it again, "+
				"and `terraform state rm <address>` stops managing it.", id, err)
	}
	return "P0 didn't finish installing the Argo CD instance",
		fmt.Sprintf("P0 refused to install the Argo CD instance %q:\n\n%s\n\n%s", id, err, installWayOut)
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
			fmt.Sprintf("P0 returned the Argo CD instance %q without its vault. "+
				"Remove the instance in the P0 app and apply again, or remove it from Terraform state with `terraform state rm <address>`.", id),
		)
		return nil
	}

	vault := vaultFromJson(jsonv.Vault)
	if vault == nil {
		diags.AddError(
			"Unsupported Argo CD instance vault",
			fmt.Sprintf("The Argo CD instance %s has a %q vault. This provider supports %q and %q. "+
				"Manage it in the P0 app, or remove it from Terraform state with `terraform state rm`.",
				id, jsonv.Vault.Type, awsSecretsManager, installvaultedcredential.GcpSecretManager),
		)
		return nil
	}

	// A namespace that P0 doesn't hold stays null here, rather than read as
	// defaultNamespace, which only the app fills in: the configuration that leaves it
	// out then plans no change.
	return &instanceModel{
		Id:          types.StringValue(id),
		Connector:   types.StringPointerValue(jsonv.Connector),
		Url:         types.StringPointerValue(jsonv.Url),
		ManagingApp: types.StringPointerValue(jsonv.ManagingApp),
		Namespace:   types.StringPointerValue(jsonv.Namespace),
		Vault:       vault,
		SecretName:  types.StringPointerValue(jsonv.SecretName),
		State:       types.StringPointerValue(jsonv.State),
	}
}

// The body of every request: the whole item. The PUT that stages the instance needs
// every field. Verify and configure get them again, so that P0, which compares each
// with the item it stored, refuses to install an instance that isn't the configured
// one. A namespace left out is left out of the body too, as P0 stores it.
func instanceToJson(data any) any {
	datav, ok := data.(*instanceModel)
	if !ok {
		return nil
	}

	json := instanceJson{
		Connector:   datav.Connector.ValueStringPointer(),
		Url:         datav.Url.ValueStringPointer(),
		ManagingApp: datav.ManagingApp.ValueStringPointer(),
		Namespace:   datav.Namespace.ValueStringPointer(),
		SecretName:  datav.SecretName.ValueStringPointer(),
	}
	if datav.Vault != nil {
		json.Vault = datav.Vault.toJson()
	}
	return &json
}

func (r *ArgocdInstance) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json instanceApi
	var data instanceModel

	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	if resp.Diagnostics.HasError() {
		return
	}

	// An instance that P0 holds is never staged again: the PUT would return an
	// installed one to "stage", and a failed check would then taint the resource, so
	// that the next apply deleted an instance Terraform never created.
	refuseExistingItem(r.installer.ProviderData, installresources.ArgocdInstance, "instance", data.Id.ValueString(), &resp.Diagnostics)
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

func (r *ArgocdInstance) Read(ctx context.Context, _ resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &instanceApi{}, &instanceModel{})
}

// A field that says where P0 writes the instance's grants, and P0's refusal of a new
// value for it on an installed instance.
type fixedLocationField struct {
	attribute string
	summary   string
}

var fixedLocationFields = []fixedLocationField{
	{"url", "An Argo CD instance's address can't change"},
	{"managing_app", "An Argo CD instance's managing application can't change"},
	{"namespace", "An Argo CD instance's namespace can't change"},
}

const addAnotherInstance = "Add a new p0_argocd_instance with its own id, and remove this one once its open access has ended."

// Refuses a new URL, managing Application or namespace for an installed instance, and
// reports whether it did. Only an installed instance can have open access. For one
// imported before it finished installing, P0's own refusal of the change says more. A
// value that Terraform doesn't know yet passes: one unknown until apply is checked
// again in Update. Any other difference from state is a new value, a null one
// included: a namespace that is set where it was left out, or the other way round,
// is a new one, even if it is defaultNamespace, since P0 compares what it stores.
func refuseNewLocation(ctx context.Context, state tfsdk.State, plan tfsdk.Plan, diags *diag.Diagnostics) bool {
	var installState types.String
	if read := state.GetAttribute(ctx, path.Root("state"), &installState); read.HasError() {
		diags.Append(read...)
		return false
	}
	if installState.ValueString() != common.StateInstalled {
		return false
	}
	refused := false
	for _, field := range fixedLocationFields {
		var before, after types.String
		read := state.GetAttribute(ctx, path.Root(field.attribute), &before)
		read.Append(plan.GetAttribute(ctx, path.Root(field.attribute), &after)...)
		if read.HasError() {
			diags.Append(read...)
			return refused
		}
		if after.IsUnknown() || before.Equal(after) {
			continue
		}
		diags.AddAttributeError(path.Root(field.attribute), field.summary, field.summary+". "+addAnotherInstance)
		refused = true
	}
	return refused
}

// Refuses a new URL, managing Application or namespace for an installed instance, and
// plans an update for one that P0 hasn't installed.
//
// Replacing an instance under its ID, which is what Terraform would do for a new value
// of one of those, deletes the item and creates one with the same ID. P0 keys open
// access by that ID, and revokes it from the argocd-rbac-cm that the item names, which
// would no longer be the one it was granted in. A new ID goes through, since it
// replaces the resource with a new item.
//
// An item that P0 hasn't installed plans an update even when the configuration hasn't
// changed, so that the next apply finishes the install: P0 grants nothing on an
// instance until its item is installed. That covers an item imported before its
// checks passed. An installed item keeps the framework's plan, which shows no
// difference while the configuration matches it.
//
// TODO(INT-576): the second half is a copy of Harbor's ModifyPlan, itself a copy of
// github-repositories'.
func (r *ArgocdInstance) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
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
	if idBefore.Equal(idAfter) && refuseNewLocation(ctx, req.State, req.Plan, &resp.Diagnostics) {
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
func (r *ArgocdInstance) checkSecretNameAcrossAccounts(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
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
// of an item that isn't installed, with the item as it is. And a URL, managing
// Application or namespace that Terraform only knows at apply time plans one, which
// is refused here if the value turns out to be a new one. An item that P0 has
// verified, at configure or installed, gets the configure step alone. Any other, a
// staged one, is verified first, as Create does.
func (r *ArgocdInstance) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	if refuseNewLocation(ctx, req.State, req.Plan, &resp.Diagnostics) {
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

func (r *ArgocdInstance) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Delete(ctx, &resp.Diagnostics, &req.State, &instanceModel{})
}

func (r *ArgocdInstance) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importById(ctx, req, resp, invalidInstanceId)
}
