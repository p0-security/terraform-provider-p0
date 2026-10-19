package installjiraai

import (
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	installvaultedcredential "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/vaulted-credential"
)

// The `vault` attribute is the app's vault-only install element, `newVaultElement` in
// packages/integrations/vaulted-credential, with its AWS Secrets Manager option hidden:
// the connector reads a site's secrets as its Cloud Run service account, which only
// Google Secret Manager accepts. It names the Google Secret Manager installation in P0
// whose project holds the site's secrets.

// TODO(INT-465): vaultJson and vaultModel are a Google Secret Manager-only copy of
// Harbor's (harbor/vault.go).
//
// The vault, as P0 stores it. "install" names the secret manager installation, by its
// Google Cloud project for `gcp-sm`.
type vaultJson struct {
	Type    string `json:"type"`
	Install string `json:"install"`
}

// Terraform names the installation for what it is, as Harbor's vault block does.
type vaultModel struct {
	Type      types.String `tfsdk:"type"`
	ProjectId types.String `tfsdk:"project_id"`
}

func (m *vaultModel) toJson() *vaultJson {
	return &vaultJson{Type: m.Type.ValueString(), Install: m.ProjectId.ValueString()}
}

// Reads the vault P0 stores, or returns nil for a type this provider doesn't support.
func vaultFromJson(json *vaultJson) *vaultModel {
	if json.Type != installvaultedcredential.GcpSecretManager {
		return nil
	}
	return &vaultModel{
		Type:      types.StringValue(json.Type),
		ProjectId: types.StringValue(json.Install),
	}
}

// The app's Terraform generator writes a site's grants only for a vault project with a
// Google Cloud project ID. The message is the provider's own.
const invalidVaultProject = "Choose the Google Secret Manager installation, by its Google Cloud project ID, that holds the site's secrets"

// Every vault field is `step: "new"` in the app's install schema, so P0 refuses a
// change once the site exists. RequiresReplace on each plans the replacement that
// works instead.
func vaultAttribute() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		Required: true,
		MarkdownDescription: "The secret manager that holds the site's secrets: the admin client's secret, and each agent account's API key. " +
			"P0 never reads them. Only the connector does, as its service account, so it must be Google Secret Manager.",
		Attributes: map[string]schema.Attribute{
			"type": fixedAttribute(
				"The secret manager. Only `gcp-sm` (Google Secret Manager) is supported.",
				stringvalidator.OneOf(installvaultedcredential.GcpSecretManager),
			),
			"project_id": fixedAttribute(
				"The ID of the Google Cloud project that holds the secrets: one of your Google Secret Manager installations in P0. "+
					"This provider has no resource for that installation, so install Google Secret Manager in the P0 app first.",
				stringvalidator.RegexMatches(gcpProjectIdRegex, invalidVaultProject),
			),
		},
	}
}
