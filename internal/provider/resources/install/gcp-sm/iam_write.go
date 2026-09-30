package installgcpsm

import (
	"context"

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
	installresources "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install"
	installgcp "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/gcp"
)

const GcpSmKey = "gcp-sm"

var _ resource.Resource = &GcpSmIamWrite{}
var _ resource.ResourceWithConfigure = &GcpSmIamWrite{}
var _ resource.ResourceWithImportState = &GcpSmIamWrite{}

type GcpSmIamWrite struct {
	installer *common.Install
}

type gcpSmIamWriteModel struct {
	Project types.String `tfsdk:"project"`
	Label   types.String `tfsdk:"label"`
	State   types.String `tfsdk:"state"`
}

type gcpSmIamWriteJson struct {
	Label *string `json:"label,omitempty"`
	State string  `json:"state"`
}

type gcpSmIamWriteApi struct {
	Item *gcpSmIamWriteJson `json:"item"`
}

func NewGcpSmIamWrite() resource.Resource {
	return &GcpSmIamWrite{}
}

func (*GcpSmIamWrite) Metadata(_ context.Context, req resource.MetadataRequest, res *resource.MetadataResponse) {
	res.TypeName = req.ProviderTypeName + "_gcp_secret_manager"
}

func (*GcpSmIamWrite) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A Google Secret Manager installation, on a single Google Cloud project.

Installing Google Secret Manager allows P0 to vault credentials and to grant just-in-time access to individual secrets.

**Important:** P0 manages Secret Manager access through the project's Google Cloud IAM Management installation, so the
same project must already be installed with ` + "`p0_gcp_iam_write`" + `.

**Note:** This integration is currently in preview.`,
		Attributes: map[string]schema.Attribute{
			// Named 'project' to align with Terraform's naming for Google Cloud resources
			"project": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: `The ID of the Google Cloud project whose Secret Manager P0 should manage; must already be installed with ` + "`p0_gcp_iam_write`",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.RegexMatches(installgcp.GcpProjectIdRegex, "GCP project IDs should consist only of alphanumeric characters and hyphens"),
				},
			},
			"label": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: `The project's label, inherited from its Google Cloud IAM Management installation`,
			},
			"state": common.StateAttribute,
		},
	}
}

func (r *GcpSmIamWrite) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:  GcpSmKey,
		Component:    installresources.IamWrite,
		ProviderData: data,
		GetId:        r.getId,
		GetItemJson:  r.getItemJson,
		FromJson:     r.fromJson,
		ToJson:       r.toJson,
	}
}

func (r *GcpSmIamWrite) getId(data any) *string {
	model, ok := data.(*gcpSmIamWriteModel)
	if !ok {
		return nil
	}
	str := model.Project.ValueString()
	return &str
}

func (r *GcpSmIamWrite) getItemJson(json any) any {
	inner, ok := json.(*gcpSmIamWriteApi)
	if !ok {
		return nil
	}
	return inner.Item
}

func (r *GcpSmIamWrite) fromJson(_ context.Context, _ *diag.Diagnostics, id string, json any) any {
	data := gcpSmIamWriteModel{}
	jsonv, ok := json.(*gcpSmIamWriteJson)
	if !ok {
		return nil
	}

	data.Project = types.StringValue(id)
	data.Label = types.StringPointerValue(jsonv.Label)
	data.State = types.StringValue(jsonv.State)

	return &data
}

func (r *GcpSmIamWrite) toJson(data any) any {
	// The component has no user-configurable fields; the label is assigned by
	// the backend from the gcloud iam-write install.
	return &struct{}{}
}

func (r *GcpSmIamWrite) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json gcpSmIamWriteApi
	var data gcpSmIamWriteModel
	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, &struct{}{})
	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data)
}

func (r *GcpSmIamWrite) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &gcpSmIamWriteApi{}, &gcpSmIamWriteModel{})
}

func (r *GcpSmIamWrite) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &gcpSmIamWriteApi{}, &gcpSmIamWriteModel{})
}

func (r *GcpSmIamWrite) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Delete(ctx, &resp.Diagnostics, &req.State, &gcpSmIamWriteModel{})
}

func (r *GcpSmIamWrite) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("project"), req, resp)
}
