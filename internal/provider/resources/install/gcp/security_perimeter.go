package installgcp

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/p0-security/terraform-provider-p0/internal"
	"github.com/p0-security/terraform-provider-p0/internal/common"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &GcpSecurityPerimeter{}
var _ resource.ResourceWithImportState = &GcpSecurityPerimeter{}
var _ resource.ResourceWithConfigure = &GcpSecurityPerimeter{}

func NewGcpSecurityPerimeter() resource.Resource {
	return &GcpSecurityPerimeter{}
}

type GcpSecurityPerimeter struct {
	installer *common.Install
}

type gcpSecurityPerimeterModel struct {
	State          types.String `tfsdk:"state"`
	Project        types.String `tfsdk:"project"`
	CloudRunUrl    types.String `tfsdk:"cloud_run_url"`
	AllowedDomains types.String `tfsdk:"allowed_domains"`
	ImageDigest    types.String `tfsdk:"image_digest"`
}

type gcpSecurityPerimeterJson struct {
	State          *string `json:"state,omitempty"`
	CloudRunUrl    *string `json:"cloudRunUrl,omitempty"`
	AllowedDomains *string `json:"allowedDomains,omitempty"`
	ImageDigest    *string `json:"imageDigest,omitempty"`
}

type gcpSecurityPerimeterApi struct {
	Item gcpSecurityPerimeterJson `json:"item"`
}

func (r *GcpSecurityPerimeter) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_gcp_security_perimeter"
}

func (r *GcpSecurityPerimeter) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `An installation of the P0 Security Perimeter, for a Google Cloud Project,
which creates a security boundary for P0.

To use this resource, you must also:
- Install the ` + "`p0_gcp_security_perimeter_staged`" + ` resource.
- Install the ` + "`p0_gcp_iam_write`" + ` resource.
- Deploy the P0 Security Perimeter cloud run service and the corresponding service account.`,
		Attributes: map[string]schema.Attribute{
			"project": projectAttribute,
			"state":   stateAttribute,
			"cloud_run_url": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: `The URL of the Cloud Run service that will be used to enforce the security perimeter.`,
				Validators: []validator.String{
					stringvalidator.RegexMatches(
						GcpCloudRunUrlRegex,
						"Value must be a valid URL.",
					),
				},
			},
			"allowed_domains": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: `The list of domains that are allowed to access the Cloud Run service.`,
			},
			"image_digest": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: `The hash value of the image that is deployed to the Cloud Run service.`,
			},
		},
	}
}

func (r *GcpSecurityPerimeter) getItemJson(json any) any {
	inner, ok := json.(*gcpSecurityPerimeterApi)
	if !ok {
		return nil
	}
	return &inner.Item
}

func (r *GcpSecurityPerimeter) fromJson(ctx context.Context, diags *diag.Diagnostics, id string, json any) any {
	data := gcpSecurityPerimeterModel{}
	jsonv, ok := json.(*gcpSecurityPerimeterJson)
	if !ok {
		return nil
	}

	data.Project = types.StringValue(id)
	data.State = types.StringNull()
	if jsonv.State != nil {
		state := types.StringValue(*jsonv.State)
		data.State = state
	}

	data.CloudRunUrl = types.StringNull()
	if jsonv.CloudRunUrl != nil {
		gcloudRunUrl := types.StringValue(*jsonv.CloudRunUrl)
		data.CloudRunUrl = gcloudRunUrl
	}

	data.AllowedDomains = types.StringNull()
	if jsonv.AllowedDomains != nil {
		allowedDomains := types.StringValue(*jsonv.AllowedDomains)
		data.AllowedDomains = allowedDomains
	}

	data.ImageDigest = types.StringNull()
	if jsonv.ImageDigest != nil {
		imageDigest := types.StringValue(*jsonv.ImageDigest)
		data.ImageDigest = imageDigest
	}

	return &data
}

func (r *GcpSecurityPerimeter) toJson(data any) any {
	json := gcpSecurityPerimeterJson{}
	datav, ok := data.(*gcpSecurityPerimeterModel)
	if !ok {
		return nil
	}

	if !datav.CloudRunUrl.IsNull() && !datav.CloudRunUrl.IsUnknown() {
		cloudRunUrl := datav.CloudRunUrl.ValueString()
		json.CloudRunUrl = &cloudRunUrl
	}

	if !datav.AllowedDomains.IsNull() && !datav.AllowedDomains.IsUnknown() {
		allowedDomains := datav.AllowedDomains.ValueString()
		json.AllowedDomains = &allowedDomains
	}

	if !datav.ImageDigest.IsNull() && !datav.ImageDigest.IsUnknown() {
		imageDigest := datav.ImageDigest.ValueString()
		json.ImageDigest = &imageDigest
	}

	return json
}

func (r *GcpSecurityPerimeter) getId(data any) *string {
	model, ok := data.(*gcpSecurityPerimeterModel)
	if !ok {
		return nil
	}

	str := model.Project.ValueString()
	return &str
}

func (r *GcpSecurityPerimeter) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (s *GcpSecurityPerimeter) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var api gcpSecurityPerimeterApi
	var model gcpSecurityPerimeterModel
	s.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &api, &model)
}

func (s *GcpSecurityPerimeter) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	s.installer.Read(ctx, &resp.Diagnostics, &resp.State, &gcpSecurityPerimeterApi{}, &gcpSecurityPerimeterModel{})
}

func (s *GcpSecurityPerimeter) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	s.installer.Delete(ctx, &resp.Diagnostics, &req.State, &gcpSecurityPerimeterModel{})
}

func (s *GcpSecurityPerimeter) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	s.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &gcpSecurityPerimeterApi{}, &gcpSecurityPerimeterModel{})
}

func (s *GcpSecurityPerimeter) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("project"), req, resp)
}
