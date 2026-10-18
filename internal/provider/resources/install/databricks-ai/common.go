package installdatabricksai

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/p0-security/terraform-provider-p0/internal"
	"github.com/p0-security/terraform-provider-p0/internal/common"
)

// A copy of the Databricks (HI) install resources' common.go, for the
// Databricks (AI) connector and account.

const DatabricksAiKey = "databricks-ai"

// The audience of the AWS identity tokens that the connector requests
// (`DATABRICKS_TOKEN_AUDIENCE` in p0-connector-databricks-ai). The connector's
// role may only request tokens for it, and the federation policy on P0's
// service principal must accept it.
const FederationAudience = "databricks"

// Ends the description of each Databricks (AI) resource, in the words of
// p0_github_repositories' beta note.
const betaNote = `**Note:** This integration is in beta.`

// The Databricks (AI) components' keys in P0's install API.
const (
	connectorComponent = "connector"
	accountComponent   = "account"
)

// All installable Databricks (AI) components. An account points at its
// connector.
var Components = []string{
	connectorComponent,
	accountComponent,
}

// The commercial Databricks account console on AWS, the only cloud the
// integration supports. The connector only accepts it, because it sends its
// AWS identity token to the console that an account names.
const accountsConsole = "https://accounts.cloud.databricks.com"

// Rejects a value that isn't a UUID, naming what it is, as the app's
// `uuidValidator` does.
func uuidValidator(label string) validator.String {
	return stringvalidator.RegexMatches(common.UuidRegex, label+" are UUIDs, e.g. 01234567-89ab-cdef-0123-456789abcdef")
}

func accountIdValidator() validator.String {
	return uuidValidator("Databricks account IDs")
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

// GetId for every Databricks (AI) model.
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

// GetItemJson for every Databricks (AI) component.
func getItemJson[T any](json any) any {
	api, ok := json.(*itemApi[T])
	if !ok || api.Item == nil {
		return nil
	}
	return api.Item
}

// The installer of a Databricks (AI) component's resources, whose item's JSON
// is T.
func newInstaller[T any](data *internal.P0ProviderData, component string, fromJson func(context.Context, *diag.Diagnostics, string, any) any, toJson func(any) any) *common.Install {
	return &common.Install{
		Integration:        DatabricksAiKey,
		Component:          component,
		ProviderData:       data,
		GetId:              getId,
		GetItemJson:        getItemJson[T],
		FromJson:           fromJson,
		ToJson:             toJson,
		DescribeCheckError: describeCheckError(component),
	}
}

// P0 runs every Databricks (AI) install check through the customer's
// connector, so a failed check usually means a problem in their AWS or
// Databricks setup, which P0's message describes. The summary doesn't presume
// where the problem is, and leaves that to P0's message.
func describeCheckError(component string) func(id string, err error) (string, string) {
	return func(id string, err error) (string, string) {
		return "Databricks (AI) install check failed",
			fmt.Sprintf("P0 rejected the install check for the Databricks (AI) %s %q:\n\n%s", component, id, err)
	}
}

func connectorAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"id": fixedAttribute(
			`The ID of the AWS account that the connector's Lambda runs in`,
			awsAccountIdValidator(),
		),
		"region": fixedAttribute(
			`The AWS region that the connector's Lambda runs in. The connector runs only in commercial AWS regions.`,
			commercialRegionValidator("The connector runs only in commercial AWS regions, e.g. us-east-1"),
		),
		"federation_audience": federationAudienceAttribute,
		"state":               common.StateAttribute,
	}
}

type connectorModel struct {
	Id                 types.String `tfsdk:"id"`
	Region             types.String `tfsdk:"region"`
	FederationAudience types.String `tfsdk:"federation_audience"`
	State              types.String `tfsdk:"state"`
}

func (m connectorModel) key() string {
	return m.Id.ValueString()
}

type connectorJson struct {
	Region *string `json:"region,omitempty"`
	State  *string `json:"state,omitempty"`
}

type connectorApi = itemApi[connectorJson]

// The staged and installed connector resources share one item, and so one
// model.
func newConnectorInstaller(data *internal.P0ProviderData) *common.Install {
	return newInstaller[connectorJson](data, connectorComponent, connectorFromJson, connectorToJson)
}

func connectorFromJson(_ context.Context, _ *diag.Diagnostics, id string, json any) any {
	item, ok := json.(*connectorJson)
	if !ok {
		return nil
	}
	return &connectorModel{
		Id:                 types.StringValue(id),
		Region:             types.StringPointerValue(item.Region),
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
		Region: model.Region.ValueStringPointer(),
	}
}

func accountStagedAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"id": fixedAttribute(
			`The Databricks account ID`,
			accountIdValidator(),
		),
		"connector": fixedAttribute(
			"The `id` of the `p0_databricks_ai_connector` that reaches this account, which is the ID of the AWS account that the connector runs in",
			awsAccountIdValidator(),
		),
		"accounts_url": fixedAttribute(
			"The URL of this account's console, `"+accountsConsole+"`. The integration supports Databricks on AWS only",
			stringvalidator.OneOf(accountsConsole),
		),
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
	ApplicationId  types.String `tfsdk:"application_id"`
	OwnerPrincipal types.String `tfsdk:"owner_principal"`
}

// P0 also saves `unityGatewayBeta` on the account, which only P0 sets. The
// provider neither models nor sends it, and reading the item ignores it.
type accountJson struct {
	Connector   *string `json:"connector,omitempty"`
	AccountsUrl *string `json:"accountsUrl,omitempty"`
	// Only p0_databricks_ai_account sends these two.
	ApplicationId  *string `json:"applicationId,omitempty"`
	OwnerPrincipal *string `json:"ownerPrincipal,omitempty"`
	State          *string `json:"state,omitempty"`
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
