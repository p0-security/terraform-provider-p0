package installvaultedcredential

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
	installaws "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/aws"
	installgcp "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/gcp"
)

// The secret manager of an integration whose connector runs on AWS Lambda or Google
// Cloud Run. An integration opts in by using these types and attributes in place of
// the Google-only ones in secret_manager.go.
//
// copy of the vault fields and rules in internal/provider/resources/install/argocd/vault.go

const AwsSecretsManager = "aws-sm"

// P0 names the installation "install": the AWS account for `aws-sm`, the Google
// Cloud project for `gcp-sm`. It sets the connector fields: only connectorRegion on
// AWS, all of them on Google Cloud.
type SecretManagerWithAwsJson struct {
	SecretManagerJson
	SecretsRegion *string `json:"secretsRegion,omitempty"`
}

// Terraform names the installation for what it is: account_id on AWS, project_id on
// Google Cloud.
type SecretManagerWithAwsStagedModel struct {
	Type                    types.String `tfsdk:"type"`
	AccountId               types.String `tfsdk:"account_id"`
	SecretsRegion           types.String `tfsdk:"secrets_region"`
	ProjectId               types.String `tfsdk:"project_id"`
	ConnectorRegion         types.String `tfsdk:"connector_region"`
	ConnectorServiceName    types.String `tfsdk:"connector_service_name"`
	ConnectorServiceAccount types.String `tfsdk:"connector_service_account"`
}

type SecretManagerWithAwsModel struct {
	SecretManagerWithAwsStagedModel
	ConnectorServiceUri types.String `tfsdk:"connector_service_uri"`
}

func (m *SecretManagerWithAwsStagedModel) ToJson() *SecretManagerWithAwsJson {
	if m.Type.ValueString() == AwsSecretsManager {
		return &SecretManagerWithAwsJson{
			SecretManagerJson: SecretManagerJson{Type: AwsSecretsManager, Install: m.AccountId.ValueString()},
			SecretsRegion:     m.SecretsRegion.ValueStringPointer(),
		}
	}
	return &SecretManagerWithAwsJson{
		SecretManagerJson: SecretManagerJson{Type: m.Type.ValueString(), Install: m.ProjectId.ValueString()},
	}
}

func SecretManagerWithAwsStagedFromJson(json *SecretManagerWithAwsJson) SecretManagerWithAwsStagedModel {
	model := SecretManagerWithAwsStagedModel{
		Type:                    types.StringValue(json.Type),
		AccountId:               types.StringNull(),
		SecretsRegion:           types.StringNull(),
		ProjectId:               types.StringNull(),
		ConnectorRegion:         types.StringPointerValue(json.ConnectorRegion),
		ConnectorServiceName:    types.StringPointerValue(json.ConnectorServiceName),
		ConnectorServiceAccount: types.StringPointerValue(json.ConnectorServiceAccount),
	}
	if json.Type == AwsSecretsManager {
		model.AccountId = types.StringValue(json.Install)
		model.SecretsRegion = types.StringPointerValue(json.SecretsRegion)
	} else {
		model.ProjectId = types.StringValue(json.Install)
	}
	return model
}

func SecretManagerWithAwsFromJson(json *SecretManagerWithAwsJson) SecretManagerWithAwsModel {
	return SecretManagerWithAwsModel{
		SecretManagerWithAwsStagedModel: SecretManagerWithAwsStagedFromJson(json),
		ConnectorServiceUri:             types.StringPointerValue(json.ConnectorServiceUri),
	}
}

// RequireSecretManager, for an integration that opts in to AWS.
func RequireSecretManagerWithAws(diags *diag.Diagnostics, integration string, id string, json *SecretManagerWithAwsJson) bool {
	if json == nil {
		return RequireSecretManager(diags, integration, id, nil)
	}
	return RequireSecretManager(diags, integration, id, &json.SecretManagerJson)
}

// An AWS region, such as us-west-2.
var awsRegionRegex = regexp.MustCompile(`^[a-z]+(?:-[a-z]+)+-\d+$`)

func replacedString(attribute schema.StringAttribute) schema.StringAttribute {
	attribute.PlanModifiers = []planmodifier.String{stringplanmodifier.RequiresReplace()}
	return attribute
}

// P0 rejects a change to any of type, account_id, secrets_region or project_id after
// staging, so a change replaces the install. ValidateSecretManagerWithAws checks which
// of them the type takes.
//
// The secret label names what the install secret holds. Adds suffix to the
// descriptions of the fields that the install takes.
func SecretManagerWithAwsAttributes(secret string, suffix string) map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"type": replacedString(schema.StringAttribute{
			Required:            true,
			MarkdownDescription: "The secret manager that stores " + secret + ": `aws-sm` (AWS Secrets Manager, with the connector on AWS Lambda) or `gcp-sm` (Google Secret Manager, with the connector on Google Cloud Run)." + suffix,
			Validators: []validator.String{
				stringvalidator.OneOf(AwsSecretsManager, GcpSecretManager),
			},
		}),
		"account_id": replacedString(schema.StringAttribute{
			Optional:            true,
			MarkdownDescription: "The AWS account that hosts the connector's Lambda function and the secret that holds " + secret + ": one of your AWS Secrets Manager installations in P0. Required for, and only valid with, `aws-sm`." + suffix,
			Validators: []validator.String{
				stringvalidator.RegexMatches(installaws.AwsAccountIdRegex, "AWS account IDs should consist of 12 numeric digits"),
			},
		}),
		"secrets_region": replacedString(schema.StringAttribute{
			Optional:            true,
			MarkdownDescription: "The AWS region that stores the secrets, such as `us-west-2`. Required for, and only valid with, `aws-sm`." + suffix,
			Validators: []validator.String{
				stringvalidator.RegexMatches(awsRegionRegex, "Enter an AWS region, such as us-west-2"),
			},
		}),
		"project_id": replacedString(schema.StringAttribute{
			Optional:            true,
			MarkdownDescription: "The GCP project that hosts the connector's Cloud Run service and the secret that holds " + secret + ". Required for, and only valid with, `gcp-sm`." + suffix,
			Validators: []validator.String{
				stringvalidator.RegexMatches(installgcp.GcpProjectIdRegex, "GCP project IDs should consist only of alphanumeric characters and hyphens"),
			},
		}),
		"connector_region":          computedConnectorAttribute(`The region in which to deploy the connector: its Lambda function's on AWS, its Cloud Run service's on Google Cloud`),
		"connector_service_name":    computedConnectorAttribute(`The name to give the connector's Cloud Run service. Google Cloud only.`),
		"connector_service_account": computedConnectorAttribute(`The email of the GCP service account that the connector must run as. Google Cloud only.`),
	}
}

// Adds the connector service URI that P0 resolves when it completes the install.
func FinalSecretManagerWithAwsAttributes(secret string, suffix string) map[string]schema.Attribute {
	attributes := SecretManagerWithAwsAttributes(secret, suffix)
	attributes["connector_service_uri"] = schema.StringAttribute{
		Computed:            true,
		MarkdownDescription: `The invocation URL of the connector's Cloud Run service, resolved by P0 during install. Google Cloud only.`,
	}
	return attributes
}

// A secret manager field, and the type that takes it.
type secretManagerField struct {
	name  string
	type_ string
}

var secretManagerFields = []secretManagerField{
	{"account_id", AwsSecretsManager},
	{"secrets_region", AwsSecretsManager},
	{"project_id", GcpSecretManager},
}

func isSet(value types.String) bool {
	return !value.IsNull() && !value.IsUnknown()
}

// The string attribute name of a secret_manager object, or null if it has none.
func stringAttribute(object types.Object, name string) types.String {
	if value, ok := object.Attributes()[name].(types.String); ok {
		return value
	}
	return types.StringNull()
}

// Rejects secret_manager fields that don't match its type: a field the type needs
// that is missing, or one that only the other type takes. Each field's own validator
// checks its value. For a resource's ValidateConfig.
//
// It reads the fields one by one, not into a model, because the staged and the final
// resource's secret_manager have different attributes.
func ValidateSecretManagerWithAws(ctx context.Context, config tfsdk.Config, diags *diag.Diagnostics) {
	var object types.Object
	getDiags := config.GetAttribute(ctx, path.Root("secret_manager"), &object)
	diags.Append(getDiags...)
	if getDiags.HasError() || object.IsNull() || object.IsUnknown() {
		return
	}
	typeValue := stringAttribute(object, "type")
	if !isSet(typeValue) {
		return
	}
	type_ := typeValue.ValueString()
	for _, field := range secretManagerFields {
		value := stringAttribute(object, field.name)
		switch {
		case field.type_ == type_ && value.IsNull():
			diags.AddAttributeError(
				path.Root("secret_manager").AtName(field.name),
				"Missing secret manager field",
				"'secret_manager."+field.name+"' is required when 'secret_manager.type' is \""+type_+"\".",
			)
		case field.type_ != type_ && isSet(value):
			diags.AddAttributeError(
				path.Root("secret_manager").AtName(field.name),
				"Unexpected secret manager field",
				"'secret_manager."+field.name+"' may only be set when 'secret_manager.type' is \""+field.type_+"\".",
			)
		}
	}
}
