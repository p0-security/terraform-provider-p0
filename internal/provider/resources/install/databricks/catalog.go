package installdatabricks

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/p0-security/terraform-provider-p0/internal"
	"github.com/p0-security/terraform-provider-p0/internal/common"
	installresources "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install"
)

var _ resource.Resource = &Catalog{}
var _ resource.ResourceWithConfigure = &Catalog{}
var _ resource.ResourceWithImportState = &Catalog{}
var _ resource.ResourceWithModifyPlan = &Catalog{}

type Catalog struct {
	installer *common.Install
}

type catalogModel struct {
	Id          types.String `tfsdk:"id"`
	WorkspaceId types.String `tfsdk:"workspace_id"`
	CatalogName types.String `tfsdk:"catalog_name"`
	State       types.String `tfsdk:"state"`
}

func (m catalogModel) key() string {
	return catalogKey(m.CatalogName.ValueString(), m.WorkspaceId.ValueString())
}

type catalogJson struct {
	Workspace *string `json:"workspace,omitempty"`
	State     *string `json:"state,omitempty"`
}

type catalogApi = itemApi[catalogJson]

func NewCatalog() resource.Resource {
	return &Catalog{}
}

func (*Catalog) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_databricks_catalog"
}

func (*Catalog) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A Unity Catalog catalog installation. P0 grants privileges in a catalog through its account's service principal, which must hold ` + "`MANAGE`" + ` on the catalog.

**Important:** Before creating this resource, the workspace that P0 reaches the catalog through must be installed (see the ` + "`p0_databricks_workspace`" + ` resource), and the service principal must hold ` + "`MANAGE`" + ` on the catalog. Grant it with ` + "`databricks_grant`" + `, never ` + "`databricks_grants`" + `, which overwrites every other grant on the catalog. Granting ` + "`MANAGE`" + ` takes the catalog's owner, a holder of ` + "`MANAGE`" + ` on it, or a metastore admin.

A catalog that P0 hasn't finished installing, such as one imported before its install check passed, plans an update, and applying it finishes the install.

` + common.NotePreview,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The catalog's key in P0, `<catalog_name>@<workspace_id>`, which is also its import ID. Catalog names are unique only within a metastore, so the key names the workspace too.",
			},
			"workspace_id": common.FixedAttribute(
				"The ID of the workspace that P0 reaches this catalog through, which is the `id` of its `p0_databricks_workspace`",
				workspaceIdValidator(),
			),
			"catalog_name": common.FixedAttribute(
				`The name of the catalog, in lowercase`,
				catalogNameValidator{},
			),
			"state": common.StateAttribute,
		},
	}
}

func (r *Catalog) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.installer = newInstaller[catalogJson](internal.Configure(&req, resp), installresources.Catalog, catalogFromJson, catalogToJson)
}

func catalogFromJson(_ context.Context, diags *diag.Diagnostics, id string, json any) any {
	item, ok := json.(*catalogJson)
	if !ok {
		return nil
	}
	catalogName, _, ok := parseCatalogKey(id)
	if !ok {
		diags.AddError("Bad catalog key", fmt.Sprintf("P0 has a catalog with the key %q, which isn't of the form <catalog name>@<workspace ID>", id))
		return nil
	}
	return &catalogModel{
		Id:          types.StringValue(id),
		WorkspaceId: types.StringPointerValue(item.Workspace),
		CatalogName: types.StringValue(catalogName),
		State:       types.StringPointerValue(item.State),
	}
}

// P0 sets the state, so it is never sent.
func catalogToJson(data any) any {
	model, ok := data.(*catalogModel)
	if !ok {
		return nil
	}
	return &catalogJson{Workspace: model.WorkspaceId.ValueStringPointer()}
}

func (r *Catalog) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	stageAndInstall(ctx, r.installer, &resp.Diagnostics, &req.Plan, &resp.State, &catalogApi{}, &catalogModel{})
}

func (r *Catalog) Read(ctx context.Context, _ resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &catalogApi{}, &catalogModel{})
}

func (*Catalog) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	common.PlanFinishingInstall(ctx, req, resp)
}

// Every attribute requires replacement, so an update only finishes an install
// that P0 hasn't.
func (r *Catalog) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.installer.UpdateFromInstallState(ctx, req, resp, &catalogApi{}, &catalogModel{})
}

func (r *Catalog) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Delete(ctx, &resp.Diagnostics, &req.State, &catalogModel{})
}

// Imports a catalog by its key, `<catalog_name>@<workspace_id>`. The key's
// catalog name and workspace ID get the same checks as the configuration's,
// because the read that follows the import puts the key in a URL path, where
// `sales%@7` doesn't parse and `sales#eu@7` reads `sales`.
func (r *Catalog) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// parseCatalogKey checks the workspace ID as workspaceIdValidator does.
	catalogName, workspaceId, ok := parseCatalogKey(req.ID)
	if !ok {
		resp.Diagnostics.AddError("Bad import ID", fmt.Sprintf("Import a catalog by <catalog name>@<workspace ID>, e.g. main@1234567890123456, not %q", req.ID))
		return
	}
	if message := catalogNameError(catalogName); message != "" {
		resp.Diagnostics.AddError("Invalid catalog name", message)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("catalog_name"), catalogName)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("workspace_id"), workspaceId)...)
}
