package installdatabricks

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/p0-security/terraform-provider-p0/internal"
	"github.com/p0-security/terraform-provider-p0/internal/common"
)

var _ resource.Resource = &Workspace{}
var _ resource.ResourceWithConfigure = &Workspace{}
var _ resource.ResourceWithImportState = &Workspace{}
var _ resource.ResourceWithModifyPlan = &Workspace{}

type Workspace struct {
	installer *common.Install
}

type workspaceModel struct {
	Id                      types.String `tfsdk:"id"`
	Account                 types.String `tfsdk:"account"`
	WorkspaceUrl            types.String `tfsdk:"workspace_url"`
	WorkspaceName           types.String `tfsdk:"workspace_name"`
	IsWorkspaceAdminEnabled types.Bool   `tfsdk:"is_workspace_admin_enabled"`
	State                   types.String `tfsdk:"state"`
}

type workspaceJson struct {
	Account                 *string `json:"account,omitempty"`
	WorkspaceUrl            *string `json:"workspaceUrl,omitempty"`
	WorkspaceName           *string `json:"workspaceName,omitempty"`
	IsWorkspaceAdminEnabled *bool   `json:"isWorkspaceAdminEnabled,omitempty"`
	State                   *string `json:"state,omitempty"`
}

type workspaceApi = itemApi[workspaceJson]

func (m workspaceModel) key() string {
	return m.Id.ValueString()
}

func NewWorkspace() resource.Resource {
	return &Workspace{}
}

func (*Workspace) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_databricks_workspace"
}

func (*Workspace) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A Databricks workspace installation. P0 calls the workspace at ` + "`workspace_url`" + ` as its account's service principal.

**Important:** Before creating this resource, the workspace's account must be installed (see the ` + "`p0_databricks_account`" + ` resource), and its service principal must be assigned to the workspace as ` + "`USER`" + `, or as ` + "`ADMIN`" + ` when ` + "`is_workspace_admin_enabled`" + ` is ` + "`true`" + `. Assign it with ` + "`databricks_mws_permission_assignment`" + ` or ` + "`databricks_permission_assignment`" + `. The workspace also needs a Unity Catalog metastore assigned to it, or P0's install check fails.

A workspace that P0 hasn't finished installing, such as one imported before its install check passed, plans an update, and applying it finishes the install.

` + betaNote,
		Attributes: map[string]schema.Attribute{
			"id": fixedAttribute("The Databricks workspace ID, e.g. `1234567890123456`. When you open the workspace in a browser, its ID is the number after `?o=` in the URL.", workspaceIdValidator()),
			"account": fixedAttribute(
				"The `id` of the `p0_databricks_account` that this workspace is in",
				accountIdValidator(),
			),
			"workspace_url": fixedAttribute(
				"The workspace's URL, e.g. `https://dbc-1234abcd-5678.cloud.databricks.com`: `https://` and a lowercase host on the workspace domain of the account's cloud, with no port, path or trailing slash. "+
					"P0 calls the workspace there, and checks that it belongs to this workspace ID. P0 can't change a workspace's URL, so a new value replaces the workspace. "+
					"A configuration that holds the workspace's `databricks_mws_workspaces` can pass its `workspace_url` here.",
				workspaceUrlValidator(),
			),
			"workspace_name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "A name for the workspace in P0, e.g. `analytics-prod`. Requests show it, and approval rules can match it with the workspace filter's name key. Changing it updates the workspace in place. A configuration that holds the workspace's `databricks_mws_workspaces` can pass its `workspace_name` here.",
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
					noWhitespaceAround(),
				},
			},
			// P0 checks the assignment on the configure step, as it checks
			// workspace_name.
			"is_workspace_admin_enabled": adminSwitchAttribute(
				"Whether users can request workspace admin on this workspace. Defaults to `false`. The service principal needs an `ADMIN` assignment, in place of `USER`, only when this is `true`, and P0 then checks that it is a workspace admin. Changing it updates the workspace in place. P0 needs the role to revoke the workspace admin it granted, so it refuses to turn this off while an approved workspace-admin request on this workspace hasn't ended. Its error starts `P0 still manages workspace admin on Databricks workspace` and lists those requests. Let them end, or revoke them, then apply again. If P0's check fails or P0 refuses, the apply fails, and the workspace stays as it was. Terraform may change the assignment in that apply before P0 refuses, so if it did, apply your previous configuration again to restore `ADMIN`.",
			),
			"state": common.StateAttribute,
		},
	}
}

func (r *Workspace) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.installer = newInstaller[workspaceJson](internal.Configure(&req, resp), workspaceComponent, workspaceFromJson, workspaceToJson)
}

func workspaceFromJson(_ context.Context, _ *diag.Diagnostics, id string, json any) any {
	item, ok := json.(*workspaceJson)
	if !ok {
		return nil
	}
	return &workspaceModel{
		Id:                      types.StringValue(id),
		Account:                 types.StringPointerValue(item.Account),
		WorkspaceUrl:            types.StringPointerValue(item.WorkspaceUrl),
		WorkspaceName:           types.StringPointerValue(item.WorkspaceName),
		IsWorkspaceAdminEnabled: types.BoolPointerValue(item.IsWorkspaceAdminEnabled),
		State:                   types.StringPointerValue(item.State),
	}
}

// P0 sets the state, so it is never sent.
func workspaceToJson(data any) any {
	model, ok := data.(*workspaceModel)
	if !ok {
		return nil
	}
	return &workspaceJson{
		Account:                 model.Account.ValueStringPointer(),
		WorkspaceUrl:            model.WorkspaceUrl.ValueStringPointer(),
		WorkspaceName:           model.WorkspaceName.ValueStringPointer(),
		IsWorkspaceAdminEnabled: model.IsWorkspaceAdminEnabled.ValueBoolPointer(),
	}
}

func (r *Workspace) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	stageAndInstall(ctx, r.installer, &resp.Diagnostics, &req.Plan, &resp.State, &workspaceApi{}, &workspaceModel{})
}

func (r *Workspace) Read(ctx context.Context, _ resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &workspaceApi{}, &workspaceModel{})
}

func (*Workspace) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	planFinishingInstall(ctx, req, resp)
}

// Only workspace_name and is_workspace_admin_enabled change in place. P0 checks
// them on the configure step, which is all an installed workspace gets, and
// saves nothing if the check fails: the workspace stays installed with its
// current values, and the next plan shows the update again. A staged workspace
// is verified first, and a failed verify saves nothing either.
func (r *Workspace) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	updateFromInstallState(ctx, r.installer, req, resp, &workspaceApi{}, &workspaceModel{})
}

func (r *Workspace) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Delete(ctx, &resp.Diagnostics, &req.State, &workspaceModel{})
}

func (r *Workspace) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importById(ctx, req, resp, workspaceIdValidator())
}
