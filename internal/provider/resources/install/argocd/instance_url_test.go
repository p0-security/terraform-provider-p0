package installargocd

import (
	"bufio"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// copy of internal/provider/resources/install/harbor/instance_url_test.go (INT-576).

// The app's cases for its rule (packages/integrations/argocd/shared/src/__tests__/
// instance-url.test.ts), then the cases of the WHATWG parser's that instance_url.go
// writes out. TestInstanceUrlOracle checks every verdict here against Node's.
var instanceUrlCases = []struct {
	name string
	url  string
	want bool
}{
	{"a host", "https://argocd.example.com", true},
	{"a single-label host", "https://argocd", true},
	{"a port", "https://localhost:8443", true},
	{"an IPv4 address", "https://10.0.0.7", true},
	{"an IDN in Punycode", "https://xn--rgocd-gra.example.com", true},

	{"a root path", "https://example.com/argocd", false},
	{"a root path and a port", "https://example.com:8443/argocd", false},
	{"a root path of several segments", "https://example.com/tools/argo-cd", false},
	{"Argo CD's API path", "https://argocd.example.com/api/v1", false},

	{"a trailing slash", "https://argocd.example.com/", false},
	{"the default port", "https://argocd.example.com:443", false},
	{"a port with a leading zero", "https://argocd.example.com:08443", false},
	{"a colon and no port", "https://argocd.example.com:", false},
	{"port 0", "https://argocd.example.com:0", false},
	{"an uppercase scheme and host", "HTTPS://ArgoCD.Example.COM", false},
	{"an uppercase letter in the host", "https://Argocd.example.com", false},
	{"no slashes after the scheme", "https:argocd.example.com", false},
	{"backslashes for slashes", `https:\\argocd.example.com\`, false},
	{"a tab and a newline inside it", "https://argocd\t.example\n.com", false},
	{"an empty query and fragment", "https://argocd.example.com/?#", false},
	{"an empty username", "https://@argocd.example.com", false},
	{"an empty username and password", "https://:@argocd.example.com", false},
	{"a trailing NUL", "https://argocd.example.com\x00", false},
	{"an IPv4 address in hexadecimal", "https://0x7f.1", false},
	{"an IPv4 address in three parts", "https://1.2.3", false},
	{"an IDN in Unicode", "https://ärgocd.example.com", false},

	{"a leading dot", "https://.example.com", false},
	{"two dots in a row", "https://argocd..example.com", false},
	{"a dot alone", "https://.", false},

	{"a quote in the host", `https://argo"cd.example.com`, false},
	{"an interpolation in the host", "https://${var.host}.example.com", false},
	{"an underscore in the host", "https://argo_cd.example.com", false},
	{"an IPv6 literal", "https://[::1]:8443", false},
	{"plain http", "http://argocd.example.com", false},
	{"a scheme that only starts with https", "httpss://argocd.example.com", false},
	{"another scheme", "ftp://argocd.example.com", false},
	{"no scheme", "argocd.example.com", false},
	{"a scheme-relative URL", "//argocd.example.com", false},
	{"an empty string", "", false},
	{"a second slash", "https://argocd.example.com//", false},
	{"a query", "https://argocd.example.com/?page=1", false},
	{"a fragment", "https://argocd.example.com/#applications", false},
	{"a username and password", "https://admin:secret@argocd.example.com", false},
	{"a username alone", "https://admin@argocd.example.com", false},
	{"a password alone", "https://:secret@argocd.example.com", false},
	{"a password and no host", "https://admin:secret@", false},
	{"a password and a port that isn't one", "https://admin:secret@argocd.example.com:port", false},

	{"port 1", "https://argocd:1", true},
	{"the highest port", "https://argocd:65535", true},
	{"a port past the highest", "https://argocd:65536", false},
	{"a port too long for a number", "https://argocd:99999999999999999999", false},
	{"port 0 with a leading zero", "https://argocd:00", false},
	{"port 443 with a leading zero", "https://argocd.example.com:0443", false},
	{"an IPv4 address with a port", "https://1.2.3.4:8443", true},
	{"an IPv4 address with the default port", "https://1.2.3.4:443", false},
	{"the highest IPv4 address", "https://255.255.255.255", true},
	{"the lowest IPv4 address", "https://0.0.0.0", true},
	{"an IPv4 address with a trailing dot", "https://1.2.3.4.", false},
	{"an IPv4 address with two trailing dots", "https://1.2.3.4..", false},
	{"an IPv4 address with a leading zero", "https://1.2.3.04", false},
	{"an IPv4 address in octal", "https://010.0.0.1", false},
	{"an IPv4 address with a hexadecimal part", "https://0x7f.0.0.1", false},
	{"an IPv4 address past the highest part", "https://1.2.3.256", false},
	{"an IPv4 address as one number", "https://4294967295", false},
	{"a bare 0", "https://0", false},
	{"a bare 0x", "https://0x", false},
	{"a name ending in a number", "https://argocd.123", false},
	{"a name ending in 0x", "https://argocd.0x", false},
	{"a name ending in a hexadecimal number", "https://argocd.0x1f", false},
	{"a name ending in a number that isn't octal", "https://x.09", false},
	{"a name ending in 0x and a letter", "https://a.0xg", true},
	{"a name ending in digits and a hyphen", "https://a.1-2", true},
	{"a name with a trailing dot", "https://argocd.example.com.", true},
	{"a name with two trailing dots", "https://argocd.example.com..", false},
	{"a name with an empty label", "https://a..b", false},
	{"a name that is a hyphen", "https://-", true},
	// Node 24's behaviour, not the URL Standard's: its parser leaves an all-ASCII host
	// as it is, without decoding its Punycode. The provider follows P0, which runs Node.
	{"a Punycode label that doesn't decode", "https://xn--a.com", true},
}

func TestIsInstanceUrl(t *testing.T) {
	for _, tc := range instanceUrlCases {
		if got := isInstanceUrl(tc.url); got != tc.want {
			t.Errorf("%s: isInstanceUrl(%q) = %v; want %v", tc.name, tc.url, got, tc.want)
		}
	}
}

// P0 checks a URL without the whitespace around it, and stores it that way. The
// provider refuses that whitespace on its own, with a message that says so.
func TestValidateInstanceUrl(t *testing.T) {
	for _, tc := range []struct {
		name, url, summary string
	}{
		{"a URL P0 takes", "https://argocd.example.com", ""},
		{"whitespace around it", "  https://argocd.example.com\n", "Whitespace around a value"},
		{"whitespace around a URL P0 refuses", " https://argocd.example.com/ ", "Invalid Argo CD instance URL"},
		{"a URL P0 refuses", "https://admin:secret@argocd.example.com", "Invalid Argo CD instance URL"},
	} {
		var diags diag.Diagnostics
		validateInstanceUrl(types.StringValue(tc.url), &diags)
		switch {
		case tc.summary == "" && diags.HasError():
			t.Errorf("%s: diagnostics = %v; want none", tc.name, diags)
		case tc.summary != "" && (len(diags) != 1 || diags[0].Summary() != tc.summary):
			t.Errorf("%s: diagnostics = %v; want one %q", tc.name, diags, tc.summary)
		}
	}

	var diags diag.Diagnostics
	validateInstanceUrl(types.StringNull(), &diags)
	validateInstanceUrl(types.StringUnknown(), &diags)
	if diags.HasError() {
		t.Errorf("a null or unknown URL: diagnostics = %v; want none", diags)
	}
}

// testdata/instance_urls.txt holds the app's rule as Node evaluated it, on the URLs of
// the table above, on Harbor's file of cases and on generated ones. Node's URL parser
// is the one P0 runs, so this is the oracle for the cases instance_url.go writes out
// by hand.
func TestInstanceUrlOracle(t *testing.T) {
	file, err := os.Open("testdata/instance_urls.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()

	verdicts := map[string]bool{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "#") {
			continue
		}
		verdict, quoted, _ := strings.Cut(line, " ")
		var url string
		if err := json.Unmarshal([]byte(quoted), &url); err != nil || (verdict != "accept" && verdict != "refuse") {
			t.Fatalf("bad line %q: %v", line, err)
		}
		verdicts[url] = verdict == "accept"
		if got := isInstanceUrl(url); got != verdicts[url] {
			t.Errorf("isInstanceUrl(%q) = %v; Node says %v", url, got, verdicts[url])
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if len(verdicts) < 3000 {
		t.Errorf("read %d cases; want the whole file", len(verdicts))
	}
	for _, tc := range instanceUrlCases {
		if want, ok := verdicts[tc.url]; !ok || want != tc.want {
			t.Errorf("%s: the table says %v for %q; Node says %v (in the file: %v)", tc.name, tc.want, tc.url, want, ok)
		}
	}
}
