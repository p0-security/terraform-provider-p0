package installawsssmai

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

// A copy of the Argo CD install resources' vault.go, trimmed to AWS Secrets
// Manager.
//
// The `vault` attribute is the app's vault element, `newVaultElement` in
// packages/integrations/vaulted-credential, without its Azure Key Vault and
// Google Secret Manager options: the AWS Secrets Manager installation in P0
// where the connector keeps each agent's SSH key.

// AWS Secrets Manager, as P0 names the vault type.
const awsSecretsManager = "aws-sm"

// The vault, as P0 stores it. "install" names the secret manager installation,
// which is an AWS account.
type vaultJson struct {
	Type          string  `json:"type"`
	Install       string  `json:"install"`
	SecretsRegion *string `json:"secretsRegion,omitempty"`
}

// Terraform names the installation for what it is, an AWS account.
type vaultModel struct {
	Type          types.String `tfsdk:"type"`
	AccountId     types.String `tfsdk:"account_id"`
	SecretsRegion types.String `tfsdk:"secrets_region"`
}

// The vault as P0 stores it, or nil for no vault.
func (m *vaultModel) toJson() *vaultJson {
	if m == nil {
		return nil
	}
	return &vaultJson{
		Type:          m.Type.ValueString(),
		Install:       m.AccountId.ValueString(),
		SecretsRegion: m.SecretsRegion.ValueStringPointer(),
	}
}

// Reads the vault P0 stores, or returns nil when P0 has none.
func vaultFromJson(json *vaultJson) *vaultModel {
	if json == nil {
		return nil
	}
	return &vaultModel{
		Type:          types.StringValue(json.Type),
		AccountId:     types.StringValue(json.Install),
		SecretsRegion: types.StringPointerValue(json.SecretsRegion),
	}
}

// A vault field that P0 fixes once the connector is staged, since each is
// `step: "new"` in the app's install schema.
func vaultFieldAttribute(description string, validators ...validator.String) schema.StringAttribute {
	return schema.StringAttribute{
		Required:            true,
		MarkdownDescription: description,
		Validators:          validators,
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.RequiresReplace(),
		},
	}
}

func vaultAttribute() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		Required: true,
		MarkdownDescription: "The secret manager where the connector keeps each agent's SSH key, as installed in P0. " +
			"The connector writes each key, and the agent's MCP server reads it through AWS OIDC, only in the connector's own AWS account, so this must be that account's AWS Secrets Manager installation. " +
			"Changing it replaces the connector.",
		Attributes: map[string]schema.Attribute{
			"type": vaultFieldAttribute(
				"The secret manager. The only value is `aws-sm` (AWS Secrets Manager).",
				stringvalidator.OneOf(awsSecretsManager),
			),
			"account_id": vaultFieldAttribute(
				"The ID of the AWS account whose AWS Secrets Manager installation in P0 (`p0_aws_secrets_manager`) keeps the keys. It must be the connector's `id`.",
				awsAccountIdValidator(),
			),
			"secrets_region": vaultFieldAttribute(
				"The AWS region the connector keeps every key in, such as `us-west-2`. It must be a commercial AWS region. The connector's role may manage secrets in this region only.",
				commercialRegionValidator(invalidSecretsRegion),
			),
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

// Rejects a vault outside the connector's AWS account, with the message of
// the app's stage hook, since the connector and the agents' roles reach
// secrets only there. A value that isn't known yet is skipped.
func validateVaultAccount(id types.String, vault *vaultModel, diags *diag.Diagnostics) {
	if id.IsNull() || id.IsUnknown() || vault.AccountId.IsNull() || vault.AccountId.IsUnknown() {
		return
	}
	if vault.AccountId.ValueString() == id.ValueString() {
		return
	}
	diags.AddAttributeError(
		path.Root("vault").AtName("account_id"),
		"Wrong AWS Secrets Manager installation",
		fmt.Sprintf("Choose the connector's own AWS account, %q, as the AWS Secrets Manager installation, since the connector and the agents' roles reach secrets only there. "+
			"If P0 has no AWS Secrets Manager installation for that account, install P0's AWS Secrets Manager integration for it first (see p0_aws_secrets_manager).", id.ValueString()),
	)
}
