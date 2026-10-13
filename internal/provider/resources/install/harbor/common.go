package installharbor

import (
	"context"
	"fmt"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	installapp "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/app"
)

const HarborKey = "harbor"

const previewNote = "**Note:** This integration is currently in preview."

// P0's rule for the ID of a Harbor item, and its message for an ID that breaks it
// (packages/integrations/harbor/shared/src/item-id.ts in the app). P0 checks the ID as
// it is, without trimming it.
var itemIdRegex = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$`)

const itemIdMaxLength = 63

const invalidItemId = "Use lowercase letters, digits and hyphens, starting and ending with a letter or digit, up to 63 characters."

func isItemId(id string) bool {
	return len(id) <= itemIdMaxLength && itemIdRegex.MatchString(id)
}

// P0's message for an ID it refuses, with the ID quoted, so that whitespace around it
// shows.
func itemIdError(id string) string {
	return fmt.Sprintf("%s Got: %q", invalidItemId, id)
}

// Rejects an `id` that P0 would refuse, at plan time and in P0's words. An ID that
// Terraform only knows at apply time is left to P0.
func validateItemId(id types.String, summary string, diags *diag.Diagnostics) {
	if installapp.IsSet(id) && !isItemId(id.ValueString()) {
		diags.AddAttributeError(path.Root("id"), summary, itemIdError(id.ValueString()))
	}
}

// Imports an item by its ID, once the ID passes P0's rule. The read that follows an
// import puts the ID in a URL path as it is, where one with a "/" or a "#" would read
// some other path.
func importById(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse, summary string) {
	if !isItemId(req.ID) {
		resp.Diagnostics.AddAttributeError(path.Root("id"), summary, itemIdError(req.ID))
		return
	}
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// Harbor is behind a feature flag in P0, which each of its components names. While
// the flag is off for an organization, P0 answers every request for a Harbor item with
// a bare 404, the request that stages one included.
func describeUnavailable(id string, err error) (string, string) {
	return "Harbor is not available in this P0 organization",
		fmt.Sprintf("P0 found no Harbor integration to stage %q in, which is how it answers while Harbor isn't enabled for an organization. "+
			"Harbor is in preview: contact support@p0.dev to have it enabled, then apply again.\n\nP0's answer: %s", id, err)
}
