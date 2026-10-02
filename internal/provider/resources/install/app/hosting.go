package installapp

import (
	"context"

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
// (packages/integrations/app/src/shared/components.ts). Other integrations whose
// connector the customer deploys reuse that element as is, so P0 stores their hosting
// in the same JSON. GitHub Repositories reuses that JSON with an attribute of its own,
// since it supports only AWS for now.

// The connector's address, as P0 stores it. The JSON is a discriminated union on
// "type": the AWS variant carries accountId, the Google Cloud variant carries
// projectId and the connectorServiceUri P0 resolves while verifying the install.
type ConnectorHostingJson struct {
	Type                string  `json:"type"`
	AccountId           *string `json:"accountId,omitempty"`
	ProjectId           *string `json:"projectId,omitempty"`
	ConnectorName       string  `json:"connectorName"`
	ConnectorRegion     string  `json:"connectorRegion"`
	ConnectorServiceUri *string `json:"connectorServiceUri,omitempty"`
}

type ConnectorHostingModel struct {
	Type                types.String `tfsdk:"type"`
	AccountId           types.String `tfsdk:"account_id"`
	ProjectId           types.String `tfsdk:"project_id"`
	ConnectorName       types.String `tfsdk:"connector_name"`
	ConnectorRegion     types.String `tfsdk:"connector_region"`
	ConnectorServiceUri types.String `tfsdk:"connector_service_uri"`
}

// ConnectorHostingValidators holds the validators of the connector's address fields.
type ConnectorHostingValidators struct {
	AccountId       []validator.String
	ProjectId       []validator.String
	ConnectorName   []validator.String
	ConnectorRegion []validator.String
}

// ConnectorHostingAttribute returns the required `hosting` attribute, whose address
// fields take the given validators.
func ConnectorHostingAttribute(validators ConnectorHostingValidators) schema.SingleNestedAttribute {
	// Every address field below is `step: "new"` in the app's install schema, and the
	// backend enforces that per leaf, not per hosting block: changing one after the
	// install exists fails the configure step instead of updating it. RequiresReplace
	// on each makes Terraform plan the destroy-and-recreate that actually works,
	// rather than an in-place update that is guaranteed to 422.
	return schema.SingleNestedAttribute{
		MarkdownDescription: `Where your connector is deployed, and how P0 addresses it`,
		Required:            true,
		Attributes: map[string]schema.Attribute{
			"type": schema.StringAttribute{
				MarkdownDescription: `The connector's hosting: either ` + "`aws`" + ` (Lambda) or ` + "`gcp`" + ` (Cloud Run)`,
				Required:            true,
				Validators: []validator.String{
					stringvalidator.OneOf(AwsHosting, GcpHosting),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"account_id": schema.StringAttribute{
				MarkdownDescription: `The AWS account ID in which the connector's Lambda function is deployed. Required for, and only valid with, ` + "`aws`" + ` hosting.`,
				Optional:            true,
				Validators:          validators.AccountId,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"project_id": schema.StringAttribute{
				MarkdownDescription: `The Google Cloud project ID in which the connector's Cloud Run service is deployed. Required for, and only valid with, ` + "`gcp`" + ` hosting.`,
				Optional:            true,
				Validators:          validators.ProjectId,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"connector_name": schema.StringAttribute{
				MarkdownDescription: `The name of the Lambda function or Cloud Run service hosting the connector`,
				Required:            true,
				Validators:          validators.ConnectorName,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"connector_region": schema.StringAttribute{
				MarkdownDescription: `The region the connector is deployed in (e.g. ` + "`us-east-1`" + ` on AWS, or ` + "`us-central1`" + ` on Google Cloud)`,
				Required:            true,
				Validators:          validators.ConnectorRegion,
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

// ConnectorHostingFromConfig reads the root `hosting` attribute of a configuration, for
// ValidateConfig. It returns nil when the block is null, or unknown until apply, as
// when it is set from a module output.
func ConnectorHostingFromConfig(ctx context.Context, config tfsdk.Config, diags *diag.Diagnostics) *ConnectorHostingModel {
	var hosting ConnectorHostingModel
	if !common.GetKnownObject(ctx, config, path.Root("hosting"), &hosting, diags) {
		return nil
	}
	return &hosting
}

// ValidateConnectorHosting rejects a hosting block whose address fields do not match
// its type: a missing account or project, or one set for the other cloud. The block
// must sit at the root `hosting` attribute. Call it from ValidateConfig, with
// ConnectorHostingFromConfig's result, so that the mistake surfaces at plan time,
// before P0 is called.
func ValidateConnectorHosting(hosting *ConnectorHostingModel, diags *diag.Diagnostics) {
	// An unknown type is a value only resolved at apply time; the type's own validator
	// has already rejected anything that is known and unsupported.
	if hosting == nil || hosting.Type.IsUnknown() {
		return
	}

	switch hosting.Type.ValueString() {
	case AwsHosting:
		if hosting.AccountId.IsNull() {
			diags.AddAttributeError(
				path.Root("hosting").AtName("account_id"),
				"Missing AWS account ID",
				"'hosting.account_id' is required when 'hosting.type' is \"aws\".",
			)
		}
		if IsSet(hosting.ProjectId) {
			diags.AddAttributeError(
				path.Root("hosting").AtName("project_id"),
				"Unexpected Google Cloud project",
				"'hosting.project_id' may only be set when 'hosting.type' is \"gcp\".",
			)
		}
	case GcpHosting:
		if hosting.ProjectId.IsNull() {
			diags.AddAttributeError(
				path.Root("hosting").AtName("project_id"),
				"Missing Google Cloud project",
				"'hosting.project_id' is required when 'hosting.type' is \"gcp\".",
			)
		}
		if IsSet(hosting.AccountId) {
			diags.AddAttributeError(
				path.Root("hosting").AtName("account_id"),
				"Unexpected AWS account ID",
				"'hosting.account_id' may only be set when 'hosting.type' is \"aws\".",
			)
		}
	}
}

// IsSet reports whether an optional value is present and resolved. An unknown value is
// not null, so checking IsNull alone would reject a field that interpolation has yet
// to fill in.
func IsSet(value types.String) bool {
	return !value.IsNull() && !value.IsUnknown()
}

// ToJson returns the hosting as P0 stores it. connector_service_uri is left out,
// because P0 resolves it.
func (m *ConnectorHostingModel) ToJson() *ConnectorHostingJson {
	return &ConnectorHostingJson{
		Type:            m.Type.ValueString(),
		AccountId:       m.AccountId.ValueStringPointer(),
		ProjectId:       m.ProjectId.ValueStringPointer(),
		ConnectorName:   m.ConnectorName.ValueString(),
		ConnectorRegion: m.ConnectorRegion.ValueString(),
	}
}

// ConnectorHostingFromJson reads the hosting P0 stores.
func ConnectorHostingFromJson(json *ConnectorHostingJson) *ConnectorHostingModel {
	return &ConnectorHostingModel{
		Type:                types.StringValue(json.Type),
		AccountId:           types.StringPointerValue(json.AccountId),
		ProjectId:           types.StringPointerValue(json.ProjectId),
		ConnectorName:       types.StringValue(json.ConnectorName),
		ConnectorRegion:     types.StringValue(json.ConnectorRegion),
		ConnectorServiceUri: types.StringPointerValue(json.ConnectorServiceUri),
	}
}
