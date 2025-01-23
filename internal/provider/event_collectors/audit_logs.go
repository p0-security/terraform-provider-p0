package installeventcollectors

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
	AuditLogsComponent = "audit-log"
	DisabledError      = "Feature Disabled"
	DisabledMessage    = "The audit logs feature is disabled."
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &AuditLogs{}
var _ resource.ResourceWithImportState = &AuditLogs{}
var _ resource.ResourceWithConfigure = &AuditLogs{}

var stateAttribute = schema.StringAttribute{
	Computed:            true,
	MarkdownDescription: common.StateMarkdownDescription,
}

var HttpsPrefixRegex = regexp.MustCompile(`^https:`)
var UuidRegex = regexp.MustCompile(`^[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{12}$`)
var HecTokenClearTextKey = "hec_token_cleartext"

func NewAuditLogs() resource.Resource {
	return &AuditLogs{}
}

type AuditLogs struct {
	installer *common.Install
}

type auditLogsModel struct {
	State             types.String `tfsdk:"state"`
	Token             types.String `tfsdk:"token"`
	HecEndpoint       types.String `tfsdk:"hec_endpoint"`
	HecTokenClearText types.String `tfsdk:"hec_token_cleartext"`
	HecTokenHash      types.String `tfsdk:"hec_token_hash"`
}

type TokenReadWrite struct {
	Hash      *string `json:"hash,omitempty"`
	ClearText *string `json:"clearText,omitempty"`
}

type auditLogsJsonReadWrite struct {
	State       *string         `json:"state,omitempty"`
	HecEndpoint *string         `json:"endpoint,omitempty"`
	HecToken    *TokenReadWrite `json:"token,omitempty"`
}

type auditLogsApiReadWrite struct {
	Item auditLogsJsonReadWrite `json:"item"`
}

func (r *AuditLogs) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_audit_logs"
}

func (r *AuditLogs) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `An installation of the HTTP Event Collector`,
		Attributes: map[string]schema.Attribute{
			"state": stateAttribute,
			"token": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: `The token ID of the HTTP event collector`,
			},
			"hec_endpoint": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: `The endpoint of the HTTP event collector`,
				Validators: []validator.String{
					stringvalidator.RegexMatches(
						HttpsPrefixRegex,
						"URL must begin with 'https:'",
					),
				},
			},
			"hec_token_cleartext": schema.StringAttribute{
				Required:            true,
				Sensitive:           true,
				MarkdownDescription: `The cleartext token of the HTTP event collector`,
				Validators: []validator.String{
					stringvalidator.RegexMatches(UuidRegex, "Token must be a valid UUID"),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"hec_token_hash": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: `The hash of the token of the HTTP event collector`,
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

	data.Token = types.StringValue(id)
	data.State = types.StringNull()
	if jsonv.State != nil {
		state := types.StringValue(*jsonv.State)
		data.State = state
	}

	data.HecEndpoint = types.StringNull()
	if jsonv.HecEndpoint != nil {
		hecEndpoint := types.StringValue(*jsonv.HecEndpoint)
		data.HecEndpoint = hecEndpoint
	}

	// data.HecTokenClearText = types.StringNull()
	data.HecTokenHash = types.StringNull()
	if jsonv.HecToken != nil {
		hecToken := types.StringValue(*jsonv.HecToken.Hash)
		data.HecTokenHash = hecToken
	}

	return &data
}

func (r *AuditLogs) toJson(data any) any {
	json := auditLogsJsonReadWrite{}
	datav, ok := data.(*auditLogsModel)
	if !ok {
		return nil
	}

	if !datav.HecEndpoint.IsNull() && !datav.HecEndpoint.IsUnknown() {
		hecEndpoint := datav.HecEndpoint.ValueString()
		json.HecEndpoint = &hecEndpoint
	}

	token := TokenReadWrite{}
	if !datav.HecTokenClearText.IsNull() && !datav.HecTokenClearText.IsUnknown() {
		hecToken := datav.HecTokenClearText.ValueString()
		token.ClearText = &hecToken
		json.HecToken = &token
	}

	if !datav.HecTokenHash.IsNull() && !datav.HecTokenHash.IsUnknown() {
		hecToken := datav.HecTokenHash.ValueString()
		token.Hash = &hecToken
		json.HecToken = &token
	}

	return json
}

func (r *AuditLogs) getId(data any) *string {
	model, ok := data.(*auditLogsModel)
	if !ok {
		return nil
	}

	str := model.Token.ValueString()
	return &str
}

func (r *AuditLogs) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	providerData := internal.Configure(&req, resp)

	// var key string

	// if providerData == nil {
	// 	key = ""
	// } else {
	// 	key, _ = providerData.Features["audit_logs"].Metadata["install_key"].(string)
	// }

	r.installer = &common.Install{
		Integration:  "",
		Component:    AuditLogsComponent,
		ProviderData: providerData,
		GetId:        r.getId,
		GetItemJson:  r.getItemJson,
		FromJson:     r.fromJson,
		ToJson:       r.toJson,
	}
}

func (s *AuditLogs) isEnabled() bool {
	enabled := s.installer.ProviderData.Features["audit_logs"].Enabled
	return enabled
}

func (s *AuditLogs) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	if !s.isEnabled() {
		resp.Diagnostics.AddError(DisabledError, DisabledMessage)
		return
	}

	var plan auditLogsModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Ensure the field is provided
	if plan.HecTokenClearText.IsNull() || plan.HecTokenClearText.ValueString() == "" {
		resp.Diagnostics.AddError(
			"Missing Required Attribute",
			"The 'hec_token_cleartext' field is required for resource creation.",
		)
		return
	}

	var api auditLogsApiReadWrite
	var model auditLogsModel

	s.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &model)
	s.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &api, &model)
	s.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &api, &model)

	// manually set the token cleartext attribute
	// this is needed because the cleartext token is not returned by the API
	resp.State.SetAttribute(ctx, path.Root(HecTokenClearTextKey), plan.HecTokenClearText)
}

func (s *AuditLogs) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	if !s.isEnabled() {
		resp.Diagnostics.AddError(DisabledError, DisabledMessage)
		return
	}

	s.installer.Read(ctx, &resp.Diagnostics, &resp.State, &auditLogsApiReadWrite{}, &auditLogsModel{})

	// manually set the token cleartext attribute
	// this is needed because the cleartext token is not returned by the API
	var currTokenClearText types.String
	req.State.GetAttribute(ctx, path.Root(HecTokenClearTextKey), &currTokenClearText)
	resp.State.SetAttribute(ctx, path.Root(HecTokenClearTextKey), currTokenClearText)
}

func (s *AuditLogs) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	if !s.isEnabled() {
		resp.Diagnostics.AddError(DisabledError, DisabledMessage)
		return
	}
	s.installer.Delete(ctx, &resp.Diagnostics, &req.State, &auditLogsModel{})
}

func (s *AuditLogs) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	if !s.isEnabled() {
		resp.Diagnostics.AddError(DisabledError, DisabledMessage)
		return
	}

	var plan auditLogsModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	s.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &auditLogsApiReadWrite{}, &auditLogsModel{})

	// manually set the token cleartext attribute
	// this is needed because the cleartext token is not returned by the API
	var currTokenClearText types.String
	req.Plan.GetAttribute(ctx, path.Root(HecTokenClearTextKey), &currTokenClearText)
	resp.State.SetAttribute(ctx, path.Root(HecTokenClearTextKey), currTokenClearText)
}

func (s *AuditLogs) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if !s.isEnabled() {
		resp.Diagnostics.AddError(DisabledError, DisabledMessage)
		return
	}

	resource.ImportStatePassthroughID(ctx, path.Root("token"), req, resp)
}
