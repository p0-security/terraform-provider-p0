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
var _ resource.Resource = &integrationItemStaged{}
var _ resource.ResourceWithConfigure = &integrationItemStaged{}
var _ resource.ResourceWithImportState = &integrationItemStaged{}

type integrationItemStaged struct {
	data *internal.P0ProviderData
}

type integrationItemStagedModel struct {
	Integration types.String `tfsdk:"integration"`
	Component   types.String `tfsdk:"component"`
	Id          types.String `tfsdk:"id"`
	Config      types.String `tfsdk:"config"`
	Item        types.String `tfsdk:"item"`
	Metadata    types.Map    `tfsdk:"metadata"`
}

func NewIntegrationItemStaged() resource.Resource {
	return &integrationItemStaged{}
}

func (*integrationItemStaged) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_integration_item_staged"
}

func (*integrationItemStaged) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attributes := keyAttributes()
	attributes["metadata"] = schema.MapAttribute{
		MarkdownDescription: `Values P0 computes for the staged item, such as the name and policies of a cloud role P0 expects to
assume. Empty for components that compute none.`,
		ElementType: types.StringType,
		Computed:    true,
	}

	resp.Schema = schema.Schema{
		MarkdownDescription: `A staged P0 install item, for any integration (unsupported).

` + unsupportedWarning + `

Staging registers the item in P0 without verifying it, and exposes the ` + "`metadata`" + ` P0 computes for it. Use
it when you need that metadata to provision something before P0 can verify the item, then complete the install with
a ` + "`p0_integration_item`" + ` for the same item. Items that need nothing provisioned first can skip this resource.

` + limitations,
		Attributes: attributes,
	}
}

func (r *integrationItemStaged) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.data = internal.Configure(&req, resp)
}

func (r *integrationItemStaged) installer(model *integrationItemStagedModel, reconcile bool) *common.Install {
	key := itemKey{Integration: model.Integration, Component: model.Component, Id: model.Id}
	return newInstaller(r.data, key, model.Config, func(ctx context.Context, diags *diag.Diagnostics, item map[string]any, api *itemApi) any {
		updated := *model
		var ok bool
		updated.Config, updated.Item, ok = itemValues(diags, model.Config, item, reconcile)
		if !ok {
			return nil
		}
		updated.Metadata = metadataMap(ctx, diags, api.Metadata)
		return &updated
	})
}

func (r *integrationItemStaged) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data integrationItemStagedModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	installer := r.installer(&data, false)
	installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	if resp.Diagnostics.HasError() {
		return
	}
	stage(ctx, &resp.Diagnostics, installer, &req.Plan, &resp.State, &data)
}

func (r *integrationItemStaged) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data integrationItemStagedModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var json itemApi
	r.installer(&data, true).Read(ctx, &resp.Diagnostics, &resp.State, &json, &data)
}

func (r *integrationItemStaged) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data integrationItemStagedModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	stage(ctx, &resp.Diagnostics, r.installer(&data, false), &req.Plan, &resp.State, &data)
}

func (r *integrationItemStaged) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data integrationItemStagedModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.installer(&data, false).Delete(ctx, &resp.Diagnostics, &req.State, &data)
}

func (*integrationItemStaged) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importState(ctx, req, resp)
}
