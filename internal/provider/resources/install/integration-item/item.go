package installintegrationitem

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/p0-security/terraform-provider-p0/internal"
	"github.com/p0-security/terraform-provider-p0/internal/common"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &integrationItem{}
var _ resource.ResourceWithConfigure = &integrationItem{}
var _ resource.ResourceWithImportState = &integrationItem{}

type integrationItem struct {
	data *internal.P0ProviderData
}

type integrationItemModel struct {
	Integration types.String `tfsdk:"integration"`
	Component   types.String `tfsdk:"component"`
	Id          types.String `tfsdk:"id"`
	Config      types.String `tfsdk:"config"`
	Item        types.String `tfsdk:"item"`
	Label       types.String `tfsdk:"label"`
	State       types.String `tfsdk:"state"`
}

func NewIntegrationItem() resource.Resource {
	return &integrationItem{}
}

func (*integrationItem) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_integration_item"
}

func (*integrationItem) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attributes := keyAttributes()
	attributes["label"] = schema.StringAttribute{
		MarkdownDescription: `The item's label in P0`,
		Computed:            true,
	}
	attributes["state"] = common.StateAttribute

	resp.Schema = schema.Schema{
		MarkdownDescription: `A P0 install item, for any integration (unsupported).

` + unsupportedWarning + `

Creating this resource stages, verifies, and configures the item, so it works on its own. When something must be
provisioned from the item's staged ` + "`metadata`" + ` before P0 can verify it, create a
` + "`p0_integration_item_staged`" + ` for the same item first, and make this resource depend on what you provision.

Destroying this resource removes the item from P0.

` + limitations,
		Attributes: attributes,
	}
}

func (r *integrationItem) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.data = internal.Configure(&req, resp)
}

func (r *integrationItem) installer(model *integrationItemModel, reconcile bool) *common.Install {
	key := itemKey{Integration: model.Integration, Component: model.Component, Id: model.Id}
	return newInstaller(r.data, key, model.Config, func(_ context.Context, diags *diag.Diagnostics, item map[string]any, _ *itemApi) any {
		updated := *model
		var ok bool
		updated.Config, updated.Item, ok = itemValues(diags, model.Config, item, reconcile)
		if !ok {
			return nil
		}
		updated.Label = stringField(item, "label")
		updated.State = stringField(item, "state")
		return &updated
	})
}

func stringField(item map[string]any, key string) types.String {
	if value, ok := item[key].(string); ok {
		return types.StringValue(value)
	}
	return types.StringNull()
}

func (r *integrationItem) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data integrationItemModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	installer := r.installer(&data, false)
	installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	if resp.Diagnostics.HasError() {
		return
	}

	// Staging first lets this resource stand alone. When a p0_integration_item_staged
	// already staged the item, this only re-assembles it.
	stage(ctx, &resp.Diagnostics, installer, &req.Plan, &resp.State, &data)
	if resp.Diagnostics.HasError() {
		return
	}

	var json itemApi
	installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data)
}

func (r *integrationItem) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data integrationItemModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var json itemApi
	r.installer(&data, true).Read(ctx, &resp.Diagnostics, &resp.State, &json, &data)
}

func (r *integrationItem) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data integrationItemModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var json itemApi
	r.installer(&data, false).UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data)
}

func (r *integrationItem) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data integrationItemModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.installer(&data, false).Delete(ctx, &resp.Diagnostics, &req.State, &data)
}

func (*integrationItem) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importState(ctx, req, resp)
}
