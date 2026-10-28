package common

import (
	"context"
	"encoding/json"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

const braekhusJwkDescription = "must be a JSON public JWK with kty EC and crv P-384, and no d member (a d member means a private key)"

type braekhusPublicJwkValidator struct{}

var _ validator.String = braekhusPublicJwkValidator{}

// BraekhusPublicJwk validates a braekhus tunnel public key at plan time. The
// value must be a JSON object with kty "EC" and crv "P-384", and must not
// carry the private "d" member. Unknown values are skipped, since the key
// usually comes from outside the configuration.
func BraekhusPublicJwk() validator.String {
	return braekhusPublicJwkValidator{}
}

func (braekhusPublicJwkValidator) Description(context.Context) string {
	return braekhusJwkDescription
}

func (v braekhusPublicJwkValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (braekhusPublicJwkValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	const summary = "Invalid braekhus public JWK"
	var jwk map[string]any
	if err := json.Unmarshal([]byte(req.ConfigValue.ValueString()), &jwk); err != nil || jwk == nil {
		resp.Diagnostics.AddAttributeError(req.Path, summary,
			"The value is not a JSON object. Supply the contents of the braekhus client's jwk.public.json.")
		return
	}
	if _, hasD := jwk["d"]; hasD {
		resp.Diagnostics.AddAttributeError(req.Path, summary,
			"The value has a \"d\" member, so it is a private key. Supply the public key (jwk.public.json), never jwk.private.json.")
		return
	}
	if jwk["kty"] != "EC" || jwk["crv"] != "P-384" {
		resp.Diagnostics.AddAttributeError(req.Path, summary,
			"The key must have \"kty\": \"EC\" and \"crv\": \"P-384\".")
	}
}
