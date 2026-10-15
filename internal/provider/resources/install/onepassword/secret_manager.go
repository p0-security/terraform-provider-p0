package installonepassword

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

// The `secret manager` attribute is the vault's `secret manager` install element
// (`OnePasswordTokenVault` in packages/integrations/one-password/shared/src/components.ts in the
// app): the P0 secret manager installation that holds the service account token's
// secret, in the customer's own AWS Secrets Manager or Google Secret Manager. P0 stores
// where the secret is, never the token.

const (
	awsSecretsManager = "aws-sm"
	gcpSecretManager  = "gcp-sm"
)

// The secret managers a secret manager can be: the one in each cloud a connector runs in.
var secretManagerTypes = []string{awsSecretsManager, gcpSecretManager}

// The secret manager, as P0 stores it. The JSON is a discriminated union on "type". "install"
// names the secret manager installation: the AWS account for `aws-sm`, which also
// carries the secrets' region, and the Google Cloud project for `gcp-sm`.
type secretManagerJson struct {
	Type          string  `json:"type"`
	Install       string  `json:"install"`
	SecretsRegion *string `json:"secretsRegion,omitempty"`
}

// Terraform names the installation for what it is, account_id or project_id, as the
// hosting block does.
type secretManagerModel struct {
	Type          types.String `tfsdk:"type"`
	AccountId     types.String `tfsdk:"account_id"`
	ProjectId     types.String `tfsdk:"project_id"`
	SecretsRegion types.String `tfsdk:"secrets_region"`
}

// The required `secret manager` attribute. description describes the secret manager, and secret names
// what it holds in the fields' descriptions.
func secretManagerAttribute(description string, secret string) schema.SingleNestedAttribute {
	// Every field below is `step: "new"` in the app's install schema, so P0 rejects a
	// change once the install exists. RequiresReplace on each plans the replacement
	// that works instead, as the hosting block does.
	return schema.SingleNestedAttribute{
		MarkdownDescription: description,
		Required:            true,
		Attributes: map[string]schema.Attribute{
			"type": schema.StringAttribute{
				MarkdownDescription: `The secret manager: either ` + "`aws-sm`" + ` (AWS Secrets Manager) or ` + "`gcp-sm`" + ` (Google Secret Manager)`,
				Required:            true,
				Validators: []validator.String{
					stringvalidator.OneOf(secretManagerTypes...),
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

// Reads the root `secret manager` attribute of a vault's configuration, for
// ValidateConfig. It returns nil when the block is null, or unknown until apply.
func secretManagerFromConfig(ctx context.Context, config tfsdk.Config, diags *diag.Diagnostics) *secretManagerModel {
	var secretManager secretManagerModel
	if !common.GetKnownObject(ctx, config, path.Root("secret_manager"), &secretManager, diags) {
		return nil
	}
	return &secretManager
}

// Rejects a secret manager block whose fields don't match its type: a missing account, project
// or region, or one set for the other secret manager.
func validateSecretManager(secretManager *secretManagerModel, diags *diag.Diagnostics) {
	// An unknown type is a value only resolved at apply time. The type's own validator
	// has already rejected anything that is known and unsupported.
	if secretManager == nil || secretManager.Type.IsUnknown() {
		return
	}

	switch secretManager.Type.ValueString() {
	case awsSecretsManager:
		if secretManager.AccountId.IsNull() {
			diags.AddAttributeError(
				path.Root("secret_manager").AtName("account_id"),
				"Missing AWS account ID",
				"'secret_manager.account_id' is required when 'secret_manager.type' is \"aws-sm\".",
			)
		}
		if secretManager.SecretsRegion.IsNull() {
			diags.AddAttributeError(
				path.Root("secret_manager").AtName("secrets_region"),
				"Missing secrets region",
				"'secret_manager.secrets_region' is required when 'secret_manager.type' is \"aws-sm\".",
			)
		}
		if isSet(secretManager.ProjectId) {
			diags.AddAttributeError(
				path.Root("secret_manager").AtName("project_id"),
				"Unexpected Google Cloud project",
				"'secret_manager.project_id' may only be set when 'secret_manager.type' is \"gcp-sm\".",
			)
		}
	case gcpSecretManager:
		if secretManager.ProjectId.IsNull() {
			diags.AddAttributeError(
				path.Root("secret_manager").AtName("project_id"),
				"Missing Google Cloud project",
				"'secret_manager.project_id' is required when 'secret_manager.type' is \"gcp-sm\".",
			)
		}
		if isSet(secretManager.AccountId) {
			diags.AddAttributeError(
				path.Root("secret_manager").AtName("account_id"),
				"Unexpected AWS account ID",
				"'secret_manager.account_id' may only be set when 'secret_manager.type' is \"aws-sm\".",
			)
		}
		if isSet(secretManager.SecretsRegion) {
			diags.AddAttributeError(
				path.Root("secret_manager").AtName("secrets_region"),
				"Unexpected secrets region",
				"'secret_manager.secrets_region' may only be set when 'secret_manager.type' is \"aws-sm\".",
			)
		}
	}
}

// The secret manager as P0 stores it.
func (m *secretManagerModel) toJson() *secretManagerJson {
	json := secretManagerJson{
		Type:          m.Type.ValueString(),
		Install:       m.AccountId.ValueString(),
		SecretsRegion: m.SecretsRegion.ValueStringPointer(),
	}
	if json.Type == gcpSecretManager {
		json.Install = m.ProjectId.ValueString()
	}
	return &json
}

// The secret manager's fields, by attribute name.
func (m *secretManagerModel) fields() map[string]types.String {
	return map[string]types.String{
		"account_id":     m.AccountId,
		"project_id":     m.ProjectId,
		"secrets_region": m.SecretsRegion,
	}
}

// Reads the secret manager P0 stores.
func secretManagerFromJson(json *secretManagerJson) *secretManagerModel {
	model := secretManagerModel{
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
