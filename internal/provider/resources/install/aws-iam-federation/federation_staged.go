package installawsiamfederation

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/p0-security/terraform-provider-p0/internal"
	"github.com/p0-security/terraform-provider-p0/internal/common"
	installresources "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install"
)

var _ resource.Resource = &AwsIamFederationStaged{}
var _ resource.ResourceWithConfigure = &AwsIamFederationStaged{}
var _ resource.ResourceWithImportState = &AwsIamFederationStaged{}

type AwsIamFederationStaged struct {
	installer *common.Install
}

type federationMetadataJson struct {
	RoleName            string `json:"roleName"`
	TrustPolicy         string `json:"trustPolicy"`
	InlinePolicy        string `json:"inlinePolicy"`
	SamlProviderName    string `json:"samlProviderName"`
	PoolPath            string `json:"poolPath"`
	PoolRoleTrustPolicy string `json:"poolRoleTrustPolicy"`
	PoolRoleSshPolicy   string `json:"poolRoleSshPolicy"`
}

type federationStagedApi struct {
	Item     *federationJson         `json:"item"`
	Metadata *federationMetadataJson `json:"metadata"`
}

type federationStagedModel struct {
	federationConfigModel
	Role             types.Object `tfsdk:"role"`
	SamlProviderName types.String `tfsdk:"saml_provider_name"`
	Pool             types.Object `tfsdk:"pool"`
	State            types.String `tfsdk:"state"`
}

var roleAttributeTypes = map[string]attr.Type{
	"name":          types.StringType,
	"trust_policy":  types.StringType,
	"inline_policy": types.StringType,
}

var poolAttributeTypes = map[string]attr.Type{
	"path":         types.StringType,
	"trust_policy": types.StringType,
	"ssh_policy":   types.StringType,
}

func NewAwsIamFederationStaged() resource.Resource {
	return &AwsIamFederationStaged{}
}

func (*AwsIamFederationStaged) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_aws_iam_federation_staged"
}

func (*AwsIamFederationStaged) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attributes := configAttributes()
	attributes["role"] = schema.SingleNestedAttribute{
		Computed:            true,
		MarkdownDescription: `P0's IAM role for this federation. P0 uses it to read the SAML provider and the pool roles, and to put policies on the pool roles.`,
		Attributes: map[string]schema.Attribute{
			"name":          schema.StringAttribute{Computed: true, MarkdownDescription: `The role's name. Also use it as the name of its inline policy.`},
			"trust_policy":  schema.StringAttribute{Computed: true, MarkdownDescription: `The role's trust policy, as JSON`},
			"inline_policy": schema.StringAttribute{Computed: true, MarkdownDescription: `The role's inline policy, as JSON`},
		},
	}
	attributes["saml_provider_name"] = schema.StringAttribute{
		Computed:            true,
		MarkdownDescription: `The name of the IAM SAML provider that trusts the federation app. Create it from the app's SAML metadata.`,
	}
	attributes["pool"] = schema.SingleNestedAttribute{
		Computed:            true,
		MarkdownDescription: "The pool roles that users sign in to. Create `pool_size` roles on this path.",
		Attributes: map[string]schema.Attribute{
			"path":         schema.StringAttribute{Computed: true, MarkdownDescription: `The IAM path of the pool roles`},
			"trust_policy": schema.StringAttribute{Computed: true, MarkdownDescription: `The trust policy of each pool role, as JSON`},
			"ssh_policy":   schema.StringAttribute{Computed: true, MarkdownDescription: `The inline policy of each pool role, as JSON. It lets users resume and end their own SSM sessions.`},
		},
	}
	attributes["state"] = common.StateAttribute
	resp.Schema = schema.Schema{
		MarkdownDescription: `A staged AWS IAM Federation installation. Staging tells P0 which AWS account trusts which federation app, and gives the names and policies of the IAM resources to create in that account.

Use the read-only ` + "`role`" + `, ` + "`saml_provider_name`" + ` and ` + "`pool`" + ` to create P0's role, the SAML provider and the pool roles. Then create a ` + "`p0_aws_iam_federation`" + ` resource with the same ` + "`id`" + ` to complete the installation.

P0 fixes every attribute when it adds the federation, so a change replaces it.

**Prerequisite:** the federation app, installed in P0's Okta (AWS IAM Federation) or PingOne (AWS IAM Federation) integration.

**Note:** This integration is currently in preview.

` + docsLink,
		Attributes: attributes,
	}
}

func (r *AwsIamFederationStaged) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (*AwsIamFederationStaged) getId(data any) *string {
	model, ok := data.(*federationStagedModel)
	if !ok {
		return nil
	}
	id := model.Id.ValueString()
	return &id
}

// Returns the whole response, because the names and policies are in its metadata.
func (*AwsIamFederationStaged) getItemJson(json any) any {
	inner, ok := json.(*federationStagedApi)
	if !ok || inner.Item == nil {
		return nil
	}
	return inner
}

func (*AwsIamFederationStaged) fromJson(_ context.Context, diags *diag.Diagnostics, id string, json any) any {
	jsonv, ok := json.(*federationStagedApi)
	if !ok || jsonv.Item == nil {
		return nil
	}
	metadata := jsonv.Metadata
	if metadata == nil {
		metadata = &federationMetadataJson{}
	}

	role, roleDiags := types.ObjectValue(roleAttributeTypes, map[string]attr.Value{
		"name":          types.StringValue(metadata.RoleName),
		"trust_policy":  types.StringValue(metadata.TrustPolicy),
		"inline_policy": types.StringValue(metadata.InlinePolicy),
	})
	diags.Append(roleDiags...)
	pool, poolDiags := types.ObjectValue(poolAttributeTypes, map[string]attr.Value{
		"path":         types.StringValue(metadata.PoolPath),
		"trust_policy": types.StringValue(metadata.PoolRoleTrustPolicy),
		"ssh_policy":   types.StringValue(metadata.PoolRoleSshPolicy),
	})
	diags.Append(poolDiags...)
	if diags.HasError() {
		return nil
	}

	return &federationStagedModel{
		federationConfigModel: configFromJson(id, jsonv.Item),
		Role:                  role,
		SamlProviderName:      types.StringValue(metadata.SamlProviderName),
		Pool:                  pool,
		State:                 types.StringPointerValue(jsonv.Item.State),
	}
}

func (*AwsIamFederationStaged) toJson(data any) any {
	datav, ok := data.(*federationStagedModel)
	if !ok {
		return nil
	}
	return datav.toJson()
}

func (r *AwsIamFederationStaged) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data federationStagedModel
	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	if resp.Diagnostics.HasError() {
		return
	}
	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &federationStagedApi{}, &data, r.toJson(&data))
}

func (r *AwsIamFederationStaged) Read(ctx context.Context, _ resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &federationStagedApi{}, &federationStagedModel{})
}

// Every configurable attribute requires replacement, so Terraform never plans an
// in-place update.
func (*AwsIamFederationStaged) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError(
		"AWS IAM Federations can't be updated in place",
		"P0 fixes a federation's attributes when it adds the federation. Replace this resource to change them.",
	)
}

func (r *AwsIamFederationStaged) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Delete(ctx, &resp.Diagnostics, &req.State, &federationStagedModel{})
}

func (*AwsIamFederationStaged) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
