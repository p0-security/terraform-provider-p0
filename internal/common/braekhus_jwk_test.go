package common

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestBraekhusPublicJwk(t *testing.T) {
	cases := []struct {
		name    string
		value   types.String
		wantErr bool
	}{
		{"valid public key", types.StringValue(`{"kty":"EC","crv":"P-384","x":"a","y":"b"}`), false},
		{"valid pretty printed", types.StringValue("{\n  \"kty\": \"EC\",\n  \"crv\": \"P-384\"\n}"), false},
		{"unknown is skipped", types.StringUnknown(), false},
		{"null is skipped", types.StringNull(), false},
		{"empty string", types.StringValue(""), true},
		{"placeholder", types.StringValue("<public-jwk-json>"), true},
		{"json null", types.StringValue("null"), true},
		{"json array", types.StringValue("[]"), true},
		{"private key", types.StringValue(`{"kty":"EC","crv":"P-384","x":"a","y":"b","d":"c"}`), true},
		{"private key with null d", types.StringValue(`{"kty":"EC","crv":"P-384","d":null}`), true},
		{"wrong curve", types.StringValue(`{"kty":"EC","crv":"P-256"}`), true},
		{"wrong type", types.StringValue(`{"kty":"RSA","crv":"P-384"}`), true},
		{"missing kty", types.StringValue(`{"crv":"P-384"}`), true},
		{"non-string crv", types.StringValue(`{"kty":"EC","crv":384}`), true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			resp := &validator.StringResponse{}
			BraekhusPublicJwk().ValidateString(context.Background(),
				validator.StringRequest{Path: path.Root("braekhus_public_jwk"), ConfigValue: c.value}, resp)
			if got := resp.Diagnostics.HasError(); got != c.wantErr {
				t.Fatalf("HasError = %v, want %v: %v", got, c.wantErr, resp.Diagnostics)
			}
		})
	}
}
