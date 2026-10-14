package installharbor

import (
	"context"
	"net"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	installapp "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/app"
	installvaultedcredential "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/vaulted-credential"
)

// P0's rules, written out character by character rather than as patterns, so that each
// fuzz target compares the provider's check with an independent reading of the rule.

func isLower(c byte) bool { return c >= 'a' && c <= 'z' }
func isUpper(c byte) bool { return c >= 'A' && c <= 'Z' }
func isDigit(c byte) bool { return c >= '0' && c <= '9' }

// item-id.ts: lowercase letters, digits and hyphens, starting and ending with a letter
// or digit, 1 to 63 characters.
func referenceItemId(id string) bool {
	if len(id) == 0 || len(id) > 63 || id[0] == '-' || id[len(id)-1] == '-' {
		return false
	}
	for i := 0; i < len(id); i++ {
		if c := id[i]; !isLower(c) && !isDigit(c) && c != '-' {
			return false
		}
	}
	return true
}

// hosting.ts, AWS: 2 to 64 characters of lowercase letters, digits and single hyphens,
// starting with a letter and ending with a letter or digit.
func referenceAwsConnectorName(name string) bool {
	if len(name) < 2 || len(name) > 64 || !isLower(name[0]) || name[len(name)-1] == '-' || strings.Contains(name, "--") {
		return false
	}
	for i := 0; i < len(name); i++ {
		if c := name[i]; !isLower(c) && !isDigit(c) && c != '-' {
			return false
		}
	}
	return true
}

// hosting.ts, Google Cloud: a service account ID, 6 to 30 characters of lowercase
// letters, digits and hyphens, starting with a letter and not ending with a hyphen.
func referenceGcpConnectorName(name string) bool {
	if len(name) < 6 || len(name) > 30 || !isLower(name[0]) || name[len(name)-1] == '-' {
		return false
	}
	for i := 0; i < len(name); i++ {
		if c := name[i]; !isLower(c) && !isDigit(c) && c != '-' {
			return false
		}
	}
	return true
}

// gcloud validator.ts: one or more ASCII letters, digits and hyphens.
func referenceGcpProjectId(id string) bool {
	if len(id) == 0 {
		return false
	}
	for i := 0; i < len(id); i++ {
		if c := id[i]; !isLower(c) && !isUpper(c) && !isDigit(c) && c != '-' {
			return false
		}
	}
	return true
}

// instance-url.ts: `https://`, a host of lowercase letters, digits, dots and hyphens,
// and maybe a port, with nothing after it, spelled as its own origin. The origin is
// the URL Standard's, written out here from the standard rather than from the
// provider's code: a port is a number up to 65535, other than 443, without a leading
// zero. A host whose last label, ignoring one trailing dot, is a decimal number or 0x
// and hexadecimal digits is an IPv4 address, written as four decimal numbers.
func referenceInstanceUrl(url string) bool {
	rest, ok := strings.CutPrefix(url, "https://")
	if !ok {
		return false
	}
	host, port, hasPort := strings.Cut(rest, ":")
	if host == "" || (hasPort && !referencePort(port)) {
		return false
	}
	for i := 0; i < len(host); i++ {
		if c := host[i]; !isLower(c) && !isDigit(c) && c != '.' && c != '-' {
			return false
		}
	}
	parts := strings.Split(host, ".")
	if len(parts) > 1 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	if !referenceNumber(parts[len(parts)-1]) {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.To4() != nil && ip.String() == host
}

func referencePort(port string) bool {
	if port == "" || len(port) > 5 || (len(port) > 1 && port[0] == '0') {
		return false
	}
	number := 0
	for i := 0; i < len(port); i++ {
		if !isDigit(port[i]) {
			return false
		}
		number = number*10 + int(port[i]-'0')
	}
	return number <= 65535 && number != 443
}

// A label the URL Standard reads as a number.
func referenceNumber(label string) bool {
	if hex, ok := strings.CutPrefix(label, "0x"); ok {
		for i := 0; i < len(hex); i++ {
			if c := hex[i]; !isDigit(c) && (c < 'a' || c > 'f') {
				return false
			}
		}
		return true
	}
	if label == "" {
		return false
	}
	for i := 0; i < len(label); i++ {
		if !isDigit(label[i]) {
			return false
		}
	}
	return true
}

// secret-name.ts: AWS Secrets Manager's rule, 1 to 512 letters, digits and /_+=.@-.
// It is also the rule for a vault whose type isn't known yet.
func referenceAwsSecretName(name string) bool {
	if len(name) == 0 || len(name) > 512 {
		return false
	}
	for i := 0; i < len(name); i++ {
		if c := name[i]; !isLower(c) && !isUpper(c) && !isDigit(c) && !strings.ContainsRune("/_+=.@-", rune(c)) {
			return false
		}
	}
	return true
}

// Google Secret Manager's rule, 1 to 255 letters, digits, underscores and hyphens.
func referenceGcpSecretName(name string) bool {
	if len(name) == 0 || len(name) > 255 {
		return false
	}
	for i := 0; i < len(name); i++ {
		if c := name[i]; !isLower(c) && !isUpper(c) && !isDigit(c) && c != '_' && c != '-' {
			return false
		}
	}
	return true
}

// Whether every validator accepts value.
func validatorsAccept(validators []validator.String, value string) bool {
	for _, v := range validators {
		resp := &validator.StringResponse{}
		v.ValidateString(context.Background(), validator.StringRequest{Path: path.Root("x"), ConfigValue: types.StringValue(value)}, resp)
		if resp.Diagnostics.HasError() {
			return false
		}
	}
	return true
}

var ruleSeeds = []string{
	"", "a", "-", "a-", "-a", "ab", "a--b", "prod", "Prod", "p0-harbor-prod", "p0-hb", "p0-hbr",
	"prod/other", "prod#x", " prod", "prod ", "acme_prod", "acme-prod", "ACME-1", "é", "a\n",
	strings.Repeat("a", 30), strings.Repeat("a", 31), strings.Repeat("a", 63), strings.Repeat("a", 64), strings.Repeat("a", 65),
}

func addSeeds(f *testing.F) {
	for _, seed := range ruleSeeds {
		f.Add(seed)
	}
}

func FuzzItemId(f *testing.F) {
	addSeeds(f)
	idValidators := itemIdAttribute("").Validators
	f.Fuzz(func(t *testing.T, id string) {
		want := referenceItemId(id)
		if got := isItemId(id); got != want {
			t.Errorf("isItemId(%q) = %v; want %v", id, got, want)
		}
		if got := validatorsAccept(idValidators, id); got != want {
			t.Errorf("the id attribute's validators accept %q: %v; want %v", id, got, want)
		}
	})
}

func FuzzConnectorName(f *testing.F) {
	addSeeds(f)
	f.Fuzz(func(t *testing.T, name string) {
		if got, _ := checkConnectorName(installapp.AwsHosting, name); got != referenceAwsConnectorName(name) {
			t.Errorf("AWS connector name %q accepted: %v; want %v", name, got, !got)
		}
		if got, _ := checkConnectorName(installapp.GcpHosting, name); got != referenceGcpConnectorName(name) {
			t.Errorf("Google Cloud connector name %q accepted: %v; want %v", name, got, !got)
		}
	})
}

func FuzzGcpProjectId(f *testing.F) {
	addSeeds(f)
	f.Fuzz(func(t *testing.T, id string) {
		if got, want := validatorsAccept(connectorHostingValidators.ProjectId, id), referenceGcpProjectId(id); got != want {
			t.Errorf("project %q accepted: %v; want %v", id, got, want)
		}
	})
}

// The refusals a field check may give: P0's message for the rule, or the one for
// whitespace around the value. Neither carries the value.
func checkRefusals(t *testing.T, diags diag.Diagnostics, attribute string, ruleMessage string) {
	t.Helper()
	whitespace := "P0 removes the whitespace around '" + attribute + "', so the value it stores wouldn't match this configuration. " +
		"Remove the whitespace, for example with trimspace()."
	for _, d := range diags {
		if d.Detail() != ruleMessage && d.Detail() != whitespace {
			t.Errorf("refused with %q; want P0's message or the whitespace one", d.Detail())
		}
	}
}

func FuzzInstanceUrl(f *testing.F) {
	addSeeds(f)
	for _, tc := range instanceUrlCases {
		f.Add(tc.url)
		f.Add(" " + tc.url)
	}
	f.Fuzz(func(t *testing.T, url string) {
		want := referenceInstanceUrl(url)
		if got := isInstanceUrl(url); got != want {
			t.Errorf("isInstanceUrl(%q) = %v; want %v", url, got, want)
		}
		var diags diag.Diagnostics
		validateInstanceUrl(types.StringValue(url), &diags)
		if accepted := !diags.HasError(); accepted != want {
			t.Errorf("the url check accepts %q: %v; want %v", url, accepted, want)
		}
		checkRefusals(t, diags, "url", invalidInstanceUrl)
	})
}

func FuzzSecretName(f *testing.F) {
	addSeeds(f)
	for _, seed := range []string{
		"harbor-robot_2", "harbor/p0-harbor/robot", "Harbor.robot+v2=@acme", "projects/acme-vault/secrets/harbor-robot",
		"arn:aws:secretsmanager:us-west-2:111111111111:secret:harbor-robot-AbC123", "projects/acme-vault/secrets/robot:latest",
		`{"name":"robot$p0-connector","secret":"not-a-real-secret"}`, "robot$p0-connector", "harbor robot", "harbor/*", " harbor-robot\n",
		strings.Repeat("s", 255), strings.Repeat("s", 256), strings.Repeat("s", 512), strings.Repeat("s", 513),
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, name string) {
		for _, tc := range []struct {
			vaultType string
			want      bool
			message   string
		}{
			{awsSecretsManager, referenceAwsSecretName(name), invalidSecretName},
			{installvaultedcredential.GcpSecretManager, referenceGcpSecretName(name), invalidGcpSecretName},
			{"", referenceAwsSecretName(name), invalidSecretName},
		} {
			rules := secretNameRules(tc.vaultType)
			if len(rules) != 1 || rules[0].test(name) != tc.want || rules[0].message != tc.message {
				t.Errorf("secretNameRules(%q) on %q: want one rule that accepts it: %v, with the message %q", tc.vaultType, name, tc.want, tc.message)
			}
			vault := &vaultModel{Type: types.StringValue(tc.vaultType)}
			if tc.vaultType == "" {
				vault.Type = types.StringUnknown()
			}
			var diags diag.Diagnostics
			validateSecretName(vault, types.StringValue(name), &diags)
			if accepted := !diags.HasError(); accepted != tc.want {
				t.Errorf("the secret_name check in a %q vault accepts %q: %v; want %v", tc.vaultType, name, accepted, tc.want)
			}
			checkRefusals(t, diags, "secret_name", tc.message)
		}
	})
}
