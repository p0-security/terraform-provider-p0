package installdatabricks

import (
	"context"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/p0-security/terraform-provider-p0/internal"
	"github.com/p0-security/terraform-provider-p0/internal/common"
	installresources "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install"
	installaws "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/aws"
	installrds "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/rds"
)

const DatabricksKey = "databricks"

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
var AccountsUrls = []string{
	"https://accounts.cloud.databricks.com",
	"https://accounts.cloud.databricks.us",
	"https://accounts.azuredatabricks.net",
	"https://accounts.gcp.databricks.com",
}

// The connector refuses a workspace ID that isn't all digits.
var WorkspaceIdRegex = regexp.MustCompile(`^\d+$`)

// Unity Catalog names exclude periods, spaces, forward slashes and ASCII
// control characters, and have at most 255 characters.
var CatalogNameRegex = regexp.MustCompile(`^[^. /\x00-\x1f\x7f]{1,255}$`)

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

func connectorAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"id": fixedAttribute(
			`The ID of the AWS account that the connector's Lambda runs in`,
			awsAccountIdValidator(),
		),
		"region": fixedAttribute(
			`The AWS region that the connector's Lambda runs in`,
			stringvalidator.RegexMatches(installrds.AwsRegionRegex, "AWS region should be in the format: us-east-1"),
		),
		"domain_pattern": fixedAttribute(
			`A regular expression that the connector matches against the whole email domain of every user it grants to, e.g. `+"`example\\.com`"+`. The connector refuses any other user. Set the same value as the Lambda's `+"`DOMAIN_ALLOW_PATTERN`"+` environment variable.`,
			stringvalidator.LengthAtLeast(1),
		),
		"state": common.StateAttribute,
	}
}

type connectorModel struct {
	Id            types.String `tfsdk:"id"`
	Region        types.String `tfsdk:"region"`
	DomainPattern types.String `tfsdk:"domain_pattern"`
	State         types.String `tfsdk:"state"`
}

type connectorJson struct {
	Region        *string `json:"region,omitempty"`
	DomainPattern *string `json:"domainPattern,omitempty"`
	State         *string `json:"state,omitempty"`
}

type connectorApi struct {
	Item *connectorJson `json:"item"`
}

// The staged and installed connector resources share one item, and so one
// model.
func newConnectorInstaller(data *internal.P0ProviderData) *common.Install {
	return &common.Install{
		Integration:  DatabricksKey,
		Component:    installresources.Connector,
		ProviderData: data,
		GetId:        connectorId,
		GetItemJson:  connectorItemJson,
		FromJson:     connectorFromJson,
		ToJson:       connectorToJson,
	}
}

func connectorId(data any) *string {
	model, ok := data.(*connectorModel)
	if !ok {
		return nil
	}
	id := model.Id.ValueString()
	return &id
}

func connectorItemJson(json any) any {
	api, ok := json.(*connectorApi)
	if !ok || api.Item == nil {
		return nil
	}
	return api.Item
}

func connectorFromJson(_ context.Context, _ *diag.Diagnostics, id string, json any) any {
	item, ok := json.(*connectorJson)
	if !ok {
		return nil
	}
	return &connectorModel{
		Id:            types.StringValue(id),
		Region:        types.StringPointerValue(item.Region),
		DomainPattern: types.StringPointerValue(item.DomainPattern),
		State:         types.StringPointerValue(item.State),
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
		"accounts_url": fixedAttribute(
			"The URL of this account's console: `https://accounts.cloud.databricks.com` (AWS), `https://accounts.cloud.databricks.us` (AWS GovCloud), `https://accounts.azuredatabricks.net` (Azure) or `https://accounts.gcp.databricks.com` (Google Cloud)",
			stringvalidator.OneOf(AccountsUrls...),
		),
		"state": common.StateAttribute,
	}
}

type accountStagedModel struct {
	Id          types.String `tfsdk:"id"`
	Connector   types.String `tfsdk:"connector"`
	AccountsUrl types.String `tfsdk:"accounts_url"`
	State       types.String `tfsdk:"state"`
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

type accountApi struct {
	Item *accountJson `json:"item"`
}

func accountItemJson(json any) any {
	api, ok := json.(*accountApi)
	if !ok || api.Item == nil {
		return nil
	}
	return api.Item
}

func accountStagedFromJson(id string, item *accountJson) accountStagedModel {
	return accountStagedModel{
		Id:          types.StringValue(id),
		Connector:   types.StringPointerValue(item.Connector),
		AccountsUrl: types.StringPointerValue(item.AccountsUrl),
		State:       types.StringPointerValue(item.State),
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
