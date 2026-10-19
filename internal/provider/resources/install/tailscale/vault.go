package installtailscale

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/p0-security/terraform-provider-p0/internal/common"
)

// copy of internal/provider/resources/install/auth0/vault.go @ 663ab81 (INT-590),
// renamed for Tailscale (INT-686). The vault sits in the tailnet's credential, so its
// paths are under `credential`, and its fields don't require replacement: the tailnet
// refuses a change to its credential instead (see refuseFixedChanges).
//
// The `vault` attribute is the OAuth client credential's `vault` install element
// (`TailscaleSecretVault` in packages/integrations/tailscale/shared/src/components.ts in
// the app): the P0 secret manager installation for the secret that holds the OAuth
// client's secret, in the customer's own AWS Secrets Manager or Google Secret Manager.
// P0 stores where the secret is, never the secret.

const (
	awsSecretsManager = "aws-sm"
	gcpSecretManager  = "gcp-sm"
)

// The secret managers a vault can be: the one in each cloud a connector runs in.
var vaultTypes = []string{awsSecretsManager, gcpSecretManager}

// Where the vault is in the tailnet's configuration.
var vaultPath = credentialPath.AtName("vault")

// The JSON is a discriminated union on "type". "install" names the secret manager
// installation: the AWS account for `aws-sm`, which also carries the secrets' region,
// and the Google Cloud project for `gcp-sm`.
type vaultJson struct {
	Type          string  `json:"type"`
	Install       string  `json:"install"`
	SecretsRegion *string `json:"secretsRegion,omitempty"`
}

// Terraform names the installation for what it is, account_id or project_id, as the
// hosting block does.
type vaultModel struct {
	Type          types.String `tfsdk:"type"`
	AccountId     types.String `tfsdk:"account_id"`
	ProjectId     types.String `tfsdk:"project_id"`
	SecretsRegion types.String `tfsdk:"secrets_region"`
}

// secret names what the vault holds, for the fields' descriptions. Only an OAuth client
// credential has a vault, so the attribute is optional.
func vaultAttribute(description string, secret string) schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		MarkdownDescription: description,
		Optional:            true,
		Attributes: map[string]schema.Attribute{
			"type": schema.StringAttribute{
				MarkdownDescription: `The secret manager: either ` + "`aws-sm`" + ` (AWS Secrets Manager) or ` + "`gcp-sm`" + ` (Google Secret Manager)`,
				Required:            true,
				Validators: []validator.String{
					stringvalidator.OneOf(vaultTypes...),
				},
			},
			"account_id": schema.StringAttribute{
				MarkdownDescription: `The ID of the AWS account that holds ` + secret + ` (in the P0 app, one of your AWS Secrets Manager installations). Required for, and only valid with, ` + "`aws-sm`" + `.`,
				Optional:            true,
			},
			"project_id": schema.StringAttribute{
				MarkdownDescription: `The ID of the Google Cloud project that holds ` + secret + ` (in the P0 app, one of your Google Secret Manager installations). Required for, and only valid with, ` + "`gcp-sm`" + `.`,
				Optional:            true,
			},
			"secrets_region": schema.StringAttribute{
				MarkdownDescription: `The AWS region ` + secret + ` is stored in, such as ` + "`us-west-2`" + `. Required for, and only valid with, ` + "`aws-sm`" + `.`,
				Optional:            true,
			},
		},
	}
}

// Returns nil when the block is null, or unknown until apply.
func vaultFromConfig(ctx context.Context, config tfsdk.Config, diags *diag.Diagnostics) *vaultModel {
	var vault vaultModel
	if !common.GetKnownObject(ctx, config, vaultPath, &vault, diags) {
		return nil
	}
	return &vault
}

// The message for a vault field that its type needs but the configuration leaves out.
func missingVaultField(field string, vaultType string) string {
	return fmt.Sprintf("'%s' is required when '%s' is %q.", vaultPath.AtName(field), vaultPath.AtName("type"), vaultType)
}

// The message for a vault field that belongs to another type.
func unexpectedVaultField(field string, vaultType string) string {
	return fmt.Sprintf("'%s' may only be set when '%s' is %q.", vaultPath.AtName(field), vaultPath.AtName("type"), vaultType)
}

// Rejects a vault block whose fields don't match its type: a missing account, project
// or region, or one set for the other secret manager.
func validateVault(vault *vaultModel, diags *diag.Diagnostics) {
	// An unknown type resolves only at apply time. The type's own validator has already
	// rejected a known type the provider doesn't take.
	if vault == nil || vault.Type.IsUnknown() {
		return
	}

	switch vault.Type.ValueString() {
	case awsSecretsManager:
		if vault.AccountId.IsNull() {
			diags.AddAttributeError(vaultPath.AtName("account_id"), "Missing AWS account ID", missingVaultField("account_id", awsSecretsManager))
		}
		if vault.SecretsRegion.IsNull() {
			diags.AddAttributeError(vaultPath.AtName("secrets_region"), "Missing secrets region", missingVaultField("secrets_region", awsSecretsManager))
		}
		if isSet(vault.ProjectId) {
			diags.AddAttributeError(vaultPath.AtName("project_id"), "Unexpected Google Cloud project", unexpectedVaultField("project_id", gcpSecretManager))
		}
	case gcpSecretManager:
		if vault.ProjectId.IsNull() {
			diags.AddAttributeError(vaultPath.AtName("project_id"), "Missing Google Cloud project", missingVaultField("project_id", gcpSecretManager))
		}
		if isSet(vault.AccountId) {
			diags.AddAttributeError(vaultPath.AtName("account_id"), "Unexpected AWS account ID", unexpectedVaultField("account_id", awsSecretsManager))
		}
		if isSet(vault.SecretsRegion) {
			diags.AddAttributeError(vaultPath.AtName("secrets_region"), "Unexpected secrets region", unexpectedVaultField("secrets_region", awsSecretsManager))
		}
	}
}

func (m *vaultModel) toJson() *vaultJson {
	json := vaultJson{
		Type:          m.Type.ValueString(),
		Install:       m.AccountId.ValueString(),
		SecretsRegion: m.SecretsRegion.ValueStringPointer(),
	}
	if json.Type == gcpSecretManager {
		json.Install = m.ProjectId.ValueString()
	}
	return &json
}

func (m *vaultModel) fields() map[string]types.String {
	return map[string]types.String{
		"account_id":     m.AccountId,
		"project_id":     m.ProjectId,
		"secrets_region": m.SecretsRegion,
	}
}

func vaultFromJson(json *vaultJson) *vaultModel {
	model := vaultModel{
		Type:          types.StringValue(json.Type),
		AccountId:     types.StringNull(),
		ProjectId:     types.StringNull(),
		SecretsRegion: types.StringPointerValue(json.SecretsRegion),
	}
	if json.Type == gcpSecretManager {
		model.ProjectId = types.StringValue(json.Install)
	} else {
		model.AccountId = types.StringValue(json.Install)
	}
	return &model
}
