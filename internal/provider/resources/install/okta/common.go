package installokta

const (
	OktaKey = "okta"
)

type Jwk struct {
	Kty string `json:"kty" tfsdk:"kty"`
	Kid string `json:"kid" tfsdk:"kid"`
	E   string `json:"e" tfsdk:"e"`
	N   string `json:"n" tfsdk:"n"`
}
