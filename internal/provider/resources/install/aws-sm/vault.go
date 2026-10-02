package installawssm

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
	installaws "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/aws"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &awsSmVault{}
var _ resource.ResourceWithConfigure = &awsSmVault{}
var _ resource.ResourceWithImportState = &awsSmVault{}

type awsSmVault struct {
	installer *common.Install
}

type awsSmVaultModel struct {
	AccountId     types.String `tfsdk:"account_id"`
	DefaultRegion types.String `tfsdk:"default_region"`
	Label         types.String `tfsdk:"label"`
	State         types.String `tfsdk:"state"`
}

type awsSmVaultJson struct {
	DefaultRegion string  `json:"defaultRegion"`
	State         *string `json:"state,omitempty"`
	Label         *string `json:"label,omitempty"`
}

type awsSmVaultApi struct {
	Item *awsSmVaultJson `json:"item"`
}

func NewAwsSmVault() resource.Resource {
	return &awsSmVault{}
}

// Metadata implements resource.ResourceWithImportState.
func (*awsSmVault) Metadata(_ context.Context, req resource.MetadataRequest, res *resource.MetadataResponse) {
	res.TypeName = req.ProviderTypeName + "_aws_secrets_manager"
}

// Schema implements resource.ResourceWithImportState.
func (*awsSmVault) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `An AWS Secrets Manager (preview) installation for an AWS account.

Installing AWS Secrets Manager lets P0 use the account's Secrets Manager as a vault, such as for the private key of a GitHub Repositories installation (` + "`p0_github_repositories`" + `), whose ` + "`vault.account_id`" + ` names this account.

**Prerequisite:** the AWS integration (` + "`p0_aws_iam_write`" + `) must be installed for the same AWS account.`,
		Attributes: map[string]schema.Attribute{
			"account_id": schema.StringAttribute{
				MarkdownDescription: `The AWS account ID. AWS IAM management must already be installed for this account.`,
				Required:            true,
				Validators: []validator.String{
					stringvalidator.RegexMatches(installaws.AwsAccountIdRegex, "AWS account IDs should be numeric"),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"default_region": schema.StringAttribute{
				MarkdownDescription: `The AWS region P0 creates secrets in by default, such as ` + "`us-west-2`",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.RegexMatches(AwsRegionRegex, "Must be an AWS region, such as us-west-2"),
				},
			},
			"label": schema.StringAttribute{
				MarkdownDescription: installaws.AwsLabelMarkdownDescription,
				Computed:            true,
			},
			"state": schema.StringAttribute{
				MarkdownDescription: common.StateMarkdownDescription,
				Computed:            true,
			},
		},
	}
}

func (r *awsSmVault) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:  AwsSmKey,
		Component:    installresources.IamWrite,
		ProviderData: data,
		GetId:        r.getId,
		GetItemJson:  r.getItemJson,
		FromJson:     r.fromJson,
		ToJson:       r.toJson,
	}
}

// The item is keyed by the bare account ID, as the AWS integration's IAM management
// item is.
func (r *awsSmVault) getId(data any) *string {
	model, ok := data.(*awsSmVaultModel)
	if !ok {
		return nil
	}

	str := model.AccountId.ValueString()
	return &str
}

func (r *awsSmVault) getItemJson(json any) any {
	inner, ok := json.(*awsSmVaultApi)
	if !ok {
		return nil
	}
	return inner.Item
}

func (r *awsSmVault) fromJson(ctx context.Context, diags *diag.Diagnostics, id string, json any) any {
	data := awsSmVaultModel{}
	jsonv, ok := json.(*awsSmVaultJson)
	if !ok {
		return nil
	}

	data.AccountId = types.StringValue(id)
	data.DefaultRegion = types.StringValue(jsonv.DefaultRegion)
	data.State = types.StringPointerValue(jsonv.State)
	data.Label = types.StringPointerValue(jsonv.Label)

	return &data
}

func (r *awsSmVault) toJson(data any) any {
	datav, ok := data.(*awsSmVaultModel)
	if !ok {
		return nil
	}

	// can omit state and label here as they're filled by the backend
	return &awsSmVaultJson{DefaultRegion: datav.DefaultRegion.ValueString()}
}

// Create implements resource.ResourceWithImportState.
func (s *awsSmVault) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json awsSmVaultApi
	var data awsSmVaultModel

	s.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	s.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, s.toJson(&data))
	s.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data)
}

func (s *awsSmVault) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	s.installer.Read(ctx, &resp.Diagnostics, &resp.State, &awsSmVaultApi{}, &awsSmVaultModel{})
}

func (s *awsSmVault) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	s.installer.Delete(ctx, &resp.Diagnostics, &req.State, &awsSmVaultModel{})
}

// Update implements resource.ResourceWithImportState.
func (s *awsSmVault) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	s.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &awsSmVaultApi{}, &awsSmVaultModel{})
}

func (s *awsSmVault) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("account_id"), req, resp)
}
