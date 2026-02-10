// Copyright (c) 2024 P0 Security, Inc
// SPDX-License-Identifier: MPL-2.0

package installk8s

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/p0-security/terraform-provider-p0/internal"
	"github.com/p0-security/terraform-provider-p0/internal/common"
	installresources "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &AwsKubernetes{}
var _ resource.ResourceWithImportState = &AwsKubernetes{}
var _ resource.ResourceWithConfigure = &AwsKubernetes{}

func NewAwsKubernetes() resource.Resource {
	return &AwsKubernetes{}
}

type AwsKubernetes struct {
	installer *common.Install
}

type kubernetesLoginModel struct {
	Type string `json:"type" tfsdk:"type"`
}

type awsKubernetesModel struct {
	Id        string                `tfsdk:"id"`
	Label     basetypes.StringValue `tfsdk:"label"`
	Token     basetypes.StringValue `tfsdk:"token"`
	PublicJwk basetypes.StringValue `tfsdk:"public_jwk"`
	State     basetypes.StringValue `tfsdk:"state"`
	Login     *kubernetesLoginModel `tfsdk:"login"`
}

type awsKubernetesJson struct {
	Label        *string `json:"label"`
	Connectivity struct {
		PublicJwk *string `json:"publicJwk"`
	} `json:"connectivity"`
	Token struct {
		ClearText *string `json:"clearText"`
	} `json:"token"`

	Login *kubernetesLoginModel `json:"login"`
	State string                `json:"state"`
}

type awsKubernetesApi struct {
	Item *awsKubernetesJson `json:"item"`
}

func (r *AwsKubernetes) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_aws_kubernetes"
}

func (r *AwsKubernetes) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `An AWS EKS (Kubernetes) installation.
**Important**: This resource should be used together with the 'kubernetes_staged' resource, with a dependency chain
requiring this resource to be updated after the 'kubernetes_staged' resource.
`,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: `The EKS cluster name`,
			},
			"label": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: `The cluster's display label`,
			},
			"token": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: `The value of the p0-service-account-secret`,
			},
			"public_jwk": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: `The public JWK token of the braekhus service`,
			},
			"state": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: common.StateMarkdownDescription,
			},
			"login": schema.SingleNestedAttribute{
				Required:            true,
				MarkdownDescription: `How users authenticate to this Kubernetes cluster`,
				Attributes: map[string]schema.Attribute{
					"type": schema.StringAttribute{
						Required: true,
						MarkdownDescription: `The authentication method:
    - 'iam': Users authenticate via AWS IAM (typical for EKS)`,
						Validators: []validator.String{
							stringvalidator.OneOf("iam"),
						},
					},
				},
			},
		},
	}
}

func (r *AwsKubernetes) getId(data any) *string {
	model, ok := data.(*awsKubernetesModel)
	if !ok {
		return nil
	}
	return &model.Id
}

func (r *AwsKubernetes) getItemJson(json any) any {
	inner, ok := json.(*awsKubernetesApi)
	if !ok {
		return nil
	}
	return inner.Item
}

func (r *AwsKubernetes) fromJson(ctx context.Context, diags *diag.Diagnostics, id string, json any) any {
	data := awsKubernetesModel{}
	jsonv, ok := json.(*awsKubernetesJson)
	if !ok {
		return nil
	}

	data.Id = id

	data.Label = types.StringNull()
	if jsonv.Label != nil {
		data.Label = types.StringValue(*jsonv.Label)
	}

	data.Token = types.StringNull()
	if jsonv.Token.ClearText != nil {
		data.Token = types.StringValue(*jsonv.Token.ClearText)
	}

	data.PublicJwk = types.StringNull()
	if jsonv.Connectivity.PublicJwk != nil {
		data.PublicJwk = types.StringValue(*jsonv.Connectivity.PublicJwk)
	}

	data.State = types.StringValue(jsonv.State)
	data.Login = jsonv.Login

	return &data
}

func (r *AwsKubernetes) toJson(data any) any {
	json := awsKubernetesJson{}

	datav, ok := data.(*awsKubernetesModel)
	if !ok {
		return nil
	}

	if !datav.Label.IsNull() && !datav.Label.IsUnknown() {
		label := datav.Label.ValueString()
		json.Label = &label
	}

	if !datav.Token.IsNull() && !datav.Token.IsUnknown() {
		token := datav.Token.ValueString()
		json.Token.ClearText = &token
	}

	if !datav.PublicJwk.IsNull() && !datav.PublicJwk.IsUnknown() {
		publicJwk := datav.PublicJwk.ValueString()
		json.Connectivity.PublicJwk = &publicJwk
	}

	// can omit state here as it's filled by the backend
	json.Login = datav.Login

	return &json
}

func (r *AwsKubernetes) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	providerData := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:  K8s,
		Component:    installresources.Kubernetes,
		ProviderData: providerData,
		GetId:        r.getId,
		GetItemJson:  r.getItemJson,
		FromJson:     r.fromJson,
		ToJson:       r.toJson,
	}
}

func (r *AwsKubernetes) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json awsKubernetesApi
	var data awsKubernetesModel
	req.Config.Get(ctx, &data)
	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data)
}

func (r *AwsKubernetes) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var json awsKubernetesApi
	var data awsKubernetesModel
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &json, &data)
}

func (r *AwsKubernetes) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var json awsKubernetesApi
	var data awsKubernetesModel
	req.Config.Get(ctx, &data)
	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data)
}

func (r *AwsKubernetes) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data awsKubernetesModel
	r.installer.Rollback(ctx, &resp.Diagnostics, &resp.State, &data)
}

func (r *AwsKubernetes) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
