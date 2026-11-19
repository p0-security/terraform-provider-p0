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
	installvaultedcredential "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/vaulted-credential"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &gcpSmVault{}
var _ resource.ResourceWithConfigure = &gcpSmVault{}
var _ resource.ResourceWithImportState = &gcpSmVault{}

type gcpSmVault struct {
	installer *common.Install
}

type gcpSmVaultModel struct {
	Project types.String `tfsdk:"project"`
	Label   types.String `tfsdk:"label"`
	State   types.String `tfsdk:"state"`
}

type gcpSmVaultJson struct {
	State *string `json:"state,omitempty"`
	Label *string `json:"label,omitempty"`
}

type gcpSmVaultApi struct {
	Item *gcpSmVaultJson `json:"item"`
}

func NewGcpSmVault() resource.Resource {
	return &gcpSmVault{}
}

func (*gcpSmVault) Metadata(_ context.Context, req resource.MetadataRequest, res *resource.MetadataResponse) {
	res.TypeName = req.ProviderTypeName + "_gcp_secret_manager"
}

func (*gcpSmVault) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A Google Secret Manager (preview) installation for a Google Cloud project.

Installing Google Secret Manager lets P0 use the project's Secret Manager as a vault, such as for the access tokens of an integration whose ` + "`secret_manager.type`" + ` is ` + "`gcp-sm`" + ` and whose ` + "`secret_manager.project_id`" + ` names this project.

**Prerequisite:** the Google Cloud integration (` + "`p0_gcp_iam_write`" + `) must be installed for the same project.`,
		Attributes: map[string]schema.Attribute{
			"project": schema.StringAttribute{
				MarkdownDescription: `The Google Cloud project ID. Google Cloud IAM management must already be installed for this project.`,
				Required:            true,
				Validators: []validator.String{
					stringvalidator.RegexMatches(installgcp.GcpProjectIdRegex, "GCP project IDs should consist only of alphanumeric characters and hyphens"),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"label": schema.StringAttribute{
				MarkdownDescription: `The label of the project's Google Cloud IAM management installation`,
				Computed:            true,
			},
			"state": schema.StringAttribute{
				MarkdownDescription: common.StateMarkdownDescription,
				Computed:            true,
			},
		},
	}
}

func (r *gcpSmVault) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:  installvaultedcredential.GcpSecretManager,
		Component:    installresources.IamWrite,
		ProviderData: data,
		GetId:        r.getId,
		GetItemJson:  r.getItemJson,
		FromJson:     r.fromJson,
		ToJson:       r.toJson,
	}
}

// P0 keys the item by the bare project ID, as the Google Cloud integration's IAM
// management item is.
func (r *gcpSmVault) getId(data any) *string {
	model, ok := data.(*gcpSmVaultModel)
	if !ok {
		return nil
	}

	str := model.Project.ValueString()
	return &str
}

func (r *gcpSmVault) getItemJson(json any) any {
	inner, ok := json.(*gcpSmVaultApi)
	if !ok {
		return nil
	}
	return inner.Item
}

func (r *gcpSmVault) fromJson(ctx context.Context, diags *diag.Diagnostics, id string, json any) any {
	jsonv, ok := json.(*gcpSmVaultJson)
	if !ok {
		return nil
	}

	return &gcpSmVaultModel{
		Project: types.StringValue(id),
		State:   types.StringPointerValue(jsonv.State),
		Label:   types.StringPointerValue(jsonv.Label),
	}
}

// The item has no configuration; P0 fills state and label.
func (r *gcpSmVault) toJson(data any) any {
	return &gcpSmVaultJson{}
}

func (s *gcpSmVault) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json gcpSmVaultApi
	var data gcpSmVaultModel

	s.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	s.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, s.toJson(&data))
	s.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data)
}

func (s *gcpSmVault) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	s.installer.Read(ctx, &resp.Diagnostics, &resp.State, &gcpSmVaultApi{}, &gcpSmVaultModel{})
}

func (s *gcpSmVault) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	s.installer.Delete(ctx, &resp.Diagnostics, &req.State, &gcpSmVaultModel{})
}

func (s *gcpSmVault) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	s.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &gcpSmVaultApi{}, &gcpSmVaultModel{})
}

func (s *gcpSmVault) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("project"), req, resp)
}
