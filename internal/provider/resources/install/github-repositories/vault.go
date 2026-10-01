package installgithubrepositories

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
	installaigateway "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/ai_gateway"
	installapp "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/app"
	installvaultedcredential "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/vaulted-credential"
)

// The `vault` attribute is the app's vault-only install element, `newVaultElement` in
// packages/integrations/vaulted-credential: the P0 secret manager installation that
// holds the GitHub App's private key. Unlike the vaulted-credential secret manager
// block, it names no connector, because the `hosting` block places the connector.

const (
	AwsSecretsManager = "aws-sm"
	GcpSecretManager  = installvaultedcredential.GcpSecretManager
)

// The hosting type that each vault type goes with. P0 rejects a vault and a connector
// in different clouds.
var vaultHostingTypes = map[string]string{
	AwsSecretsManager: installapp.AwsHosting,
	GcpSecretManager:  installapp.GcpHosting,
}

// The vault, as P0 stores it. The JSON is a discriminated union on "type". "install"
// names the secret manager installation, which is the AWS account for `aws-sm` and the
// Google Cloud project for `gcp-sm`. Only `aws-sm` has a region.
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

func (m *vaultModel) toJson() *vaultJson {
	install := m.ProjectId
	if m.Type.ValueString() == AwsSecretsManager {
		install = m.AccountId
	}
	return &vaultJson{
		Type:          m.Type.ValueString(),
		Install:       install.ValueString(),
		SecretsRegion: m.SecretsRegion.ValueStringPointer(),
	}
}

// The vault's fields, by attribute name.
func (m *vaultModel) fields() map[string]types.String {
	return map[string]types.String{
		"account_id":     m.AccountId,
		"project_id":     m.ProjectId,
		"secrets_region": m.SecretsRegion,
	}
}

func vaultFromJson(json *vaultJson) *vaultModel {
	vault := &vaultModel{
		Type:          types.StringValue(json.Type),
		AccountId:     types.StringNull(),
		ProjectId:     types.StringNull(),
		SecretsRegion: types.StringPointerValue(json.SecretsRegion),
	}
	switch json.Type {
	case AwsSecretsManager:
		vault.AccountId = types.StringValue(json.Install)
	case GcpSecretManager:
		vault.ProjectId = types.StringValue(json.Install)
	}
	return vault
}

// Every vault field is `step: "new"` in the app's install schema, so P0 rejects a
// change once the install exists. RequiresReplace on each plans the replacement that
// works instead, as the hosting block does. ValidateConfig checks each field's value
// against P0's rules, in validateDeployable.
func vaultAttribute() schema.SingleNestedAttribute {
	fields := map[string][]string{
		AwsSecretsManager: {"account_id", "secrets_region"},
		GcpSecretManager:  {"project_id"},
	}
	return schema.SingleNestedAttribute{
		Required:            true,
		MarkdownDescription: `The secret manager that holds the GitHub App's private key. P0 never reads the key. Your connector does. It must be in the same cloud as ` + "`hosting`" + `.`,
		Attributes: map[string]schema.Attribute{
			"type": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: `The secret manager: either ` + "`aws-sm`" + ` (AWS Secrets Manager) or ` + "`gcp-sm`" + ` (Google Secret Manager)`,
				Validators: []validator.String{
					stringvalidator.OneOf(AwsSecretsManager, GcpSecretManager),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"account_id": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: `The ID of the AWS account that holds the private key secret (in the P0 app, one of your AWS Secrets Manager installations). Required for, and only valid with, ` + "`aws-sm`" + `.`,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"secrets_region": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: `The AWS region the private key secret is stored in, such as ` + "`us-west-2`" + `. Required for, and only valid with, ` + "`aws-sm`" + `.`,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"project_id": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: `The ID of the Google Cloud project that holds the private key secret (in the P0 app, one of your Google Secret Manager installations). Required for, and only valid with, ` + "`gcp-sm`" + `.`,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
		},
		Validators: []validator.Object{
			installaigateway.RequiredWhenAttr("type", fields),
			installaigateway.ExclusiveToAttr("type", fields),
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

// Rejects a vault and a connector in different clouds, which P0 rejects too. The
// wording is P0's own.
func validateSameCloud(vault *vaultModel, hosting *installapp.ConnectorHostingModel, diags *diag.Diagnostics) {
	if vault == nil || hosting == nil || !installapp.IsSet(vault.Type) || !installapp.IsSet(hosting.Type) {
		return
	}
	want, ok := vaultHostingTypes[vault.Type.ValueString()]
	// The type's own validator rejects a vault type that is not listed.
	if !ok || hosting.Type.ValueString() == want {
		return
	}
	diags.AddAttributeError(
		path.Root("hosting").AtName("type"),
		"Vault and connector in different clouds",
		fmt.Sprintf("The vault and the connector need to be in the same cloud. Pick AWS for both or GCP for both. "+
			"'vault.type' %q needs 'hosting.type' %q.", vault.Type.ValueString(), want),
	)
}
