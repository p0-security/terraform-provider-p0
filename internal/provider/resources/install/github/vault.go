package installgithubrepositories

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

// The `vault` attribute is the app's vault-only install element, `newVaultElement` in
// packages/integrations/vaulted-credential: the P0 secret manager installation that
// holds the GitHub App's private key. Unlike the vaulted-credential secret manager
// block, it names no connector, because the `hosting` block places the connector.
//
// This resource supports only AWS Secrets Manager for now.

const AwsSecretsManager = "aws-sm"

// The vault, as P0 stores it. The JSON is a discriminated union on "type". "install"
// names the secret manager installation, which is the AWS account for `aws-sm`.
type vaultJson struct {
	Type          string  `json:"type"`
	Install       string  `json:"install"`
	SecretsRegion *string `json:"secretsRegion,omitempty"`
}

// Terraform names the installation for what it is, account_id, as the hosting block
// does.
type vaultModel struct {
	Type          types.String `tfsdk:"type"`
	AccountId     types.String `tfsdk:"account_id"`
	SecretsRegion types.String `tfsdk:"secrets_region"`
}

func (m *vaultModel) toJson() *vaultJson {
	return &vaultJson{
		Type:          m.Type.ValueString(),
		Install:       m.AccountId.ValueString(),
		SecretsRegion: m.SecretsRegion.ValueStringPointer(),
	}
}

// The vault's fields, by attribute name.
func (m *vaultModel) fields() map[string]types.String {
	return map[string]types.String{
		"account_id":     m.AccountId,
		"secrets_region": m.SecretsRegion,
	}
}

func vaultFromJson(json *vaultJson) *vaultModel {
	return &vaultModel{
		Type:          types.StringValue(json.Type),
		AccountId:     types.StringValue(json.Install),
		SecretsRegion: types.StringPointerValue(json.SecretsRegion),
	}
}

// Every vault field is `step: "new"` in the app's install schema, so P0 rejects a
// change once the install exists. RequiresReplace on each plans the replacement that
// works instead, as the hosting block does. ValidateConfig checks each field's value
// against P0's rules, in validateDeployable.
func vaultAttribute() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		Required:            true,
		MarkdownDescription: `The secret manager that holds the GitHub App's private key. P0 never reads the key. Your connector does. Only AWS Secrets Manager is supported for now.`,
		Attributes: map[string]schema.Attribute{
			"type": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: `The secret manager: ` + "`aws-sm`" + ` (AWS Secrets Manager), the only one supported for now`,
				Validators: []validator.String{
					stringvalidator.OneOf(AwsSecretsManager),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"account_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: `The ID of the AWS account that holds the private key secret (in the P0 app, one of your AWS Secrets Manager installations)`,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"secrets_region": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: `The AWS region the private key secret is stored in, such as ` + "`us-west-2`",
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
