package installgcp

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/p0-security/terraform-provider-p0/internal"
	installresources "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &Gcp{}
var _ resource.ResourceWithImportState = &Gcp{}
var _ resource.ResourceWithConfigure = &Gcp{}

func NewGcp() resource.Resource {
	return &Gcp{}
}

type Gcp struct {
	installer *installresources.RootInstall
}

type gcpModel struct {
	OrganizationId      basetypes.StringValue `tfsdk:"organization_id"`
	ServiceAccountEmail basetypes.StringValue `tfsdk:"service_account_email"`
}

type gcpApi struct {
	Config struct {
		Root struct {
			Singleton struct {
				OrganizationId      string  `json:"organizationId"`
				ServiceAccountEmail *string `json:"serviceAccountEmail"`
			} `json:"_"`
		} `json:"root"`
	} `json:"config"`
}

func (r *Gcp) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_gcp"
}

func (r *Gcp) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A Google Cloud installation.`,
		Attributes: map[string]schema.Attribute{
			"organization_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: `The Google Cloud organization ID`,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.RegexMatches(GcpOrganizationIdRegex, "GCP organization IDs should be numeric"),
				},
			},
			"service_account_email": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: `The identity that P0 uses to communicate with your Google Cloud organization`,
			},
		},
	}
}

func (r *Gcp) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	providerData := internal.Configure(&req, resp)
	r.installer = &installresources.RootInstall{
		Integration:  GcpKey,
		ProviderData: providerData,
		FromJson:     r.fromJson,
		ToJson:       r.toJson,
	}
}

func (r *Gcp) fromJson(json any) any {
	data := gcpModel{}

	jsonv, ok := json.(*gcpApi)
	if !ok {
		return nil
	}

	root := jsonv.Config.Root.Singleton

	data.OrganizationId = types.StringValue(root.OrganizationId)
	data.ServiceAccountEmail = types.StringPointerValue(root.ServiceAccountEmail)

	return &data
}

func (r *Gcp) toJson(data any) any {
	json := gcpApi{}

	datav, ok := data.(*gcpModel)
	if !ok {
		return nil
	}

	json.Config.Root.Singleton.OrganizationId = datav.OrganizationId.ValueString()
	json.Config.Root.Singleton.ServiceAccountEmail = datav.OrganizationId.ValueStringPointer()

	// can omit state here as it's filled by the backend
	return &json
}

func (r *Gcp) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json gcpApi
	var data gcpModel
	r.installer.Create(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data)
}

func (r *Gcp) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var json gcpApi
	var data gcpModel
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &json, &data)
}

func (r *Gcp) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError("Not Updateable", "Modifying P0's GCP integration forces replacement")
}

func (r *Gcp) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data gcpModel
	r.installer.Delete(ctx, &resp.Diagnostics, &resp.State, &data)
}

func (r *Gcp) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("organization_id"), req, resp)
}
