package installharbor

import (
	"bufio"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// The app's cases for its rule (packages/integrations/harbor/shared/src/__tests__/
// instance-url.test.ts), then the cases of the WHATWG parser's that instance_url.go
// writes out. TestInstanceUrlOracle checks every verdict here against Node's.
var instanceUrlCases = []struct {
	name string
	url  string
	want bool
}{
	{"a host", "https://harbor.example.com", true},
	{"a single-label host", "https://harbor", true},
	{"a port", "https://localhost:8443", true},
	{"an IPv4 address", "https://10.0.0.7", true},
	{"an IDN in Punycode", "https://xn--hrbor-gra.example.com", true},

	{"a trailing slash", "https://harbor.example.com/", false},
	{"the default port", "https://harbor.example.com:443", false},
	{"a port with a leading zero", "https://harbor.example.com:08443", false},
	{"a colon and no port", "https://harbor.example.com:", false},
	{"an uppercase scheme and host", "HTTPS://Harbor.Example.COM", false},
	{"an uppercase letter in the host", "https://Harbor.example.com", false},
	{"no slashes after the scheme", "https:harbor.example.com", false},
	{"backslashes for slashes", `https:\\harbor.example.com\`, false},
	{"a tab and a newline inside it", "https://harbor\t.example\n.com", false},
	{"an empty query and fragment", "https://harbor.example.com/?#", false},
	{"an empty username", "https://@harbor.example.com", false},
	{"an empty username and password", "https://:@harbor.example.com", false},
	{"a trailing NUL", "https://harbor.example.com\x00", false},
	{"an IPv4 address in hexadecimal", "https://0x7f.1", false},
	{"an IPv4 address in three parts", "https://1.2.3", false},
	{"an IDN in Unicode", "https://härbor.example.com", false},
	{"a quote in the host", `https://har"bor.example.com`, false},
	{"an interpolation in the host", "https://${var.host}.example.com", false},
	{"an underscore in the host", "https://harbor_1.example.com", false},
	{"an IPv6 literal", "https://[::1]:8443", false},
	{"plain http", "http://harbor.example.com", false},
	{"a scheme that only starts with https", "httpss://harbor.example.com", false},
	{"another scheme", "ftp://harbor.example.com", false},
	{"no scheme", "harbor.example.com", false},
	{"a scheme-relative URL", "//harbor.example.com", false},
	{"an empty string", "", false},
	{"a path", "https://harbor.example.com/harbor", false},
	{"Harbor's API path", "https://harbor.example.com/api/v2.0", false},
	{"a second slash", "https://harbor.example.com//", false},
	{"a query", "https://harbor.example.com/?page=1", false},
	{"a fragment", "https://harbor.example.com/#projects", false},
	{"a username and password", "https://robot$p0-connector:secret@harbor.example.com", false},
	{"a username alone", "https://robot@harbor.example.com", false},
	{"a password alone", "https://:secret@harbor.example.com", false},
	{"a password and no host", "https://robot:secret@", false},
	{"a password and a port that isn't one", "https://robot:secret@harbor.example.com:port", false},

	{"port 0", "https://harbor:0", true},
	{"the highest port", "https://harbor:65535", true},
	{"a port past the highest", "https://harbor:65536", false},
	{"a port too long for a number", "https://harbor:99999999999999999999", false},
	{"port 0 with a leading zero", "https://harbor:00", false},
	{"port 443 with a leading zero", "https://harbor.example.com:0443", false},
	{"an IPv4 address with a port", "https://1.2.3.4:8443", true},
	{"an IPv4 address with the default port", "https://1.2.3.4:443", false},
	{"the highest IPv4 address", "https://255.255.255.255", true},
	{"the lowest IPv4 address", "https://0.0.0.0", true},
	{"an IPv4 address with a trailing dot", "https://1.2.3.4.", false},
	{"an IPv4 address with two trailing dots", "https://1.2.3.4..", true},
	{"an IPv4 address with a leading zero", "https://1.2.3.04", false},
	{"an IPv4 address in octal", "https://010.0.0.1", false},
	{"an IPv4 address with a hexadecimal part", "https://0x7f.0.0.1", false},
	{"an IPv4 address past the highest part", "https://1.2.3.256", false},
	{"an IPv4 address as one number", "https://4294967295", false},
	{"a bare 0", "https://0", false},
	{"a bare 0x", "https://0x", false},
	{"a name ending in a number", "https://harbor.123", false},
	{"a name ending in 0x", "https://harbor.0x", false},
	{"a name ending in a hexadecimal number", "https://harbor.0x1f", false},
	{"a name ending in a number that isn't octal", "https://x.09", false},
	{"a name ending in 0x and a letter", "https://a.0xg", true},
	{"a name ending in digits and a hyphen", "https://a.1-2", true},
	{"a name with a trailing dot", "https://harbor.example.com.", true},
	{"a name with an empty label", "https://a..b", true},
	{"a name that is a hyphen", "https://-", true},
	{"a name that is a dot", "https://.", true},
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
		{"a URL P0 takes", "https://harbor.example.com", ""},
		{"whitespace around it", "  https://harbor.example.com\n", "Whitespace around a value"},
		{"whitespace around a URL P0 refuses", " https://harbor.example.com/ ", "Invalid Harbor instance URL"},
		{"a URL P0 refuses", "https://robot:secret@harbor.example.com", "Invalid Harbor instance URL"},
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
// the table above and on about 3,000 generated ones. Node's URL parser is the one P0
// runs, so this is the oracle for the cases instance_url.go writes out by hand.
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
