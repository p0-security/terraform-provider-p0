package installharbor

import (
	"context"
	"fmt"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
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

// Harbor is behind a feature flag in P0, which each of its components names. While
// the flag is off for an organization, P0 answers every request for a Harbor item with
// a bare 404, the request that stages one included.
func describeHarborNotEnabled(id string, err error) (string, string) {
	return "Harbor isn't enabled for this P0 organization",
		fmt.Sprintf("P0 couldn't stage the Harbor connector %q. This usually means Harbor isn't enabled for your organization yet. "+
			"Harbor is in preview: contact %s to have it enabled, then apply again.\n\nP0's response: %s", id, supportEmail, err)
}

// A read gets the same bare 404 while the flag is off. Removing the resource from
// state then would have the next apply stage the connector again, which returns an
// installed connector to the "stage" state. So the read fails, and says how to go on
// if the connector is gone for good.
func describeHarborConnectorNotFound(id string, err error) (string, string) {
	return "Harbor connector not found in P0",
		fmt.Sprintf("P0 has no Harbor connector %q. Either Harbor isn't enabled for your organization, or the connector was removed in P0. "+
			"If Harbor should be enabled, contact %s. If the connector was removed, remove it from Terraform state with `terraform state rm`, "+
			"and the next apply stages it again.\n\nP0's response: %s", id, supportEmail, err)
}
