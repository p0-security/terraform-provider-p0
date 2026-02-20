package installmysql

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/p0-security/terraform-provider-p0/internal"
	"github.com/p0-security/terraform-provider-p0/internal/common"
	installresources "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &mysqlIamWrite{}
var _ resource.ResourceWithConfigure = &mysqlIamWrite{}
var _ resource.ResourceWithImportState = &mysqlIamWrite{}

type mysqlIamWrite struct {
	installer *common.Install
}

type mysqlIamWriteModel struct {
	Id        types.String `tfsdk:"id" json:"id,omitempty"`
	Hostname  types.String `tfsdk:"hostname" json:"hostname,omitempty"`
	Port      types.String `tfsdk:"port" json:"port,omitempty"`
	DefaultDb types.String `tfsdk:"default_db" json:"defaultDb,omitempty"`
	State     types.String `tfsdk:"state" json:"state,omitempty"`
}

type mysqlIamWriteJson struct {
	Hostname  *string `json:"hostname,omitempty"`
	Port      *string `json:"port,omitempty"`
	DefaultDb *string `json:"defaultDb,omitempty"`
	State     string  `json:"state"`
}

type mysqlIamWriteApi struct {
	Item *mysqlIamWriteJson `json:"item"`
}

func NewMysqlIamWrite() resource.Resource {
	return &mysqlIamWrite{}
}

func (*mysqlIamWrite) Metadata(_ context.Context, req resource.MetadataRequest, res *resource.MetadataResponse) {
	res.TypeName = req.ProviderTypeName + "_mysql"
}

func (*mysqlIamWrite) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A MySQL Installation for AWS RDS.

Installing MySQL allows you to manage access to your MySQL database instances using IAM authentication.

**Note:** This integration is currently experimental and only supports AWS RDS MySQL instances.`,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: `A unique identifier for this MySQL installation (can be any string, e.g., "production-db" or "staging-mysql")`,
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"hostname": schema.StringAttribute{
				MarkdownDescription: `The hostname or IP address of the MySQL instance`,
				Computed:            true,
			},
			"port": schema.StringAttribute{
				MarkdownDescription: `The MySQL port number (defaults to 3306)`,
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString(MysqlDefaultPort),
				Validators: []validator.String{
					stringvalidator.RegexMatches(PortRegex, "Must be a valid port number (1-65535)"),
				},
			},
			"default_db": schema.StringAttribute{
				MarkdownDescription: `Optional default database for access requests`,
				Optional:            true,
			},
			"state": schema.StringAttribute{
				MarkdownDescription: common.StateMarkdownDescription,
				Computed:            true,
			},
		},
	}
}

func (r *mysqlIamWrite) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:  MysqlKey,
		Component:    installresources.IamWrite,
		ProviderData: data,
		GetId:        r.getId,
		GetItemJson:  r.getItemJson,
		FromJson:     r.fromJson,
		ToJson:       r.toJson,
	}
}

func (r *mysqlIamWrite) getId(data any) *string {
	model, ok := data.(*mysqlIamWriteModel)
	if !ok {
		return nil
	}

	str := model.Id.ValueString()
	return &str
}

func (r *mysqlIamWrite) getItemJson(json any) any {
	inner, ok := json.(*mysqlIamWriteApi)
	if !ok {
		return nil
	}
	return inner.Item
}

func (r *mysqlIamWrite) fromJson(ctx context.Context, diags *diag.Diagnostics, id string, json any) any {
	data := mysqlIamWriteModel{}
	jsonv, ok := json.(*mysqlIamWriteJson)
	if !ok {
		return nil
	}

	data.Id = types.StringValue(id)
	data.State = types.StringValue(jsonv.State)
	data.Hostname = types.StringNull()
	if jsonv.Hostname != nil {
		data.Hostname = types.StringValue(*jsonv.Hostname)
	}

	data.Port = types.StringNull()
	if jsonv.Port != nil {
		data.Port = types.StringValue(*jsonv.Port)
	}

	data.DefaultDb = types.StringNull()
	if jsonv.DefaultDb != nil {
		data.DefaultDb = types.StringValue(*jsonv.DefaultDb)
	}

	return &data
}

func (r *mysqlIamWrite) toJson(data any) any {
	json := mysqlIamWriteJson{}

	datav, ok := data.(*mysqlIamWriteModel)
	if !ok {
		return nil
	}

	if !datav.Hostname.IsNull() && !datav.Hostname.IsUnknown() {
		hostname := datav.Hostname.ValueString()
		json.Hostname = &hostname
	}

	if !datav.Port.IsNull() && !datav.Port.IsUnknown() {
		port := datav.Port.ValueString()
		json.Port = &port
	}

	if !datav.DefaultDb.IsNull() && !datav.DefaultDb.IsUnknown() {
		defaultDb := datav.DefaultDb.ValueString()
		json.DefaultDb = &defaultDb
	}

	return &json
}

func (s *mysqlIamWrite) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json mysqlIamWriteApi
	var data mysqlIamWriteModel

	s.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)

	// Convert the model to JSON for the Stage call
	// This ensures fields marked with step: "new" are sent during the assemble step
	inputJson := s.toJson(&data)

	s.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, inputJson)
	s.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data)
}

func (s *mysqlIamWrite) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	s.installer.Read(ctx, &resp.Diagnostics, &resp.State, &mysqlIamWriteApi{}, &mysqlIamWriteModel{})
}

func (s *mysqlIamWrite) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	s.installer.Rollback(ctx, &resp.Diagnostics, &req.State, &mysqlIamWriteModel{})
}

func (s *mysqlIamWrite) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	s.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &mysqlIamWriteApi{}, &mysqlIamWriteModel{})
}

func (s *mysqlIamWrite) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
