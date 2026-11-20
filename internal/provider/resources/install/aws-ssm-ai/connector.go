package installawsssmai

import (
	"context"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/p0-security/terraform-provider-p0/internal"
	"github.com/p0-security/terraform-provider-p0/internal/common"
)

var _ resource.Resource = &Connector{}
var _ resource.ResourceWithConfigure = &Connector{}
var _ resource.ResourceWithImportState = &Connector{}
var _ resource.ResourceWithModifyPlan = &Connector{}
var _ resource.ResourceWithValidateConfig = &Connector{}

// The Linux user that the deprecated sudo switch lets agents run as.
const rootUser = "root"

// A Linux user name, as useradd takes it by default: up to 32 lowercase
// letters, digits, underscores and hyphens, starting with a letter or an
// underscore.
var linuxUserNameRegex = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)

const invalidRunAsUser = "must be a Linux user name, such as root or deploy: up to 32 lowercase letters, digits, underscores and hyphens, starting with a letter or an underscore"

type Connector struct {
	installer *common.Install
}

func NewConnector() resource.Resource {
	return &Connector{}
}

func (*Connector) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_aws_ssm_ai_connector"
}

func (*Connector) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attributes := connectorStagedAttributes()
	attributes["instance_regions"] = schema.ListAttribute{
		Required:            true,
		ElementType:         types.StringType,
		MarkdownDescription: "The AWS regions whose instances agents can request, such as `[\"us-west-2\", \"us-east-1\"]`. Each must be a commercial AWS region, listed once. P0 checks that the connector can list the instances SSM manages in each. Changing it updates the connector in place, and P0 checks the install again.",
		Validators: []validator.List{
			listvalidator.SizeAtLeast(1),
			listvalidator.UniqueValues(),
			listvalidator.ValueStringsAre(commercialRegionValidator(invalidInstanceRegions)),
		},
	}
	attributes["run_as_users"] = schema.ListAttribute{
		Optional:            true,
		ElementType:         types.StringType,
		MarkdownDescription: "The Linux users that agents can ask to run as on an instance, such as `[\"root\", \"deploy\"]`. Each must be a Linux user name, listed once. An agent always signs in as its own Linux user, then switches to the user it asked for with `sudo`. Leave it out to let agents run only as their own users. P0 refuses requests for any user not listed. Changing it updates the connector in place.",
		Validators: []validator.List{
			listvalidator.SizeAtLeast(1),
			listvalidator.NoNullValues(),
			listvalidator.UniqueValues(),
			listvalidator.ValueStringsAre(stringvalidator.RegexMatches(linuxUserNameRegex, invalidRunAsUser)),
			listvalidator.ConflictsWith(path.MatchRoot("is_sudo_enabled")),
		},
	}
	sudo := switchAttribute(
		"Use `run_as_users` instead. `true` lets agents run as `root`, as `run_as_users = [\"root\"]` does. Defaults to `false`. It can't be set with `run_as_users`. Changing it updates the connector in place.",
	)
	sudo.DeprecationMessage = "Use run_as_users = [\"root\"] in place of is_sudo_enabled = true. is_sudo_enabled will be removed in a future release."
	attributes["is_sudo_enabled"] = sudo

	resp.Schema = schema.Schema{
		MarkdownDescription: `An AWS SSM (AI) connector installation.

The connector lets P0 give AI agents SSH access to the EC2 instances that SSM manages in its AWS account: for each agent's request it adds the agent's SSH key to an instance, and removes the key when the access ends. It keeps each key in AWS Secrets Manager, in the connector's own account, where the agent's MCP server reads it through AWS OIDC.

**Important:** Before creating this resource you must stage the connector with ` + "`p0_aws_ssm_ai_connector_staged`" + `, and set up the following in the staged AWS account and region:

- An IAM role named ` + "`" + connectorFunctionName + "`" + ` that Lambda can assume, with policies that let it list the instances SSM manages, run the ` + "`AWS-RunShellScript`" + ` document on them, and manage the agents' secrets in ` + "`vault.secrets_region`" + `. That document runs any command as root, so this role can run any command as root on every instance SSM manages in the account. Protect the Lambda and its role as you would root on those instances.
- The ` + "`" + connectorFunctionName + "`" + ` Lambda, running as that role.
- A policy on P0's AWS IAM management role that allows ` + "`lambda:InvokeFunction`" + ` on the Lambda.

Every apply that creates or changes this resource has P0 check the install: that it can invoke the Lambda, and that the connector can list the instances SSM manages in each of ` + "`instance_regions`" + `. P0 retries for about half a minute while a new IAM policy takes effect. If a check fails, the apply fails with P0's message, and the connector keeps its current values. If the first apply is denied access, apply again.

A connector that P0 hasn't finished installing, such as one imported before its install check passed, plans an update, and applying it finishes the install.

Destroying this resource returns the connector to staged, which P0 refuses once AWS IAM management or AWS OIDC is gone for the account. Keep both until the connector is destroyed, for example with a ` + "`depends_on`" + ` on the staged resource.

` + previewNote + `

` + docsNote,
		Attributes: attributes,
	}
}

func (r *Connector) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.installer = newInstaller(internal.Configure(&req, resp), connectorFromJson, connectorToJson)
}

func connectorFromJson(ctx context.Context, diags *diag.Diagnostics, id string, json any) any {
	item, ok := json.(*connectorJson)
	if !ok {
		return nil
	}
	// The state's sudo switch is off here, and setSudoSwitch turns it back on
	// when the configuration uses it.
	return &connectorModel{
		connectorStagedModel: newConnectorStagedModel(id, item),
		InstanceRegions:      listFromJson(ctx, diags, item.InstanceRegions),
		RunAsUsers:           listFromJson(ctx, diags, runAsUsersFromJson(item)),
		IsSudoEnabled:        types.BoolValue(false),
	}
}

// The run-as users that P0 lets agents run as. An item that has no run-as
// users but has the old sudo switch on lets them run as root, so it reads as
// root. Otherwise a configuration without run-as users would show no change,
// and never clear the switch.
func runAsUsersFromJson(item *connectorJson) *string {
	if item.RunAsUsers == nil && item.IsSudoEnabled != nil && *item.IsSudoEnabled {
		root := rootUser
		return &root
	}
	return item.RunAsUsers
}

func connectorToJson(data any) any {
	model, ok := data.(*connectorModel)
	if !ok {
		return nil
	}
	json := model.toJson()
	json.InstanceRegions = listToJson(model.InstanceRegions)
	json.RunAsUsers = runAsUsersToJson(model)
	return json
}

// The run-as users to send. P0 merges what the provider sends over the item it
// stores, so a configuration without any sends an empty string, which clears
// them, in place of leaving the field out, which would keep them.
func runAsUsersToJson(model *connectorModel) *string {
	if users := listToJson(model.RunAsUsers); users != nil {
		return users
	}
	users := ""
	if model.IsSudoEnabled.ValueBool() {
		users = rootUser
	}
	return &users
}

// The value of an attribute, from a plan or a state.
type attributeGetter interface {
	GetAttribute(ctx context.Context, path path.Path, target interface{}) diag.Diagnostics
}

// Puts the deprecated sudo switch back in state. A configuration that turns it
// on, without run_as_users, sends root as the only run-as user, and P0 answers
// with that list and no switch. So when P0 has exactly root, the state keeps
// the switch and no run_as_users, as configured, and the next plan shows no
// change. Any other list from P0 stays in state, so the next plan shows the
// difference.
func setSudoSwitch(ctx context.Context, diags *diag.Diagnostics, configured attributeGetter, state *tfsdk.State) {
	if diags.HasError() || state.Raw.IsNull() {
		return
	}
	var sudo types.Bool
	var configuredUsers types.List
	diags.Append(configured.GetAttribute(ctx, path.Root("is_sudo_enabled"), &sudo)...)
	diags.Append(configured.GetAttribute(ctx, path.Root("run_as_users"), &configuredUsers)...)
	if diags.HasError() || !sudo.ValueBool() || !configuredUsers.IsNull() {
		return
	}

	var users types.List
	diags.Append(state.GetAttribute(ctx, path.Root("run_as_users"), &users)...)
	if diags.HasError() || !users.Equal(types.ListValueMust(types.StringType, []attr.Value{types.StringValue(rootUser)})) {
		return
	}
	diags.Append(state.SetAttribute(ctx, path.Root("run_as_users"), types.ListNull(types.StringType))...)
	diags.Append(state.SetAttribute(ctx, path.Root("is_sudo_enabled"), types.BoolValue(true))...)
}

func (*Connector) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	validateConnectorConfig(ctx, req.Config, &resp.Diagnostics)
}

func (r *Connector) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	stageAndInstall(ctx, r.installer, &resp.Diagnostics, &req.Plan, &resp.State, &connectorApi{}, &connectorModel{})
	setSudoSwitch(ctx, &resp.Diagnostics, req.Plan, &resp.State)
}

func (r *Connector) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &connectorApi{}, &connectorModel{})
	setSudoSwitch(ctx, &resp.Diagnostics, req.State, &resp.State)
}

func (*Connector) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	planFinishingInstall(ctx, req, resp)
}

// Only instance_regions, run_as_users and is_sudo_enabled change in place. P0
// checks them on the configure step, which is all an installed connector gets,
// and saves nothing if the check fails: the connector stays installed with its
// current values, and the next plan shows the update again. A staged connector
// is verified first, and a failed verify saves nothing either.
func (r *Connector) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	updateFromInstallState(ctx, r.installer, req, resp, &connectorApi{}, &connectorModel{})
	setSudoSwitch(ctx, &resp.Diagnostics, req.Plan, &resp.State)
}

// Returns the connector to the "stage" state, so that the staged resource
// deletes it.
func (r *Connector) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Rollback(ctx, &resp.Diagnostics, &req.State, &connectorModel{})
}

func (r *Connector) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importById(ctx, req, resp, awsAccountIdValidator())
}
