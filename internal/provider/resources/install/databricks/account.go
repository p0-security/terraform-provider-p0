package installdatabricks

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
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
		MarkdownDescription: "The application ID of the service principal that the connector exchanges its AWS identity for, e.g. `databricks_service_principal.application_id`",
		Validators: []validator.String{
			stringvalidator.RegexMatches(common.UuidRegex, "Application IDs are UUIDs, e.g. 01234567-89ab-cdef-0123-456789abcdef"),
		},
	}

	resp.Schema = schema.Schema{
		MarkdownDescription: `A Databricks account installation.

**Important:** Before creating this resource you must stage the account with ` + "`p0_databricks_account_staged`" + `, and create a service principal in the account with the ` + "`account_admin`" + ` role and a federation policy that trusts the connector. The policy's issuer is the issuer of the connector's AWS account, its subject is the ARN of the connector's role, and its audience is ` + "`databricks`" + `. Creating them takes a Databricks account admin.

` + notePreview,
		Attributes: attributes,
	}
}

func (r *Account) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.installer = &common.Install{
		Integration:  DatabricksKey,
		Component:    installresources.Account,
		ProviderData: internal.Configure(&req, resp),
		GetId:        accountId,
		GetItemJson:  accountItemJson,
		FromJson:     accountFromItem,
		ToJson:       accountToJson,
	}
}

func accountId(data any) *string {
	model, ok := data.(*accountModel)
	if !ok {
		return nil
	}
	id := model.Id.ValueString()
	return &id
}

func accountFromItem(_ context.Context, _ *diag.Diagnostics, id string, json any) any {
	item, ok := json.(*accountJson)
	if !ok {
		return nil
	}
	return &accountModel{
		accountStagedModel: accountStagedFromJson(id, item),
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
	install(ctx, r.installer, &resp.Diagnostics, &req.Plan, &resp.State, &accountApi{}, &accountModel{})
}

func (r *Account) Read(ctx context.Context, _ resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &accountApi{}, &accountModel{})
}

// Only the application ID can change in place: P0 accepts a new one at the
// verify and configure steps.
func (r *Account) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &accountApi{}, &accountModel{})
}

// Returns the account to the "stage" state, so that the staged resource
// deletes it.
func (r *Account) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Rollback(ctx, &resp.Diagnostics, &req.State, &accountModel{})
}

func (r *Account) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
