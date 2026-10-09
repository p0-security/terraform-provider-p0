package installdatabricks

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/p0-security/terraform-provider-p0/internal"
	"github.com/p0-security/terraform-provider-p0/internal/common"
	installresources "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install"
)

var _ resource.Resource = &Workspace{}
var _ resource.ResourceWithConfigure = &Workspace{}
var _ resource.ResourceWithImportState = &Workspace{}
var _ resource.ResourceWithModifyPlan = &Workspace{}

type Workspace struct {
	installer *common.Install
}

type workspaceModel struct {
	Id      types.String `tfsdk:"id"`
	Account types.String `tfsdk:"account"`
	State   types.String `tfsdk:"state"`
}

type workspaceJson struct {
	Account *string `json:"account,omitempty"`
	State   *string `json:"state,omitempty"`
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
		MarkdownDescription: `A Databricks workspace installation. P0 manages a workspace through its account's service principal, which must be a workspace admin there.

**Important:** Before creating this resource, the workspace's account must be installed (see the ` + "`p0_databricks_account`" + ` resource), and its service principal must be assigned to the workspace with the ` + "`ADMIN`" + ` permission, for example with ` + "`databricks_mws_permission_assignment`" + `.

A workspace that P0 hasn't finished installing, such as one imported before its install check passed, plans an update, and applying it finishes the install.

` + common.NotePreview,
		Attributes: map[string]schema.Attribute{
			"id": common.FixedAttribute(`The Databricks workspace ID`, workspaceIdValidator()),
			"account": common.FixedAttribute(
				"The `id` of the `p0_databricks_account` that this workspace is in",
				uuidValidator("Databricks account IDs"),
			),
			"state": common.StateAttribute,
		},
	}
}

func (r *Workspace) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.installer = newInstaller[workspaceJson](internal.Configure(&req, resp), installresources.Workspace, workspaceFromJson, workspaceToJson)
}

func workspaceFromJson(_ context.Context, _ *diag.Diagnostics, id string, json any) any {
	item, ok := json.(*workspaceJson)
	if !ok {
		return nil
	}
	return &workspaceModel{
		Id:      types.StringValue(id),
		Account: types.StringPointerValue(item.Account),
		State:   types.StringPointerValue(item.State),
	}
}

// P0 sets the state, so it is never sent.
func workspaceToJson(data any) any {
	model, ok := data.(*workspaceModel)
	if !ok {
		return nil
	}
	return &workspaceJson{Account: model.Account.ValueStringPointer()}
}

func (r *Workspace) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	stageAndInstall(ctx, r.installer, &resp.Diagnostics, &req.Plan, &resp.State, &workspaceApi{}, &workspaceModel{})
}

func (r *Workspace) Read(ctx context.Context, _ resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &workspaceApi{}, &workspaceModel{})
}

func (*Workspace) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	common.PlanFinishingInstall(ctx, req, resp)
}

// Every attribute requires replacement, so an update only finishes an install
// that P0 hasn't.
func (r *Workspace) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.installer.UpdateFromInstallState(ctx, req, resp, &workspaceApi{}, &workspaceModel{})
}

func (r *Workspace) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Delete(ctx, &resp.Diagnostics, &req.State, &workspaceModel{})
}

func (r *Workspace) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
