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
var _ resource.Resource = &azureIamWrite{}
var _ resource.ResourceWithImportState = &azureIamWrite{}
var _ resource.ResourceWithConfigure = &azureIamWrite{}

func NewAzureIamWrite() resource.Resource {
	return &azureIamWrite{}
}

type azureIamWriteModel struct {
	SubscriptionId types.String `tfsdk:"subscription_id"`
	Label          types.String `tfsdk:"label"`
	State          types.String `tfsdk:"state"`
}

type azureIamWriteJson struct {
	Label string `json:"label"`
	State string `json:"state"`
}

type azureIamWriteApi struct {
	Item azureIamWriteJson `json:"item"`
}

type azureIamWrite struct {
	installer *common.Install
}

func (r *azureIamWrite) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_azure_iam_write"
}

func (r *azureIamWrite) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `An installation of P0, on a single Microsoft Azure Cloud Subscription, for IAM management.

To use this resource, you must also:
- create an app registration in Azure for P0,
- create federated credentials for P0 to communicate with Azure through the app registration,
- create a custom role allowing IAM operations,
- assign this custom role to P0's app registration at the subscription level,
- (optional) constraint role assignment to specific roles or principals,
- install the ` + "`p0_azure`" + ` resource,
- install the ` + "`p0_azure_app`" + ` resource,
- install the ` + "`p0_azure_iam_write_staged`" + ` resource,

See the example usage for the recommended pattern to define this infrastructure.`,
		Attributes: map[string]schema.Attribute{
			"subscription_id": subscriptionIdAttribute,
			"label":           labelAttribute,
			"state":           common.StateAttribute,
		},
	}
}

func (r *azureIamWrite) getId(data any) *string {
	model, ok := data.(*azureIamWriteModel)
	if !ok {
		return nil
	}
	return model.SubscriptionId.ValueStringPointer()
}

func (r *azureIamWrite) getItemJson(json any) any {
	inner, ok := json.(*azureIamWriteApi)
	if !ok {
		return nil
	}
	return &inner.Item
}

func (r *azureIamWrite) fromJson(ctx context.Context, diags *diag.Diagnostics, id string, json any) any {
	data := azureIamWriteModel{}
	jsonv, ok := json.(*azureIamWriteJson)
	if !ok {
		return nil
	}

	data.SubscriptionId = types.StringValue(id)
	data.State = types.StringValue(jsonv.State)
	data.Label = types.StringValue(jsonv.Label)

	return &data
}

func (r *azureIamWrite) toJson(data any) any {
	json := azureIamWriteApi{}

	// can omit state here as it's filled by the backend
	return json
}

func (r *azureIamWrite) newItemInstaller(component string, providerData *internal.P0ProviderData) *common.Install {
	return &common.Install{
		Integration:  AzureKey,
		Component:    component,
		ProviderData: providerData,
		GetId:        r.getId,
		GetItemJson:  r.getItemJson,
		FromJson:     r.fromJson,
		ToJson:       r.toJson,
	}
}

func (r *azureIamWrite) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	providerData := internal.Configure(&req, resp)
	r.installer = r.newItemInstaller(installresources.IamWrite, providerData)
}

func (s *azureIamWrite) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	s.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &azureIamWriteApi{}, &azureIamWriteModel{})
}

func (s *azureIamWrite) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	s.installer.Read(ctx, &resp.Diagnostics, &resp.State, &azureIamWriteApi{}, &azureIamWriteModel{})
}

func (s *azureIamWrite) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	s.installer.Rollback(ctx, &resp.Diagnostics, &req.State, &azureIamWriteModel{})
}

func (s *azureIamWrite) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	s.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &azureIamWriteApi{}, &azureIamWriteModel{})
}

func (s *azureIamWrite) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("subscription_id"), req, resp)
}
