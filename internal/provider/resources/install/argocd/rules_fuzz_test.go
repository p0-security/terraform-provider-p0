package installargocd

import (
	"context"
	"net"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	installapp "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/app"
	installvaultedcredential "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/vaulted-credential"
)

// copy of internal/provider/resources/install/harbor/rules_fuzz_test.go (INT-576), with
// Argo CD's URL rule and its managing Application, namespace and vault rules.

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

// instance-url.ts: `https://`, a host of dot-separated labels of lowercase letters,
// digits and hyphens, none empty but for one trailing dot, and maybe a port, with
// nothing after it, spelled as its own origin. The origin is the URL Standard's,
// written out here from the standard rather than from the provider's code: a port is
// a number from 1 to 65535, other than 443, without a leading zero. A host whose last
// label, ignoring one trailing dot, is a decimal number or 0x and hexadecimal digits
// is an IPv4 address, written as four decimal numbers.
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
	parts := strings.Split(strings.TrimSuffix(host, "."), ".")
	for _, part := range parts {
		if part == "" {
			return false
		}
	}
	if !referenceNumber(parts[len(parts)-1]) {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.To4() != nil && ip.String() == host
}

func referencePort(port string) bool {
	if port == "" || len(port) > 5 || port[0] == '0' {
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

// managing-app.ts: two Kubernetes object names joined by one slash. An object's name
// is 1 to 253 characters of dot-separated labels, each of lowercase letters, digits and
// hyphens, starting and ending with a letter or digit.
func referenceManagingApp(app string) bool {
	slash := strings.IndexByte(app, '/')
	return slash >= 0 && referenceObjectName(app[:slash]) && referenceObjectName(app[slash+1:])
}

func referenceObjectName(name string) bool {
	if len(name) == 0 || len(name) > 253 {
		return false
	}
	for _, label := range strings.Split(name, ".") {
		if !referenceLabel(label, len(label)) {
			return false
		}
	}
	return true
}

// A label of 1 to maxLength lowercase letters, digits and hyphens, starting and ending
// with a letter or digit.
func referenceLabel(label string, maxLength int) bool {
	if len(label) == 0 || len(label) > maxLength || label[0] == '-' || label[len(label)-1] == '-' {
		return false
	}
	for i := 0; i < len(label); i++ {
		if c := label[i]; !isLower(c) && !isDigit(c) && c != '-' {
			return false
		}
	}
	return true
}

// namespace.ts: a DNS-1123 label, 1 to 63 characters.
func referenceNamespace(namespace string) bool {
	return referenceLabel(namespace, 63)
}

// vault.ts and aws-hosting.ts: an AWS account's ID is 12 digits.
func referenceAwsAccountId(id string) bool {
	if len(id) != 12 {
		return false
	}
	for i := 0; i < len(id); i++ {
		if !isDigit(id[i]) {
			return false
		}
	}
	return true
}

// aws-hosting.ts: an AWS region is two or more words of lowercase letters and then a
// number, each joined by a hyphen, as in us-west-2.
func referenceAwsRegion(region string) bool {
	parts := strings.Split(region, "-")
	if len(parts) < 3 {
		return false
	}
	for i, part := range parts {
		if part == "" {
			return false
		}
		for j := 0; j < len(part); j++ {
			if c := part[j]; (i == len(parts)-1 && !isDigit(c)) || (i < len(parts)-1 && !isLower(c)) {
				return false
			}
		}
	}
	return true
}

// aws-hosting.ts: a region outside AWS's commercial partition, which the connector
// can't read a secret in.
func referenceOtherPartition(region string) bool {
	if strings.HasPrefix(region, "us-gov-") || strings.HasPrefix(region, "cn-") || strings.HasPrefix(region, "eusc-") {
		return true
	}
	first, rest, ok := strings.Cut(region, "-")
	if !ok || first == "" || strings.Trim(first, "abcdefghijklmnopqrstuvwxyz") != "" {
		return false
	}
	second, _, ok := strings.Cut(rest, "-")
	return ok && strings.HasPrefix(second, "iso") && strings.Trim(second, "abcdefghijklmnopqrstuvwxyz") == ""
}

// vault.ts: a Google Cloud project's ID, 6 to 30 lowercase letters, digits and
// hyphens, starting with a letter and not ending with a hyphen.
func referenceGcpVaultProject(id string) bool {
	return len(id) >= 6 && len(id) <= 30 && isLower(id[0]) && referenceLabel(id, 30)
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
	"", "a", "-", "a-", "-a", "ab", "a--b", "prod", "Prod", "p0-argocd-prod", "p0-ac", "p0-acd",
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
		"argocd-token_2", "argocd/p0-argocd/token", "ArgoCD.token+v2=@acme", "projects/acme-vault/secrets/argocd-token",
		"arn:aws:secretsmanager:us-west-2:111111111111:secret:argocd-token-AbC123", "projects/acme-vault/secrets/token:latest",
		`{"token":"not-a-real-token"}`, "p0-connector", "argocd token", "argocd/*", " argocd-token\n",
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

func FuzzManagingApp(f *testing.F) {
	addSeeds(f)
	for _, seed := range []string{
		"infra/argocd", "default/argocd-config", "platform.team/argocd.rbac", "a/b", "1/2", "argocd", "/argocd", "infra/",
		"infra/argocd/argocd", "Infra/ArgoCD", "infra/argo_cd", "infra/-argocd", "infra-/argocd", ".infra/argocd", "infra/argo..cd",
		"infra/argo cd", "infra/argocd,admin", "infra/*", " infra/argocd\n",
		strings.Repeat("p", 253) + "/" + strings.Repeat("a", 253), "infra/" + strings.Repeat("a", 254), strings.Repeat("p", 254) + "/argocd",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, app string) {
		want := referenceManagingApp(app)
		if got := isManagingApp(app); got != want {
			t.Errorf("isManagingApp(%q) = %v; want %v", app, got, want)
		}
		// The check refuses whitespace around the value, which P0 would trim.
		var diags diag.Diagnostics
		validateManagingApp(types.StringValue(app), &diags)
		if accepted := !diags.HasError(); accepted != want {
			t.Errorf("the managing_app check accepts %q: %v; want %v", app, accepted, want)
		}
		checkRefusals(t, diags, "managing_app", invalidManagingApp)
	})
}

func FuzzNamespace(f *testing.F) {
	addSeeds(f)
	for _, seed := range []string{"argocd", "argo-cd", "123", "ArgoCD", "argo.cd", "argo_cd", "argocd/argocd", "-argocd", "argocd-", " argocd\n"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, namespace string) {
		want := referenceNamespace(namespace)
		var diags diag.Diagnostics
		validateNamespace(types.StringValue(namespace), &diags)
		if accepted := !diags.HasError(); accepted != want {
			t.Errorf("the namespace check accepts %q: %v; want %v", namespace, accepted, want)
		}
		checkRefusals(t, diags, "namespace", invalidNamespace)
	})
}

// The vault's fields: each attribute's validator, and the region's partition rule,
// which validateVault applies.
func FuzzVaultFields(f *testing.F) {
	addSeeds(f)
	for _, seed := range []string{
		"123456789012", "12345678901", "1234567890123", "12345678901a", "us-west-2", "us-west2", "us west 2", "eu-central-1",
		"us-gov-west-1", "cn-north-1", "us-iso-east-1", "us-isob-east-1", "eu-isoe-west-1", "eusc-de-east-1", "ap-southeast-iso-1",
		"acme-secrets", "acme_secrets", "Acme-Secrets", "acme", "acme-secrets-", "1acme-secrets",
	} {
		f.Add(seed)
	}
	attributes := vaultAttribute().Attributes
	fieldValidators := map[string][]validator.String{}
	for _, name := range []string{"account_id", "project_id", "secrets_region"} {
		attribute, ok := attributes[name].(schema.StringAttribute)
		if !ok {
			f.Fatalf("vault.%s isn't a string attribute", name)
		}
		fieldValidators[name] = attribute.Validators
	}
	validators := func(name string) []validator.String { return fieldValidators[name] }
	f.Fuzz(func(t *testing.T, value string) {
		if got, want := validatorsAccept(validators("account_id"), value), referenceAwsAccountId(value); got != want {
			t.Errorf("vault.account_id %q accepted: %v; want %v", value, got, want)
		}
		if got, want := validatorsAccept(validators("project_id"), value), referenceGcpVaultProject(value); got != want {
			t.Errorf("vault.project_id %q accepted: %v; want %v", value, got, want)
		}
		region := referenceAwsRegion(value)
		if got := validatorsAccept(validators("secrets_region"), value); got != region {
			t.Errorf("vault.secrets_region %q accepted: %v; want %v", value, got, region)
		}
		if !region {
			return
		}
		var diags diag.Diagnostics
		validateVault(&vaultModel{
			Type: types.StringValue(awsSecretsManager), AccountId: types.StringValue("123456789012"),
			SecretsRegion: types.StringValue(value), ProjectId: types.StringNull(),
		}, &diags)
		if refused, want := diags.HasError(), referenceOtherPartition(value); refused != want {
			t.Errorf("the vault check refuses the region %q: %v; want %v (%v)", value, refused, want, diags)
		}
	})
}
