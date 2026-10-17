package installjira

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
	installapp "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/app"
	installvaultedcredential "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/vaulted-credential"
)

// The `vault` attribute is the app's vault-only install element, `newVaultElement` in
// packages/integrations/vaulted-credential, without its Azure Key Vault option: the
// secret manager installation in P0 that holds the site's OAuth client secret. It
// names no connector, because the `hosting` block places the connector.
//
// A copy of Harbor's vault block (harbor/vault.go), with the Jira client secret's
// messages.

// AWS Secrets Manager, as P0 names the vault type. Google Secret Manager's name is
// installvaultedcredential.GcpSecretManager.
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
// change once the site exists. RequiresReplace on each plans the replacement that
// works instead. ValidateConfig checks which fields the type takes, and P0's rules for
// each, in validateVault.
func vaultAttribute() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		Required: true,
		MarkdownDescription: "The secret manager that holds the site's OAuth client secret, as installed in P0. P0 never reads the secret. The connector does, " +
			"so the secret manager must be in the connector's cloud: AWS Secrets Manager for a connector on AWS Lambda, Google Secret Manager for one on Google Cloud Run.",
		Attributes: map[string]schema.Attribute{
			"type": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The secret manager: `aws-sm` (AWS Secrets Manager) or `gcp-sm` (Google Secret Manager). It must match `hosting.type`: `aws-sm` with `aws`, `gcp-sm` with `gcp`.",
				Validators: []validator.String{
					stringvalidator.OneOf(awsSecretsManager, installvaultedcredential.GcpSecretManager),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"account_id": schema.StringAttribute{
				Optional: true,
				MarkdownDescription: "The ID of the AWS account that holds the secret: one of your AWS Secrets Manager installations in P0. " +
					"It may be another account than the connector's. Required for, and only valid with, `aws-sm`.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"secrets_region": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "The AWS region the secret is stored in, such as `us-west-2`. Required for, and only valid with, `aws-sm`.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"project_id": schema.StringAttribute{
				Optional: true,
				MarkdownDescription: "The ID of the Google Cloud project that holds the secret: one of your Google Secret Manager installations in P0. " +
					"This provider has no resource for that installation, so install Google Secret Manager in the P0 app first. " +
					"Required for, and only valid with, `gcp-sm`.",
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

// P0's rules for the vault's fields, and its messages for a value that breaks one
// (packages/integrations/jira/backend/src/install/vault.ts in the app).
var (
	// A Google Cloud project's ID as the Jira connector takes one: it becomes a segment
	// of the path the secret is read at. The connector's own project has the same rule.
	gcpVaultProjectRegex = gcpProjectIdRegex
)

const (
	invalidAwsVaultAccount = "Choose the AWS Secrets Manager installation, by its 12-digit AWS account ID, that holds the Jira client secret."
	invalidAwsVaultRegion  = "Enter the AWS region the secret is in, such as us-west-2."
	invalidGcpVaultProject = "Choose the Google Secret Manager installation, by its Google Cloud project ID, that holds the Jira client secret."
)

// The rules for each vault type's fields, in the order P0 checks them.
var vaultRules = map[string][]fieldRules{
	awsSecretsManager: {
		{name: "account_id", rules: []rule{{summary: "Invalid AWS account ID for the client secret", message: invalidAwsVaultAccount, test: awsAccountRegex.MatchString}}},
		{name: "secrets_region", rules: awsRegionRules("the client secret", invalidAwsVaultRegion)},
	},
	installvaultedcredential.GcpSecretManager: {
		{name: "project_id", rules: []rule{{summary: "Invalid Google Cloud project for the client secret", message: invalidGcpVaultProject, test: gcpVaultProjectRegex.MatchString}}},
	},
}

// The vault's fields, by attribute name.
func (m *vaultModel) fields() map[string]types.String {
	values := map[string]types.String{}
	for _, field := range vaultFields {
		values[field.name] = field.value(m)
	}
	return values
}

// Rejects vault fields that don't match the vault's type, a field the type needs that
// is missing or one that only the other type takes, and each field that breaks P0's
// rule. An unknown type is a value only resolved at apply time, and the type's own
// validator has already rejected anything that is known and unsupported.
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
	validateFields(path.Root("vault"), vault.fields(), vaultRules[vaultType], diags)
}

// The cloud each vault type is in, as a hosting names it. The connector reads the
// site's secret with its own cloud's identity, so the vault has to be in the
// connector's cloud.
var vaultClouds = map[string]string{
	awsSecretsManager:                         installapp.AwsHosting,
	installvaultedcredential.GcpSecretManager: installapp.GcpHosting,
}

// P0's message for a new site whose vault is in another cloud than its connector, by
// where the connector runs (VAULT_FOR_CLOUD in
// packages/integrations/jira/backend/src/install/site.ts in the app).
var vaultForCloud = map[string]string{
	installapp.AwsHosting: "The connector runs on AWS, so the client secret has to be in AWS Secrets Manager.",
	installapp.GcpHosting: "The connector runs on Google Cloud, so the client secret has to be in Google Secret Manager.",
}

// Rejects a vault in another cloud than the connector. Unlike Harbor, Argo CD and
// Docker, whose vault and hosting are on different resources, a site holds both, so
// this can be checked at plan time. A vault or hosting whose type isn't known yet, or
// that a validator has already refused, is skipped.
func validateVaultCloud(vault *vaultModel, hosting *installapp.ConnectorHostingModel, diags *diag.Diagnostics) {
	if vault == nil || hosting == nil || !installapp.IsSet(vault.Type) || !installapp.IsSet(hosting.Type) {
		return
	}
	cloud, knownVault := vaultClouds[vault.Type.ValueString()]
	message, knownHosting := vaultForCloud[hosting.Type.ValueString()]
	if !knownVault || !knownHosting || cloud == hosting.Type.ValueString() {
		return
	}
	diags.AddAttributeError(path.Root("vault").AtName("type"), "Vault in another cloud than the connector", message)
}
