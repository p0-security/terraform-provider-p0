package installredshiftai

import (
	"context"
	"fmt"
	"net/url"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/p0-security/terraform-provider-p0/internal"
	"github.com/p0-security/terraform-provider-p0/internal/common"
)

// A copy of the Databricks (AI) install resources' common.go, for the
// Redshift (AI) connector and cluster.

const RedshiftAiKey = "redshift-ai"

// Ends the description of each Redshift (AI) resource, in the words of
// p0_github_repositories' beta note.
const betaNote = `**Note:** This integration is in beta.`

// The full install steps, which end the description of each Redshift (AI)
// resource.
const docsNote = `For the full install steps, see [Redshift MCP servers](https://docs.p0.dev/integrations/resource-integrations/agentic-gateway/mcp-server/redshift).`

// The Redshift (AI) components' keys in P0's install API.
const (
	connectorComponent = "connector"
	clusterComponent   = "cluster"
)

// All installable Redshift (AI) components. A cluster points at its connector.
var Components = []string{
	connectorComponent,
	clusterComponent,
}

// The values of a cluster's `type`, the options of the app's `type` select.
var clusterTypes = []string{"provisioned", "serverless"}

// The patterns of the app's `database` and `objectOwner` validators, which
// match the connector's IDENTIFIER and USER_NAME. Neither allows whitespace,
// so a value always reads back from P0 as it was sent.
var (
	databaseRegex    = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_$]{0,126}$`)
	objectOwnerRegex = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_:+=,.@-]{0,126}$`)
)

// A model that names its item's key.
type keyed interface {
	key() string
}

// GetId for every Redshift (AI) model. It escapes the key, since the
// installer puts it in a URL path and a cluster's key holds slashes. The
// installer gives the escaped key back to FromJson, which reads it with
// keyFromPath.
func getId(data any) *string {
	model, ok := data.(keyed)
	if !ok {
		return nil
	}
	key := url.PathEscape(model.key())
	return &key
}

// The key that getId escaped.
func keyFromPath(diags *diag.Diagnostics, id string) string {
	key, err := url.PathUnescape(id)
	if err != nil {
		diags.AddError("Bad item ID", fmt.Sprintf("Could not read the item ID %q: %s", id, err))
	}
	return key
}

// The install API's response for one item.
type itemApi[T any] struct {
	Item *T `json:"item"`
}

// GetItemJson for every Redshift (AI) component.
func getItemJson[T any](json any) any {
	api, ok := json.(*itemApi[T])
	if !ok || api.Item == nil {
		return nil
	}
	return api.Item
}

// The installer of a Redshift (AI) component's resources, whose item's JSON is
// T.
func newInstaller[T any](data *internal.P0ProviderData, component string, fromJson func(context.Context, *diag.Diagnostics, string, any) any, toJson func(any) any) *common.Install {
	return &common.Install{
		Integration:        RedshiftAiKey,
		Component:          component,
		ProviderData:       data,
		GetId:              getId,
		GetItemJson:        getItemJson[T],
		FromJson:           fromJson,
		ToJson:             toJson,
		DescribeCheckError: describeCheckError(component),
	}
}

// P0 runs every Redshift (AI) install check through the customer's connector,
// so a failed check usually means a problem in their AWS or Redshift setup,
// which P0's message describes. The summary doesn't presume where the problem
// is, and leaves that to P0's message.
func describeCheckError(component string) func(id string, err error) (string, string) {
	return func(id string, err error) (string, string) {
		return "Redshift (AI) install check failed",
			fmt.Sprintf("P0 rejected the install check for the Redshift (AI) %s %q:\n\n%s", component, id, err)
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
		"state": common.StateAttribute,
	}
}

type connectorModel struct {
	Id     types.String `tfsdk:"id"`
	Region types.String `tfsdk:"region"`
	State  types.String `tfsdk:"state"`
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

func connectorFromJson(_ context.Context, diags *diag.Diagnostics, id string, json any) any {
	item, ok := json.(*connectorJson)
	if !ok {
		return nil
	}
	return &connectorModel{
		Id:     types.StringValue(keyFromPath(diags, id)),
		Region: types.StringPointerValue(item.Region),
		State:  types.StringPointerValue(item.State),
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

func clusterStagedAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"id": fixedAttribute(
			"The cluster's key, `<AWS account ID>/<region>/<name>`, where the name is the provisioned cluster identifier or the Serverless workgroup name, e.g. `123456789012/us-west-2/analytics`",
			clusterIdValidator(),
		),
		"connector": fixedAttribute(
			"The `id` of the `p0_redshift_ai_connector` in this cluster's AWS account, which is that account's ID",
			awsAccountIdValidator(),
		),
		"type": fixedAttribute(
			"Whether the cluster is a `provisioned` cluster or a `serverless` workgroup",
			stringvalidator.OneOf(clusterTypes...),
		),
		"state": common.StateAttribute,
	}
}

type clusterStagedModel struct {
	Id        types.String `tfsdk:"id"`
	Connector types.String `tfsdk:"connector"`
	Type      types.String `tfsdk:"type"`
	State     types.String `tfsdk:"state"`
}

func (m clusterStagedModel) key() string {
	return m.Id.ValueString()
}

type clusterModel struct {
	clusterStagedModel
	Database    types.String `tfsdk:"database"`
	ObjectOwner types.String `tfsdk:"object_owner"`
	Arn         types.String `tfsdk:"arn"`
}

// The app's `type` select, which P0 stores as an object.
type clusterTypeJson struct {
	Type string `json:"type"`
}

// P0 also saves `warnings` on the cluster, which only P0 sets. The provider
// neither models nor sends it, and reading the item ignores it.
type clusterJson struct {
	Connector *string          `json:"connector,omitempty"`
	Type      *clusterTypeJson `json:"type,omitempty"`
	// Only p0_redshift_ai_cluster sends these two.
	Database    *string `json:"database,omitempty"`
	ObjectOwner *string `json:"objectOwner,omitempty"`
	// P0 sets these.
	Arn   *string `json:"arn,omitempty"`
	State *string `json:"state,omitempty"`
}

type clusterApi = itemApi[clusterJson]

func newClusterStagedModel(diags *diag.Diagnostics, id string, item *clusterJson) clusterStagedModel {
	model := clusterStagedModel{
		Id:        types.StringValue(keyFromPath(diags, id)),
		Connector: types.StringPointerValue(item.Connector),
		Type:      types.StringNull(),
		State:     types.StringPointerValue(item.State),
	}
	if item.Type != nil {
		model.Type = types.StringValue(item.Type.Type)
	}
	return model
}

// P0 sets the state, so it is never sent.
func (m *clusterStagedModel) toJson() *clusterJson {
	json := &clusterJson{Connector: m.Connector.ValueStringPointer()}
	if !m.Type.IsNull() && !m.Type.IsUnknown() {
		json.Type = &clusterTypeJson{Type: m.Type.ValueString()}
	}
	return json
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

func databaseValidator() validator.String {
	return stringvalidator.RegexMatches(databaseRegex, "Use a database name made of letters, digits, '_' and '$', starting with a letter or '_'")
}

func objectOwnerValidator() validator.String {
	return stringvalidator.RegexMatches(objectOwnerRegex, "Use a Redshift user name, such as etl_owner or IAMR:etl-role")
}
