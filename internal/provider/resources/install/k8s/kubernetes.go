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
	Id                   string                `tfsdk:"id"`
	AccountId            basetypes.StringValue `tfsdk:"account_id"`
	Region               basetypes.StringValue `tfsdk:"region"`
	Label                basetypes.StringValue `tfsdk:"label"`
	ServiceAccountSecret basetypes.StringValue `tfsdk:"service_account_secret"`
	JWKPublicToken       basetypes.StringValue `tfsdk:"jwk_public_token"`
	State                basetypes.StringValue `tfsdk:"state"`
	Login                *kubernetesLoginModel `tfsdk:"login"`
}

type awsKubernetesJson struct {
	Label                *string               `json:"label"`
	State                string                `json:"state"`
	AccountId            *string               `json:"accountId"`
	Region               *string               `json:"region"`
	ServiceAccountSecret *string               `json:"serviceAccountSecret"`
	JWKPublicToken       *string               `json:"jwkPublicToken"`
	Login                *kubernetesLoginModel `json:"login"`
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

**Important**: This resource should be used together with the 'aws_kubernetes_staged' resource, with a dependency chain
requiring this resource to be updated after the 'aws_kubernetes_staged' resource.

P0 recommends you use these resources according to the following pattern:

` + "```" +
			`
resource "p0_aws_kubernetes_staged" "staged_cluster" {
  id         = "my-cluster"
  account_id = "123456789012"
  region     = "us-west-2"
}

resource "kubernetes_manifest" "p0_resources" {
  manifest = yamldecode(p0_aws_kubernetes_staged.staged_cluster.manifests.manifest)
}

resource "p0_aws_kubernetes" "installed_cluster" {
  id         = p0_aws_kubernetes_staged.staged_cluster.id
  account_id = p0_aws_kubernetes_staged.staged_cluster.account_id
  region     = p0_aws_kubernetes_staged.staged_cluster.region
  depends_on = [kubernetes_manifest.p0_resources]

  login {
    type = "iam"
  }
}
` + "```",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: `The EKS cluster name`,
			},
			"account_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: `The AWS account ID that owns the EKS cluster`,
				Validators: []validator.String{
					stringvalidator.RegexMatches(AwsAccountIdRegex, "AWS account IDs should consist of 12 numeric digits"),
				},
			},
			"region": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: `The AWS region where the EKS cluster is located`,
			},
			"service_account_secret": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: `The secret used by P0's service account`,
			},
			"label": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: `The cluster's display label`,
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

	data.AccountId = types.StringNull()
	if jsonv.AccountId != nil {
		data.AccountId = types.StringValue(*jsonv.AccountId)
	}

	data.Region = types.StringNull()
	if jsonv.Region != nil {
		data.Region = types.StringValue(*jsonv.Region)
	}

	data.ServiceAccountSecret = types.StringNull()
	if jsonv.ServiceAccountSecret != nil {
		data.ServiceAccountSecret = types.StringValue(*jsonv.ServiceAccountSecret)
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

	if !datav.AccountId.IsNull() && !datav.AccountId.IsUnknown() {
		accountId := datav.AccountId.ValueString()
		json.AccountId = &accountId
	}

	if !datav.Region.IsNull() && !datav.Region.IsUnknown() {
		region := datav.Region.ValueString()
		json.Region = &region
	}

	if !datav.ServiceAccountSecret.IsNull() && !datav.ServiceAccountSecret.IsUnknown() {
		saSecret := datav.ServiceAccountSecret.ValueString()
		json.ServiceAccountSecret = &saSecret
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
