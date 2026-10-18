package installjira

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	installapp "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/app"
)

const JiraKey = "jira"

// The install component of a Jira site. It is kept here, and not in
// installresources, so that this package shares nothing with Jira (AI)'s.
const siteComponent = "site"

// P0's rule for a site's host, the ID of a `site` item, and its message for a host it
// refuses (packages/integrations/jira/shared/src/site.ts in the app). P0 checks the ID
// as it is, without trimming it.
var siteHostRegex = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*\.atlassian\.net$`)

const invalidSiteHost = "Enter the site's host in lowercase, as <name>.atlassian.net, such as acme.atlassian.net. Leave out https:// and anything after the host."

const invalidSiteHostSummary = "Invalid Jira site"

// A rule for a value, and P0's message for a value that breaks it.
type rule struct {
	summary string
	message string
	test    func(value string) bool
}

// Rejects value, at attribute, with P0's message for the first of rules that it
// breaks. Like P0, it checks the value without the whitespace around it. That
// whitespace is rejected on its own: P0 trims the strings it stores, so the value
// would read back different from the configuration, which Terraform reports as an
// inconsistent result after apply. A null value is left to the checks that a field is
// set. The value is never repeated: a secret's name or a client ID may be the client
// secret, pasted into the wrong field.
//
// A copy of github-repositories' validateField.
func validateField(attribute path.Path, value types.String, rules []rule, diags *diag.Diagnostics) {
	if !installapp.IsSet(value) {
		return
	}
	trimmed := strings.TrimSpace(value.ValueString())
	for _, r := range rules {
		if !r.test(trimmed) {
			diags.AddAttributeError(attribute, r.summary, r.message)
			return
		}
	}
	if trimmed != value.ValueString() {
		diags.AddAttributeError(
			attribute,
			"Whitespace around a value",
			fmt.Sprintf("P0 removes the whitespace around '%s', so the value it stores wouldn't match this configuration. "+
				"Remove the whitespace, for example with trimspace().", attribute),
		)
	}
}

// The value as P0 stores it, without the whitespace around it.
func trimmed(value types.String) string {
	return strings.TrimSpace(value.ValueString())
}

// P0 checks the item's ID as it is, without trimming it.
func validateSiteHost(site types.String, diags *diag.Diagnostics) {
	if installapp.IsSet(site) && !siteHostRegex.MatchString(site.ValueString()) {
		diags.AddAttributeError(path.Root("site"), invalidSiteHostSummary, invalidSiteHost)
	}
}
