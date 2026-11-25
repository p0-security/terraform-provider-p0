package installpingfederateawsiamfederation

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/p0-security/terraform-provider-p0/internal/common"
)

const attributeSourceRoleSourceType = "attribute-source"

// The `jwks` option of the app's attribute source: P0 reads PingFederate's JWKS
// endpoint, or uses the keys pasted into it.
const (
	jwksFromUrl  = "url"
	jwksFromKeys = "keys"
)

var attributeSourcePath = path.Root("attribute_source")

type attributeSourceModel struct {
	EntityId types.String `tfsdk:"entity_id"`
	Issuer   types.String `tfsdk:"issuer"`
	ClientId types.String `tfsdk:"client_id"`
	Jwks     types.String `tfsdk:"jwks"`
	Audience types.String `tfsdk:"audience"`
	RolesUrl types.String `tfsdk:"roles_url"`
}

type jwksJson struct {
	Type string  `json:"type"`
	Keys *string `json:"keys,omitempty"`
}

// PingFederate reads each user's roles from P0 at sign-in.
type attributeSourceRoleSource struct{}

func (attributeSourceRoleSource) attribute() schema.Attribute {
	assigned := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	return schema.SingleNestedAttribute{
		MarkdownDescription: "PingFederate reads each user's AWS roles from P0 at each sign-in, through a REST API data store. Set this or `scim`, not both.",
		Optional:            true,
		PlanModifiers:       roleSourcePlanModifiers(),
		Attributes: map[string]schema.Attribute{
			"entity_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "PingFederate's SAML entity ID, under System > Server > Protocol Settings > Federation Info. Each AWS account's SAML provider trusts it.",
			},
			"issuer": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The Issuer Claim Value of the JWT access token manager that signs the REST API data store's tokens",
			},
			"client_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The client ID of the OAuth client that the REST API data store gets its tokens with. P0 refuses the tokens of other clients.",
			},
			"jwks": schema.StringAttribute{
				Optional: true,
				MarkdownDescription: "The JSON of PingFederate's public keys, from `<pingfederate_url>/pf/JWKS`, for P0 to check the data store's tokens with. " +
					"Paste it again when PingFederate rotates its signing key. " +
					"Leave it unset to have P0 read `<pingfederate_url>/pf/JWKS` itself, which needs no change on a rotation, but needs P0 to reach PingFederate's runtime. " +
					"P0 removes the whitespace around the value, so remove it here too, for example with `trimspace(file(\"jwks.json\"))`.",
			},
			"audience": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The Audience Claim Value to set on the JWT access token manager. P0 sets it for the app.",
				PlanModifiers:       assigned,
			},
			"roles_url": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The base URL of the REST API data store: P0's roles route for your organization",
				PlanModifiers:       assigned,
			},
		},
	}
}

// P0's checks (`validateFields` in
// packages/integrations/pingfederate-aws-iam-federation/backend/src/role-sources/attribute-source/install.ts
// in the app).
func (attributeSourceRoleSource) validate(ctx context.Context, config tfsdk.Config, diags *diag.Diagnostics) {
	var entityId, issuer, clientId, jwks types.String
	diags.Append(config.GetAttribute(ctx, attributeSourcePath.AtName("entity_id"), &entityId)...)
	diags.Append(config.GetAttribute(ctx, attributeSourcePath.AtName("issuer"), &issuer)...)
	diags.Append(config.GetAttribute(ctx, attributeSourcePath.AtName("client_id"), &clientId)...)
	diags.Append(config.GetAttribute(ctx, attributeSourcePath.AtName("jwks"), &jwks)...)
	if diags.HasError() {
		return
	}

	notEmpty := func(value string) bool { return value != "" }
	validateField(attributeSourcePath.AtName("entity_id"), entityId, []rule{{summary: "Missing entity ID", message: invalidEntityId, test: notEmpty}}, diags)
	validateField(attributeSourcePath.AtName("issuer"), issuer, []rule{{summary: "Missing token issuer", message: invalidIssuer, test: notEmpty}}, diags)
	validateField(attributeSourcePath.AtName("client_id"), clientId, []rule{{summary: "Missing OAuth client ID", message: invalidClientId, test: notEmpty}}, diags)
	validateField(attributeSourcePath.AtName("jwks"), jwks, []rule{{summary: "Invalid JWKS", message: invalidJwks, test: isJwks}}, diags)
}

// P0's `JwksSchema`: an object whose "keys" list has at least one key with a "kty".
func isJwks(text string) bool {
	var jwks struct {
		Keys []struct {
			Kty *string `json:"kty"`
		} `json:"keys"`
	}
	if err := json.Unmarshal([]byte(text), &jwks); err != nil || len(jwks.Keys) == 0 {
		return false
	}
	for _, key := range jwks.Keys {
		if key.Kty == nil {
			return false
		}
	}
	return true
}

// P0 sets the audience itself, so the request leaves it out.
func (attributeSourceRoleSource) toJson(model *pingFederateAwsIamFederationAppModel) *roleSourceJson {
	source := model.AttributeSource
	if source == nil {
		return nil
	}
	jwks := jwksJson{Type: jwksFromUrl}
	if isSet(source.Jwks) {
		jwks = jwksJson{Type: jwksFromKeys, Keys: source.Jwks.ValueStringPointer()}
	}
	return &roleSourceJson{
		Type:     attributeSourceRoleSourceType,
		EntityId: source.EntityId.ValueStringPointer(),
		Issuer:   source.Issuer.ValueStringPointer(),
		ClientId: source.ClientId.ValueStringPointer(),
		Jwks:     &jwks,
	}
}

func (attributeSourceRoleSource) fromJson(installer *common.Install, diags *diag.Diagnostics, id string, json *roleSourceJson, model *pingFederateAwsIamFederationAppModel) {
	jwks := types.StringNull()
	if json.Jwks != nil {
		switch json.Jwks.Type {
		case jwksFromUrl:
		case jwksFromKeys:
			jwks = types.StringPointerValue(json.Jwks.Keys)
		default:
			diags.AddError(
				"Unsupported PingFederate (AWS IAM Federation) app",
				fmt.Sprintf("The PingFederate (AWS IAM Federation) app %s gets its public keys from %q. This provider supports only %s and %s. "+
					"Upgrade the provider, manage the app in the P0 app, or remove it from Terraform state with `terraform state rm`.",
					id, json.Jwks.Type, jwksFromUrl, jwksFromKeys),
			)
			return
		}
	}

	model.AttributeSource = &attributeSourceModel{
		EntityId: types.StringPointerValue(json.EntityId),
		Issuer:   types.StringPointerValue(json.Issuer),
		ClientId: types.StringPointerValue(json.ClientId),
		Jwks:     jwks,
		Audience: types.StringPointerValue(json.Audience),
		RolesUrl: rolesUrlOf(installer),
	}
}

// The provider calls P0 at `<host>/o/<org>`, and PingFederate reads the roles at
// `<host>/integrations/pingfederate-aws-iam-federation/<org>`.
func rolesUrlOf(installer *common.Install) types.String {
	if installer == nil || installer.ProviderData == nil {
		return types.StringNull()
	}
	host, org, ok := strings.Cut(installer.ProviderData.BaseUrl, "/o/")
	if !ok {
		return types.StringNull()
	}
	return types.StringValue(fmt.Sprintf("%s/integrations/%s/%s", host, PingFederateAwsIamFederationKey, org))
}
