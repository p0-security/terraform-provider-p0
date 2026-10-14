package installharbor

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	installapp "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/app"
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
