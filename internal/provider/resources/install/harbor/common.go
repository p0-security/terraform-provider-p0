package installharbor

import (
	"context"
	"fmt"
	"net/http"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/p0-security/terraform-provider-p0/internal"
	installvaultedcredential "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/vaulted-credential"
)

const HarborKey = "harbor"

const previewNote = "**Note:** This integration is currently in preview."

const supportEmail = "support@p0.dev"

// P0's rule for the ID of a Harbor item (packages/integrations/harbor/shared/src/item-id.ts
// in the app). P0 checks the ID as it is, without trimming it.
var itemIdRegex = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$`)

const itemIdMaxLength = 63

// The rule, in P0's words, for the descriptions and for each refusal.
var itemIdRule = fmt.Sprintf("lowercase letters, digits and hyphens, starting and ending with a letter or digit, up to %d characters", itemIdMaxLength)

func isItemId(id string) bool {
	return len(id) <= itemIdMaxLength && itemIdRegex.MatchString(id)
}

// The `id` attribute, which P0 fixes when it creates the item, with P0's rule checked
// at plan time.
func itemIdAttribute(description string) schema.StringAttribute {
	return installvaultedcredential.IdAttribute(
		description,
		stringvalidator.LengthAtMost(itemIdMaxLength),
		stringvalidator.RegexMatches(itemIdRegex, "Use "+itemIdRule),
	)
}

// Imports an item by its ID, once the ID passes P0's rule. The read that follows an
// import puts the ID in a URL path as it is, where one with a "/" or a "#" would read
// some other path. The ID is quoted, so that whitespace around it shows.
func importById(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse, summary string) {
	if !isItemId(req.ID) {
		resp.Diagnostics.AddAttributeError(path.Root("id"), summary, fmt.Sprintf("Use %s. Got: %q", itemIdRule, req.ID))
		return
	}
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func itemPath(component string, id string) string {
	return fmt.Sprintf("integrations/%s/config/%s/%s", HarborKey, component, id)
}

// Refuses to create a Harbor item that P0 already holds, such as one added in the P0
// app. kind names the item, as in "the Harbor connector". P0 accepts a PUT that
// changes nothing over an item it holds, and the PUT returns an installed item to
// "stage", where it does no work until it is verified again. P0 answers 404 for an
// item it doesn't have, and also while Harbor's flag is off, which the PUT that
// follows reports.
func refuseExistingItem(data *internal.P0ProviderData, component string, kind string, id string, diags *diag.Diagnostics) {
	resp, err := data.Get(itemPath(component, id), &struct{}{})
	switch {
	case resp != nil && resp.StatusCode == http.StatusNotFound:
	case err != nil:
		diags.AddError("Error communicating with P0", fmt.Sprintf("Could not check whether the Harbor %s %q exists, got error:\n%s", kind, id, err))
	default:
		diags.AddError(
			fmt.Sprintf("Harbor %s already exists in P0", kind),
			fmt.Sprintf("P0 already has a Harbor %s %q, so Terraform won't create it. "+
				"To manage it here, import it with `terraform import <address> %s`. Otherwise give this resource another `id`.", kind, id, id),
		)
	}
}

// Harbor is behind a feature flag in P0, which each of its components names. While
// the flag is off for an organization, P0 answers every request for a Harbor item with
// a bare 404: the request that stages one, and the verify and configure steps.
func describeHarborNotEnabled(id string, err error) (string, string) {
	return "Harbor isn't enabled for this P0 organization",
		fmt.Sprintf("P0 couldn't stage or install the Harbor connector %q. This usually means Harbor isn't enabled for your organization yet. "+
			"Harbor is in preview: contact %s to have it enabled, then apply again.\n\nP0's response: %s", id, supportEmail, err)
}

// A read gets the same bare 404 while the flag is off. Removing the resource from
// state then would have the next apply stage the connector again, which returns an
// installed connector to the "stage" state. So the read fails, which fails every
// plan, destroy and import too, and the diagnostic says how to go on.
func describeHarborConnectorNotFound(id string, err error) (string, string) {
	return "Harbor connector not found in P0",
		fmt.Sprintf("P0 has no Harbor connector %q. Either Harbor isn't enabled for your organization, or the connector was removed in P0. "+
			"Until one of those changes, every refresh of this resource fails, and so do plan, destroy and import.\n\n"+
			"If Harbor should be enabled, contact %s. To plan or apply other changes meanwhile, run Terraform with `-refresh=false`. "+
			"If the connector was removed in P0, remove this resource from Terraform state with `terraform state rm <address>`, "+
			"and the next apply stages the connector again.\n\nP0's response: %s", id, supportEmail, err)
}

// What the resources' pages say about the read failure above.
var notFoundRecoveryNote = "**If P0 stops finding the connector:** while Harbor isn't enabled for your organization, or after the connector is removed in P0, " +
	"every refresh of this resource fails, and so do `plan`, `destroy` and `import`. Terraform keeps the resource in state rather than stage the connector again. " +
	"If Harbor should be enabled, contact " + supportEmail + ". To plan or apply other changes meanwhile, run Terraform with `-refresh=false`. " +
	"If the connector was removed in P0, remove the resource from state with `terraform state rm <address>`, and the next apply stages it again."
