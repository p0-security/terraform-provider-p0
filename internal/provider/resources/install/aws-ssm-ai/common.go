package installawsssmai

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/p0-security/terraform-provider-p0/internal"
	"github.com/p0-security/terraform-provider-p0/internal/common"
)

// A copy of the Redshift (AI) install resources' common.go, trimmed to the
// connector, for AWS SSM (AI).

const AwsSsmAiKey = "aws-ssm-ai"

// Ends the description of each AWS SSM (AI) resource, in the words of
// p0_looker_ai's preview note.
const previewNote = `**Note:** This integration is in preview.`

// The full install steps, which end the description of each AWS SSM (AI)
// resource.
//
// TODO(INT-1517): the docs page doesn't exist yet. Check the link once it
// publishes.
const docsNote = `For the full install steps, see [AWS SSM MCP server](https://docs.p0.dev/integrations/resource-integrations/agentic-gateway/mcp-server/aws-ssm).`

// The AWS SSM (AI) connector's key in P0's install API, and its only component.
const connectorComponent = "connector"

// The connector's Lambda function name. P0 invokes the connector by it.
const connectorFunctionName = "p0-connector-aws-ssm-ai"

// A model that names its item's key.
type keyed interface {
	key() string
}

// GetId for every AWS SSM (AI) model. The key is an AWS account ID, which needs no
// escaping in a URL path.
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

// GetItemJson for the AWS SSM (AI) connector.
func getItemJson[T any](json any) any {
	api, ok := json.(*itemApi[T])
	if !ok || api.Item == nil {
		return nil
	}
	return api.Item
}

// The installer of the connector's resources, whose item's JSON is
// connectorJson.
func newInstaller(data *internal.P0ProviderData, fromJson func(context.Context, *diag.Diagnostics, string, any) any, toJson func(any) any) *common.Install {
	return &common.Install{
		Integration:        AwsSsmAiKey,
		Component:          connectorComponent,
		ProviderData:       data,
		GetId:              getId,
		GetItemJson:        getItemJson[connectorJson],
		FromJson:           fromJson,
		ToJson:             toJson,
		DescribeCheckError: describeCheckError,
	}
}

// P0 runs every AWS SSM (AI) install check through the customer's connector, so a
// failed check usually means a problem in their AWS setup, which P0's message
// describes. The summary doesn't presume where the problem is, and leaves
// that to P0's message.
func describeCheckError(id string, err error) (string, string) {
	return "AWS SSM (AI) install check failed",
		fmt.Sprintf("P0 rejected the install check for the AWS SSM (AI) connector %q:\n\n%s", id, err)
}

func connectorStagedAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"id": fixedAttribute(
			`The ID of the AWS account that the connector's Lambda runs in. P0 reaches every instance that SSM manages in this account through the connector.`,
			awsAccountIdValidator(),
		),
		"region": fixedAttribute(
			`The AWS region that the connector's Lambda runs in. The connector runs only in commercial AWS regions.`,
			commercialRegionValidator(invalidRegion),
		),
		"vault": vaultAttribute(),
		"state": common.StateAttribute,
	}
}

type connectorStagedModel struct {
	Id     types.String `tfsdk:"id"`
	Region types.String `tfsdk:"region"`
	Vault  *vaultModel  `tfsdk:"vault"`
	State  types.String `tfsdk:"state"`
}

func (m connectorStagedModel) key() string {
	return m.Id.ValueString()
}

type connectorModel struct {
	connectorStagedModel
	InstanceRegions types.List `tfsdk:"instance_regions"`
	RunAsUsers      types.List `tfsdk:"run_as_users"`
	// Deprecated: run_as_users replaces it.
	IsSudoEnabled types.Bool `tfsdk:"is_sudo_enabled"`
}

type connectorJson struct {
	Region *string    `json:"region,omitempty"`
	Vault  *vaultJson `json:"vault,omitempty"`
	// Only p0_aws_ssm_ai_connector sends these two. P0 takes each list as one
	// string, separated by commas.
	InstanceRegions *string `json:"instanceRegions,omitempty"`
	RunAsUsers      *string `json:"runAsUsers,omitempty"`
	// Read only. P0 lets agents run as root when an item installed before
	// runAsUsers has this on and no run-as users.
	IsSudoEnabled *bool `json:"isSudoEnabled,omitempty"`
	// P0 sets this.
	State *string `json:"state,omitempty"`
}

type connectorApi = itemApi[connectorJson]

func newConnectorStagedModel(id string, item *connectorJson) connectorStagedModel {
	return connectorStagedModel{
		Id:     types.StringValue(id),
		Region: types.StringPointerValue(item.Region),
		Vault:  vaultFromJson(item.Vault),
		State:  types.StringPointerValue(item.State),
	}
}

// P0 sets the state, so it is never sent.
func (m *connectorStagedModel) toJson() *connectorJson {
	return &connectorJson{
		Region: m.Region.ValueStringPointer(),
		Vault:  m.Vault.toJson(),
	}
}

// A list as P0 stores it: one string, separated by commas.
func listToJson(list types.List) *string {
	if list.IsNull() || list.IsUnknown() {
		return nil
	}
	var values []string
	for _, element := range list.Elements() {
		if value, ok := element.(types.String); ok {
			values = append(values, value.ValueString())
		}
	}
	joined := strings.Join(values, ",")
	return &joined
}

// A list that P0 stores, or null when it has none, as for an item that was
// only staged.
func listFromJson(ctx context.Context, diags *diag.Diagnostics, value *string) types.List {
	if value == nil || strings.TrimSpace(*value) == "" {
		return types.ListNull(types.StringType)
	}
	var values []string
	for _, element := range strings.Split(*value, ",") {
		values = append(values, strings.TrimSpace(element))
	}
	list, listDiags := types.ListValueFrom(ctx, types.StringType, values)
	diags.Append(listDiags...)
	return list
}

// Rejects a configuration whose vault isn't in the connector's own AWS
// account, as P0 does when it stages the connector. Values that Terraform only
// knows at apply time are skipped while planning, and checked when Terraform
// validates the configuration again before it applies.
func validateConnectorConfig(ctx context.Context, config tfsdk.Config, diags *diag.Diagnostics) {
	var id types.String
	diags.Append(config.GetAttribute(ctx, path.Root("id"), &id)...)
	vault := vaultFromConfig(ctx, config, diags)
	if diags.HasError() || vault == nil {
		return
	}
	validateVaultAccount(id, vault, diags)
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
