package installgoogledriveai

import (
	"context"
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
	installresources "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install"
	installgcp "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/gcp"
)

// The name requestors type to name the drive, including in the P0 CLI, so P0 keeps it
// to characters that need no quoting in a shell.
var SharedDriveIdRegex = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

const SharedDriveIdMaxLength = 63

// A shared drive's URL as the browser shows it with the drive open, optionally under an
// account selector (/u/<n>/) and with a query or fragment. P0 reads the drive's ID from
// it, and rejects any other Drive URL.
var SharedDriveUrlRegex = regexp.MustCompile(`^https://drive\.google\.com/drive/(?:u/\d+/)?folders/[\w-]+/?(?:[?#].*)?$`)

var _ resource.Resource = &GoogleDriveSharedDrive{}
var _ resource.ResourceWithConfigure = &GoogleDriveSharedDrive{}
var _ resource.ResourceWithImportState = &GoogleDriveSharedDrive{}

type GoogleDriveSharedDrive struct {
	installer *common.Install
}

type googleDriveSharedDriveModel struct {
	Id            types.String `tfsdk:"id"`
	DriveUrl      types.String `tfsdk:"drive_url"`
	ProjectId     types.String `tfsdk:"project_id"`
	GoogleDriveId types.String `tfsdk:"google_drive_id"`
	State         types.String `tfsdk:"state"`
}

type googleDriveSharedDriveJson struct {
	GoogleDriveUrl string  `json:"googleDriveUrl"`
	ProjectId      string  `json:"projectId"`
	GoogleDriveId  *string `json:"googleDriveId,omitempty"`
	State          *string `json:"state,omitempty"`
}

type googleDriveSharedDriveApi struct {
	Item *googleDriveSharedDriveJson `json:"item"`
}

func NewGoogleDriveSharedDrive() resource.Resource {
	return &GoogleDriveSharedDrive{}
}

func (*GoogleDriveSharedDrive) Metadata(_ context.Context, req resource.MetadataRequest, res *resource.MetadataResponse) {
	res.TypeName = req.ProviderTypeName + "_google_drive_shared_drive"
}

func (*GoogleDriveSharedDrive) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A Google shared drive that P0 can grant agentic sessions access in.

**Prerequisite:** the Google Drive connector (` + "`p0_google_drive_connector`" + `) must be installed for ` + "`project_id`" + `.

**Important:** P0 grants access in the drive as the connector's service account, so that account must be a member of the drive. In Google Drive, open the shared drive, choose **Manage members**, and add the connector's ` + "`connector_service_account`" + ` with the **Manager** role. Only a manager of the drive can do this, and Terraform cannot. Requests for access in the drive fail until it is done.

**Note:** This integration is currently in beta.`,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: `A name you choose for the drive. Requestors, access policies and the P0 CLI name the drive by it. Use lowercase letters, digits and hyphens only, starting and ending with a letter or digit, up to 63 characters.`,
				Validators: []validator.String{
					stringvalidator.LengthAtMost(SharedDriveIdMaxLength),
					stringvalidator.RegexMatches(SharedDriveIdRegex, "Use lowercase letters, digits, and hyphens only, starting and ending with a letter or digit"),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"drive_url": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: `The shared drive's URL, copied from the browser with the drive open, such as ` + "`https://drive.google.com/drive/folders/0AbCdEfGhIjKlMnOpQr`",
				Validators: []validator.String{
					stringvalidator.RegexMatches(SharedDriveUrlRegex, "Must be a shared drive's URL as it appears in the browser when the drive is open, such as https://drive.google.com/drive/folders/0AbCdEfGhIjKlMnOpQr"),
				},
				// P0 reads the drive's ID from the URL once, when the item is created.
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"project_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: `The Google Cloud project whose Google Drive connector manages this drive. P0 creates the service accounts it grants access in this drive to in the same project.`,
				Validators: []validator.String{
					stringvalidator.RegexMatches(installgcp.GcpProjectIdRegex, "GCP project IDs should consist only of alphanumeric characters and hyphens"),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"google_drive_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: `Google's ID for the shared drive, read from ` + "`drive_url`",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"state": common.StateAttribute,
		},
	}
}

func (r *GoogleDriveSharedDrive) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:  GoogleDriveAiKey,
		Component:    installresources.SharedDrive,
		ProviderData: data,
		GetId:        r.getId,
		GetItemJson:  r.getItemJson,
		FromJson:     r.fromJson,
		ToJson:       r.toJson,
	}
}

func (r *GoogleDriveSharedDrive) getId(data any) *string {
	model, ok := data.(*googleDriveSharedDriveModel)
	if !ok {
		return nil
	}
	str := model.Id.ValueString()
	return &str
}

func (r *GoogleDriveSharedDrive) getItemJson(json any) any {
	inner, ok := json.(*googleDriveSharedDriveApi)
	if !ok {
		return nil
	}
	return inner.Item
}

func (r *GoogleDriveSharedDrive) fromJson(_ context.Context, _ *diag.Diagnostics, id string, json any) any {
	data := googleDriveSharedDriveModel{}
	jsonv, ok := json.(*googleDriveSharedDriveJson)
	if !ok {
		return nil
	}

	data.Id = types.StringValue(id)
	data.DriveUrl = types.StringValue(jsonv.GoogleDriveUrl)
	data.ProjectId = types.StringValue(jsonv.ProjectId)
	data.GoogleDriveId = types.StringPointerValue(jsonv.GoogleDriveId)
	data.State = types.StringPointerValue(jsonv.State)

	return &data
}

func (r *GoogleDriveSharedDrive) toJson(data any) any {
	datav, ok := data.(*googleDriveSharedDriveModel)
	if !ok {
		return nil
	}

	// P0 reads the drive's ID from its URL, and sets the state.
	return &googleDriveSharedDriveJson{
		GoogleDriveUrl: datav.DriveUrl.ValueString(),
		ProjectId:      datav.ProjectId.ValueString(),
	}
}

func (r *GoogleDriveSharedDrive) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json googleDriveSharedDriveApi
	var data googleDriveSharedDriveModel

	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, r.toJson(&data))
	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data)
}

func (r *GoogleDriveSharedDrive) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &googleDriveSharedDriveApi{}, &googleDriveSharedDriveModel{})
}

// Every configurable attribute requires replacement, so Terraform never plans an
// in-place update.
func (r *GoogleDriveSharedDrive) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &googleDriveSharedDriveApi{}, &googleDriveSharedDriveModel{})
}

func (r *GoogleDriveSharedDrive) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Delete(ctx, &resp.Diagnostics, &req.State, &googleDriveSharedDriveModel{})
}

func (r *GoogleDriveSharedDrive) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
