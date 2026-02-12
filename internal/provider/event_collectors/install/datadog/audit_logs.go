package installdatadog

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
)

const (
	DatadogIntegration = "datadog"
	AuditLogsComponent = "audit-log"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &AuditLogs{}
var _ resource.ResourceWithImportState = &AuditLogs{}
var _ resource.ResourceWithConfigure = &AuditLogs{}

var IntakeUrlRegex = regexp.MustCompile(`^https://http-intake\.logs\.`)
var ApiKeyClearTextKey = "api_key_cleartext"

func NewAuditLogs() resource.Resource {
	return &AuditLogs{}
}

type AuditLogs struct {
	installer *common.Install
}

type auditLogsModel struct {
	State           types.String `tfsdk:"state"`
	Identifier      types.String `tfsdk:"identifier"`
	IntakeUrl       types.String `tfsdk:"intake_url"`
	ApiKeyClearText types.String `tfsdk:"api_key_cleartext"`
	ApiKeyHash      types.String `tfsdk:"api_key_hash"`
	Service         types.String `tfsdk:"service"`
}

type ApiKeyReadWrite struct {
	Hash      *string `json:"hash,omitempty"`
	ClearText *string `json:"clearText,omitempty"`
}

type auditLogsJsonReadWrite struct {
	State     *string          `json:"state,omitempty"`
	IntakeUrl *string          `json:"endpoint,omitempty"`
	ApiKey    *ApiKeyReadWrite `json:"apiKey,omitempty"`
	Service   *string          `json:"service,omitempty"`
}

type auditLogsApiReadWrite struct {
	Item auditLogsJsonReadWrite `json:"item"`
}

func (r *AuditLogs) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_datadog_audit_logs"
}

func (r *AuditLogs) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `Allows P0 to send access events and security findings to Datadog Logs`,
		Attributes: map[string]schema.Attribute{
			"state": common.StateAttribute,
			"identifier": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: `A user-specified identifier for this Datadog audit logs configuration`,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"intake_url": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: `Datadog logs intake URL (e.g., https://http-intake.logs.datadoghq.com)`,
				Validators: []validator.String{
					stringvalidator.RegexMatches(
						IntakeUrlRegex,
						"Intake URL must have the form 'https://http-intake.logs.<site>' (e.g., https://http-intake.logs.datadoghq.com)",
					),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"api_key_cleartext": schema.StringAttribute{
				Required:            true,
				Sensitive:           true,
				MarkdownDescription: `Datadog API key for authentication`,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"api_key_hash": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: `The hash of the API key`,
			},
			"service": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: `Service name for log attribution (optional). Defaults to 'p0'.`,
			},
		},
	}
}

func (r *AuditLogs) getItemJson(json any) any {
	inner, ok := json.(*auditLogsApiReadWrite)
	if !ok {
		return nil
	}
	return &inner.Item
}

func (r *AuditLogs) fromJson(ctx context.Context, diags *diag.Diagnostics, id string, json any) any {
	data := auditLogsModel{}
	jsonv, ok := json.(*auditLogsJsonReadWrite)
	if !ok {
		return nil
	}

	data.Identifier = types.StringValue(id)

	data.IntakeUrl = types.StringNull()
	if jsonv.IntakeUrl != nil {
		intakeUrl := types.StringValue(*jsonv.IntakeUrl)
		data.IntakeUrl = intakeUrl
	}

	data.State = types.StringNull()
	if jsonv.State != nil {
		state := types.StringValue(*jsonv.State)
		data.State = state
	}

	data.ApiKeyHash = types.StringNull()
	if jsonv.ApiKey != nil && jsonv.ApiKey.Hash != nil {
		apiKeyHash := types.StringValue(*jsonv.ApiKey.Hash)
		data.ApiKeyHash = apiKeyHash
	}

	data.Service = types.StringNull()
	if jsonv.Service != nil {
		service := types.StringValue(*jsonv.Service)
		data.Service = service
	}

	return &data
}

func (r *AuditLogs) toJson(data any) any {
	json := auditLogsJsonReadWrite{}
	datav, ok := data.(*auditLogsModel)
	if !ok {
		return nil
	}

	if !datav.IntakeUrl.IsNull() && !datav.IntakeUrl.IsUnknown() {
		intakeUrl := datav.IntakeUrl.ValueString()
		json.IntakeUrl = &intakeUrl
	}

	// Only send cleartext when writing, not the hash
	// The hash is computed by the backend and returned when reading
	if !datav.ApiKeyClearText.IsNull() && !datav.ApiKeyClearText.IsUnknown() {
		clearText := datav.ApiKeyClearText.ValueString()
		json.ApiKey = &ApiKeyReadWrite{
			ClearText: &clearText,
		}
	}

	if !datav.Service.IsNull() && !datav.Service.IsUnknown() {
		service := datav.Service.ValueString()
		json.Service = &service
	}

	return json
}

func (r *AuditLogs) getId(data any) *string {
	model, ok := data.(*auditLogsModel)
	if !ok {
		return nil
	}

	str := model.Identifier.ValueString()
	return &str
}

func (r *AuditLogs) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	providerData := internal.Configure(&req, resp)

	r.installer = &common.Install{
		Integration:  DatadogIntegration,
		Component:    AuditLogsComponent,
		ProviderData: providerData,
		GetId:        r.getId,
		GetItemJson:  r.getItemJson,
		FromJson:     r.fromJson,
		ToJson:       r.toJson,
	}
}

func (s *AuditLogs) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {

	var plan auditLogsModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Ensure the API key cleartext is provided
	if plan.ApiKeyClearText.IsNull() || plan.ApiKeyClearText.ValueString() == "" {
		resp.Diagnostics.AddError(
			"Missing Required Attribute",
			"The 'api_key_cleartext' field is required for resource creation.",
		)
		return
	}

	var api auditLogsApiReadWrite
	var model auditLogsModel

	s.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &model)
	s.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &api, &model, &struct{}{})
	s.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &api, &model)

	// manually set the api key cleartext attribute
	// this is needed because the cleartext api key is not returned by the API
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root(ApiKeyClearTextKey), plan.ApiKeyClearText)...)
}

func (s *AuditLogs) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	s.installer.Read(ctx, &resp.Diagnostics, &resp.State, &auditLogsApiReadWrite{}, &auditLogsModel{})

	// manually set the api key cleartext attribute
	// this is needed because the cleartext api key is not returned by the API
	var currApiKeyClearText types.String
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root(ApiKeyClearTextKey), &currApiKeyClearText)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root(ApiKeyClearTextKey), currApiKeyClearText)...)
}

func (s *AuditLogs) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	s.installer.Delete(ctx, &resp.Diagnostics, &req.State, &auditLogsModel{})
}

func (s *AuditLogs) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {

	var plan auditLogsModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	s.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &auditLogsApiReadWrite{}, &auditLogsModel{})

	// manually set the api key cleartext attribute
	// this is needed because the cleartext api key is not returned by the API
	var currApiKeyClearText types.String
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root(ApiKeyClearTextKey), &currApiKeyClearText)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root(ApiKeyClearTextKey), currApiKeyClearText)...)
}

func (s *AuditLogs) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Import by identifier
	var model auditLogsModel
	model.Identifier = types.StringValue(req.ID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}
