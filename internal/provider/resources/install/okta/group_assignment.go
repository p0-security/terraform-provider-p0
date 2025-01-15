package installokta

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/p0-security/terraform-provider-p0/internal"
	installresources "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install"
)

var _ resource.Resource = &OktaGroupAssignment{}
var _ resource.ResourceWithImportState = &OktaGroupAssignment{}
var _ resource.ResourceWithConfigure = &OktaGroupAssignment{}

func NewOktaGroupAssignment() resource.Resource {
	return &OktaGroupAssignment{}
}

type OktaGroupAssignment struct {
	installer *installresources.Install
}

type oktaGroupAssignmentModel struct {
	Domain types.String `tfsdk:"domain"`
}

type oktaGroupAssignmentJson struct {
	State string `json:"state"`
}

type oktaGroupAssignmentApi struct {
	Item oktaGroupAssignmentJson `json:"item"`
}

func (r *OktaGroupAssignment) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_okta_group_assignment"
}

func (r *OktaGroupAssignment) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `Final resource for Okta group assignment.

To use this resource, you must also:
- install the ` + "`p0_okta_group_assignment_staged`" + ` resource,
- Grant the P0 Okta application the "Group Membership Administrator" role assignment,

See the example usage for the recommended pattern to define this infrastructure.`,
		Attributes: map[string]schema.Attribute{
			"domain": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The Okta domain for group assignment",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

func (r *OktaGroupAssignment) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	providerData := internal.Configure(&req, resp)
	r.installer = &installresources.Install{
		Integration:  "okta",
		Component:    installresources.GroupAssignment,
		ProviderData: providerData,
		GetId:        r.getId,
		GetItemJson:  r.getItemJson,
		FromJson:     r.fromJson,
		ToJson:       r.toJson,
	}
}

func (r *OktaGroupAssignment) getId(data any) *string {
	model, ok := data.(*oktaGroupAssignmentModel)
	if !ok {
		return nil
	}
	str := model.Domain.ValueString()
	return &str
}

func (r *OktaGroupAssignment) getItemJson(json any) any {
	api, ok := json.(*oktaGroupAssignmentApi)
	if !ok {
		return nil
	}
	return api.Item
}

func (r *OktaGroupAssignment) fromJson(ctx context.Context, diags *diag.Diagnostics, id string, jsonData any) any {
	model := oktaGroupAssignmentModel{}
	_, ok := jsonData.(oktaGroupAssignmentJson)
	if !ok {
		return nil
	}
	model.Domain = types.StringValue(id)
	return &model
}

func (r *OktaGroupAssignment) toJson(data any) any {
	out := oktaGroupAssignmentApi{}
	out.Item.State = "configure"
	return &out
}

func (r *OktaGroupAssignment) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json oktaGroupAssignmentApi
	var data oktaGroupAssignmentModel
	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data)
}

func (r *OktaGroupAssignment) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var json oktaGroupAssignmentApi
	var data oktaGroupAssignmentModel
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &json, &data)
}

func (r *OktaGroupAssignment) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var json oktaGroupAssignmentApi
	var data oktaGroupAssignmentModel
	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data)
}

func (r *OktaGroupAssignment) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data oktaGroupAssignmentModel
	r.installer.Rollback(ctx, &resp.Diagnostics, &req.State, &data)
}

func (r *OktaGroupAssignment) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("domain"), req, resp)
}
