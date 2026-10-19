package installjiraai

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/p0-security/terraform-provider-p0/internal"
	"github.com/p0-security/terraform-provider-p0/internal/common"
	installresources "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install"
)

var _ resource.Resource = &JiraAiPoolAccount{}
var _ resource.ResourceWithConfigure = &JiraAiPoolAccount{}
var _ resource.ResourceWithImportState = &JiraAiPoolAccount{}
var _ resource.ResourceWithModifyPlan = &JiraAiPoolAccount{}

// The Jira (AI) `pool-account` item: one Atlassian service account that P0 leases to
// one agent session at a time on a site
// (packages/integrations/jira-ai/shared/src/components.ts in the app).
type JiraAiPoolAccount struct {
	installer *common.Install
}

const invalidPoolAccountId = "Invalid Jira agent account ID"

type poolAccountModel struct {
	Id         types.String `tfsdk:"id"`
	Site       types.String `tfsdk:"site"`
	Email      types.String `tfsdk:"email"`
	AccountId  types.String `tfsdk:"account_id"`
	SecretName types.String `tfsdk:"secret_name"`
	State      types.String `tfsdk:"state"`
}

// The item, as P0 stores it.
type poolAccountJson struct {
	Site       *string `json:"site,omitempty"`
	Email      *string `json:"email,omitempty"`
	AccountId  *string `json:"accountId,omitempty"`
	SecretName *string `json:"secretName,omitempty"`
	State      *string `json:"state,omitempty"`
}

type poolAccountApi struct {
	Item *poolAccountJson `json:"item"`
}

// The app's message for a missing email (backend/src/install/pool-account.ts), with a
// note about whitespace added, since the provider also refuses a padded value that P0
// would trim.
const invalidEmail = "Enter the service account's email, as Atlassian shows it, with no whitespace around it"

func NewJiraAiPoolAccount() resource.Resource {
	return &JiraAiPoolAccount{}
}

func (*JiraAiPoolAccount) Metadata(_ context.Context, req resource.MetadataRequest, res *resource.MetadataResponse) {
	res.TypeName = req.ProviderTypeName + "_jira_ai_pool_account"
}

func (*JiraAiPoolAccount) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `An agent account: an Atlassian service account that P0 leases to one agent session at a time on a Jira site. When an agent is granted access to a project, P0 has the site's connector add the account to the project's role, and remove it when the access ends. Add one for each agent session the site should serve at once.

P0 never holds the account's API key. When P0 adds the account it names the secret that holds it, ` + "`secret_name`" + `, in the site's vault. Create that secret and store the key in it, outside Terraform, which keeps it out of Terraform's state. Only the connector reads it, under the grant the site's Terraform gives it.

**Important:** Install the site first, in an earlier apply or in the same one with this resource referring to it, as the example does. Creating this resource makes P0 look up the account's Atlassian ID through the site's connector, as the site's admin service account. That also checks the site's admin client. If the check fails, the apply fails with P0's message.

An agent account's attributes can't change once it is created, so changing any of them replaces the account. To rotate the API key, add a new version to the same secret, which needs no change here.

An agent account that already exists in P0, such as one added in the P0 app, isn't created again: the apply fails, and says to import it with ` + "`terraform import`" + `. An imported account that P0 hasn't finished installing plans an update, and applying it finishes the install.

**Prerequisite:** an Atlassian service account that is a member of the site, with an API key, and no project roles of its own: P0 gives it each role an agent is granted.

` + betaNote,
		Attributes: map[string]schema.Attribute{
			"id": fixedAttribute(
				"A name you choose for the account, such as `agent-1`. Use "+itemIdRule+".",
				idValidators(itemIdMaxLength)...,
			),
			"site": fixedAttribute(
				"The `id` of the `p0_jira_ai_site` this account works on. The site must be installed.",
				idValidators(siteIdMaxLength)...,
			),
			"email": fixedAttribute(
				"The service account's email, as Atlassian shows it.",
				trimmedValidator(invalidEmail),
			),
			"account_id": generatedAttribute("Atlassian's ID for the account, which P0 looks up through the site's connector when it checks the install."),
			"secret_name": generatedAttribute(
				"The name of the secret in the site's vault that holds the account's API key. P0 names it when it adds the account: " +
					"`p0_install_jira-ai_<site>_<id>_api-key`. Create it, and store the key as its whole value.",
			),
			"state": common.StateAttribute,
		},
	}
}

func (r *JiraAiPoolAccount) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:        JiraAiKey,
		Component:          installresources.PoolAccount,
		ProviderData:       data,
		GetId:              poolAccountId,
		GetItemJson:        poolAccountItemJson,
		FromJson:           poolAccountFromJson,
		ToJson:             poolAccountToJson,
		DescribeCheckError: describeCheckError("agent account"),
	}
}

func poolAccountId(data any) *string {
	model, ok := data.(*poolAccountModel)
	if !ok {
		return nil
	}
	str := model.Id.ValueString()
	return &str
}

func poolAccountItemJson(json any) any {
	// An answer with no item leaves Item nil, which has to come back as an untyped
	// nil: a nil *poolAccountJson in an any isn't nil, and would reach FromJson.
	inner, ok := json.(*poolAccountApi)
	if !ok || inner.Item == nil {
		return nil
	}
	return inner.Item
}

func poolAccountFromJson(_ context.Context, _ *diag.Diagnostics, id string, json any) any {
	jsonv, ok := json.(*poolAccountJson)
	if !ok {
		return nil
	}

	return &poolAccountModel{
		Id:         types.StringValue(id),
		Site:       types.StringPointerValue(jsonv.Site),
		Email:      types.StringPointerValue(jsonv.Email),
		AccountId:  types.StringPointerValue(jsonv.AccountId),
		SecretName: types.StringPointerValue(jsonv.SecretName),
		State:      types.StringPointerValue(jsonv.State),
	}
}

// The body of every request: the fields the configuration sets. Verify and configure
// get them again, so that P0 refuses to install an account that isn't the configured
// one. P0 names the secret, looks up the account's ID, and sets the state.
func poolAccountToJson(data any) any {
	datav, ok := data.(*poolAccountModel)
	if !ok {
		return nil
	}

	return &poolAccountJson{
		Site:  datav.Site.ValueStringPointer(),
		Email: datav.Email.ValueStringPointer(),
	}
}

func (r *JiraAiPoolAccount) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data poolAccountModel
	createItem(ctx, r.installer, "agent account", req, resp, &poolAccountApi{}, &data)
}

func (r *JiraAiPoolAccount) Read(ctx context.Context, _ resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &poolAccountApi{}, &poolAccountModel{})
}

// Plans an update for an account that P0 hasn't installed, so that the next apply
// finishes the install. That is when P0 looks the account's ID up, so it is unknown
// until then.
func (*JiraAiPoolAccount) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	planUnfinishedInstall(ctx, req, resp, path.Root("account_id"))
}

func (r *JiraAiPoolAccount) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	finishInstall(ctx, r.installer, req, resp, &poolAccountApi{}, &poolAccountModel{})
}

func (r *JiraAiPoolAccount) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Delete(ctx, &resp.Diagnostics, &req.State, &poolAccountModel{})
}

func (r *JiraAiPoolAccount) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importById(ctx, req, resp, invalidPoolAccountId, isItemId, itemIdRule)
}
