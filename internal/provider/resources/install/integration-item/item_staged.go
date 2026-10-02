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
	itemFields
	Metadata types.Map `tfsdk:"metadata"`
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

Updating this resource stages the item again; if it was installed, it is then verified and installed again, so the
update fails if P0 can no longer verify it.

` + limitations,
		Attributes: attributes,
	}
}

func (r *integrationItemStaged) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.data = internal.Configure(&req, resp)
}

func (r *integrationItemStaged) installer(model *integrationItemStagedModel, reconcile bool) *common.Install {
	return newInstaller(r.data, model.itemFields, reconcile, func(ctx context.Context, diags *diag.Diagnostics, fields itemFields, _ map[string]any, api *itemApi) any {
		return &integrationItemStagedModel{
			itemFields: fields,
			Metadata:   toMetadataMap(ctx, diags, api.Metadata),
		}
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
	installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &itemApi{}, &data, installer.ToJson(&data))
}

func (r *integrationItemStaged) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data integrationItemStagedModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.installer(&data, true).Read(ctx, &resp.Diagnostics, &resp.State, &itemApi{}, &data)
}

func (r *integrationItemStaged) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var prior, data integrationItemStagedModel
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	installer := r.installer(&data, false)
	installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &itemApi{}, &data, installer.ToJson(&data))
	if resp.Diagnostics.HasError() {
		return
	}

	// Re-staging leaves the item uninstalled. The p0_integration_item that installed it
	// was planned before this update, so it would not notice until the next plan;
	// install the item again here instead.
	if isInstalled(prior.Item) {
		installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &itemApi{}, &data)
	}
}

// Whether a stored `item` attribute records the item as installed in P0.
func isInstalled(item types.String) bool {
	if item.IsNull() || item.IsUnknown() {
		return false
	}
	parsed, err := parseObject([]byte(item.ValueString()))
	return err == nil && parsed["state"] == "installed"
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
