package installdocker

import (
	"context"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/p0-security/terraform-provider-p0/internal/common"
)

// The `hosting` attribute is the app's `ConnectorHosting` install element
// (packages/integrations/app/shared/src/components.ts), and P0 stores the Docker Hub
// connector's hosting in that element's JSON. Both variants are supported: a Lambda
// function on AWS, and a Cloud Run service on Google Cloud.

const (
	awsHosting = "aws"
	gcpHosting = "gcp"
)

// The connector's address, as P0 stores it. The JSON is a discriminated union on
// "type": the AWS variant carries accountId, the Google Cloud variant carries
// projectId and the connectorServiceUri P0 resolves while verifying the install.
type hostingJson struct {
	Type                string  `json:"type"`
	AccountId           *string `json:"accountId,omitempty"`
	ProjectId           *string `json:"projectId,omitempty"`
	ConnectorName       string  `json:"connectorName"`
	ConnectorRegion     string  `json:"connectorRegion"`
	ConnectorServiceUri *string `json:"connectorServiceUri,omitempty"`
}

// The final connector's hosting: the address, and the URL P0 resolves for a Cloud Run
// service.
type hostingModel struct {
	Type                types.String `tfsdk:"type"`
	AccountId           types.String `tfsdk:"account_id"`
	ProjectId           types.String `tfsdk:"project_id"`
	ConnectorName       types.String `tfsdk:"connector_name"`
	ConnectorRegion     types.String `tfsdk:"connector_region"`
	ConnectorServiceUri types.String `tfsdk:"connector_service_uri"`
}

// Returns the hosting as P0 stores it. connector_service_uri is left out, because P0
// resolves it.
func (m *hostingModel) toJson() *hostingJson {
	return &hostingJson{
		Type:            m.Type.ValueString(),
		AccountId:       m.AccountId.ValueStringPointer(),
		ProjectId:       m.ProjectId.ValueStringPointer(),
		ConnectorName:   m.ConnectorName.ValueString(),
		ConnectorRegion: m.ConnectorRegion.ValueString(),
	}
}

// Reads the hosting P0 stores.
func hostingFromJson(json *hostingJson) *hostingModel {
	return &hostingModel{
		Type:                types.StringValue(json.Type),
		AccountId:           types.StringPointerValue(json.AccountId),
		ProjectId:           types.StringPointerValue(json.ProjectId),
		ConnectorName:       types.StringValue(json.ConnectorName),
		ConnectorRegion:     types.StringValue(json.ConnectorRegion),
		ConnectorServiceUri: types.StringPointerValue(json.ConnectorServiceUri),
	}
}

// Reads the root `hosting` attribute of a final connector's configuration, for
// ValidateConfig. It returns nil when the block is null, or unknown until apply, as
// when it is set from the staged connector's output.
func hostingFromConfig(ctx context.Context, config tfsdk.Config, diags *diag.Diagnostics) *hostingModel {
	var hosting hostingModel
	if !common.GetKnownObject(ctx, config, path.Root("hosting"), &hosting, diags) {
		return nil
	}
	return &hosting
}

// Reports whether an optional value is present and resolved. An unknown value is not
// null, so checking IsNull alone would reject a field that interpolation has yet to
// fill in.
func isSet(value types.String) bool {
	return !value.IsNull() && !value.IsUnknown()
}

// The staged connector's hosting: the address alone. P0 resolves a Cloud Run
// service's URL when it verifies the install, so only the final resource's hosting has
// `connector_service_uri`. Leaving it out here also lets the final resource take this
// block whole, as `hosting = p0_docker_connector_staged.example.hosting`.
type stagedHostingModel struct {
	Type            types.String `tfsdk:"type"`
	AccountId       types.String `tfsdk:"account_id"`
	ProjectId       types.String `tfsdk:"project_id"`
	ConnectorName   types.String `tfsdk:"connector_name"`
	ConnectorRegion types.String `tfsdk:"connector_region"`
}

func (m *stagedHostingModel) address() *hostingModel {
	return &hostingModel{
		Type:                m.Type,
		AccountId:           m.AccountId,
		ProjectId:           m.ProjectId,
		ConnectorName:       m.ConnectorName,
		ConnectorRegion:     m.ConnectorRegion,
		ConnectorServiceUri: types.StringNull(),
	}
}

func stagedHostingFromJson(json *hostingJson) *stagedHostingModel {
	hosting := hostingFromJson(json)
	return &stagedHostingModel{
		Type:            hosting.Type,
		AccountId:       hosting.AccountId,
		ProjectId:       hosting.ProjectId,
		ConnectorName:   hosting.ConnectorName,
		ConnectorRegion: hosting.ConnectorRegion,
	}
}

// The final connector's required `hosting` attribute. P0's rules for the address
// fields are checked in ValidateConfig, in validateHosting, so the fields have no
// validators of their own.
func hostingAttribute(description string) schema.SingleNestedAttribute {
	// P0 refuses a change to any address field once the connector is added, so
	// RequiresReplace on each makes Terraform plan the destroy-and-recreate that
	// works, rather than an in-place update that P0 would refuse.
	return schema.SingleNestedAttribute{
		MarkdownDescription: description,
		Required:            true,
		Attributes: map[string]schema.Attribute{
			"type": schema.StringAttribute{
				MarkdownDescription: `The connector's hosting: either ` + "`aws`" + ` (Lambda) or ` + "`gcp`" + ` (Cloud Run)`,
				Required:            true,
				Validators: []validator.String{
					stringvalidator.OneOf(awsHosting, gcpHosting),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"account_id": schema.StringAttribute{
				MarkdownDescription: `The AWS account ID in which the connector's Lambda function is deployed. Required for, and only valid with, ` + "`aws`" + ` hosting.`,
				Optional:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"project_id": schema.StringAttribute{
				MarkdownDescription: `The Google Cloud project ID in which the connector's Cloud Run service is deployed. Required for, and only valid with, ` + "`gcp`" + ` hosting.`,
				Optional:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"connector_name": schema.StringAttribute{
				MarkdownDescription: `The name of the Lambda function or Cloud Run service hosting the connector`,
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"connector_region": schema.StringAttribute{
				MarkdownDescription: `The region the connector is deployed in (e.g. ` + "`us-east-1`" + ` on AWS, or ` + "`us-central1`" + ` on Google Cloud)`,
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"connector_service_uri": schema.StringAttribute{
				MarkdownDescription: `The connector's invocation URL, resolved by P0 during install. Only populated for ` + "`gcp`" + ` hosting.`,
				Computed:            true,
			},
		},
	}
}

func stagedHostingAttribute(description string) schema.SingleNestedAttribute {
	attribute := hostingAttribute(description)
	delete(attribute.Attributes, "connector_service_uri")
	return attribute
}

// Reads the root `hosting` attribute of a staged connector's configuration, for
// ValidateConfig. It returns nil when the block is null, or unknown until apply.
func stagedHostingFromConfig(ctx context.Context, config tfsdk.Config, diags *diag.Diagnostics) *hostingModel {
	var hosting stagedHostingModel
	if !common.GetKnownObject(ctx, config, path.Root("hosting"), &hosting, diags) {
		return nil
	}
	return hosting.address()
}

// Where the connector can run: P0's rules for the hosting fields, which it checks when
// it creates the item (`validateHosting` in
// packages/integrations/docker/backend/src/install/validation.ts in the app). The
// connector's Terraform can't be deployed with a value that breaks one. The patterns
// and the messages are P0's.

var (
	// A connector name that works for both things the connector's Google Terraform
	// names after it: a Cloud Run service and its service account. The service account
	// is why it has 6 to 30 characters.
	gcpConnectorNamePattern = regexp.MustCompile(`^[a-z][-a-z0-9]{4,28}[a-z0-9]$`)
	// A Google Cloud region, such as us-west1.
	gcpRegionPattern = regexp.MustCompile(`^[a-z]+-[a-z]+\d+$`)
)

// The rules for each hosting type's fields, in the order P0 checks them.
var hostingRules = map[string][]fieldRules{
	awsHosting: {
		{name: "account_id", rules: awsAccountRules(connectorPlace)},
		{name: "connector_name", rules: awsConnectorNameRules()},
		{name: "connector_region", rules: awsRegionRules(connectorPlace)},
	},
	gcpHosting: {
		{name: "project_id", rules: gcpProjectRules(connectorPlace)},
		{name: "connector_name", rules: []rule{{
			summary: "Invalid connector name",
			message: "Use 6 to 30 characters for the connector name: lowercase letters, digits and hyphens, " +
				"starting with a letter and ending with a letter or digit. The connector's service account takes the same name.",
			test: gcpConnectorNamePattern.MatchString,
		}}},
		{name: "connector_region", rules: []rule{{
			summary: "Invalid Google Cloud region for " + connectorPlace.what,
			message: "Enter the Google Cloud region the connector runs in, such as us-west1.",
			test:    gcpRegionPattern.MatchString,
		}}},
	},
}

// The hosting's address fields, by attribute name.
func hostingFields(hosting *hostingModel) map[string]types.String {
	return map[string]types.String{
		"account_id":       hosting.AccountId,
		"project_id":       hosting.ProjectId,
		"connector_name":   hosting.ConnectorName,
		"connector_region": hosting.ConnectorRegion,
	}
}

// Rejects a hosting block whose address fields don't match its type, and each field
// that P0 refuses for where the connector runs. A block of a type that isn't known yet
// is skipped. Validating here rather than in Create surfaces mistakes at plan time,
// before P0 is called, in P0's words.
func validateHosting(hosting *hostingModel, diags *diag.Diagnostics) {
	validateHostingType(hosting, diags)
	if hosting != nil && isSet(hosting.Type) {
		validateFields(path.Root("hosting"), hostingFields(hosting), hostingRules[hosting.Type.ValueString()], diags)
	}
}

// Rejects a hosting block whose address fields do not match its type: a missing
// account or project, or one set for the other cloud.
func validateHostingType(hosting *hostingModel, diags *diag.Diagnostics) {
	// An unknown type is a value only resolved at apply time; the type's own validator
	// has already rejected anything that is known and unsupported.
	if hosting == nil || hosting.Type.IsUnknown() {
		return
	}

	switch hosting.Type.ValueString() {
	case awsHosting:
		if hosting.AccountId.IsNull() {
			diags.AddAttributeError(
				path.Root("hosting").AtName("account_id"),
				"Missing AWS account ID",
				"'hosting.account_id' is required when 'hosting.type' is \"aws\".",
			)
		}
		if isSet(hosting.ProjectId) {
			diags.AddAttributeError(
				path.Root("hosting").AtName("project_id"),
				"Unexpected Google Cloud project",
				"'hosting.project_id' may only be set when 'hosting.type' is \"gcp\".",
			)
		}
	case gcpHosting:
		if hosting.ProjectId.IsNull() {
			diags.AddAttributeError(
				path.Root("hosting").AtName("project_id"),
				"Missing Google Cloud project",
				"'hosting.project_id' is required when 'hosting.type' is \"gcp\".",
			)
		}
		if isSet(hosting.AccountId) {
			diags.AddAttributeError(
				path.Root("hosting").AtName("account_id"),
				"Unexpected AWS account ID",
				"'hosting.account_id' may only be set when 'hosting.type' is \"aws\".",
			)
		}
	}
}
