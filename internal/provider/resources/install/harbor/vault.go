package installharbor

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
	installapp "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/app"
	installaws "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/aws"
	installvaultedcredential "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/vaulted-credential"
)

// The `vault` attribute is the app's vault-only install element, `newVaultElement` in
// packages/integrations/vaulted-credential: the secret manager installation in P0
// that holds the robot account's secret. It names no connector: the instance's
// `connector` does. github-repositories' vault block supports only AWS, so Harbor has
// its own.

// AWS Secrets Manager, as P0 names the vault type. Google Secret Manager's name is
// installvaultedcredential.GcpSecretManager.
//
// TODO(INT-417): this constant, vaultJson and vaultFromConfig are copies of
// github-repositories', whose vault block they can merge with.
const awsSecretsManager = "aws-sm"

// The vault, as P0 stores it. The JSON is a discriminated union on "type". "install"
// names the secret manager installation: the AWS account for `aws-sm`, and the Google
// Cloud project for `gcp-sm`.
type vaultJson struct {
	Type          string  `json:"type"`
	Install       string  `json:"install"`
	SecretsRegion *string `json:"secretsRegion,omitempty"`
}

// Terraform names the installation for what it is, as the hosting block does:
// account_id on AWS, project_id on Google Cloud.
type vaultModel struct {
	Type          types.String `tfsdk:"type"`
	AccountId     types.String `tfsdk:"account_id"`
	SecretsRegion types.String `tfsdk:"secrets_region"`
	ProjectId     types.String `tfsdk:"project_id"`
}

// The vault as P0 stores it, or nil for a type this provider doesn't support, which
// the type's validator refuses.
func (m *vaultModel) toJson() *vaultJson {
	switch m.Type.ValueString() {
	case installvaultedcredential.GcpSecretManager:
		return &vaultJson{Type: installvaultedcredential.GcpSecretManager, Install: m.ProjectId.ValueString()}
	case awsSecretsManager:
		return &vaultJson{
			Type:          awsSecretsManager,
			Install:       m.AccountId.ValueString(),
			SecretsRegion: m.SecretsRegion.ValueStringPointer(),
		}
	default:
		return nil
	}
}

// Reads the vault P0 stores, or returns nil for a type this provider doesn't support.
func vaultFromJson(json *vaultJson) *vaultModel {
	switch json.Type {
	case awsSecretsManager:
		return &vaultModel{
			Type:          types.StringValue(json.Type),
			AccountId:     types.StringValue(json.Install),
			SecretsRegion: types.StringPointerValue(json.SecretsRegion),
			ProjectId:     types.StringNull(),
		}
	case installvaultedcredential.GcpSecretManager:
		return &vaultModel{
			Type:          types.StringValue(json.Type),
			AccountId:     types.StringNull(),
			SecretsRegion: types.StringNull(),
			ProjectId:     types.StringValue(json.Install),
		}
	default:
		return nil
	}
}

// Every vault field is `step: "new"` in the app's install schema, so P0 refuses a
// change once the instance exists. RequiresReplace on each plans the replacement that
// works instead. ValidateConfig checks which fields the type takes, in validateVault.
func vaultAttribute() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		Required: true,
		MarkdownDescription: `The secret manager that holds the connector robot account's secret, as installed in P0. P0 never reads the secret. The connector does, ` +
			`so the secret manager must be in the connector's cloud: AWS Secrets Manager for a connector on AWS Lambda, Google Secret Manager for one on Google Cloud Run.`,
		Attributes: map[string]schema.Attribute{
			"type": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The secret manager: `aws-sm` (AWS Secrets Manager) or `gcp-sm` (Google Secret Manager)",
				Validators: []validator.String{
					stringvalidator.OneOf(awsSecretsManager, installvaultedcredential.GcpSecretManager),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"account_id": schema.StringAttribute{
				Optional: true,
				MarkdownDescription: "The ID of the AWS account that holds the secret: one of your AWS Secrets Manager installations in P0 (`p0_aws_secrets_manager`). " +
					"Required for, and only valid with, `aws-sm`.",
				Validators: []validator.String{
					stringvalidator.RegexMatches(installaws.AwsAccountIdRegex, "AWS account IDs should consist of 12 numeric digits"),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"secrets_region": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "The AWS region the secret is stored in, such as `us-west-2`. Required for, and only valid with, `aws-sm`.",
				Validators: []validator.String{
					stringvalidator.RegexMatches(installapp.ConnectorRegionRegex, "Must be a valid AWS region, such as us-west-2"),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"project_id": schema.StringAttribute{
				Optional: true,
				MarkdownDescription: "The ID of the Google Cloud project that holds the secret: one of your Google Secret Manager installations in P0. " +
					"This provider has no resource for that installation, so install Google Secret Manager in the P0 app first. " +
					"Required for, and only valid with, `gcp-sm`.",
				Validators: []validator.String{
					stringvalidator.RegexMatches(gcpProjectIdRegex, invalidGcpProjectId),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

// Reads the root `vault` attribute of a configuration, for ValidateConfig. It returns
// nil when the block is null, or unknown until apply.
func vaultFromConfig(ctx context.Context, config tfsdk.Config, diags *diag.Diagnostics) *vaultModel {
	var vault vaultModel
	if !common.GetKnownObject(ctx, config, path.Root("vault"), &vault, diags) {
		return nil
	}
	return &vault
}

// A vault field, and the vault type that takes it.
type vaultField struct {
	name      string
	vaultType string
	value     func(*vaultModel) types.String
}

var vaultFields = []vaultField{
	{"account_id", awsSecretsManager, func(v *vaultModel) types.String { return v.AccountId }},
	{"secrets_region", awsSecretsManager, func(v *vaultModel) types.String { return v.SecretsRegion }},
	{"project_id", installvaultedcredential.GcpSecretManager, func(v *vaultModel) types.String { return v.ProjectId }},
}

// A region outside AWS's commercial partition: GovCloud, China, the ISO regions or the
// European Sovereign Cloud. P0 has the connector read the secret by an `arn:aws:` ARN,
// so the secret can't be in one.
//
// TODO(INT-417): a copy of github-repositories' rule and message (hosting.go there),
// with Harbor's name in the message.
var awsOtherPartitionRegionRegex = regexp.MustCompile(`^(?:us-gov-|cn-|[a-z]+-iso[a-z]*-|eusc-)`)

const unsupportedAwsPartition = "Harbor doesn't support AWS GovCloud or China regions yet."

var secretsRegionRules = []rule{{
	summary: "Unsupported AWS region for the robot account's secret",
	message: unsupportedAwsPartition,
	test:    func(region string) bool { return !awsOtherPartitionRegionRegex.MatchString(region) },
}}

// Rejects vault fields that don't match the vault's type: a field the type needs that
// is missing, or one that only the other type takes. Each field's own validator checks
// its value. An unknown type is a value only resolved at apply time, and the type's
// own validator has already rejected anything that is known and unsupported.
func validateVault(vault *vaultModel, diags *diag.Diagnostics) {
	if vault == nil || !installapp.IsSet(vault.Type) {
		return
	}
	vaultType := vault.Type.ValueString()
	for _, field := range vaultFields {
		value := field.value(vault)
		switch {
		case field.vaultType == vaultType && value.IsNull():
			diags.AddAttributeError(
				path.Root("vault").AtName(field.name),
				"Missing vault field",
				"'vault."+field.name+"' is required when 'vault.type' is \""+vaultType+"\".",
			)
		case field.vaultType != vaultType && installapp.IsSet(value):
			diags.AddAttributeError(
				path.Root("vault").AtName(field.name),
				"Unexpected vault field",
				"'vault."+field.name+"' may only be set when 'vault.type' is \""+field.vaultType+"\".",
			)
		}
	}
	if vaultType == awsSecretsManager {
		validateField(path.Root("vault").AtName("secrets_region"), vault.SecretsRegion, secretsRegionRules, diags)
	}
}
