package installpingfederateawsiamfederation

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/objectvalidator"
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

const scimRoleSourceType = "scim"

var scimPath = path.Root("scim")

type scimModel struct {
	Connector        types.String `tfsdk:"connector"`
	RoleAttribute    types.String `tfsdk:"role_attribute"`
	ScimUsername     types.String `tfsdk:"scim_username"`
	Vault            *vaultModel  `tfsdk:"vault"`
	ScimPasswordName types.String `tfsdk:"scim_password_name"`
	CaCertificate    types.String `tfsdk:"ca_certificate"`
}

// P0's connector writes each user's roles through PingFederate's SCIM endpoint.
type scimRoleSource struct{}

func (scimRoleSource) attribute() schema.Attribute {
	// Every field is `step: "new"` in the app's install schema, so P0 refuses a change
	// once the app exists.
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	return schema.SingleNestedAttribute{
		MarkdownDescription: "P0's connector writes each user's AWS roles to Active Directory through PingFederate's SCIM endpoint. Set this or `attribute_source`, not both.",
		Optional:            true,
		Validators:          []validator.Object{objectvalidator.ExactlyOneOf(path.MatchRoot("attribute_source"))},
		PlanModifiers:       roleSourcePlanModifiers(),
		Attributes: map[string]schema.Attribute{
			"connector": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The connector that acts on this app: the `id` of a `p0_pingfederate_aws_iam_federation_connector` resource. Requires that connector to be installed. It must reach PingFederate's runtime, including `/pf-scim`.",
				Validators:          connectorIdValidators(),
				PlanModifiers:       replace,
			},
			"role_attribute": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The multi-valued attribute in the SCIM connection's custom schema that holds each user's AWS roles, such as `awsRole`. Active Directory has an attribute of the same name. The SP connection maps it to AWS's Role attribute.",
				PlanModifiers:       replace,
			},
			"scim_username": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The user name of the HTTP Basic credentials on PingFederate's SCIM IdP connection, which writes to Active Directory",
				PlanModifiers:       replace,
			},
			"vault": vaultAttribute(
				`The secret manager that holds the SCIM password. Your connector reads the secret, and P0 never does. It must be in the connector's cloud, and can be in another account or project there.`,
				secretPlace.what,
			),
			"scim_password_name": schema.StringAttribute{
				Required: true,
				MarkdownDescription: `The name of the secret that holds the SCIM connection's password, such as ` + "`pingfederate/acme/p0`" + ` in AWS Secrets Manager or ` + "`pingfederate-acme-p0`" + ` in Google Secret Manager: the name alone, not the secret's ARN or its full resource name. ` +
					`In AWS Secrets Manager the name can't end in a hyphen and six letters or digits, like ` + "`-AbCdEf`" + `, because Secrets Manager can't find a secret by such a name. ` +
					`To rotate the SCIM password, store the new one as a new version of the same secret, which needs no change here.`,
				PlanModifiers: replace,
			},
			"ca_certificate": schema.StringAttribute{
				Optional: true,
				MarkdownDescription: `PEM certificates of the CA that issued PingFederate's TLS certificate, if it isn't a public CA. ` +
					`P0 removes the whitespace around the value, so remove it here too, for example with ` + "`trimspace(file(\"ca.pem\"))`" + `.`,
				PlanModifiers: replace,
			},
		},
	}
}

// P0's checks (`validateRoleAttribute`, `validateScimUsername`,
// `validateCaCertificate` and `validateScimPassword` in
// packages/integrations/pingfederate-aws-iam-federation/backend/src/install/validation.ts in
// the app). None repeats the value, because it may be the pasted SCIM password.
func (scimRoleSource) validate(ctx context.Context, config tfsdk.Config, diags *diag.Diagnostics) {
	var roleAttribute, scimUsername, secretName, caCertificate types.String
	diags.Append(config.GetAttribute(ctx, scimPath.AtName("role_attribute"), &roleAttribute)...)
	diags.Append(config.GetAttribute(ctx, scimPath.AtName("scim_username"), &scimUsername)...)
	diags.Append(config.GetAttribute(ctx, scimPath.AtName("scim_password_name"), &secretName)...)
	diags.Append(config.GetAttribute(ctx, scimPath.AtName("ca_certificate"), &caCertificate)...)
	vault := vaultFromConfig(ctx, config, diags)
	if diags.HasError() {
		return
	}

	validateField(scimPath.AtName("role_attribute"), roleAttribute, []rule{{summary: "Invalid role attribute", message: invalidRoleAttribute, test: RoleAttributeRegex.MatchString}}, diags)
	validateField(scimPath.AtName("scim_username"), scimUsername, []rule{{summary: "Invalid SCIM user name", message: invalidScimUsername, test: ScimUsernameRegex.MatchString}}, diags)
	// P0 accepts an empty certificate.
	validateField(scimPath.AtName("ca_certificate"), caCertificate, []rule{{summary: "Invalid CA certificate", message: invalidCaCertificate, test: func(pem string) bool {
		return pem == "" || strings.Contains(pem, pemCertificateBegin)
	}}}, diags)
	validateScimPassword(vault, secretName, diags)
}

func (scimRoleSource) toJson(model *pingFederateAwsIamFederationAppModel) *roleSourceJson {
	scim := model.Scim
	if scim == nil {
		return nil
	}
	json := roleSourceJson{
		Type:             scimRoleSourceType,
		Connector:        scim.Connector.ValueStringPointer(),
		RoleAttribute:    scim.RoleAttribute.ValueStringPointer(),
		ScimUsername:     scim.ScimUsername.ValueStringPointer(),
		ScimPasswordName: scim.ScimPasswordName.ValueStringPointer(),
		CaCertificate:    scim.CaCertificate.ValueStringPointer(),
	}
	if scim.Vault != nil {
		json.Vault = scim.Vault.toJson()
	}
	return &json
}

func (scimRoleSource) fromJson(_ *common.Install, diags *diag.Diagnostics, id string, json *roleSourceJson, model *pingFederateAwsIamFederationAppModel) {
	// vault is Required, so a nil vault would surface as Terraform core's "provider
	// produced inconsistent result after apply", which tells the reader nothing to do.
	if json.Vault == nil {
		diags.AddError(
			"Bad API response",
			fmt.Sprintf("The PingFederate (AWS IAM Federation) app %s carries no vault configuration. "+
				"Remove it in the P0 app and add it again, or remove it from Terraform state with `terraform state rm`.", id),
		)
		return
	}

	// A vault of a type this provider doesn't know can't be read into the attribute's
	// fields.
	if !slices.Contains(vaultTypes, json.Vault.Type) {
		diags.AddError(
			"Unsupported PingFederate (AWS IAM Federation) app",
			fmt.Sprintf("The PingFederate (AWS IAM Federation) app %s has a %q vault. This provider supports only %s. "+
				"Upgrade the provider, manage the app in the P0 app, or remove it from Terraform state with `terraform state rm`.",
				id, json.Vault.Type, strings.Join(vaultTypes, " and ")),
		)
		return
	}

	model.Scim = &scimModel{
		Connector:        types.StringPointerValue(json.Connector),
		RoleAttribute:    types.StringPointerValue(json.RoleAttribute),
		ScimUsername:     types.StringPointerValue(json.ScimUsername),
		Vault:            vaultFromJson(json.Vault),
		ScimPasswordName: types.StringPointerValue(json.ScimPasswordName),
		CaCertificate:    types.StringPointerValue(json.CaCertificate),
	}
}
