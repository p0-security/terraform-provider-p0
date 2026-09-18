package installapp

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
	installgcp "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/gcp"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &appIamWrite{}
var _ resource.ResourceWithConfigure = &appIamWrite{}
var _ resource.ResourceWithImportState = &appIamWrite{}
var _ resource.ResourceWithValidateConfig = &appIamWrite{}

type appIamWrite struct {
	installer *common.Install
}

// The connector's address, as P0 stores it. The JSON is a discriminated union on
// "type": the AWS variant carries accountId, the Google Cloud variant carries
// projectId and the connectorServiceUri P0 resolves while verifying the install.
type connectorHostingJson struct {
	Type                string  `json:"type"`
	AccountId           *string `json:"accountId,omitempty"`
	ProjectId           *string `json:"projectId,omitempty"`
	ConnectorName       string  `json:"connectorName"`
	ConnectorRegion     string  `json:"connectorRegion"`
	ConnectorServiceUri *string `json:"connectorServiceUri,omitempty"`
}

type connectorHostingModel struct {
	Type                types.String `tfsdk:"type"`
	AccountId           types.String `tfsdk:"account_id"`
	ProjectId           types.String `tfsdk:"project_id"`
	ConnectorName       types.String `tfsdk:"connector_name"`
	ConnectorRegion     types.String `tfsdk:"connector_region"`
	ConnectorServiceUri types.String `tfsdk:"connector_service_uri"`
}

type appIamWriteJson struct {
	Hosting *connectorHostingJson `json:"hosting,omitempty"`
	Label   *string               `json:"label,omitempty"`
	State   *string               `json:"state,omitempty"`
}

type appIamWriteApi struct {
	Item *appIamWriteJson `json:"item"`
}

type appIamWriteModel struct {
	Id      types.String           `tfsdk:"id"`
	Hosting *connectorHostingModel `tfsdk:"hosting"`
	Label   types.String           `tfsdk:"label"`
	State   types.String           `tfsdk:"state"`
}

func NewAppIamWrite() resource.Resource {
	return &appIamWrite{}
}

func (*appIamWrite) Metadata(_ context.Context, req resource.MetadataRequest, res *resource.MetadataResponse) {
	res.TypeName = req.ProviderTypeName + "_app"
}

func (*appIamWrite) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A custom application installation (preview).

Registers a connector you built with [` + "`@p0security/connector-sdk`" + `](https://github.com/p0-security/connector) and
deployed into your own AWS or Google Cloud account, so that access to your application can be requested through P0.

Deploy the connector and grant P0 permission to invoke it before applying this resource. Creating it verifies that
P0 can reach the connector, and fails if it cannot.

**Prerequisites:**
- For AWS Lambda hosting, ` + "`p0_aws_iam_write`" + ` must be installed for the account the connector runs in. Grant
  that installation's role ` + "`lambda:InvokeFunction`" + ` on the connector's function.
- For Google Cloud Run hosting, ` + "`p0_gcp`" + ` must be installed. Grant its service account
  ` + "`roles/run.invoker`" + ` on the connector's service, and pass the same address to the connector as its
  ` + "`INVOKER_SA_EMAIL`" + ` environment variable.`,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: `A unique identifier for this application (can be any string, e.g. "internal-admin-tool")`,
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"hosting": schema.SingleNestedAttribute{
				MarkdownDescription: `Where your connector is deployed, and how P0 addresses it`,
				Required:            true,
				Attributes: map[string]schema.Attribute{
					"type": schema.StringAttribute{
						MarkdownDescription: `The connector's hosting: either ` + "`aws`" + ` (Lambda) or ` + "`gcp`" + ` (Cloud Run)`,
						Required:            true,
						Validators: []validator.String{
							stringvalidator.OneOf(AwsHosting, GcpHosting),
						},
						PlanModifiers: []planmodifier.String{
							stringplanmodifier.RequiresReplace(),
						},
					},
					"account_id": schema.StringAttribute{
						MarkdownDescription: `The AWS account ID in which the connector's Lambda function is deployed. Required for, and only valid with, ` + "`aws`" + ` hosting.`,
						Optional:            true,
						Validators: []validator.String{
							stringvalidator.RegexMatches(installaws.AwsAccountIdRegex, "AWS account IDs should be numeric"),
						},
					},
					"project_id": schema.StringAttribute{
						MarkdownDescription: `The Google Cloud project ID in which the connector's Cloud Run service is deployed. Required for, and only valid with, ` + "`gcp`" + ` hosting.`,
						Optional:            true,
						Validators: []validator.String{
							stringvalidator.RegexMatches(installgcp.GcpProjectIdRegex, "Must be a valid Google Cloud project ID"),
						},
					},
					"connector_name": schema.StringAttribute{
						MarkdownDescription: `The name of the Lambda function or Cloud Run service hosting the connector`,
						Required:            true,
						Validators: []validator.String{
							stringvalidator.RegexMatches(ConnectorNameRegex, "Must be a valid Lambda function or Cloud Run service name"),
						},
					},
					"connector_region": schema.StringAttribute{
						MarkdownDescription: `The region the connector is deployed in (e.g. ` + "`us-east-1`" + ` on AWS, or ` + "`us-central1`" + ` on Google Cloud)`,
						Required:            true,
						Validators: []validator.String{
							stringvalidator.RegexMatches(ConnectorRegionRegex, "Must be a valid AWS or Google Cloud region"),
						},
					},
					"connector_service_uri": schema.StringAttribute{
						MarkdownDescription: `The connector's invocation URL, resolved by P0 during install. Only populated for ` + "`gcp`" + ` hosting.`,
						Computed:            true,
					},
				},
			},
			"label": schema.StringAttribute{
				MarkdownDescription: `The label for this installation (defaults to the application identifier)`,
				Computed:            true,
			},
			"state": schema.StringAttribute{
				MarkdownDescription: common.StateMarkdownDescription,
				Computed:            true,
			},
		},
	}
}

// Rejects a hosting block whose address fields do not match its type. Validating here
// rather than in Create surfaces the mistake at plan time, before P0 is called.
func (*appIamWrite) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var data appIamWriteModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() || data.Hosting == nil {
		return
	}

	hosting := data.Hosting
	// An unknown type is a value only resolved at apply time; the type's own validator
	// has already rejected anything that is known and unsupported.
	if hosting.Type.IsUnknown() {
		return
	}

	switch hosting.Type.ValueString() {
	case AwsHosting:
		if hosting.AccountId.IsNull() {
			resp.Diagnostics.AddAttributeError(
				path.Root("hosting").AtName("account_id"),
				"Missing AWS account ID",
				"'hosting.account_id' is required when 'hosting.type' is \"aws\".",
			)
		}
		if !hosting.ProjectId.IsNull() {
			resp.Diagnostics.AddAttributeError(
				path.Root("hosting").AtName("project_id"),
				"Unexpected Google Cloud project",
				"'hosting.project_id' may only be set when 'hosting.type' is \"gcp\".",
			)
		}
	case GcpHosting:
		if hosting.ProjectId.IsNull() {
			resp.Diagnostics.AddAttributeError(
				path.Root("hosting").AtName("project_id"),
				"Missing Google Cloud project",
				"'hosting.project_id' is required when 'hosting.type' is \"gcp\".",
			)
		}
		if !hosting.AccountId.IsNull() {
			resp.Diagnostics.AddAttributeError(
				path.Root("hosting").AtName("account_id"),
				"Unexpected AWS account ID",
				"'hosting.account_id' may only be set when 'hosting.type' is \"aws\".",
			)
		}
	}
}

func (r *appIamWrite) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:  AppKey,
		Component:    installresources.IamWrite,
		ProviderData: data,
		GetId:        r.getId,
		GetItemJson:  r.getItemJson,
		FromJson:     r.fromJson,
		ToJson:       r.toJson,
	}
}

func (r *appIamWrite) getId(data any) *string {
	model, ok := data.(*appIamWriteModel)
	if !ok {
		return nil
	}

	str := model.Id.ValueString()
	return &str
}

func (r *appIamWrite) getItemJson(json any) any {
	inner, ok := json.(*appIamWriteApi)
	if !ok {
		return nil
	}
	return inner.Item
}

func (r *appIamWrite) fromJson(ctx context.Context, diags *diag.Diagnostics, id string, json any) any {
	data := appIamWriteModel{}
	jsonv, ok := json.(*appIamWriteJson)
	if !ok {
		return nil
	}

	data.Id = types.StringValue(id)
	data.Label = types.StringPointerValue(jsonv.Label)
	data.State = types.StringPointerValue(jsonv.State)

	if jsonv.Hosting != nil {
		data.Hosting = &connectorHostingModel{
			Type:                types.StringValue(jsonv.Hosting.Type),
			AccountId:           types.StringPointerValue(jsonv.Hosting.AccountId),
			ProjectId:           types.StringPointerValue(jsonv.Hosting.ProjectId),
			ConnectorName:       types.StringValue(jsonv.Hosting.ConnectorName),
			ConnectorRegion:     types.StringValue(jsonv.Hosting.ConnectorRegion),
			ConnectorServiceUri: types.StringPointerValue(jsonv.Hosting.ConnectorServiceUri),
		}
	}

	return &data
}

func (r *appIamWrite) toJson(data any) any {
	json := appIamWriteJson{}

	datav, ok := data.(*appIamWriteModel)
	if !ok {
		return nil
	}

	if datav.Hosting != nil {
		json.Hosting = &connectorHostingJson{
			Type:            datav.Hosting.Type.ValueString(),
			AccountId:       datav.Hosting.AccountId.ValueStringPointer(),
			ProjectId:       datav.Hosting.ProjectId.ValueStringPointer(),
			ConnectorName:   datav.Hosting.ConnectorName.ValueString(),
			ConnectorRegion: datav.Hosting.ConnectorRegion.ValueString(),
		}
	}

	// label and state are omitted; P0 fills both in.
	return &json
}

func (r *appIamWrite) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json appIamWriteApi
	var data appIamWriteModel

	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	if resp.Diagnostics.HasError() {
		return
	}

	// Stage with the configuration fields already present, so that the verify step
	// that follows has the connector's address to probe.
	inputJson := r.toJson(&data)

	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, inputJson)
	if resp.Diagnostics.HasError() {
		return
	}

	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data)
}

func (r *appIamWrite) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var json appIamWriteApi
	var data appIamWriteModel
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &json, &data)
}

func (r *appIamWrite) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var json appIamWriteApi
	var data appIamWriteModel
	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data)
}

func (r *appIamWrite) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data appIamWriteModel
	r.installer.Delete(ctx, &resp.Diagnostics, &req.State, &data)
}

func (r *appIamWrite) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
