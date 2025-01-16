package installokta

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

const (
	OktaKey = "okta"
)

type Jwk struct {
	Kty string `json:"kty" tfsdk:"kty"`
	Kid string `json:"kid" tfsdk:"kid"`
	E   string `json:"e" tfsdk:"e"`
	N   string `json:"n" tfsdk:"n"`
}

func GetJwkObject(ctx context.Context, diags *diag.Diagnostics, jwk Jwk) *basetypes.ObjectValue {
	jwkAttr, jwkDiags := types.ObjectValueFrom(ctx, map[string]attr.Type{
		"kty": types.StringType,
		"kid": types.StringType,
		"e":   types.StringType,
		"n":   types.StringType,
	}, jwk)
	if jwkDiags.HasError() {
		diags.Append(jwkDiags...)
		return nil
	}
	return &jwkAttr
}
