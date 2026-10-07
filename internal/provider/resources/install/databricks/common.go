package installdatabricks

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf16"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/p0-security/terraform-provider-p0/internal"
	"github.com/p0-security/terraform-provider-p0/internal/common"
	installresources "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install"
	installaws "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/aws"
)

const DatabricksKey = "databricks"

// The audience of the AWS identity tokens that the connector requests
// (`DATABRICKS_TOKEN_AUDIENCE` in p0-connector-databricks). The connector's
// role may only request tokens for it, and every Databricks federation policy
// that trusts the connector must accept it.
const FederationAudience = "databricks"

// All installable Databricks components. Each one points at the component
// before it: an account at its connector, a workspace at its account, and a
// catalog at its workspace.
var Components = []string{
	installresources.Connector,
	installresources.Account,
	installresources.Workspace,
	installresources.Catalog,
}

// The Databricks account consoles, one per cloud that Databricks runs on. The
// connector only accepts these, because it sends its AWS identity token to the
// console that an account names.
var accountsConsoles = []struct{ url, cloud string }{
	{url: "https://accounts.cloud.databricks.com", cloud: "AWS"},
	{url: "https://accounts.cloud.databricks.us", cloud: "AWS GovCloud"},
	{url: "https://accounts.azuredatabricks.net", cloud: "Azure"},
	{url: "https://accounts.gcp.databricks.com", cloud: "Google Cloud"},
}

func accountsUrls() []string {
	urls := make([]string, len(accountsConsoles))
	for i, console := range accountsConsoles {
		urls[i] = console.url
	}
	return urls
}

// The connector refuses a workspace ID that isn't all digits.
var WorkspaceIdRegex = regexp.MustCompile(`^\d+$`)

// The most characters that Unity Catalog allows in a name (`UC_NAME_MAX_LENGTH`
// in the app).
const catalogNameMaxLength = 255

// Why name can't be a catalog's name, or "" if it can. This is the app's
// `catalogNameError` (packages/integrations/databricks/src/shared/components.ts),
// with its messages, so that Terraform refuses the names that P0's installer
// refuses, in the same words. Like the app, it counts a name's length in UTF-16
// code units, as JavaScript does.
//
// The period that Unity Catalog forbids is also what the connector splits
// securable names on. P0's install API carries item keys in its URL paths
// unescaped, so a catalog named `sales#eu` would install `sales`.
func catalogNameError(name string) string {
	lowercase := strings.ToLower(name)
	switch {
	case name == "":
		return "Enter the catalog's name"
	case len(utf16.Encode([]rune(name))) > catalogNameMaxLength:
		return fmt.Sprintf("Catalog names are at most %d characters", catalogNameMaxLength)
	case strings.ContainsAny(name, ". /") || strings.ContainsFunc(name, isControlCharacter):
		return "Catalog names can't contain a period, a space, a forward slash or a control character"
	case name != lowercase:
		return "Unity Catalog stores catalog names in lowercase, so enter " + lowercase
	case strings.ContainsAny(name, `#?%\`):
		return `P0 can't install a catalog whose name contains #, ?, % or \`
	}
	return ""
}

// An ASCII control character, as the app's catalog name check counts them.
func isControlCharacter(r rune) bool {
	return r < 0x20 || r == 0x7f
}

type catalogNameValidator struct{}

func (catalogNameValidator) Description(context.Context) string {
	return "value must be a Unity Catalog catalog name, in lowercase"
}

func (v catalogNameValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (catalogNameValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if message := catalogNameError(req.ConfigValue.ValueString()); message != "" {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid catalog name", message)
	}
}

// A catalog's item key. Catalog names are unique only within a metastore, so
// the key names the workspace that P0 reaches the catalog through too.
func catalogKey(catalogName, workspaceId string) string {
	return catalogName + "@" + workspaceId
}

// Splits a catalog's item key into its catalog name and workspace ID. The
// workspace ID is all digits, so the last "@" separates them.
func parseCatalogKey(key string) (catalogName, workspaceId string, ok bool) {
	at := strings.LastIndex(key, "@")
	if at < 0 {
		return "", "", false
	}
	catalogName, workspaceId = key[:at], key[at+1:]
	if catalogName == "" || !WorkspaceIdRegex.MatchString(workspaceId) {
		return "", "", false
	}
	return catalogName, workspaceId, true
}

// Rejects a value that isn't a UUID, naming what it is, as the app's
// `uuidValidator` does.
func uuidValidator(label string) validator.String {
	return stringvalidator.RegexMatches(common.UuidRegex, label+" are UUIDs, e.g. 01234567-89ab-cdef-0123-456789abcdef")
}

func workspaceIdValidator() validator.String {
	return stringvalidator.RegexMatches(WorkspaceIdRegex, "Databricks workspace IDs are numeric")
}

// P0 never stores the audience: it is the same for every connector.
var federationAudienceAttribute = schema.StringAttribute{
	Computed:            true,
	Default:             stringdefault.StaticString(FederationAudience),
	MarkdownDescription: "The audience of the AWS identity tokens that the connector exchanges for Databricks tokens. The connector's role may request tokens only for this audience, and the federation policy of each account's service principal must accept it.",
}

// A model that names its item's key.
type keyed interface {
	key() string
}

// GetId for every Databricks model.
func getId(data any) *string {
	model, ok := data.(keyed)
	if !ok {
		return nil
	}
	key := model.key()
	return &key
}

// The install API's response for one item.
type itemApi[T any] struct {
	Item *T `json:"item"`
}

// GetItemJson for every Databricks component.
func getItemJson[T any](json any) any {
	api, ok := json.(*itemApi[T])
	if !ok || api.Item == nil {
		return nil
	}
	return api.Item
}

// The installer of a Databricks component's resources, whose item's JSON is T.
func newInstaller[T any](data *internal.P0ProviderData, component string, fromJson func(context.Context, *diag.Diagnostics, string, any) any, toJson func(any) any) *common.Install {
	return &common.Install{
		Integration:        DatabricksKey,
		Component:          component,
		ProviderData:       data,
		GetId:              getId,
		GetItemJson:        getItemJson[T],
		FromJson:           fromJson,
		ToJson:             toJson,
		DescribeCheckError: describeCheckError(component),
	}
}

// P0 runs every Databricks install check through the customer's connector, so a
// failed check usually means a problem in their AWS or Databricks setup, which
// P0's message describes. The summary doesn't presume where the problem is, and
// leaves that to P0's message.
func describeCheckError(component string) func(id string, err error) (string, string) {
	return func(id string, err error) (string, string) {
		return "Databricks install check failed",
			fmt.Sprintf("P0 rejected the install check for the Databricks %s %q:\n\n%s", component, id, err)
	}
}

func connectorAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"id": common.FixedAttribute(
			`The ID of the AWS account that the connector's Lambda runs in`,
			installaws.AwsAccountIdValidator(),
		),
		"region": common.FixedAttribute(
			`The AWS region that the connector's Lambda runs in. The connector runs only in commercial AWS regions.`,
			installaws.CommercialRegionValidator("The connector runs only in commercial AWS regions, e.g. us-east-1"),
		),
		"domain_pattern": common.FixedAttribute(
			`A regular expression that the connector matches against the whole email domain of every user it grants to, e.g. `+"`example\\.com`"+`. The connector refuses any other user. Set the same value as the Lambda's `+"`DOMAIN_PATTERN`"+` environment variable.`,
			stringvalidator.LengthAtLeast(1),
			common.NoWhitespaceAround(),
		),
		"federation_audience": federationAudienceAttribute,
		"state":               common.StateAttribute,
	}
}

type connectorModel struct {
	Id                 types.String `tfsdk:"id"`
	Region             types.String `tfsdk:"region"`
	DomainPattern      types.String `tfsdk:"domain_pattern"`
	FederationAudience types.String `tfsdk:"federation_audience"`
	State              types.String `tfsdk:"state"`
}

func (m connectorModel) key() string {
	return m.Id.ValueString()
}

type connectorJson struct {
	Region        *string `json:"region,omitempty"`
	DomainPattern *string `json:"domainPattern,omitempty"`
	State         *string `json:"state,omitempty"`
}

type connectorApi = itemApi[connectorJson]

// The staged and installed connector resources share one item, and so one
// model.
func newConnectorInstaller(data *internal.P0ProviderData) *common.Install {
	return newInstaller[connectorJson](data, installresources.Connector, connectorFromJson, connectorToJson)
}

func connectorFromJson(_ context.Context, _ *diag.Diagnostics, id string, json any) any {
	item, ok := json.(*connectorJson)
	if !ok {
		return nil
	}
	return &connectorModel{
		Id:                 types.StringValue(id),
		Region:             types.StringPointerValue(item.Region),
		DomainPattern:      types.StringPointerValue(item.DomainPattern),
		FederationAudience: types.StringValue(FederationAudience),
		State:              types.StringPointerValue(item.State),
	}
}

// P0 sets the state, so it is never sent.
func connectorToJson(data any) any {
	model, ok := data.(*connectorModel)
	if !ok {
		return nil
	}
	return &connectorJson{
		Region:        model.Region.ValueStringPointer(),
		DomainPattern: model.DomainPattern.ValueStringPointer(),
	}
}

func accountsUrlDescription() string {
	options := make([]string, len(accountsConsoles))
	for i, console := range accountsConsoles {
		options[i] = fmt.Sprintf("`%s` (%s)", console.url, console.cloud)
	}
	last := len(options) - 1
	return "The URL of this account's console: " + strings.Join(options[:last], ", ") + " or " + options[last]
}

func accountStagedAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"id": common.FixedAttribute(
			`The Databricks account ID`,
			uuidValidator("Databricks account IDs"),
		),
		"connector": common.FixedAttribute(
			"The `id` of the `p0_databricks_connector` that reaches this account, which is the ID of the AWS account that the connector runs in",
			installaws.AwsAccountIdValidator(),
		),
		"accounts_url":        common.FixedAttribute(accountsUrlDescription(), stringvalidator.OneOf(accountsUrls()...)),
		"federation_audience": federationAudienceAttribute,
		"state":               common.StateAttribute,
	}
}

type accountStagedModel struct {
	Id                 types.String `tfsdk:"id"`
	Connector          types.String `tfsdk:"connector"`
	AccountsUrl        types.String `tfsdk:"accounts_url"`
	FederationAudience types.String `tfsdk:"federation_audience"`
	State              types.String `tfsdk:"state"`
}

func (m accountStagedModel) key() string {
	return m.Id.ValueString()
}

type accountModel struct {
	accountStagedModel
	ApplicationId types.String `tfsdk:"application_id"`
}

type accountJson struct {
	Connector     *string `json:"connector,omitempty"`
	AccountsUrl   *string `json:"accountsUrl,omitempty"`
	ApplicationId *string `json:"applicationId,omitempty"`
	State         *string `json:"state,omitempty"`
}

type accountApi = itemApi[accountJson]

func newAccountStagedModel(id string, item *accountJson) accountStagedModel {
	return accountStagedModel{
		Id:                 types.StringValue(id),
		Connector:          types.StringPointerValue(item.Connector),
		AccountsUrl:        types.StringPointerValue(item.AccountsUrl),
		FederationAudience: types.StringValue(FederationAudience),
		State:              types.StringPointerValue(item.State),
	}
}

// P0 sets the state, so it is never sent.
func (m *accountStagedModel) toJson() *accountJson {
	return &accountJson{
		Connector:   m.Connector.ValueStringPointer(),
		AccountsUrl: m.AccountsUrl.ValueStringPointer(),
	}
}

// Creates the integration in P0 if it doesn't exist yet, then stages the item.
func ensureConfigAndStage(ctx context.Context, installer *common.Install, diags *diag.Diagnostics, plan *tfsdk.Plan, state *tfsdk.State, api any, model any) {
	installer.EnsureConfig(ctx, diags, plan, state, model)
	if diags.HasError() {
		return
	}
	installer.Stage(ctx, diags, plan, state, api, model, installer.ToJson(model))
}

// Stages the item, then verifies and configures it.
func stageAndInstall(ctx context.Context, installer *common.Install, diags *diag.Diagnostics, plan *tfsdk.Plan, state *tfsdk.State, api any, model any) {
	ensureConfigAndStage(ctx, installer, diags, plan, state, api, model)
	if diags.HasError() {
		return
	}
	installer.UpsertFromStage(ctx, diags, plan, state, api, model)
}

// Update for every installed resource: it applies the plan from the step that P0
// has the item at, as RepositoryAccess.Update does. Only an account's
// application ID changes in place. Every other attribute requires replacement,
// so an update otherwise finishes an install that P0 hasn't (see
// common.PlanFinishingInstall).
//
// An item that P0 has verified, at configure or installed, gets the configure
// step alone. The account checks everything there, and P0 saves nothing when a
// step fails, so a new application ID that fails P0's check leaves the account
// installed with its current one. The connector, workspace and catalog check
// on verify, which a verified item passed with the values it still has. Any
// other item, such as a staged one, is verified first, as Create does.
func upsertFromState(ctx context.Context, installer *common.Install, req resource.UpdateRequest, resp *resource.UpdateResponse, api any, model any) {
	var state types.String
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("state"), &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	switch state.ValueString() {
	case common.StateConfigure, common.StateInstalled:
		installer.UpsertFromConfigure(ctx, &resp.Diagnostics, &req.Plan, &resp.State, api, model)
	default:
		installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, api, model)
	}
}
