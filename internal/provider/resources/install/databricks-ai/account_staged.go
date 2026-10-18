package installdatabricksai

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/p0-security/terraform-provider-p0/internal"
	"github.com/p0-security/terraform-provider-p0/internal/common"
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
	resp.TypeName = req.ProviderTypeName + "_databricks_ai_account_staged"
}

func (*AccountStaged) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A staged Databricks (AI) account. P0 manages a Databricks account through a service principal there that the connector exchanges its AWS identity for.

Staging records the account in P0 and checks its ID and accounts URL before you create the service principal. Then create the service principal and its federation policy, give it the ` + "`account_admin`" + ` role, make it metastore admin of every metastore in the account, and create a ` + "`p0_databricks_ai_account`" + ` resource with the same ` + "`id`" + ` to complete the installation.

P0 also grants the service principal ` + "`MANAGE`" + ` on ` + "`system.ai`" + ` in each metastore, and makes it workspace admin of each workspace an agent lists or requests, and of the lowest-ID workspace when the account is saved. P0 never undoes either.

**Prerequisite:** The connector that reaches the account must be installed (see the ` + "`p0_databricks_ai_connector`" + ` resource).

` + betaNote + `

For the full install steps, see [Databricks MCP servers](https://docs.p0.dev/integrations/resource-integrations/agentic-gateway/mcp-server/databricks).`,
		Attributes: accountStagedAttributes(),
	}
}

func (r *AccountStaged) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.installer = newInstaller[accountJson](internal.Configure(&req, resp), accountComponent, accountStagedFromJson, accountStagedToJson)
}

func accountStagedFromJson(_ context.Context, _ *diag.Diagnostics, id string, json any) any {
	item, ok := json.(*accountJson)
	if !ok {
		return nil
	}
	model := newAccountStagedModel(id, item)
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
	ensureConfigAndStage(ctx, r.installer, &resp.Diagnostics, &req.Plan, &resp.State, &accountApi{}, &accountStagedModel{})
}

func (r *AccountStaged) Read(ctx context.Context, _ resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &accountApi{}, &accountStagedModel{})
}

// Every attribute that can be set requires replacement, so Terraform never
// updates a staged account in place. Staging it again would also return an
// installed account to the "stage" state.
func (r *AccountStaged) Update(context.Context, resource.UpdateRequest, *resource.UpdateResponse) {
}

func (r *AccountStaged) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Delete(ctx, &resp.Diagnostics, &req.State, &accountStagedModel{})
}

func (r *AccountStaged) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importById(ctx, req, resp, accountIdValidator())
}
