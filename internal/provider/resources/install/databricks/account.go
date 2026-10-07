package installdatabricks

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/p0-security/terraform-provider-p0/internal"
	"github.com/p0-security/terraform-provider-p0/internal/common"
	installresources "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install"
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
	resp.TypeName = req.ProviderTypeName + "_databricks_account"
}

func (*Account) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attributes := accountStagedAttributes()
	// The customer supplies the application ID, because P0 can't discover it:
	// the connector needs it to authenticate, as the client ID of its token
	// exchange. The validator and its message match the app's, which accepts
	// exactly the IDs the connector parses.
	attributes["application_id"] = schema.StringAttribute{
		Required:            true,
		MarkdownDescription: "The application ID of the service principal that the connector signs in to Databricks as, e.g. `databricks_service_principal.application_id`. Changing it updates the account in place, and P0 checks the install again. If the check fails, the apply fails, and the account keeps its current application ID.",
		Validators:          []validator.String{uuidValidator("Application IDs")},
	}

	resp.Schema = schema.Schema{
		MarkdownDescription: `A Databricks account installation.

**Important:** Before creating this resource you must stage the account with ` + "`p0_databricks_account_staged`" + `, and create a service principal in the account with the ` + "`account_admin`" + ` role and a federation policy that trusts the connector. The policy's issuer is the outbound identity federation issuer URL of the connector's AWS account, its subject is the ARN of the connector's role, and its audience is ` + "`federation_audience`" + ` (` + "`" + FederationAudience + "`" + `). Creating them takes a Databricks account admin.

An account that P0 hasn't finished installing, such as one imported before its install check passed, plans an update, and applying it finishes the install.

` + common.NotePreview,
		Attributes: attributes,
	}
}

func (r *Account) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.installer = newInstaller[accountJson](internal.Configure(&req, resp), installresources.Account, accountFromJson, accountToJson)
}

func accountFromJson(_ context.Context, _ *diag.Diagnostics, id string, json any) any {
	item, ok := json.(*accountJson)
	if !ok {
		return nil
	}
	return &accountModel{
		accountStagedModel: newAccountStagedModel(id, item),
		ApplicationId:      types.StringPointerValue(item.ApplicationId),
	}
}

func accountToJson(data any) any {
	model, ok := data.(*accountModel)
	if !ok {
		return nil
	}
	json := model.toJson()
	json.ApplicationId = model.ApplicationId.ValueStringPointer()
	return json
}

func (r *Account) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	stageAndInstall(ctx, r.installer, &resp.Diagnostics, &req.Plan, &resp.State, &accountApi{}, &accountModel{})
}

func (r *Account) Read(ctx context.Context, _ resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &accountApi{}, &accountModel{})
}

func (*Account) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	common.PlanFinishingInstall(ctx, req, resp)
}

// Only the application ID changes in place. P0 checks it on the configure step,
// which is all an installed account gets, and saves nothing if the check fails:
// the account stays installed with its current ID, and the next plan shows the
// update again. A staged account is verified first, and a failed verify saves
// nothing either.
func (r *Account) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	upsertFromState(ctx, r.installer, req, resp, &accountApi{}, &accountModel{})
}

// Returns the account to the "stage" state, so that the staged resource
// deletes it.
func (r *Account) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Rollback(ctx, &resp.Diagnostics, &req.State, &accountModel{})
}

func (r *Account) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
