package installazure

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/p0-security/terraform-provider-p0/internal"
	"github.com/p0-security/terraform-provider-p0/internal/common"
	installresources "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &AzureIamWriteStaged{}
var _ resource.ResourceWithImportState = &AzureIamWriteStaged{}
var _ resource.ResourceWithConfigure = &AzureIamWriteStaged{}

func NewAzureIamWriteStaged() resource.Resource {
	return &AzureIamWriteStaged{}
}

type AzureIamWriteStaged struct {
	installer *common.Install
}

type AzureIamWriteStagedModel struct {
	ManagementGroupId string       `tfsdk:"management_group_id"`
	State             types.String `tfsdk:"state"`
}

type AzureIamWriteStagedApi struct {
	Item struct {
		State string `json:"state"`
	} `json:"item"`
}

func (r *AzureIamWriteStaged) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_azure_iam_write_staged"
}

func (r *AzureIamWriteStaged) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `An installation of P0, on a single Microsoft Azure Management Group, for IAM management.

For instructions on using this resource, see the documentation for ` + "`p0_azure_iam_write`.",
		Attributes: map[string]schema.Attribute{
			"management_group_id": managementGroupIdAttribute,
			"state":               common.StateAttribute,
		},
	}
}

func (r *AzureIamWriteStaged) getId(data any) *string {
	model, ok := data.(*AzureIamWriteStagedModel)
	if !ok {
		return nil
	}
	return &model.ManagementGroupId
}

func (r *AzureIamWriteStaged) getItemJson(json any) any {
	return json
}

func (r *AzureIamWriteStaged) fromJson(ctx context.Context, diags *diag.Diagnostics, id string, json any) any {
	data := AzureIamWriteStagedModel{}
	jsonv, ok := json.(*AzureIamWriteStagedApi)
	if !ok {
		return nil
	}

	data.ManagementGroupId = id
	data.State = types.StringValue(jsonv.Item.State)

	return &data
}

func (r *AzureIamWriteStaged) toJson(data any) any {
	json := AzureIamWriteStagedApi{}
	return &json.Item
}

func (r *AzureIamWriteStaged) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	providerData := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:  AzureKey,
		Component:    installresources.IamWrite,
		ProviderData: providerData,
		GetId:        r.getId,
		GetItemJson:  r.getItemJson,
		FromJson:     r.fromJson,
		ToJson:       r.toJson,
	}
}

func (s *AzureIamWriteStaged) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json AzureIamWriteStagedApi
	var data AzureIamWriteStagedModel
	s.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data)
}

func (s *AzureIamWriteStaged) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	s.installer.Read(ctx, &resp.Diagnostics, &resp.State, &AzureIamWriteStagedApi{}, &AzureIamWriteStagedModel{})
}

// Skips the unstaging step, as it is not needed for ssh integrations and instead performs a full delete.
func (s *AzureIamWriteStaged) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	s.installer.Delete(ctx, &resp.Diagnostics, &req.State, &AzureIamWriteStagedModel{})
}

// Update implements resource.ResourceWithImportState.
func (s *AzureIamWriteStaged) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	s.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &AzureIamWriteStagedApi{}, &AzureIamWriteStagedModel{})
}

func (s *AzureIamWriteStaged) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("management_group_id"), req, resp)
}
