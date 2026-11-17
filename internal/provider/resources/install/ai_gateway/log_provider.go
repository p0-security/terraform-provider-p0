// Copyright (c) 2025 P0 Security, Inc
// SPDX-License-Identifier: MPL-2.0

package installaigateway

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
)

// logProviderComponent is the agentic integration's component for a source of
// tool-call activity logs.
const logProviderComponent = "log-provider"

const (
	logProviderTypeVictoriaLogs = "victorialogs"
	logProviderTypeGcp          = "gcp"
)

var _ resource.Resource = &LogProvider{}
var _ resource.ResourceWithImportState = &LogProvider{}
var _ resource.ResourceWithConfigure = &LogProvider{}
var _ resource.ResourceWithValidateConfig = &LogProvider{}

func NewLogProvider() resource.Resource {
	return &LogProvider{}
}

type LogProvider struct {
	installer *common.Install
}

type logProviderModel struct {
	Id        string       `tfsdk:"id"`
	Type      types.String `tfsdk:"type"`
	ProjectId types.String `tfsdk:"project_id"`
}

// logProviderSettingsJson is the item's `provider` select. P0 rebuilds the item
// from the selected option's schema on each step, so a switch to victorialogs
// drops a stored projectId.
type logProviderSettingsJson struct {
	Type      string  `json:"type"`
	ProjectId *string `json:"projectId,omitempty"`
}

type logProviderJson struct {
	Provider logProviderSettingsJson `json:"provider"`
	State    string                  `json:"state"`
}

type logProviderApi struct {
	Item logProviderJson `json:"item"`
}

type logProviderStageJson struct {
	Provider logProviderSettingsJson `json:"provider"`
}

func (r *LogProvider) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ai_gateway_log_provider"
}

func (r *LogProvider) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A source of tool-call activity logs that one or more AI gateways share. P0 reads a gateway's
MCP tool-call activity from its log provider, and shows it in the Event Log.

To link a gateway to this log provider, use the ` + "`p0_ai_gateway_log_provider_link`" + ` resource.

P0 does not check that the logs exist when you create this resource. A missing or wrong source shows up only in the
Event Log.`,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "A unique identifier for this log provider",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"type": schema.StringAttribute{
				Required: true,
				MarkdownDescription: `Where the gateway's activity logs are stored:
    - 'victorialogs': the VictoriaLogs instance that runs next to the gateway. P0 reads it through the gateway's
      management API, so it needs no other settings
    - 'gcp': Google Cloud Logging, in the project that 'project_id' names`,
				Validators: []validator.String{
					stringvalidator.OneOf(logProviderTypeVictoriaLogs, logProviderTypeGcp),
				},
			},
			"project_id": schema.StringAttribute{
				Optional: true,
				MarkdownDescription: `Required, and may only be used, if 'type' is 'gcp'. The GCP project that holds the log bucket
where the gateway's Cloud Logging entries land. This may be a different project from the one the gateway runs in.`,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
		},
	}
}

// ValidateConfig requires project_id for the gcp type, and rejects it for every
// other type.
func (r *LogProvider) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var providerType, projectId types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("type"), &providerType)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("project_id"), &projectId)...)
	if resp.Diagnostics.HasError() || providerType.IsNull() || providerType.IsUnknown() {
		return
	}

	isGcp := providerType.ValueString() == logProviderTypeGcp
	switch {
	case isGcp && projectId.IsNull():
		resp.Diagnostics.AddAttributeError(
			path.Root("project_id"),
			"Missing required attribute",
			"`project_id` is required when `type` is \"gcp\".",
		)
	case !isGcp && !projectId.IsNull() && !projectId.IsUnknown():
		resp.Diagnostics.AddAttributeError(
			path.Root("project_id"),
			"Attribute not allowed",
			"`project_id` is not valid when `type` is "+providerType.String()+".",
		)
	}
}

func (r *LogProvider) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	providerData := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:  IntegrationKey,
		Component:    logProviderComponent,
		ProviderData: providerData,
		GetId:        r.getId,
		GetItemJson:  r.getItemJson,
		FromJson:     r.fromJson,
		ToJson:       r.toJson,
	}
}

func (r *LogProvider) getId(data any) *string {
	model, ok := data.(*logProviderModel)
	if !ok {
		return nil
	}
	return &model.Id
}

func (r *LogProvider) getItemJson(json any) any {
	api, ok := json.(*logProviderApi)
	if !ok {
		return nil
	}
	return &api.Item
}

func (r *LogProvider) fromJson(ctx context.Context, diags *diag.Diagnostics, id string, jsonData any) any {
	json, ok := jsonData.(*logProviderJson)
	if !ok {
		return nil
	}
	return &logProviderModel{
		Id:        id,
		Type:      types.StringValue(json.Provider.Type),
		ProjectId: types.StringPointerValue(json.Provider.ProjectId),
	}
}

func (r *LogProvider) toSettingsJson(model *logProviderModel) logProviderSettingsJson {
	return logProviderSettingsJson{
		Type:      model.Type.ValueString(),
		ProjectId: model.ProjectId.ValueStringPointer(),
	}
}

func (r *LogProvider) toJson(data any) any {
	model, ok := data.(*logProviderModel)
	if !ok {
		return nil
	}
	return &logProviderJson{
		Provider: r.toSettingsJson(model),
		State:    common.Config,
	}
}

func (r *LogProvider) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &logProviderModel{})
	if resp.Diagnostics.HasError() {
		return
	}

	var inputData logProviderModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &inputData)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &logProviderApi{}, &logProviderModel{}, &logProviderStageJson{Provider: r.toSettingsJson(&inputData)})
	if resp.Diagnostics.HasError() {
		return
	}
	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &logProviderApi{}, &logProviderModel{})
}

func (r *LogProvider) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &logProviderApi{}, &logProviderModel{})
}

// Update reconfigures the installed item. P0 does not check a log provider's
// settings on verify, so the configure step alone is enough.
func (r *LogProvider) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.installer.UpsertFromConfigure(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &logProviderApi{}, &logProviderModel{})
}

// Delete removes the item from P0. P0 does not check for gateways that still use
// it. Destroy their p0_ai_gateway_log_provider_link resources first, which
// Terraform does when the link refers to this resource's id.
func (r *LogProvider) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Delete(ctx, &resp.Diagnostics, &req.State, &logProviderModel{})
}

func (r *LogProvider) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
