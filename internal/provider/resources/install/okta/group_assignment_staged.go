package installokta

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/p0-security/terraform-provider-p0/internal"
	installresources "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install"
)

var _ resource.Resource = &OktaGroupAssignmentStaged{}
var _ resource.ResourceWithImportState = &OktaGroupAssignmentStaged{}
var _ resource.ResourceWithConfigure = &OktaGroupAssignmentStaged{}

func NewOktaGroupAssignmentStaged() resource.Resource {
	return &OktaGroupAssignmentStaged{}
}

type OktaGroupAssignmentStaged struct {
	installer *installresources.Install
}

type oktaGroupAssignmentStagedModel struct {
	Domain string `tfsdk:"domain"`
}

type oktaGroupAssignmentStagedApi struct {
	Item struct {
		State string `json:"state"`
	} `json:"item"`
}

func (r *OktaGroupAssignmentStaged) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_okta_group_assignment_staged"
}

func (r *OktaGroupAssignmentStaged) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A staged installation of P0, on an Okta organization for group assignment.

For instructions on using this resource, see the documentation for ` + "`p0_okta_group_assignment`.",
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

func (r *OktaGroupAssignmentStaged) getId(data any) *string {
	m, ok := data.(*oktaGroupAssignmentStagedModel)
	if !ok {
		return nil
	}
	return &m.Domain
}

func (r *OktaGroupAssignmentStaged) getItemJson(json any) any {
	return json
}

func (r *OktaGroupAssignmentStaged) fromJson(ctx context.Context, diags *diag.Diagnostics, id string, jsonData any) any {
	m := oktaGroupAssignmentStagedModel{}
	m.Domain = id
	return &m
}

func (r *OktaGroupAssignmentStaged) toJson(data any) any {
	json := oktaGroupAssignmentStagedApi{}
	return &json.Item
}

func (r *OktaGroupAssignmentStaged) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *OktaGroupAssignmentStaged) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json oktaGroupAssignmentStagedApi
	var model oktaGroupAssignmentStagedModel
	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &model)
}

func (r *OktaGroupAssignmentStaged) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &oktaGroupAssignmentStagedApi{}, &oktaGroupAssignmentStagedModel{})
}

func (r *OktaGroupAssignmentStaged) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &oktaGroupAssignmentStagedApi{}, &oktaGroupAssignmentStagedModel{})
}

func (r *OktaGroupAssignmentStaged) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Delete(ctx, &resp.Diagnostics, &req.State, &oktaGroupAssignmentStagedModel{})
}

func (r *OktaGroupAssignmentStaged) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("domain"), req, resp)
}
