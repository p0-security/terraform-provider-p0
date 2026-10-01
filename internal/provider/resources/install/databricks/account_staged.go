package installdatabricks

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/p0-security/terraform-provider-p0/internal"
	"github.com/p0-security/terraform-provider-p0/internal/common"
	installresources "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install"
)

var _ resource.Resource = &AccountStaged{}
var _ resource.ResourceWithConfigure = &AccountStaged{}
var _ resource.ResourceWithImportState = &AccountStaged{}

type AccountStaged struct {
	installer *common.Install
}

func NewAccountStaged() resource.Resource {
	return &AccountStaged{}
}

func (*AccountStaged) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_databricks_account_staged"
}

func (*AccountStaged) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A staged Databricks account. P0 manages a Databricks account through a service principal there that holds account admin, and that the connector exchanges its AWS identity for.

Staging records the account in P0 and checks its ID and accounts URL before you create the service principal. Then create the service principal, its federation policy and its account admin role, and create a ` + "`p0_databricks_account`" + ` resource with the same ` + "`id`" + ` to complete the installation.

**Prerequisite:** The connector that reaches the account must be installed (see the ` + "`p0_databricks_connector`" + ` resource).

` + notePreview,
		Attributes: accountStagedAttributes(),
	}
}

func (r *AccountStaged) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.installer = &common.Install{
		Integration:  DatabricksKey,
		Component:    installresources.Account,
		ProviderData: internal.Configure(&req, resp),
		GetId:        accountStagedId,
		GetItemJson:  accountItemJson,
		FromJson:     accountStagedFromItem,
		ToJson:       accountStagedToJson,
	}
}

func accountStagedId(data any) *string {
	model, ok := data.(*accountStagedModel)
	if !ok {
		return nil
	}
	id := model.Id.ValueString()
	return &id
}

func accountStagedFromItem(_ context.Context, _ *diag.Diagnostics, id string, json any) any {
	item, ok := json.(*accountJson)
	if !ok {
		return nil
	}
	model := accountStagedFromJson(id, item)
	return &model
}

func accountStagedToJson(data any) any {
	model, ok := data.(*accountStagedModel)
	if !ok {
		return nil
	}
	return model.toJson()
}

func (r *AccountStaged) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	stage(ctx, r.installer, &resp.Diagnostics, &req.Plan, &resp.State, &accountApi{}, &accountStagedModel{})
}

func (r *AccountStaged) Read(ctx context.Context, _ resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &accountApi{}, &accountStagedModel{})
}

func (r *AccountStaged) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	stage(ctx, r.installer, &resp.Diagnostics, &req.Plan, &resp.State, &accountApi{}, &accountStagedModel{})
}

func (r *AccountStaged) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Delete(ctx, &resp.Diagnostics, &req.State, &accountStagedModel{})
}

func (r *AccountStaged) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
