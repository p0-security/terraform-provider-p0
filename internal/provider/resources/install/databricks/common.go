package installdatabricks

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
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

// The state of an item that P0 has finished installing.
const installedState = "installed"

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
	{"https://accounts.cloud.databricks.com", "AWS"},
	{"https://accounts.cloud.databricks.us", "AWS GovCloud"},
	{"https://accounts.azuredatabricks.net", "Azure"},
	{"https://accounts.gcp.databricks.com", "Google Cloud"},
}

func accountsUrls() []string {
	urls := make([]string, len(accountsConsoles))
	for i, console := range accountsConsoles {
		urls[i] = console.url
	}
	return urls
}

// The connector builds its ARNs in the `aws` partition, so it runs only in
// commercial AWS regions: not GovCloud (`us-gov-*`), China (`cn-*`) or the
// isolated regions.
var CommercialRegionRegex = regexp.MustCompile(`^(?:af|ap|ca|eu|il|me|mx|sa|us)-(?:central|east|north|northeast|northwest|south|southeast|southwest|west)-\d+$`)

// The connector refuses a workspace ID that isn't all digits.
var WorkspaceIdRegex = regexp.MustCompile(`^\d+$`)

// Unity Catalog names exclude periods, spaces, forward slashes and ASCII
// control characters, and have at most 255 characters.
var CatalogNameRegex = regexp.MustCompile(`^[^. /\x00-\x1f\x7f]{1,255}$`)

// Unity Catalog stores catalog names in lowercase.
var lowercaseRegex = regexp.MustCompile(`^[^\p{Lu}\p{Lt}]*$`)

// P0's install API carries item keys in its URL paths unescaped, so a catalog
// named `sales#eu` would install `sales`.
var urlPathSafeRegex = regexp.MustCompile(`^[^#?%\\]*$`)

// The validators and their messages match the app's, which rejects the same
// catalog names.
func catalogNameValidators() []validator.String {
	return []validator.String{
		stringvalidator.RegexMatches(CatalogNameRegex, "Catalog names have at most 255 characters, and no periods, spaces, forward slashes or control characters"),
		stringvalidator.RegexMatches(lowercaseRegex, "Unity Catalog stores catalog names in lowercase, so enter the name in lowercase"),
		stringvalidator.RegexMatches(urlPathSafeRegex, `P0 can't install a catalog whose name contains #, ?, % or \`),
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

const notePreview = `**Note:** This integration is currently in preview.`

func awsAccountIdValidator() validator.String {
	return stringvalidator.RegexMatches(installaws.AwsAccountIdRegex, "AWS account IDs should consist of 12 numeric digits")
}

func databricksAccountIdValidator() validator.String {
	return stringvalidator.RegexMatches(common.UuidRegex, "Databricks account IDs are UUIDs, e.g. 01234567-89ab-cdef-0123-456789abcdef")
}

func workspaceIdValidator() validator.String {
	return stringvalidator.RegexMatches(WorkspaceIdRegex, "Databricks workspace IDs are numeric")
}

// An item's key, and every field marked `step: "new"` in the app's install
// schema, are fixed once the item exists: a new key names a different item, and
// P0 rejects a change to a `step: "new"` field at the verify and configure
// steps. RequiresReplace makes Terraform plan the destroy-and-create that
// works, instead of an in-place update.
func fixedAttribute(description string, validators ...validator.String) schema.StringAttribute {
	return schema.StringAttribute{
		Required:            true,
		MarkdownDescription: description,
		Validators:          validators,
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.RequiresReplace(),
		},
	}
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
func itemKey(data any) *string {
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
func itemJson[T any](json any) any {
	api, ok := json.(*itemApi[T])
	if !ok || api.Item == nil {
		return nil
	}
	return api.Item
}

func connectorAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"id": fixedAttribute(
			`The ID of the AWS account that the connector's Lambda runs in`,
			awsAccountIdValidator(),
		),
		"region": fixedAttribute(
			`The AWS region that the connector's Lambda runs in. The connector runs only in commercial AWS regions.`,
			stringvalidator.RegexMatches(CommercialRegionRegex, "The connector runs only in commercial AWS regions, e.g. us-east-1"),
		),
		"domain_pattern": fixedAttribute(
			`A regular expression that the connector matches against the whole email domain of every user it grants to, e.g. `+"`example\\.com`"+`. The connector refuses any other user. Set the same value as the Lambda's `+"`DOMAIN_PATTERN`"+` environment variable.`,
			stringvalidator.LengthAtLeast(1),
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
	return &common.Install{
		Integration:  DatabricksKey,
		Component:    installresources.Connector,
		ProviderData: data,
		GetId:        itemKey,
		GetItemJson:  itemJson[connectorJson],
		FromJson:     connectorFromJson,
		ToJson:       connectorToJson,
	}
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
		"id": fixedAttribute(
			`The Databricks account ID`,
			databricksAccountIdValidator(),
		),
		"connector": fixedAttribute(
			"The `id` of the `p0_databricks_connector` that reaches this account, which is the ID of the AWS account that the connector runs in",
			awsAccountIdValidator(),
		),
		"accounts_url":        fixedAttribute(accountsUrlDescription(), stringvalidator.OneOf(accountsUrls()...)),
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
func stage(ctx context.Context, installer *common.Install, diags *diag.Diagnostics, plan *tfsdk.Plan, state *tfsdk.State, api any, model any) {
	installer.EnsureConfig(ctx, diags, plan, state, model)
	if diags.HasError() {
		return
	}
	installer.Stage(ctx, diags, plan, state, api, model, installer.ToJson(model))
}

// Stages the item, then verifies and configures it.
func install(ctx context.Context, installer *common.Install, diags *diag.Diagnostics, plan *tfsdk.Plan, state *tfsdk.State, api any, model any) {
	stage(ctx, installer, diags, plan, state, api, model)
	if diags.HasError() {
		return
	}
	installer.UpsertFromStage(ctx, diags, plan, state, api, model)
}

// Reads an installed resource's item, and drops the resource if P0 no longer
// has the item installed, so that the next plan installs it again. An update
// that fails at the configure step leaves the item in "configure" with the new
// values already saved, so without this the next plan would show no changes.
func readInstalled(ctx context.Context, installer *common.Install, diags *diag.Diagnostics, state *tfsdk.State, api any, model any) {
	installer.Read(ctx, diags, state, api, model)
	if diags.HasError() || state.Raw.IsNull() {
		return
	}
	var itemState types.String
	diags.Append(state.GetAttribute(ctx, path.Root("state"), &itemState)...)
	if !diags.HasError() && itemState.ValueString() != installedState {
		state.RemoveResource(ctx)
	}
}
