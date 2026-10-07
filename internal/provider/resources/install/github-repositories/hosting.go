package installgithubrepositories

import (
	"context"
	"fmt"
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
	installapp "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/app"
	installaws "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/aws"
)

// The `hosting` attribute is the app's `ConnectorHosting` install element
// (packages/integrations/app/src/shared/components.ts), as p0_app's is, and P0 stores
// it in the same JSON. This resource supports only the AWS variant for now, a Lambda
// function, so its attribute has only that variant's fields.

type hostingModel struct {
	Type            types.String `tfsdk:"type"`
	AccountId       types.String `tfsdk:"account_id"`
	ConnectorName   types.String `tfsdk:"connector_name"`
	ConnectorRegion types.String `tfsdk:"connector_region"`
}

func (m *hostingModel) toJson() *installapp.ConnectorHostingJson {
	return &installapp.ConnectorHostingJson{
		Type:            m.Type.ValueString(),
		AccountId:       m.AccountId.ValueStringPointer(),
		ConnectorName:   m.ConnectorName.ValueString(),
		ConnectorRegion: m.ConnectorRegion.ValueString(),
	}
}

func hostingFromJson(json *installapp.ConnectorHostingJson) *hostingModel {
	return &hostingModel{
		Type:            types.StringValue(json.Type),
		AccountId:       types.StringPointerValue(json.AccountId),
		ConnectorName:   types.StringValue(json.ConnectorName),
		ConnectorRegion: types.StringValue(json.ConnectorRegion),
	}
}

// Every address field is `step: "new"` in the app's install schema, so P0 rejects a
// change once the install exists. RequiresReplace on each plans the replacement that
// works instead. P0's rules for the fields are checked in ValidateConfig, in
// validateDeployable, so the fields have no validators of their own.
func hostingAttribute() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		Required:            true,
		MarkdownDescription: `Where your connector is deployed, and how P0 addresses it. Only AWS Lambda is supported for now.`,
		Attributes: map[string]schema.Attribute{
			"type": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: `The connector's hosting: ` + "`aws`" + ` (AWS Lambda), the only one supported for now`,
				Validators: []validator.String{
					stringvalidator.OneOf(installapp.AwsHosting),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"account_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: `The ID of the AWS account the connector's Lambda function is deployed in`,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"connector_name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: `The name of the Lambda function hosting the connector`,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"connector_region": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: `The AWS region the connector is deployed in, such as ` + "`us-east-1`",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

// Reads the root `hosting` attribute of a configuration, for ValidateConfig. It returns
// nil when the block is null, or unknown until apply.
func hostingFromConfig(ctx context.Context, config tfsdk.Config, diags *diag.Diagnostics) *hostingModel {
	var hosting hostingModel
	if !common.GetKnownObject(ctx, config, path.Root("hosting"), &hosting, diags) {
		return nil
	}
	return &hosting
}

// Where the connector can run and where the key's secret can be: P0's rules for the
// hosting and vault fields, which it checks when it creates the item
// (packages/integrations/github-repositories-shared/src/hosting.ts in the app). The
// connector's Terraform can't be deployed with a value that breaks one. The patterns
// and the messages are P0's.

// The regions come from installaws: P0 invokes the connector by an `arn:aws:` ARN, so
// neither the connector nor its secret can be outside AWS's commercial partition.
var (
	// A connector name that works for everything the connector's Terraform names after
	// it: a Lambda function with its ECR repository and IAM role. ECR is why a name has
	// at least two characters and no two hyphens in a row.
	connectorNamePattern = regexp.MustCompile(`^[a-z](?:-?[a-z0-9])+$`)
	awsAccountPattern    = regexp.MustCompile(`^\d{12}$`)
)

// The longest connector name for each hosting: Lambda and IAM cap a function or role
// name at 64 characters.
var connectorNameMaxLength = map[string]int{installapp.AwsHosting: 64}

const UnsupportedAwsPartition = "GitHub Repositories doesn't support AWS GovCloud or China regions yet."

// A rule for a value, and P0's message for a value that breaks it.
type rule struct {
	summary string
	message string
	test    func(value string) bool
}

// A field of a hosting or vault block, by its attribute name, and its rules.
type fieldRules struct {
	name  string
	rules []rule
}

// What a field places: the connector, or the key's secret. P0's messages say where it
// is, and a summary says what, since Terraform shows a nested attribute's error at its
// resource rather than at the attribute.
type place struct {
	what  string
	where string
}

var (
	connectorPlace = place{what: "the connector", where: "the connector runs in"}
	secretPlace    = place{what: "the private key's secret", where: "the private key's secret is in"}
)

func connectorNameRules(hostingType string) []rule {
	maxLength := connectorNameMaxLength[hostingType]
	return []rule{{
		summary: "Invalid connector name",
		message: fmt.Sprintf("Use 2 to %d characters for the connector name: lowercase letters, digits and single hyphens, "+
			"starting with a letter and ending with a letter or digit.", maxLength),
		test: func(name string) bool { return connectorNamePattern.MatchString(name) && len(name) <= maxLength },
	}}
}

func awsAccountRules(p place) []rule {
	return []rule{{
		summary: "Invalid AWS account ID for " + p.what,
		message: fmt.Sprintf("Enter the 12-digit ID of the AWS account %s.", p.where),
		test:    awsAccountPattern.MatchString,
	}}
}

func awsRegionRules(p place) []rule {
	return []rule{
		{
			summary: "Invalid AWS region for " + p.what,
			message: fmt.Sprintf("Enter the AWS region %s, such as us-west-2.", p.where),
			test:    installaws.AwsRegionRegex.MatchString,
		},
		{
			summary: "Unsupported AWS region for " + p.what,
			message: UnsupportedAwsPartition,
			test:    func(region string) bool { return !installaws.AwsOtherPartitionRegionRegex.MatchString(region) },
		},
	}
}

// The rules for each hosting type's fields, in the order P0 checks them.
var hostingRules = map[string][]fieldRules{
	installapp.AwsHosting: {
		{name: "account_id", rules: awsAccountRules(connectorPlace)},
		{name: "connector_name", rules: connectorNameRules(installapp.AwsHosting)},
		{name: "connector_region", rules: awsRegionRules(connectorPlace)},
	},
}

// The rules for each vault type's fields, in the order P0 checks them.
var vaultRules = map[string][]fieldRules{
	AwsSecretsManager: {
		{name: "account_id", rules: awsAccountRules(secretPlace)},
		{name: "secrets_region", rules: awsRegionRules(secretPlace)},
	},
}

// Rejects each hosting and vault field that P0 refuses, for where the connector runs
// or where the key's secret is. A block of a type that isn't known yet is skipped.
func validateDeployable(vault *vaultModel, hosting *hostingModel, diags *diag.Diagnostics) {
	if hosting != nil && installapp.IsSet(hosting.Type) {
		validateFields(path.Root("hosting"), hostingFields(hosting), hostingRules[hosting.Type.ValueString()], diags)
	}
	if vault != nil && installapp.IsSet(vault.Type) {
		validateFields(path.Root("vault"), vault.fields(), vaultRules[vault.Type.ValueString()], diags)
	}
}

func validateFields(block path.Path, values map[string]types.String, fields []fieldRules, diags *diag.Diagnostics) {
	for _, field := range fields {
		validateField(block.AtName(field.name), values[field.name], field.rules, diags)
	}
}

// The hosting's address fields, by attribute name.
func hostingFields(hosting *hostingModel) map[string]types.String {
	return map[string]types.String{
		"account_id":       hosting.AccountId,
		"connector_name":   hosting.ConnectorName,
		"connector_region": hosting.ConnectorRegion,
	}
}
