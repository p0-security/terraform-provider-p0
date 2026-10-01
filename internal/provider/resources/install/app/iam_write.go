package installapp

import (
	"context"
	"fmt"

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

type appIamWriteJson struct {
	Hosting *ConnectorHostingJson `json:"hosting,omitempty"`
	Label   *string               `json:"label,omitempty"`
	State   *string               `json:"state,omitempty"`
}

type appIamWriteApi struct {
	Item *appIamWriteJson `json:"item"`
}

type appIamWriteModel struct {
	Id      types.String           `tfsdk:"id"`
	Hosting *ConnectorHostingModel `tfsdk:"hosting"`
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
			"hosting": ConnectorHostingAttribute(appHostingValidators),
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

// The checks on a custom application's connector address. A field's validator can't
// see the hosting's type, so each accepts what either Lambda or Cloud Run does, and
// ValidateConfig narrows the connector's name for `gcp`.
var appHostingValidators = ConnectorHostingValidators{
	AccountId: []validator.String{
		stringvalidator.RegexMatches(installaws.AwsAccountIdRegex, "AWS account IDs should be numeric"),
	},
	ProjectId: []validator.String{
		stringvalidator.RegexMatches(installgcp.GcpProjectIdRegex, "Must be a valid Google Cloud project ID"),
	},
	ConnectorName: []validator.String{
		stringvalidator.RegexMatches(ConnectorNameRegex, "Must be a valid Lambda function or Cloud Run service name"),
	},
	ConnectorRegion: []validator.String{
		stringvalidator.RegexMatches(ConnectorRegionRegex, "Must be a valid AWS or Google Cloud region"),
	},
}

// Rejects a hosting block whose address fields do not match its type. Validating here
// rather than in Create surfaces the mistake at plan time, before P0 is called.
func (*appIamWrite) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	hosting := ConnectorHostingFromConfig(ctx, req.Config, &resp.Diagnostics)
	ValidateConnectorHosting(hosting, &resp.Diagnostics)
	validateCloudRunServiceName(hosting, &resp.Diagnostics)
}

// Cloud Run's naming rules are stricter than the connector name's own validator, which
// has to accept Lambda function names too.
func validateCloudRunServiceName(hosting *ConnectorHostingModel, diags *diag.Diagnostics) {
	if hosting == nil || !IsSet(hosting.Type) || hosting.Type.ValueString() != GcpHosting || !IsSet(hosting.ConnectorName) {
		return
	}
	if !CloudRunServiceNameRegex.MatchString(hosting.ConnectorName.ValueString()) {
		diags.AddAttributeError(
			path.Root("hosting").AtName("connector_name"),
			"Invalid Cloud Run service name",
			"'hosting.connector_name' names a Cloud Run service when 'hosting.type' is \"gcp\": at most 49 characters of "+
				"lowercase letters, digits and hyphens, starting with a letter and not ending with one.",
		)
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

	// hosting is Required, so leaving it nil here would surface as Terraform core's
	// "provider produced inconsistent result after apply" rather than something the
	// reader can act on.
	if jsonv.Hosting == nil {
		diags.AddError(
			"Bad API response",
			fmt.Sprintf("The custom-application install %s carries no hosting configuration. "+
				"Configure it in the P0 app, or remove it from Terraform state with `terraform state rm`.", id),
		)
		return nil
	}

	data.Hosting = ConnectorHostingFromJson(jsonv.Hosting)

	return &data
}

func (r *appIamWrite) toJson(data any) any {
	json := appIamWriteJson{}

	datav, ok := data.(*appIamWriteModel)
	if !ok {
		return nil
	}

	if datav.Hosting != nil {
		json.Hosting = datav.Hosting.ToJson()
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
