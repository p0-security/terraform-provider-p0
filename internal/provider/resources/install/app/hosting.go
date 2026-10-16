package installapp

import (
	"context"
	"fmt"

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
//
// A custom application also offers Azure Container Apps, through the app's
// `AppConnectorHosting` element. That is p0_app's AppConnectorHostingAttribute. The
// other integrations keep ConnectorHostingAttribute, without Azure, as their elements
// do.

// The connector's address, as P0 stores it. The JSON is a discriminated union on
// "type": the AWS variant carries accountId, the Google Cloud variant carries
// projectId, and the Azure variant carries directoryId, subscriptionId and
// resourceGroup. P0 also sets fields the provider only reads: on Google Cloud,
// connectorServiceAccount, for integrations whose connector must run as that
// account, and on Google Cloud and Azure, connectorServiceUri once it has verified
// the install. On Azure it also sets connectorClientId, the client ID of the
// connector's app registration.
type ConnectorHostingJson struct {
	Type                    string  `json:"type"`
	AccountId               *string `json:"accountId,omitempty"`
	ProjectId               *string `json:"projectId,omitempty"`
	DirectoryId             *string `json:"directoryId,omitempty"`
	SubscriptionId          *string `json:"subscriptionId,omitempty"`
	ResourceGroup           *string `json:"resourceGroup,omitempty"`
	ConnectorName           string  `json:"connectorName"`
	ConnectorRegion         string  `json:"connectorRegion"`
	ConnectorServiceAccount *string `json:"connectorServiceAccount,omitempty"`
	ConnectorServiceUri     *string `json:"connectorServiceUri,omitempty"`
	ConnectorClientId       *string `json:"connectorClientId,omitempty"`
}

type ConnectorHostingModel struct {
	Type                types.String `tfsdk:"type"`
	AccountId           types.String `tfsdk:"account_id"`
	ProjectId           types.String `tfsdk:"project_id"`
	ConnectorName       types.String `tfsdk:"connector_name"`
	ConnectorRegion     types.String `tfsdk:"connector_region"`
	ConnectorServiceUri types.String `tfsdk:"connector_service_uri"`
}

// The hosting of an integration whose connector must, on Google Cloud, run as a
// service account that P0 generates. A custom application's connector may run as any
// service account, so p0_app's hosting has no such field.
type ConnectorHostingWithServiceAccountModel struct {
	ConnectorHostingModel
	ConnectorServiceAccount types.String `tfsdk:"connector_service_account"`
}

// The hosting of a custom application, which may also run on Azure Container Apps.
type AppConnectorHostingModel struct {
	ConnectorHostingModel
	DirectoryId       types.String `tfsdk:"directory_id"`
	SubscriptionId    types.String `tfsdk:"subscription_id"`
	ResourceGroup     types.String `tfsdk:"resource_group"`
	ConnectorClientId types.String `tfsdk:"connector_client_id"`
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
			"connector_name": requiredAddressAttribute(
				`The name of the Lambda function or Cloud Run service hosting the connector`,
				validators.ConnectorName,
			),
			"connector_region": requiredAddressAttribute(
				`The region the connector is deployed in (e.g. `+"`us-east-1`"+` on AWS, or `+"`us-central1`"+` on Google Cloud)`,
				validators.ConnectorRegion,
			),
			"connector_service_uri": schema.StringAttribute{
				MarkdownDescription: `The connector's invocation URL, resolved by P0 during install. Only populated for ` + "`gcp`" + ` hosting.`,
				Computed:            true,
			},
		},
	}
}

// ConnectorHostingWithServiceAccountAttribute returns the required `hosting` attribute
// with the read-only `connector_service_account` added, for resources whose hosting is
// a ConnectorHostingWithServiceAccountModel.
func ConnectorHostingWithServiceAccountAttribute(validators ConnectorHostingValidators) schema.SingleNestedAttribute {
	hosting := ConnectorHostingAttribute(validators)
	hosting.Attributes["connector_service_account"] = schema.StringAttribute{
		MarkdownDescription: `The email of the service account the connector must run as, generated by P0. Only populated for ` + "`gcp`" + ` hosting.`,
		Computed:            true,
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.UseStateForUnknown(),
		},
	}
	return hosting
}

// AppConnectorHostingAttribute returns p0_app's required `hosting` attribute:
// ConnectorHostingAttribute plus Azure Container Apps, whose address fields are
// directory_id, subscription_id and resource_group, and whose app registration's
// client ID P0 reads at install.
func AppConnectorHostingAttribute(validators ConnectorHostingValidators) schema.SingleNestedAttribute {
	hosting := ConnectorHostingAttribute(validators)
	azureOnly := `Required for, and only valid with, ` + "`azure`" + ` hosting.`

	hosting.Attributes["type"] = schema.StringAttribute{
		MarkdownDescription: `The connector's hosting: ` + "`aws`" + ` (Lambda), ` + "`gcp`" + ` (Cloud Run) or ` + "`azure`" + ` (Container Apps)`,
		Required:            true,
		Validators: []validator.String{
			stringvalidator.OneOf(AwsHosting, GcpHosting, AzureHosting),
		},
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.RequiresReplace(),
		},
	}
	hosting.Attributes["directory_id"] = schema.StringAttribute{
		MarkdownDescription: `The Microsoft Entra directory (tenant) ID in which the connector's Container App is deployed. P0's Azure integration must be installed for it. ` + azureOnly,
		Optional:            true,
		Validators: []validator.String{
			stringvalidator.RegexMatches(common.UuidRegex, "Azure directory IDs must be UUIDs"),
		},
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.RequiresReplace(),
		},
	}
	hosting.Attributes["subscription_id"] = schema.StringAttribute{
		MarkdownDescription: `The Azure subscription ID in which the connector's Container App is deployed. ` + azureOnly,
		Optional:            true,
		Validators: []validator.String{
			stringvalidator.RegexMatches(common.UuidRegex, "Azure subscription IDs must be UUIDs"),
		},
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.RequiresReplace(),
		},
	}
	hosting.Attributes["resource_group"] = schema.StringAttribute{
		MarkdownDescription: `The resource group in which the connector's Container App is deployed. ` + azureOnly,
		Optional:            true,
		Validators: []validator.String{
			stringvalidator.RegexMatches(ResourceGroupRegex, "Must be a valid Azure resource group name"),
		},
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.RequiresReplace(),
		},
	}
	hosting.Attributes["connector_name"] = requiredAddressAttribute(
		`The name of the Lambda function, Cloud Run service or Container App hosting the connector`,
		validators.ConnectorName,
	)
	hosting.Attributes["connector_region"] = requiredAddressAttribute(
		`The region the connector is deployed in (e.g. `+"`us-east-1`"+` on AWS, `+"`us-central1`"+` on Google Cloud, or `+"`eastus`"+` on Azure)`,
		validators.ConnectorRegion,
	)
	hosting.Attributes["connector_service_uri"] = schema.StringAttribute{
		MarkdownDescription: `The connector's invocation URL, resolved by P0 during install. Only populated for ` + "`gcp`" + ` and ` + "`azure`" + ` hosting.`,
		Computed:            true,
	}
	hosting.Attributes["connector_client_id"] = schema.StringAttribute{
		MarkdownDescription: `The client ID of the connector's app registration, which P0 requests its tokens for. P0 reads it from the Container App during install. Only populated for ` + "`azure`" + ` hosting.`,
		Computed:            true,
	}
	return hosting
}

// A required address field that only a new install can set. p0_app describes its
// connector_name and connector_region apart from the shared block, to name Azure.
func requiredAddressAttribute(description string, validators []validator.String) schema.StringAttribute {
	return schema.StringAttribute{
		MarkdownDescription: description,
		Required:            true,
		Validators:          validators,
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.RequiresReplace(),
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

// ConnectorHostingWithServiceAccountFromConfig is ConnectorHostingFromConfig for a
// `hosting` attribute made by ConnectorHostingWithServiceAccountAttribute. Reading an
// object into a struct fails unless their fields match, so that attribute needs a
// reader of its own. ValidateConnectorHosting takes the embedded ConnectorHostingModel
// of a non-nil result.
func ConnectorHostingWithServiceAccountFromConfig(ctx context.Context, config tfsdk.Config, diags *diag.Diagnostics) *ConnectorHostingWithServiceAccountModel {
	var hosting ConnectorHostingWithServiceAccountModel
	if !common.GetKnownObject(ctx, config, path.Root("hosting"), &hosting, diags) {
		return nil
	}
	return &hosting
}

// AppConnectorHostingFromConfig is ConnectorHostingFromConfig for a `hosting` attribute
// made by AppConnectorHostingAttribute.
func AppConnectorHostingFromConfig(ctx context.Context, config tfsdk.Config, diags *diag.Diagnostics) *AppConnectorHostingModel {
	var hosting AppConnectorHostingModel
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

// ValidateAppConnectorHosting is ValidateConnectorHosting for p0_app's hosting, which
// may also be `azure`. The Azure address fields are required with `azure`, and refused
// with the other types, as the AWS and Google Cloud ones are.
func ValidateAppConnectorHosting(hosting *AppConnectorHostingModel, diags *diag.Diagnostics) {
	if hosting == nil || hosting.Type.IsUnknown() {
		return
	}
	ValidateConnectorHosting(&hosting.ConnectorHostingModel, diags)

	azureFields := []struct {
		name  string
		label string
		value types.String
	}{
		{"directory_id", "Azure directory ID", hosting.DirectoryId},
		{"subscription_id", "Azure subscription ID", hosting.SubscriptionId},
		{"resource_group", "Azure resource group", hosting.ResourceGroup},
	}

	if hosting.Type.ValueString() != AzureHosting {
		for _, field := range azureFields {
			if IsSet(field.value) {
				diags.AddAttributeError(
					path.Root("hosting").AtName(field.name),
					"Unexpected "+field.label,
					fmt.Sprintf("'hosting.%s' may only be set when 'hosting.type' is \"azure\".", field.name),
				)
			}
		}
		return
	}

	for _, field := range azureFields {
		if field.value.IsNull() {
			diags.AddAttributeError(
				path.Root("hosting").AtName(field.name),
				"Missing "+field.label,
				fmt.Sprintf("'hosting.%s' is required when 'hosting.type' is \"azure\".", field.name),
			)
		}
	}
	if IsSet(hosting.AccountId) {
		diags.AddAttributeError(
			path.Root("hosting").AtName("account_id"),
			"Unexpected AWS account ID",
			"'hosting.account_id' may only be set when 'hosting.type' is \"aws\".",
		)
	}
	if IsSet(hosting.ProjectId) {
		diags.AddAttributeError(
			path.Root("hosting").AtName("project_id"),
			"Unexpected Google Cloud project",
			"'hosting.project_id' may only be set when 'hosting.type' is \"gcp\".",
		)
	}
}

// IsSet reports whether an optional value is present and resolved. An unknown value is
// not null, so checking IsNull alone would reject a field that interpolation has yet
// to fill in.
func IsSet(value types.String) bool {
	return !value.IsNull() && !value.IsUnknown()
}

// ToJson returns the hosting as P0 stores it. connector_service_account and
// connector_service_uri are left out, because P0 sets both. Both models send the same
// JSON.
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

// ToJson returns a custom application's hosting as P0 stores it. As with
// ConnectorHostingModel's, the fields P0 sets are left out: here connector_client_id
// as well as connector_service_uri.
func (m *AppConnectorHostingModel) ToJson() *ConnectorHostingJson {
	json := m.ConnectorHostingModel.ToJson()
	json.DirectoryId = m.DirectoryId.ValueStringPointer()
	json.SubscriptionId = m.SubscriptionId.ValueStringPointer()
	json.ResourceGroup = m.ResourceGroup.ValueStringPointer()
	return json
}

// AppConnectorHostingFromJson reads a custom application's hosting as P0 stores it.
// The Azure fields are null for the other types.
func AppConnectorHostingFromJson(json *ConnectorHostingJson) *AppConnectorHostingModel {
	return &AppConnectorHostingModel{
		ConnectorHostingModel: *ConnectorHostingFromJson(json),
		DirectoryId:           types.StringPointerValue(json.DirectoryId),
		SubscriptionId:        types.StringPointerValue(json.SubscriptionId),
		ResourceGroup:         types.StringPointerValue(json.ResourceGroup),
		ConnectorClientId:     types.StringPointerValue(json.ConnectorClientId),
	}
}

// ConnectorHostingWithServiceAccountFromJson reads the hosting P0 stores, along with
// the connector's service account. That is null on AWS, and when P0 generated none.
func ConnectorHostingWithServiceAccountFromJson(json *ConnectorHostingJson) *ConnectorHostingWithServiceAccountModel {
	return &ConnectorHostingWithServiceAccountModel{
		ConnectorHostingModel:   *ConnectorHostingFromJson(json),
		ConnectorServiceAccount: types.StringPointerValue(json.ConnectorServiceAccount),
	}
}
