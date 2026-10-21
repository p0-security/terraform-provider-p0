package installdatabricksai

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/p0-security/terraform-provider-p0/internal"
	"github.com/p0-security/terraform-provider-p0/internal/common"
)

var _ resource.Resource = &Account{}
var _ resource.ResourceWithConfigure = &Account{}
var _ resource.ResourceWithImportState = &Account{}
var _ resource.ResourceWithModifyPlan = &Account{}

type Account struct {
	installer *common.Install
}

func NewAccount() resource.Resource {
	return &Account{}
}

func (*Account) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_databricks_ai_account"
}

func (*Account) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attributes := accountStagedAttributes()
	// The customer supplies the application ID, because P0 can't discover it:
	// the connector needs it to authenticate, as the client ID of its token
	// exchange. The validator and its message match the app's, which accepts
	// exactly the IDs the connector parses.
	attributes["application_id"] = schema.StringAttribute{
		Required:            true,
		MarkdownDescription: "The application ID of the service principal that the connector signs in to Databricks as, e.g. `databricks_service_principal.application_id`. Changing it updates the account in place, and P0 checks the install again.",
		Validators:          []validator.String{uuidValidator("Application IDs")},
	}
	// P0 trims what it saves, so whitespace around the value would read back as
	// a change. The connector checks that the value names one principal.
	attributes["owner_principal"] = schema.StringAttribute{
		Required:            true,
		MarkdownDescription: "Who owns what an agent creates once its access ends: a group's display name, a user's `userName`, or a service principal's application ID. Before P0 deletes an agent's service principal, it hands each table, view, materialized view, volume and function the agent created to this principal. P0 checks that it names exactly one group, user or service principal in the account. Changing it updates the account in place, and P0 checks the install again.",
		Validators:          []validator.String{noWhitespaceAround()},
	}

	resp.Schema = schema.Schema{
		MarkdownDescription: `A Databricks (AI) account installation.

**Important:** Before creating this resource you must stage the account with ` + "`p0_databricks_ai_account_staged`" + `, and create a service principal in the account with a federation policy that trusts the connector. The policy's issuer is the outbound identity federation issuer URL of the connector's AWS account, its subject is the ARN of the connector's role, and its audience is ` + "`federation_audience`" + ` (` + "`" + FederationAudience + "`" + `). The service principal needs the ` + "`account_admin`" + ` role, and must be metastore admin of every metastore in the account, for example as a member of each metastore's admin group. Creating them takes a Databricks account admin.

P0 also grants the service principal ` + "`MANAGE`" + ` on ` + "`system.ai`" + ` in each metastore, and makes it workspace admin of each workspace an agent lists or requests, and of the lowest-ID workspace when the account is saved. P0 never undoes either.

Every apply that creates or changes this resource has P0 check the install: that the connector can sign in as the service principal, that the principal is account admin and metastore admin, and that ` + "`owner_principal`" + ` names one principal. If a check fails, the apply fails with P0's message. The same step also checks the account's lowest-ID workspace for the Unity Gateway beta.

An account that P0 hasn't finished installing, such as one imported before its install check passed, plans an update, and applying it finishes the install.

` + betaNote + `

For the full install steps, see [Databricks MCP servers](https://docs.p0.dev/integrations/resource-integrations/agentic-gateway/mcp-server/databricks).`,
		Attributes: attributes,
	}
}

func (r *Account) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.installer = newInstaller[accountJson](internal.Configure(&req, resp), accountComponent, accountFromJson, accountToJson)
}

func accountFromJson(_ context.Context, _ *diag.Diagnostics, id string, json any) any {
	item, ok := json.(*accountJson)
	if !ok {
		return nil
	}
	return &accountModel{
		accountStagedModel: newAccountStagedModel(id, item),
		ApplicationId:      types.StringPointerValue(item.ApplicationId),
		OwnerPrincipal:     types.StringPointerValue(item.OwnerPrincipal),
	}
}

func accountToJson(data any) any {
	model, ok := data.(*accountModel)
	if !ok {
		return nil
	}
	json := model.toJson()
	json.ApplicationId = model.ApplicationId.ValueStringPointer()
	json.OwnerPrincipal = model.OwnerPrincipal.ValueStringPointer()
	return json
}

func (r *Account) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	stageAndInstall(ctx, r.installer, &resp.Diagnostics, &req.Plan, &resp.State, &accountApi{}, &accountModel{})
}

func (r *Account) Read(ctx context.Context, _ resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &accountApi{}, &accountModel{})
}

func (*Account) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	planFinishingInstall(ctx, req, resp)
}

// Only application_id and owner_principal change in place. P0 checks them on
// the configure step, which is all an installed account gets, and saves nothing
// if the check fails: the account stays installed with its current values, and
// the next plan shows the update again. A staged account is verified first, and
// a failed verify saves nothing either.
func (r *Account) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	updateFromInstallState(ctx, r.installer, req, resp, &accountApi{}, &accountModel{})
}

// Returns the account to the "stage" state, so that the staged resource
// deletes it.
func (r *Account) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Rollback(ctx, &resp.Diagnostics, &req.State, &accountModel{})
}

func (r *Account) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importById(ctx, req, resp, accountIdValidator())
}
