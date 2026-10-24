package installjiraai

import (
	"context"
	"fmt"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/p0-security/terraform-provider-p0/internal"
	"github.com/p0-security/terraform-provider-p0/internal/common"
	installresources "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install"
	installvaultedcredential "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/vaulted-credential"
)

var _ resource.Resource = &JiraAiSite{}
var _ resource.ResourceWithConfigure = &JiraAiSite{}
var _ resource.ResourceWithImportState = &JiraAiSite{}
var _ resource.ResourceWithModifyPlan = &JiraAiSite{}

// The Jira (AI) `site` item: a Jira Cloud site, the connector that reaches it, and the
// admin service account's OAuth client the connector manages project roles with
// (packages/integrations/jira-ai/shared/src/components.ts in the app).
type JiraAiSite struct {
	installer *common.Install
}

const invalidSiteId = "Invalid Jira site ID"

type siteModel struct {
	Id              types.String `tfsdk:"id"`
	Connector       types.String `tfsdk:"connector"`
	SiteUrl         types.String `tfsdk:"site_url"`
	CloudId         types.String `tfsdk:"cloud_id"`
	Vault           *vaultModel  `tfsdk:"vault"`
	AdminClientId   types.String `tfsdk:"admin_client_id"`
	AdminSecretName types.String `tfsdk:"admin_secret_name"`
	State           types.String `tfsdk:"state"`
}

// The item, as P0 stores it.
type siteJson struct {
	Connector       *string    `json:"connector,omitempty"`
	SiteUrl         *string    `json:"siteUrl,omitempty"`
	CloudId         *string    `json:"cloudId,omitempty"`
	Vault           *vaultJson `json:"vault,omitempty"`
	AdminClientId   *string    `json:"adminClientId,omitempty"`
	AdminSecretName *string    `json:"adminSecretName,omitempty"`
	State           *string    `json:"state,omitempty"`
}

type siteApi struct {
	Item *siteJson `json:"item"`
}

// The app's rules for a site's fields (site-url.ts, secret-name.ts and
// backend/src/install/site.ts). The site URL and secret name messages are the app's.
// The client ID message is the app's with a note about whitespace added,
// since the provider also refuses a padded value that P0 would trim. The app matches the
// site URL in any case.
var (
	siteUrlRegex       = regexp.MustCompile(`(?i)^https://[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.atlassian\.net/?$`)
	gcpSecretNameRegex = regexp.MustCompile(`^[A-Za-z0-9_-]{1,255}$`)
)

const (
	invalidSiteUrl       = "Enter the site's address as https://<your-site>.atlassian.net, with no path"
	invalidGcpSecretName = "That isn't a secret's name in Google Secret Manager. Enter the name alone, without its project or a version: " +
		"letters, digits, underscores and hyphens, up to 255 characters"
	invalidAdminClientId = "Enter the admin OAuth client's ID, with no whitespace around it"
)

func NewJiraAiSite() resource.Resource {
	return &JiraAiSite{}
}

func (*JiraAiSite) Metadata(_ context.Context, req resource.MetadataRequest, res *resource.MetadataResponse) {
	res.TypeName = req.ProviderTypeName + "_jira_ai_site"
}

func (*JiraAiSite) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A Jira Cloud site whose projects P0 can give agents access to.

P0 reaches the site through a Jira connector, which you install with ` + "`p0_jira_ai_connector`" + `. The connector manages project roles as an Atlassian service account that administers the site's projects, with that account's OAuth client. The client's secret stays in your own Google Secret Manager: P0 never reads it, and only the connector does. When an agent is granted access to a project, P0 has the connector add one of the site's agent accounts (` + "`p0_jira_ai_pool_account`" + `) to a project role, and remove it when the access ends. In a company-managed project that is one of P0's own site roles, ` + "`P0 agent: read`" + ` or ` + "`P0 agent: write`" + `, which the connector gives its permissions in the project's permission scheme. In a team-managed project it is the project's own role that the request names.

**Important:** Install the connector first: P0 adds a site only for a connector that is installed. Creating this resource has P0 look up the site's cloud ID from Atlassian, then check through the connector that the admin service account signs in and holds Administer Jira. The same check creates P0's two site roles, marked with your P0 organization.

Only one P0 organization can use a site: if another P0 organization already uses it, the apply fails. The apply also fails when the site has a role named ` + "`P0 agent: read`" + ` or ` + "`P0 agent: write`" + ` that P0 didn't make for your organization, until a Jira admin renames or deletes it.

A site's attributes can't change once it is created, so changing any of them replaces the site. To rotate the admin client's secret, add a new version to the same secret, which needs no change here.

**Warning:** Replacing or destroying this resource removes the site from P0. P0 still revokes the site's open access afterwards, through the site's connector, its grants in the vault project and its admin secret, so keep all three in place until that access has ended.

A site that already exists in P0, such as one added in the P0 app, isn't created again: the apply fails, and says to import it with ` + "`terraform import`" + `. An imported site that P0 hasn't finished installing plans an update, and applying it finishes the install.

**Prerequisites:**

- An Atlassian service account with Administer Jira, the global Jira admin permission, and an OAuth client. Store the client's secret, as the secret's whole value, in the Google Cloud project that ` + "`vault`" + ` names, outside Terraform, which keeps it out of Terraform's state.

- Google Secret Manager installed in P0 for that project. This provider has no resource for it, so install it in the P0 app.

- Grants in that project for the connector's service account, ` + "`hosting.connector_service_account`" + ` of ` + "`p0_jira_ai_connector`" + `: read on the admin client's secret and on the site's agent account secrets, which start with ` + "`p0_install_jira-ai_<id>_`" + `, and create, read, delete and add versions to the site's token secrets, which start with ` + "`p0_access-token_jira-ai_<id>_`" + `. P0's Jira installer generates them, as in the example.

` + betaNote,
		Attributes: map[string]schema.Attribute{
			"id": fixedAttribute(
				"A name you choose for the site, such as `acme`. Agent accounts refer to the site by it, and a P0 policy's `site` filter matches it. Use "+siteIdRule+".",
				idValidators(siteIdMaxLength)...,
			),
			"connector": fixedAttribute(
				"The `id` of the `p0_jira_ai_connector` that reaches this site. The connector must be installed.",
				idValidators(itemIdMaxLength)...,
			),
			"site_url": fixedAttribute(
				"The address of your Jira Cloud site, such as `https://acme.atlassian.net`, with no path.",
				stringvalidator.RegexMatches(siteUrlRegex, invalidSiteUrl),
			),
			"cloud_id": generatedAttribute("Atlassian's ID for the site, which P0 looks up from the site's address when it adds the site."),
			"vault":    vaultAttribute(),
			"admin_client_id": fixedAttribute(
				"The client ID of the OAuth client of an Atlassian service account that administers the site's projects.",
				trimmedValidator(invalidAdminClientId),
			),
			"admin_secret_name": fixedAttribute(
				"The name of the secret, in `vault`, that holds the admin client's secret as its whole value: the name alone, without its project or a version. "+
					"Up to 255 letters, digits, underscores and hyphens. P0 never reads it. Only your connector does.",
				stringvalidator.RegexMatches(gcpSecretNameRegex, invalidGcpSecretName),
			),
			"state": common.StateAttribute,
		},
	}
}

func (r *JiraAiSite) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:        JiraAiKey,
		Component:          installresources.Site,
		ProviderData:       data,
		GetId:              siteId,
		GetItemJson:        siteItemJson,
		FromJson:           siteFromJson,
		ToJson:             siteToJson,
		DescribeCheckError: describeCheckError("site"),
	}
}

func siteId(data any) *string {
	model, ok := data.(*siteModel)
	if !ok {
		return nil
	}
	str := model.Id.ValueString()
	return &str
}

func siteItemJson(json any) any {
	// An answer with no item leaves Item nil, which has to come back as an untyped
	// nil: a nil *siteJson in an any isn't nil, and would reach FromJson.
	inner, ok := json.(*siteApi)
	if !ok || inner.Item == nil {
		return nil
	}
	return inner.Item
}

func siteFromJson(_ context.Context, diags *diag.Diagnostics, id string, json any) any {
	jsonv, ok := json.(*siteJson)
	if !ok {
		return nil
	}

	// vault is Required, so leaving it nil here would surface as Terraform core's
	// "provider produced inconsistent result after apply" rather than something the
	// reader can act on.
	if jsonv.Vault == nil {
		diags.AddError(
			"Bad API response",
			fmt.Sprintf("P0 returned the Jira site %q without its vault. "+
				"Remove the site in the P0 app and apply again, or remove it from Terraform state with `terraform state rm <address>`.", id),
		)
		return nil
	}

	vault := vaultFromJson(jsonv.Vault)
	if vault == nil {
		diags.AddError(
			"Unsupported Jira site vault",
			fmt.Sprintf("The Jira site %s has a %q vault. This provider supports only %q. "+
				"Manage it in the P0 app, or remove it from Terraform state with `terraform state rm`.",
				id, jsonv.Vault.Type, installvaultedcredential.GcpSecretManager),
		)
		return nil
	}

	return &siteModel{
		Id:              types.StringValue(id),
		Connector:       types.StringPointerValue(jsonv.Connector),
		SiteUrl:         types.StringPointerValue(jsonv.SiteUrl),
		CloudId:         types.StringPointerValue(jsonv.CloudId),
		Vault:           vault,
		AdminClientId:   types.StringPointerValue(jsonv.AdminClientId),
		AdminSecretName: types.StringPointerValue(jsonv.AdminSecretName),
		State:           types.StringPointerValue(jsonv.State),
	}
}

// The body of every request: every field the site's configuration sets. The PUT that
// stages the site needs them all. Verify and configure get them again, so that P0,
// which compares each with the item it stored, refuses to install a site that isn't
// the configured one. P0 looks up the cloud ID, and sets the state.
func siteToJson(data any) any {
	datav, ok := data.(*siteModel)
	if !ok {
		return nil
	}

	json := siteJson{
		Connector:       datav.Connector.ValueStringPointer(),
		SiteUrl:         datav.SiteUrl.ValueStringPointer(),
		AdminClientId:   datav.AdminClientId.ValueStringPointer(),
		AdminSecretName: datav.AdminSecretName.ValueStringPointer(),
	}
	if datav.Vault != nil {
		json.Vault = datav.Vault.toJson()
	}
	return &json
}

func (r *JiraAiSite) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data siteModel
	createItem(ctx, r.installer, "site", req, resp, &siteApi{}, &data)
}

func (r *JiraAiSite) Read(ctx context.Context, _ resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &siteApi{}, &siteModel{})
}

// Plans an update for a site that P0 hasn't installed, so that the next apply
// finishes the install. P0 looks the cloud ID up when it stages the site, so it is
// known already.
func (*JiraAiSite) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	planUnfinishedInstall(ctx, req, resp)
}

func (r *JiraAiSite) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	finishInstall(ctx, r.installer, req, resp, &siteApi{}, &siteModel{})
}

func (r *JiraAiSite) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Delete(ctx, &resp.Diagnostics, &req.State, &siteModel{})
}

func (r *JiraAiSite) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importById(ctx, req, resp, invalidSiteId, isSiteId, siteIdRule)
}
