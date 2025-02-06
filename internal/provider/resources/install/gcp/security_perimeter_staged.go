package installgcp

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/p0-security/terraform-provider-p0/internal"
	"github.com/p0-security/terraform-provider-p0/internal/common"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &GcpSecurityPerimeterStage{}
var _ resource.ResourceWithImportState = &GcpSecurityPerimeterStage{}
var _ resource.ResourceWithConfigure = &GcpSecurityPerimeterStage{}

func NewGcpSecurityPerimeterStage() resource.Resource {
	return &GcpSecurityPerimeterStage{}
}

type GcpSecurityPerimeterStage struct {
	installer *common.Install
}

type gcpSecurityPerimeterStageModel struct {
	State             types.String `tfsdk:"state"`
	Project           types.String `tfsdk:"project"`
	AllowedDomains    types.String `tfsdk:"allowed_domains"`
	ImageDigest       types.String `tfsdk:"image_digest"`
	CustomRole        types.Object `tfsdk:"custom_role"`
	Permissions       types.List   `tfsdk:"required_permissions"`
	ProjectReaderRole types.Object `tfsdk:"project_reader_role"`
}

type gcpProjectReaderRoleMetadata struct {
	CustomRole  gcpRoleMetadata `json:"customRole" tfsdk:"custom_role"`
	Permissions []string        `json:"requiredPermissions" tfsdk:"required_permissions"`
}

type gcpSecurityPerimeterStageMetadata struct {
	CustomRole        gcpRoleMetadata              `json:"customRole" tfsdk:"custom_role"`
	Permissions       []string                     `json:"requiredPermissions" tfsdk:"required_permissions"`
	ProjectReaderRole gcpProjectReaderRoleMetadata `json:"projectReaderRole" tfsdk:"project_reader_role"`
}

type gcpSecurityPerimeterStageApi struct {
	Item struct {
		State          *string `json:"state,omitempty"`
		AllowedDomains *string `json:"allowedDomains,omitempty"`
		ImageDigest    *string `json:"imageDigest,omitempty"`
	} `json:"item"`
	Metadata gcpSecurityPerimeterStageMetadata `json:"metadata"`
}

var customRoleAttrTypes = map[string]attr.Type{"id": types.StringType, "name": types.StringType}

var projectReaderRoleAttrTypes = types.ObjectType{
	AttrTypes: map[string]attr.Type{
		"custom_role": types.ObjectType{
			AttrTypes: customRoleAttrTypes,
		},
		"required_permissions": types.ListType{ElemType: types.StringType},
	},
}

func (r *GcpSecurityPerimeterStage) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_gcp_security_perimeter_staged"
}

func (r *GcpSecurityPerimeterStage) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A staged installation of the P0 Security Perimeter, for a Google Cloud Project,
which creates a security boundary for P0.

To use this resource, you must also:
- Install the ` + "`p0_gcp_iam_write`" + ` resource.
- Deploy the P0 Security Perimeter cloud run service and the corresponding service account.
- install the ` + "`p0_gcp_security_perimeter`" + ` resource.`,
		Attributes: map[string]schema.Attribute{
			"project": projectAttribute,
			"state":   common.StateAttribute,
			"allowed_domains": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: `The list of domains that are allowed to access the Cloud Run service.`,
			},
			"image_digest": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: `The hash value of the image that is deployed to the Cloud Run service.`,
			},
			"custom_role": customRole,
			"required_permissions": schema.ListAttribute{
				ElementType:         types.StringType,
				Computed:            true,
				MarkdownDescription: `A list of permissions required by the security perimeter invoker role.`,
			},
			"project_reader_role": schema.SingleNestedAttribute{
				Computed:    true,
				Description: `Describes the project reader role that should be created and assigned to P0's service account`,
				Attributes: map[string]schema.Attribute{
					"custom_role": customRole,
					"required_permissions": schema.ListAttribute{
						ElementType:         types.StringType,
						Computed:            true,
						MarkdownDescription: "Described the permissions that the project reader role should contain.",
					},
				},
			},
		},
	}
}

func (r *GcpSecurityPerimeterStage) getItemJson(json any) any {
	return json
}

func (r *GcpSecurityPerimeterStage) fromJson(ctx context.Context, diags *diag.Diagnostics, id string, json any) any {
	data := gcpSecurityPerimeterStageModel{}
	jsonv, ok := json.(*gcpSecurityPerimeterStageApi)
	if !ok {
		return nil
	}

	data.Project = types.StringValue(id)
	data.State = types.StringNull()
	if jsonv.Item.State != nil {
		state := types.StringValue(*jsonv.Item.State)
		data.State = state
	}

	data.AllowedDomains = types.StringNull()
	if jsonv.Item.AllowedDomains != nil {
		allowedDomains := types.StringValue(*jsonv.Item.AllowedDomains)
		data.AllowedDomains = allowedDomains
	}

	data.ImageDigest = types.StringNull()
	if jsonv.Item.ImageDigest != nil {
		imageDigest := types.StringValue(*jsonv.Item.ImageDigest)
		data.ImageDigest = imageDigest
	}

	customRole, objErr := types.ObjectValueFrom(ctx, customRoleAttrTypes, jsonv.Metadata.CustomRole)
	if objErr.HasError() {
		diags.Append(objErr...)
		return nil
	}
	data.CustomRole = customRole

	permissions, objErr := types.ListValueFrom(ctx, types.StringType, jsonv.Metadata.Permissions)
	if objErr.HasError() {
		diags.Append(objErr...)
		return nil
	}
	data.Permissions = permissions

	projectReaderRole, objErr := types.ObjectValueFrom(ctx, projectReaderRoleAttrTypes.AttrTypes, jsonv.Metadata.ProjectReaderRole)
	if objErr.HasError() {
		diags.Append(objErr...)
		return nil
	}
	data.ProjectReaderRole = projectReaderRole

	return &data
}

func (r *GcpSecurityPerimeterStage) toJson(data any) any {
	json := gcpSecurityPerimeterStageApi{}
	datav, ok := data.(*gcpSecurityPerimeterStageModel)
	if !ok {
		return nil
	}

	if !datav.AllowedDomains.IsNull() && !datav.AllowedDomains.IsUnknown() {
		allowedDomains := datav.AllowedDomains.ValueString()
		json.Item.AllowedDomains = &allowedDomains
	}

	if !datav.ImageDigest.IsNull() && !datav.ImageDigest.IsUnknown() {
		imageDigest := datav.ImageDigest.ValueString()
		json.Item.ImageDigest = &imageDigest
	}

	return json
}

func (r *GcpSecurityPerimeterStage) getId(data any) *string {
	model, ok := data.(*gcpSecurityPerimeterStageModel)
	if !ok {
		return nil
	}

	str := model.Project.ValueString()
	return &str
}

func (r *GcpSecurityPerimeterStage) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	providerData := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:  GcpKey,
		Component:    SecurityPerimeter,
		ProviderData: providerData,
		GetId:        r.getId,
		GetItemJson:  r.getItemJson,
		FromJson:     r.fromJson,
		ToJson:       r.toJson,
	}
}

func (s *GcpSecurityPerimeterStage) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var api gcpSecurityPerimeterStageApi
	var model gcpSecurityPerimeterStageModel
	s.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &model)
	s.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &api, &model)
}

func (s *GcpSecurityPerimeterStage) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	s.installer.Read(ctx, &resp.Diagnostics, &resp.State, &gcpSecurityPerimeterStageApi{}, &gcpSecurityPerimeterStageModel{})
}

func (s *GcpSecurityPerimeterStage) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	s.installer.Delete(ctx, &resp.Diagnostics, &req.State, &gcpSecurityPerimeterStageModel{})
}

func (s *GcpSecurityPerimeterStage) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	s.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &gcpSecurityPerimeterStageApi{}, &gcpSecurityPerimeterStageModel{})
}

func (s *GcpSecurityPerimeterStage) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("project"), req, resp)
}
