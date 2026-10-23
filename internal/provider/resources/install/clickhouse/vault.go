package installclickhouse

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

// copy of internal/provider/resources/install/auth0/vault.go (#245),
// itself a copy of Docker Hub's, renamed for ClickHouse Cloud (INT-695).
//
// The `vault` attribute is the organization's `vault` install element
// (`ClickHouseSecretVault` in packages/integrations/clickhouse/shared/src/components.ts
// in the app): the P0 secret manager installation for the secret that holds the API
// key, in the customer's own AWS Secrets Manager or Google Secret Manager. P0 stores
// where the secret is, never the secret.

const (
	awsSecretsManager = "aws-sm"
	gcpSecretManager  = "gcp-sm"
)

// The secret managers a vault can be: the one in each cloud a connector runs in.
var vaultTypes = []string{awsSecretsManager, gcpSecretManager}

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

// secret names what the vault holds, for the fields' descriptions.
func vaultAttribute(description string, secret string) schema.SingleNestedAttribute {
	// Every field below is `step: "new"` in the app's install schema, so P0 rejects a
	// change once the install exists. RequiresReplace on each plans the replacement
	// that P0 accepts instead, as the hosting block does.
	return schema.SingleNestedAttribute{
		MarkdownDescription: description,
		Required:            true,
		Attributes: map[string]schema.Attribute{
			"type": schema.StringAttribute{
				MarkdownDescription: `The secret manager: either ` + "`aws-sm`" + ` (AWS Secrets Manager) or ` + "`gcp-sm`" + ` (Google Secret Manager)`,
				Required:            true,
				Validators: []validator.String{
					stringvalidator.OneOf(vaultTypes...),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"account_id": schema.StringAttribute{
				MarkdownDescription: `The ID of the AWS account that holds ` + secret + ` (in the P0 app, one of your AWS Secrets Manager installations). Required for, and only valid with, ` + "`aws-sm`" + `.`,
				Optional:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"project_id": schema.StringAttribute{
				MarkdownDescription: `The ID of the Google Cloud project that holds ` + secret + ` (in the P0 app, one of your Google Secret Manager installations). Required for, and only valid with, ` + "`gcp-sm`" + `.`,
				Optional:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"secrets_region": schema.StringAttribute{
				MarkdownDescription: `The AWS region ` + secret + ` is stored in, such as ` + "`us-west-2`" + `. Required for, and only valid with, ` + "`aws-sm`" + `.`,
				Optional:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

// Returns nil when the block is null, or unknown until apply.
func vaultFromConfig(ctx context.Context, config tfsdk.Config, diags *diag.Diagnostics) *vaultModel {
	var vault vaultModel
	if !common.GetKnownObject(ctx, config, path.Root("vault"), &vault, diags) {
		return nil
	}
	return &vault
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
			diags.AddAttributeError(
				path.Root("vault").AtName("account_id"),
				"Missing AWS account ID",
				"'vault.account_id' is required when 'vault.type' is \"aws-sm\".",
			)
		}
		if vault.SecretsRegion.IsNull() {
			diags.AddAttributeError(
				path.Root("vault").AtName("secrets_region"),
				"Missing secrets region",
				"'vault.secrets_region' is required when 'vault.type' is \"aws-sm\".",
			)
		}
		if isSet(vault.ProjectId) {
			diags.AddAttributeError(
				path.Root("vault").AtName("project_id"),
				"Unexpected Google Cloud project",
				"'vault.project_id' may only be set when 'vault.type' is \"gcp-sm\".",
			)
		}
	case gcpSecretManager:
		if vault.ProjectId.IsNull() {
			diags.AddAttributeError(
				path.Root("vault").AtName("project_id"),
				"Missing Google Cloud project",
				"'vault.project_id' is required when 'vault.type' is \"gcp-sm\".",
			)
		}
		if isSet(vault.AccountId) {
			diags.AddAttributeError(
				path.Root("vault").AtName("account_id"),
				"Unexpected AWS account ID",
				"'vault.account_id' may only be set when 'vault.type' is \"aws-sm\".",
			)
		}
		if isSet(vault.SecretsRegion) {
			diags.AddAttributeError(
				path.Root("vault").AtName("secrets_region"),
				"Unexpected secrets region",
				"'vault.secrets_region' may only be set when 'vault.type' is \"aws-sm\".",
			)
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
