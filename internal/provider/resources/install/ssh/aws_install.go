package installssh

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/p0-security/terraform-provider-p0/internal"
	installresources "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install"
	installaws "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/aws"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &AwsSshIamWrite{}
var _ resource.ResourceWithConfigure = &AwsSshIamWrite{}
var _ resource.ResourceWithImportState = &AwsSshIamWrite{}

type AwsSshIamWrite struct {
	installer *installresources.Install
}

type awsSshIamWriteModel struct {
	AccountId types.String `tfsdk:"account_id" json:"accountId,omitempty"`
	GroupKey  types.String `tfsdk:"group_key" json:"groupKey,omitempty"`
	State     types.String `tfsdk:"state" json:"state,omitempty"`
	Label     types.String `tfsdk:"label" json:"label,omitempty"`
}

type awsSshIamWriteJson struct {
	GroupKey *string `json:"groupKey"`
	State    string  `json:"state"`
	Label    *string `json:"label,omitempty"`
}

type awsSshIamWriteApi struct {
	Item *awsSshIamWriteJson `json:"item"`
}

func NewAwsSshIamWrite() resource.Resource {
	return &AwsSshIamWrite{}
}

// Metadata implements resource.ResourceWithImportState.
func (*AwsSshIamWrite) Metadata(_ context.Context, req resource.MetadataRequest, res *resource.MetadataResponse) {
	res.TypeName = req.ProviderTypeName + "_aws_ssh_install"
}

// Schema implements resource.ResourceWithImportState.
func (*AwsSshIamWrite) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		// Note that the TF doc generator clobbers _most_ underscores :(
		MarkdownDescription: `An AWS SSH Installation.
		
Installing SSH allows you to manage access to your servers on AWS.`,
		Attributes: map[string]schema.Attribute{
			"account_id": schema.StringAttribute{
				MarkdownDescription: `The AWS account ID.`,
				Required:            true,
				Validators: []validator.String{
					stringvalidator.RegexMatches(installaws.AwsAccountIdRegex, "AWS account IDs should be numeric"),
				},
			},
			"group_key": schema.StringAttribute{
				MarkdownDescription: `If present, AWS instances will be grouped by the value of this tag. Access can be requested, in one request, to all instances with a shared tag value.`,
				Optional:            true,
			},
			"state": schema.StringAttribute{
				MarkdownDescription: installresources.StateMarkdownDescription,
				Computed:            true,
			},
			"label": schema.StringAttribute{
				MarkdownDescription: installresources.AwsLabelMarkdownDescription,
				Computed:            true,
				Optional:            true,
			},
		},
	}
}

func (r *AwsSshIamWrite) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := internal.Configure(&req, resp)
	r.installer = &installresources.Install{
		ProviderData: data,
		GetItemPath:  r.getItemPath,
		GetId:        r.getId,
		GetItemJson:  r.getItemJson,
		FromJson:     r.fromJson,
		ToJson:       r.toJson,
	}
	if data == nil {
		return
	}
}

func (r *AwsSshIamWrite) getId(data any) *string {
	model, ok := data.(*awsSshIamWriteModel)
	if !ok {
		return nil
	}

	str := model.AccountId.ValueString()
	return &str
}

func (r *AwsSshIamWrite) getItemPath(id string) string {
	return fmt.Sprintf("integrations/%s/config/%s/aws:%s", SshKey, installresources.IamWrite, id)
}

func (r *AwsSshIamWrite) getItemJson(json any) any {
	inner, ok := json.(*awsSshIamWriteApi)
	if !ok {
		return nil
	}
	return inner.Item
}

func (r *AwsSshIamWrite) fromJson(id string, json any) any {
	data := awsSshIamWriteModel{}
	jsonv, ok := json.(*awsSshIamWriteJson)
	if !ok {
		return nil
	}

	data.AccountId = types.StringValue(id)
	data.Label = types.StringNull()
	if jsonv.Label != nil {
		label := types.StringValue(*jsonv.Label)
		data.Label = label
	}

	data.Label = types.StringNull()
	data.State = types.StringValue(jsonv.State)
	data.GroupKey = types.StringNull()
	if jsonv.GroupKey != nil {
		group := types.StringValue(*jsonv.GroupKey)
		data.GroupKey = group
	}

	return &data
}

func (r *AwsSshIamWrite) toJson(data any) any {
	json := awsSshIamWriteJson{}

	datav, ok := data.(*awsSshIamWriteModel)
	if !ok {
		return nil
	}

	if !datav.Label.IsNull() && !datav.Label.IsUnknown() {
		label := datav.Label.ValueString()
		json.Label = &label
	}

	// can omit state here as it's filled by the backend
	if !datav.GroupKey.IsNull() {
		group := datav.GroupKey.ValueString()
		json.GroupKey = &group
	}

	return &json
}

// Create implements resource.ResourceWithImportState.
func (s *AwsSshIamWrite) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan awsSshIamWriteModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	throwaway_response := struct{}{}
	err := s.installer.ProviderData.Post("integrations/ssh/config", struct{}{}, &throwaway_response)
	if err != nil {
		if !strings.Contains(err.Error(), "409 Conflict") {
			resp.Diagnostics.AddError("Failed to install IAM write", err.Error())
			return
		}
	}

	s.installer.Upsert(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &awsSshIamWriteApi{}, &awsSshIamWriteModel{})
}

func (s *AwsSshIamWrite) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	s.installer.Read(ctx, &resp.Diagnostics, &resp.State, &awsSshIamWriteApi{}, &awsSshIamWriteModel{})
}

func (s *AwsSshIamWrite) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state awsSshIamWriteModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	id := s.getId(&state)
	path := s.getItemPath(*id)
	// delete the staged component.
	err := s.installer.ProviderData.Delete(path)
	if err != nil {
		resp.Diagnostics.AddError("Could not delete component", fmt.Sprintf("%s", err))
		return
	}
}

// Update implements resource.ResourceWithImportState.
func (s *AwsSshIamWrite) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	s.installer.Upsert(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &awsSshIamWriteApi{}, &awsSshIamWriteModel{})
}

func (s *AwsSshIamWrite) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("account_id"), req, resp)
}
