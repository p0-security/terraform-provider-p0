package installlaunchdarkly

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

// A copy of Harbor's (internal/provider/resources/install/harbor), kept apart so that
// LaunchDarkly changes no other integration.

const LaunchDarklyKey = "launchdarkly"

const betaNote = "**Note:** This integration is currently in beta."

const supportEmail = "support@p0.dev"

// The page that describes the install, for the resources' descriptions.
const installDocs = "For the full install steps, see [Installing LaunchDarkly](https://docs.p0.dev/integrations/resource-integrations/launchdarkly/installation)."

// P0's rule for the ID of a LaunchDarkly item (packages/integrations/launchdarkly/shared/src/item-id.ts
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
	return fmt.Sprintf("integrations/%s/config/%s/%s", LaunchDarklyKey, component, id)
}

// Refuses to create a LaunchDarkly item that P0 already holds, such as one added in
// the P0 app. kind names the item, as in "the LaunchDarkly connector". P0 accepts a
// PUT that changes nothing over an item it holds and hasn't installed, and the PUT
// returns the item to "stage", where it does no work until it is verified again. P0
// answers 404 for an item it doesn't have.
func refuseExistingItem(data *internal.P0ProviderData, component string, kind string, id string, diags *diag.Diagnostics) {
	resp, err := data.Get(itemPath(component, id), &struct{}{})
	switch {
	case resp != nil && resp.StatusCode == http.StatusNotFound:
	case err != nil:
		diags.AddError("Error communicating with P0", fmt.Sprintf("Could not check whether the LaunchDarkly %s %q exists, got error:\n%s", kind, id, err))
	default:
		diags.AddError(
			fmt.Sprintf("LaunchDarkly %s already exists in P0", kind),
			fmt.Sprintf("P0 already has a LaunchDarkly %s %q, so Terraform won't create it. "+
				"To manage it here, import it with `terraform import <address> %s`. Otherwise give this resource another `id`.", kind, id, id),
		)
	}
}
