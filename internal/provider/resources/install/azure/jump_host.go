// Azure jump host management registers the customer's Azure Function App (and the app registration
// P0 authenticates as via workload identity federation) with P0, so P0 can send privileged commands
// (for example, terminating a live SSH session) to jump host VMs in the customer's tenant.
//
// This is a singleton component: the tenant and P0 service account are written server-side by the
// install assembler, and the user supplies only `client_id` and `function_app_resource_id`.
// See `examples/resources/p0_azure_jump_host/`.

package installazure

import (
	"context"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/p0-security/terraform-provider-p0/internal"
	"github.com/p0-security/terraform-provider-p0/internal/common"
	installresources "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install"
)

// Mirrors the backend's isAzureFunctionAppResourceId validation
// (shared/src/integrations/resources/azure/asset.ts).
var functionAppResourceIdRegex = regexp.MustCompile(
	`(?i)^/subscriptions/[^/]+/resourceGroups/[^/]+/providers/Microsoft\.Web/sites/[^/]+$`,
)

const functionAppResourceIdExample = "/subscriptions/<id>/resourceGroups/<rg>/providers/Microsoft.Web/sites/<name>"

var _ resource.Resource = &azureJumpHost{}
var _ resource.ResourceWithImportState = &azureJumpHost{}
var _ resource.ResourceWithConfigure = &azureJumpHost{}

func NewAzureJumpHost() resource.Resource {
	return &azureJumpHost{}
}

type azureJumpHost struct {
	installer *common.Install
}

type azureJumpHostModel struct {
	ClientId              types.String `tfsdk:"client_id"`
	FunctionAppResourceId types.String `tfsdk:"function_app_resource_id"`
	DirectoryId           types.String `tfsdk:"directory_id"`
	ServiceAccountEmail   types.String `tfsdk:"service_account_email"`
	ServiceAccountId      types.String `tfsdk:"service_account_id"`
	Label                 types.String `tfsdk:"label"`
	State                 types.String `tfsdk:"state"`
}

// Request payload for the P0 API: only the user-configurable fields. The tenant
// and P0 service account are written server-side by the install assembler.
type jumpHostReqJson struct {
	ClientId              string `json:"clientId"`
	FunctionAppResourceId string `json:"functionAppResourceId"`
}

type jumpHostJson struct {
	ClientId              string `json:"clientId"`
	FunctionAppResourceId string `json:"functionAppResourceId"`
	DirectoryId           string `json:"directoryId"`
	ServiceAccountEmail   string `json:"serviceAccountEmail"`
	ServiceAccountId      string `json:"serviceAccountId"`
	Label                 string `json:"label"`
	State                 string `json:"state"`
}

type jumpHostApi struct {
	Item jumpHostJson `json:"item"`
}

func (r *azureJumpHost) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_azure_jump_host"
}

func (r *azureJumpHost) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `Registers Azure jump host management (beta) with P0, allowing P0 to send privileged commands (for example, terminating a live SSH session) to your jump hosts through an Azure Function App.

To use this resource, you must also:
- install the ` + "`p0_azure`" + ` resource,
- install the ` + "`p0_azure_app`" + ` resource,
- create an Azure app registration for P0 to authenticate as, with a federated credential (workload identity federation) trusting P0's service account,
- deploy the Azure Function App that dispatches privileged commands to your jump hosts.
` + "\n\nExample:\n\n```terraform\n" +
			"resource \"p0_azure_jump_host\" \"example\" {\n" +
			"  depends_on = [\n" +
			"    p0_azure.example,\n" +
			"    p0_azure_app.example,\n" +
			"  ]\n" +
			"\n" +
			"  client_id                = \"12345678-1234-1234-1234-123456789012\"\n" +
			"  function_app_resource_id = \"" + functionAppResourceIdExample + "\"\n" +
			"}\n" +
			"```\n",
		Attributes: map[string]schema.Attribute{
			"client_id": schema.StringAttribute{
				Description: "The application (client) ID of the app registration P0 uses (via workload identity federation) to authenticate to the jump host management Azure Function.",
				Required:    true,
				Validators: []validator.String{
					stringvalidator.RegexMatches(common.UuidRegex, "Expected application client ID (UUID format)."),
				},
			},
			"function_app_resource_id": schema.StringAttribute{
				Description: "The Azure resource ID of the Function App P0 sends requests to, e.g. " + functionAppResourceIdExample + ".",
				Required:    true,
				Validators: []validator.String{
					stringvalidator.RegexMatches(functionAppResourceIdRegex, "Enter a valid Function App resource ID, e.g. "+functionAppResourceIdExample+"."),
				},
			},
			"directory_id": schema.StringAttribute{
				Description: "The Azure tenant (directory) ID this install belongs to (computed from P0).",
				Computed:    true,
			},
			"service_account_email": schema.StringAttribute{
				Description: "The human-readable identifier of the P0 service account the app registration's federated credential must trust (computed from P0).",
				Computed:    true,
			},
			"service_account_id": schema.StringAttribute{
				Description: "The machine identifier (subject) of the P0 service account the app registration's federated credential must trust (computed from P0).",
				Computed:    true,
			},
			"label": schema.StringAttribute{
				Description: "The Function App name, used to label the component (computed from P0).",
				Computed:    true,
			},
			"state": common.StateAttribute,
		},
	}
}

func (r *azureJumpHost) getItemJson(json any) any {
	inner, ok := json.(*jumpHostApi)
	if !ok {
		return nil
	}
	return &inner.Item
}

func (r *azureJumpHost) fromJson(ctx context.Context, diags *diag.Diagnostics, id string, json any) any {
	data := azureJumpHostModel{}
	jsonv, ok := json.(*jumpHostJson)
	if !ok {
		return nil
	}

	data.ClientId = types.StringValue(jsonv.ClientId)
	data.FunctionAppResourceId = types.StringValue(jsonv.FunctionAppResourceId)
	data.DirectoryId = types.StringValue(jsonv.DirectoryId)
	data.ServiceAccountEmail = types.StringValue(jsonv.ServiceAccountEmail)
	data.ServiceAccountId = types.StringValue(jsonv.ServiceAccountId)
	data.Label = types.StringValue(jsonv.Label)
	data.State = types.StringValue(jsonv.State)

	return &data
}

func (r *azureJumpHost) toJson(data any) any {
	datav, ok := data.(*azureJumpHostModel)
	if !ok {
		return nil
	}

	// can omit the computed fields here; they are filled by the backend
	return &jumpHostReqJson{
		ClientId:              datav.ClientId.ValueString(),
		FunctionAppResourceId: datav.FunctionAppResourceId.ValueString(),
	}
}

func (r *azureJumpHost) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	providerData := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:  AzureKey,
		Component:    installresources.JumpHost,
		ProviderData: providerData,
		GetId:        singletonGetId,
		GetItemJson:  r.getItemJson,
		FromJson:     r.fromJson,
		ToJson:       r.toJson,
	}
}

func (s *azureJumpHost) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json jumpHostApi
	var data azureJumpHostModel
	// Stage runs the install assembler, which writes the tenant and P0 service
	// account server-side; UpsertFromStage then sends client_id and
	// function_app_resource_id through the configurer.
	s.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, &struct{}{})
	s.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data)
}

func (s *azureJumpHost) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	s.installer.Read(ctx, &resp.Diagnostics, &resp.State, &jumpHostApi{}, &azureJumpHostModel{})
}

func (s *azureJumpHost) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	s.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &jumpHostApi{}, &azureJumpHostModel{})
}

func (s *azureJumpHost) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	s.installer.Delete(ctx, &resp.Diagnostics, &req.State, &azureJumpHostModel{})
}

func (s *azureJumpHost) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Empty(), req, resp)
}
