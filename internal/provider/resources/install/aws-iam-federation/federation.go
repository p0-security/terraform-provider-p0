package installawsiamfederation

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

var _ resource.Resource = &AwsIamFederation{}
var _ resource.ResourceWithConfigure = &AwsIamFederation{}
var _ resource.ResourceWithImportState = &AwsIamFederation{}

type AwsIamFederation struct {
	installer *common.Install
}

type federationApi struct {
	Item *federationJson `json:"item"`
}

type federationModel struct {
	federationConfigModel
	State types.String `tfsdk:"state"`
}

func NewAwsIamFederation() resource.Resource {
	return &AwsIamFederation{}
}

func (*AwsIamFederation) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_aws_iam_federation"
}

func (*AwsIamFederation) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attributes := configAttributes()
	id := attributes["id"].(schema.StringAttribute)
	id.MarkdownDescription = "The `id` of the `p0_aws_iam_federation_staged` resource whose install this finishes"
	attributes["id"] = id
	attributes["state"] = common.StateAttribute
	resp.Schema = schema.Schema{
		MarkdownDescription: `An AWS IAM Federation installation. Users sign in to one AWS account through a federation app in Okta or PingOne, and P0 grants them access by putting policies on the account's roles and pool roles.

**Important:** Before you create this resource, stage the federation with ` + "`p0_aws_iam_federation_staged`" + `, and create P0's role, the SAML provider and the pool roles that it describes. Creating this resource has P0 check that the SAML provider trusts the federation app and that the pool has ` + "`pool_size`" + ` roles that trust the SAML provider. If a check fails, the apply fails with P0's message.

To have an account's users sign in through the federation, set its ` + "`p0_aws_iam_write`" + ` login to ` + "`type = \"aws-iam-federation\"`" + ` and ` + "`federation`" + ` to this resource's ` + "`id`" + `.

Set every other attribute to the staged resource's value, so that a change to the staged federation replaces this resource too.

Destroying this resource returns the federation to its staged state, which ` + "`p0_aws_iam_federation_staged`" + ` still owns.

**Note:** This integration is currently in preview.

` + docsLink,
		Attributes: attributes,
	}
}

func (r *AwsIamFederation) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.installer = &common.Install{
		Integration:  AwsIamFederationKey,
		Component:    installresources.Federation,
		ProviderData: internal.Configure(&req, resp),
		GetId:        r.getId,
		GetItemJson:  r.getItemJson,
		FromJson:     r.fromJson,
		ToJson:       r.toJson,
	}
}

func (*AwsIamFederation) getId(data any) *string {
	model, ok := data.(*federationModel)
	if !ok {
		return nil
	}
	id := model.Id.ValueString()
	return &id
}

func (*AwsIamFederation) getItemJson(json any) any {
	inner, ok := json.(*federationApi)
	if !ok {
		return nil
	}
	return inner.Item
}

func (*AwsIamFederation) fromJson(_ context.Context, _ *diag.Diagnostics, id string, json any) any {
	jsonv, ok := json.(*federationJson)
	if !ok {
		return nil
	}
	return &federationModel{federationConfigModel: configFromJson(id, jsonv), State: types.StringPointerValue(jsonv.State)}
}

func (*AwsIamFederation) toJson(data any) any {
	datav, ok := data.(*federationModel)
	if !ok {
		return nil
	}
	return datav.toJson()
}

func (r *AwsIamFederation) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &federationApi{}, &federationModel{})
}

func (r *AwsIamFederation) Read(ctx context.Context, _ resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &federationApi{}, &federationModel{})
}

// Every configurable attribute requires replacement, so Terraform never plans an
// in-place update.
func (r *AwsIamFederation) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &federationApi{}, &federationModel{})
}

// Returns the item to the "stage" state; the staged resource's Delete removes it.
func (r *AwsIamFederation) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Rollback(ctx, &resp.Diagnostics, &req.State, &federationModel{})
}

func (*AwsIamFederation) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
