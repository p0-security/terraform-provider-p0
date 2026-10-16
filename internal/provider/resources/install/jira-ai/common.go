package installjiraai

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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/p0-security/terraform-provider-p0/internal"
	"github.com/p0-security/terraform-provider-p0/internal/common"
)

// The Jira (AI) integration's install items: a connector, the Jira Cloud sites it
// reaches, and the agent accounts P0 leases on each site
// (packages/integrations/jira-ai/shared/src/components.ts in the app).
const JiraAiKey = "jira-ai"

// betaNote, supportEmail, the item ID rule and the helpers below are copied from
// Harbor's package (harbor/common.go) unless a comment says otherwise.

const betaNote = "**Note:** This integration is currently in beta."

const supportEmail = "support@p0.dev"

// P0's rule for the ID of a Jira (AI) item (packages/integrations/jira-ai/shared/src/item-id.ts
// in the app). P0 checks the ID as it is, without trimming it. A site's ID is shorter,
// because the IDs of the custom roles its Terraform creates carry it.
var itemIdRegex = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$`)

const (
	itemIdMaxLength = 63
	siteIdMaxLength = 48
)

// The ID rules as the app words them, for the descriptions and the refusals.
var (
	itemIdRule = fmt.Sprintf("lowercase letters, digits and hyphens, starting and ending with a letter or digit, up to %d characters", itemIdMaxLength)
	siteIdRule = fmt.Sprintf("lowercase letters, digits and hyphens, starting and ending with a letter or digit, up to %d characters", siteIdMaxLength)
)

func isItemId(id string) bool {
	return len(id) <= itemIdMaxLength && itemIdRegex.MatchString(id)
}

func isSiteId(id string) bool {
	return len(id) <= siteIdMaxLength && itemIdRegex.MatchString(id)
}

func itemIdValidators() []validator.String {
	return []validator.String{
		stringvalidator.LengthAtMost(itemIdMaxLength),
		stringvalidator.RegexMatches(itemIdRegex, "Use "+itemIdRule),
	}
}

func siteIdValidators() []validator.String {
	return []validator.String{
		stringvalidator.LengthAtMost(siteIdMaxLength),
		stringvalidator.RegexMatches(itemIdRegex, "Use "+siteIdRule),
	}
}

// A required string that P0 fixes when it creates the item and refuses to change, so
// changing it replaces the item.
func fixedAttribute(description string, validators ...validator.String) schema.StringAttribute {
	return schema.StringAttribute{
		Required:            true,
		MarkdownDescription: description,
		Validators:          validators,
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.RequiresReplace(),
		},
	}
}

// A string that P0 sets once, and that stays as it is until the item is replaced.
func generatedAttribute(description string) schema.StringAttribute {
	return schema.StringAttribute{
		Computed:            true,
		MarkdownDescription: description,
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.UseStateForUnknown(),
		},
	}
}

// A value that isn't blank and has no whitespace around it. P0 trims the strings it
// stores, so a value with whitespace around it would read back different from the
// configuration.
var trimmedRegex = regexp.MustCompile(`(?s)^\S(?:.*\S)?$`)

func trimmedValidator(message string) validator.String {
	return stringvalidator.RegexMatches(trimmedRegex, message)
}

// A copy of Harbor's importById (harbor/common.go), with the ID rule as a parameter,
// since a site's ID has its own.
//
// Imports an item by its ID once the ID passes the app's rule. The read after an import
// puts the ID in a URL path unescaped, so an ID with "/" or "#" would read another
// path. The message quotes the ID so whitespace shows.
func importById(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse, summary string, valid func(string) bool, rule string) {
	if !valid(req.ID) {
		resp.Diagnostics.AddAttributeError(path.Root("id"), summary, fmt.Sprintf("Use %s. Got: %q", rule, req.ID))
		return
	}
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// Copies of Harbor's itemPath and refuseExistingItem (harbor/common.go).
func itemPath(component string, id string) string {
	return fmt.Sprintf("integrations/%s/config/%s/%s", JiraAiKey, component, id)
}

// Refuses to create an item that P0 already holds, such as one added in the P0 app,
// and says to import it. P0 would accept a PUT over it and return it to "stage", where
// it does nothing until it's verified again. P0 answers 404 for an item it doesn't
// have. kind names the item in the message, as in "connector".
func refuseExistingItem(data *internal.P0ProviderData, component string, kind string, id string, diags *diag.Diagnostics) {
	resp, err := data.Get(itemPath(component, id), &struct{}{})
	switch {
	case resp != nil && resp.StatusCode == http.StatusNotFound:
	case err != nil:
		diags.AddError("Error communicating with P0", fmt.Sprintf("Could not check whether the Jira %s %q exists, got error:\n%s", kind, id, err))
	default:
		diags.AddError(
			fmt.Sprintf("Jira %s already exists in P0", kind),
			fmt.Sprintf("P0 already has a Jira %s %q, so Terraform won't create it. "+
				"To manage it here, import it with `terraform import <address> %s`. Otherwise give this resource another `id`.", kind, id, id),
		)
	}
}

// A copy of describeCheckError in the onepassword package.
//
// Most install checks run against the customer's own setup (the connector, the Jira
// site, the admin client and its secret), so a failure usually points there. The
// summary doesn't guess which, and leaves the cause to P0's message.
func describeCheckError(kind string) common.DescribeError {
	return func(id string, err error) (string, string) {
		return fmt.Sprintf("Jira %s install check failed", kind),
			fmt.Sprintf("P0 rejected the install check for the Jira %s %q:\n\n%s", kind, id, err)
	}
}

// Plans an update for an item that P0 hasn't finished installing, even when the
// configuration hasn't changed, so the next apply finishes the install. That covers an
// item imported before its checks passed, or one staged again outside Terraform.
//
// The item's state and the attributes in computed, which the install checks set, are
// unknown until apply. Update reads them from P0's response.
//
// A copy of planUnfinishedInstall in the onepassword package.
func planUnfinishedInstall(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse, computed ...path.Path) {
	// Nothing is installed yet on create, and nothing is left to finish on destroy.
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}

	var state types.String
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("state"), &state)...)
	if resp.Diagnostics.HasError() || state.ValueString() == common.StateInstalled {
		return
	}

	for _, attribute := range append([]path.Path{path.Root("state")}, computed...) {
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, attribute, types.StringUnknown())...)
	}
}

// Finishes an install from the step P0 has the item at. Every settable attribute
// requires replacement, so this is the only update Terraform plans. An item P0 has
// verified gets the configure step alone. A staged item is verified first.
func finishInstall(ctx context.Context, installer *common.Install, req resource.UpdateRequest, resp *resource.UpdateResponse, json any, model any) {
	var state types.String
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("state"), &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	switch state.ValueString() {
	case common.StateConfigure, common.StateInstalled:
		installer.UpsertFromConfigure(ctx, &resp.Diagnostics, &req.Plan, &resp.State, json, model)
	default:
		installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, json, model)
	}
}

// Creates an item that one resource installs: stage, then verify and configure. The
// Create of Harbor's p0_harbor_instance (harbor/instance.go), shared by the site and
// the agent account.
//
// It refuses an item P0 already holds. The PUT would return an installed item to
// "stage", and if a check then failed, the tainted resource's next apply would delete
// an item Terraform never created.
//
// If verify or configure fails, the item stays where it got to and the resource is
// tainted, so the next apply deletes the item and creates it again.
func createItem(ctx context.Context, installer *common.Install, kind string, req resource.CreateRequest, resp *resource.CreateResponse, json any, model any) {
	installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, model)
	if resp.Diagnostics.HasError() {
		return
	}

	id := installer.GetId(model)
	if id == nil {
		resp.Diagnostics.AddError("Internal error", "Could not read the "+kind+"'s id from the plan.")
		return
	}
	refuseExistingItem(installer.ProviderData, installer.Component, kind, *id, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, json, model, installer.ToJson(model))
	if resp.Diagnostics.HasError() {
		return
	}

	installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, json, model)
}
