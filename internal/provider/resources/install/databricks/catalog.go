package installdatabricks

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
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

type Catalog struct {
	installer *common.Install
}

type catalogModel struct {
	Id        types.String `tfsdk:"id"`
	Workspace types.String `tfsdk:"workspace"`
	State     types.String `tfsdk:"state"`
}

type catalogJson struct {
	Workspace *string `json:"workspace,omitempty"`
	State     *string `json:"state,omitempty"`
}

type catalogApi struct {
	Item *catalogJson `json:"item"`
}

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

` + notePreview,
		Attributes: map[string]schema.Attribute{
			"id": fixedAttribute(
				`The name of the catalog`,
				stringvalidator.RegexMatches(CatalogNameRegex, "Catalog names have at most 255 characters, and no periods, spaces, forward slashes or control characters"),
			),
			"workspace": fixedAttribute(
				"The `id` of the `p0_databricks_workspace` that P0 reaches this catalog through",
				workspaceIdValidator(),
			),
			"state": common.StateAttribute,
		},
	}
}

func (r *Catalog) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.installer = &common.Install{
		Integration:  DatabricksKey,
		Component:    installresources.Catalog,
		ProviderData: internal.Configure(&req, resp),
		GetId:        catalogId,
		GetItemJson:  catalogItemJson,
		FromJson:     catalogFromJson,
		ToJson:       catalogToJson,
	}
}

func catalogId(data any) *string {
	model, ok := data.(*catalogModel)
	if !ok {
		return nil
	}
	id := model.Id.ValueString()
	return &id
}

func catalogItemJson(json any) any {
	api, ok := json.(*catalogApi)
	if !ok || api.Item == nil {
		return nil
	}
	return api.Item
}

func catalogFromJson(_ context.Context, _ *diag.Diagnostics, id string, json any) any {
	item, ok := json.(*catalogJson)
	if !ok {
		return nil
	}
	return &catalogModel{
		Id:        types.StringValue(id),
		Workspace: types.StringPointerValue(item.Workspace),
		State:     types.StringPointerValue(item.State),
	}
}

// P0 sets the state, so it is never sent.
func catalogToJson(data any) any {
	model, ok := data.(*catalogModel)
	if !ok {
		return nil
	}
	return &catalogJson{Workspace: model.Workspace.ValueStringPointer()}
}

func (r *Catalog) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	install(ctx, r.installer, &resp.Diagnostics, &req.Plan, &resp.State, &catalogApi{}, &catalogModel{})
}

func (r *Catalog) Read(ctx context.Context, _ resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &catalogApi{}, &catalogModel{})
}

func (r *Catalog) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &catalogApi{}, &catalogModel{})
}

func (r *Catalog) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Delete(ctx, &resp.Diagnostics, &req.State, &catalogModel{})
}

func (r *Catalog) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
