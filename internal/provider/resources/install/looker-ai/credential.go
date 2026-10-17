package installlookerai

import (
	"context"

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
	installvaultedcredential "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/vaulted-credential"
)

var _ resource.Resource = &LookerAiCredential{}
var _ resource.ResourceWithConfigure = &LookerAiCredential{}
var _ resource.ResourceWithImportState = &LookerAiCredential{}
var _ resource.ResourceWithModifyPlan = &LookerAiCredential{}

type LookerAiCredential struct {
	installer *common.Install
}

type lookerAiCredentialModel struct {
	Id              types.String                                 `tfsdk:"id"`
	InstanceUrl     types.String                                 `tfsdk:"instance_url"`
	AdminKeySecret  types.String                                 `tfsdk:"admin_key_secret"`
	SecretManager   *installvaultedcredential.SecretManagerModel `tfsdk:"secret_manager"`
	ContentFolderId types.String                                 `tfsdk:"content_folder_id"`
	State           types.String                                 `tfsdk:"state"`
}

type lookerAiCredentialJson struct {
	InstanceUrl     *string                                     `json:"instanceUrl,omitempty"`
	AdminKeySecret  *string                                     `json:"adminKeySecret,omitempty"`
	SecretManager   *installvaultedcredential.SecretManagerJson `json:"secretManager,omitempty"`
	ContentFolderId *string                                     `json:"contentFolderId,omitempty"`
	State           *string                                     `json:"state,omitempty"`
}

type lookerAiCredentialApi struct {
	Item *lookerAiCredentialJson `json:"item"`
}

func NewLookerAiCredential() resource.Resource {
	return &LookerAiCredential{}
}

func (*LookerAiCredential) Metadata(_ context.Context, req resource.MetadataRequest, res *resource.MetadataResponse) {
	res.TypeName = req.ProviderTypeName + "_looker_ai"
}

const sameAsStaged = " Must match the `p0_looker_ai_staged` resource."

func (*LookerAiCredential) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A Looker installation for agentic access.

With it, P0 can grant agents just-in-time access to the LookML models of a Looker (Google Cloud core) instance. Each grant gets its own Looker service account, which P0 deletes when the grant ends.

**Important:** Before you create this resource, stage the installation with ` + "`p0_looker_ai_staged`" + `, deploy the connector's Cloud Run service, and add the Looker Admin key as a version of the Admin key's secret. Creating this resource runs P0's install check. The connector signs in to the instance with that key, so the check fails until the version exists.

**Note:** This integration is in preview.

For the full install steps, see [Looker](https://docs.p0.dev/integrations/resource-integrations/agentic-gateway/mcp-server/looker).`,
		Attributes: map[string]schema.Attribute{
			"id":               idAttribute("The `id` of the `p0_looker_ai_staged` resource being finalized"),
			"instance_url":     instanceUrlAttribute(sameAsStaged),
			"admin_key_secret": adminKeySecretAttribute(sameAsStaged),
			"secret_manager": schema.SingleNestedAttribute{
				Required:            true,
				MarkdownDescription: `Where P0's Looker connector runs and where the Looker instance's Admin key is stored`,
				Attributes:          installvaultedcredential.FinalSecretManagerAttributes(secretLabel, sameAsStaged),
			},
			"content_folder_id": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "The ID of the Looker folder that agents save content to, the last part of its URL (`/folders/<ID>`). An edit grant gets edit access to it, and content saved there outlives the grant. Without a folder, content that an agent saves goes to Trash when its grant ends. Once you set it, you can change it to another folder but not remove it.",
				Validators: []validator.String{
					stringvalidator.RegexMatches(folderIdRegex, "Enter the folder's ID, the last part of its URL: /folders/<ID>"),
				},
			},
			"state": common.StateAttribute,
		},
	}
}

func (r *LookerAiCredential) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:        LookerAiKey,
		Component:          installresources.Credential,
		ProviderData:       data,
		GetId:              r.getId,
		GetItemJson:        r.getItemJson,
		FromJson:           r.fromJson,
		ToJson:             r.toJson,
		DescribeCheckError: describeCheckError,
	}
}

// P0 merges each write over the item it stores, so leaving content_folder_id out
// keeps the folder that P0 has. Removing it would then fail the apply with an
// inconsistent result, so the plan refuses it.
func (r *LookerAiCredential) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}

	var prior, planned types.String
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("content_folder_id"), &prior)...)
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("content_folder_id"), &planned)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !prior.IsNull() && planned.IsNull() {
		resp.Diagnostics.AddAttributeError(
			path.Root("content_folder_id"),
			"Cannot remove content_folder_id",
			"P0 keeps the Looker content folder once it is set, so removing content_folder_id would not remove it. "+
				"Set it to another folder, or destroy both p0_looker_ai and p0_looker_ai_staged and create them again without it.",
		)
	}
}

func (r *LookerAiCredential) getId(data any) *string {
	model, ok := data.(*lookerAiCredentialModel)
	if !ok {
		return nil
	}
	str := model.Id.ValueString()
	return &str
}

func (r *LookerAiCredential) getItemJson(json any) any {
	inner, ok := json.(*lookerAiCredentialApi)
	if !ok {
		return nil
	}
	return inner.Item
}

func (r *LookerAiCredential) fromJson(_ context.Context, diags *diag.Diagnostics, id string, json any) any {
	jsonv, ok := json.(*lookerAiCredentialJson)
	if !ok {
		return nil
	}
	if !installvaultedcredential.RequireSecretManager(diags, integrationLabel, id, jsonv.SecretManager) {
		return nil
	}

	secretManager := installvaultedcredential.SecretManagerFromJson(jsonv.SecretManager)
	return &lookerAiCredentialModel{
		Id:              types.StringValue(id),
		InstanceUrl:     types.StringPointerValue(jsonv.InstanceUrl),
		AdminKeySecret:  types.StringPointerValue(jsonv.AdminKeySecret),
		SecretManager:   &secretManager,
		ContentFolderId: types.StringPointerValue(jsonv.ContentFolderId),
		State:           types.StringPointerValue(jsonv.State),
	}
}

func (r *LookerAiCredential) toJson(data any) any {
	datav, ok := data.(*lookerAiCredentialModel)
	if !ok {
		return nil
	}

	json := lookerAiCredentialJson{
		InstanceUrl:     datav.InstanceUrl.ValueStringPointer(),
		AdminKeySecret:  datav.AdminKeySecret.ValueStringPointer(),
		ContentFolderId: datav.ContentFolderId.ValueStringPointer(),
	}
	if datav.SecretManager != nil {
		json.SecretManager = datav.SecretManager.ToJson()
	}
	return &json
}

func (r *LookerAiCredential) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json lookerAiCredentialApi
	var data lookerAiCredentialModel

	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	if resp.Diagnostics.HasError() {
		return
	}

	inputJson := r.toJson(&data)
	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, inputJson)
	if resp.Diagnostics.HasError() {
		return
	}

	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data)
}

func (r *LookerAiCredential) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &lookerAiCredentialApi{}, &lookerAiCredentialModel{})
}

func (r *LookerAiCredential) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &lookerAiCredentialApi{}, &lookerAiCredentialModel{})
}

func (r *LookerAiCredential) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Rollback(ctx, &resp.Diagnostics, &req.State, &lookerAiCredentialModel{})
}

func (r *LookerAiCredential) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
