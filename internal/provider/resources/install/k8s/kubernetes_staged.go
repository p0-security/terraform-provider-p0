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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/p0-security/terraform-provider-p0/internal"
	"github.com/p0-security/terraform-provider-p0/internal/common"
	installresources "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &AwsKubernetesStaged{}
var _ resource.ResourceWithImportState = &AwsKubernetesStaged{}
var _ resource.ResourceWithConfigure = &AwsKubernetesStaged{}

func NewKubernetesStagedAws() resource.Resource {
	return &AwsKubernetesStaged{}
}

type AwsKubernetesStaged struct {
	installer *common.Install
}

type awsKubernetesStagedApi struct {
	Item struct {
		Connectivity struct {
			ConnectivityType *string `json:"type"`
		} `json:"connectivity"`
		Hosting struct {
			HostingType *string `json:"type"`
			ClusterArn  *string `json:"arn"`
		} `json:"hosting"`
		ClusterEndpoint      *string `json:"endpoint"`
		CertificateAuthority *string `json:"ca"`
	} `json:"item"`
	Metadata struct {
		CaBundle   *string `json:"caBundle"`
		ServerCert *string `json:"serverCert"`
		ServerKey  *string `json:"serverKey"`
	} `json:"metadata"`
}

type awsKubernetesStagedModel struct {
	Id                   string       `tfsdk:"id"`
	ConnectivityType     string       `tfsdk:"connectivity_type"`
	HostingType          string       `tfsdk:"hosting_type"`
	ClusterArn           string       `tfsdk:"cluster_arn"`
	ClusterEndpoint      types.String `tfsdk:"cluster_endpoint"`
	CertificateAuthority types.String `tfsdk:"certificate_authority"`
	CaBundle             types.String `tfsdk:"ca_bundle"`
	ServerCert           types.String `tfsdk:"server_cert"`
	ServerKey            types.String `tfsdk:"server_key"`
}

func (r *AwsKubernetesStaged) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_eks_kubernetes_staged"
}

func (r *AwsKubernetesStaged) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A staged K8s integration. Staged resources are used to generate configurations and PKI values.
		
**Important**: This resource only initiates the installation process for a k8s integration. It is intended to be used in conjunction with the 
'p0_eks_kubernetes' resource, which completes the final steps of the installation. Before using this resource, please read the instructions 
for the 'p0_eks_kubernetes' resource.`,

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: `The display name of the EKS cluster.`,
			},
			"connectivity_type": schema.StringAttribute{
				Optional: true,
				Computed: true,
				Default:  stringdefault.StaticString("proxy"),
				MarkdownDescription: `One of:
				- 'proxy' (default): The integration will connect to the cluster via P0's proxy service. 
				- 'public': The integration will connect to the cluster via the public internet.`,
				Validators: []validator.String{
					stringvalidator.OneOf("public", "proxy"),
				},
			},
			"hosting_type": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("eks"),
				MarkdownDescription: `The hosting type for the cluster (e.g. 'eks').`,
			},
			"cluster_arn": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: `The ARN of the cluster`,
			},
			"cluster_endpoint": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: `The Server API endpoint of the cluster`,
			},
			"certificate_authority": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: `The base-64 encoded Certificate Authority of the cluster`,
			},
			"ca_bundle": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: `The certificate authority bundle to be used by the integration; used by the p0_eks_kubernetes resource.`,
			},
			"server_cert": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: `The certficate to be used by the integration; used by the p0_eks_kubernetes resource.`,
			},
			"server_key": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: `The private key to be used by the integration; used by the p0_eks_kubernetes resource.`,
			},
		},
	}
}

func (r *AwsKubernetesStaged) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	providerData := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:  K8s,
		Component:    installresources.IamWrite,
		ProviderData: providerData,
		GetId:        r.getId,
		GetItemJson:  r.getItemJson,
		FromJson:     r.fromJson,
		ToJson:       r.toJson,
	}
}

func (r *AwsKubernetesStaged) getId(data any) *string {
	model, ok := data.(*awsKubernetesStagedModel)
	if !ok {
		return nil
	}
	return &model.Id
}

func (r *AwsKubernetesStaged) getItemJson(json any) any {
	inner, ok := json.(*awsKubernetesStagedApi)
	if !ok {
		return nil
	}
	return inner
}

func (r *AwsKubernetesStaged) fromJson(ctx context.Context, diags *diag.Diagnostics, id string, json any) any {
	data := awsKubernetesStagedModel{}

	jsonv, ok := json.(*awsKubernetesStagedApi)
	if !ok {
		return nil
	}

	data.Id = id

	if jsonv.Item.Connectivity.ConnectivityType != nil {
		data.ConnectivityType = *jsonv.Item.Connectivity.ConnectivityType
	}

	if jsonv.Item.Hosting.HostingType != nil {
		data.HostingType = *jsonv.Item.Hosting.HostingType
	}

	if jsonv.Item.Hosting.ClusterArn != nil {
		data.ClusterArn = *jsonv.Item.Hosting.ClusterArn
	}

	if jsonv.Item.ClusterEndpoint != nil {
		data.ClusterEndpoint = types.StringValue(*jsonv.Item.ClusterEndpoint)
	}

	if jsonv.Item.CertificateAuthority != nil {
		data.CertificateAuthority = types.StringValue(*jsonv.Item.CertificateAuthority)
	}

	if jsonv.Metadata.CaBundle != nil {
		data.CaBundle = types.StringValue(*jsonv.Metadata.CaBundle)
	}

	if jsonv.Metadata.ServerCert != nil {
		data.ServerCert = types.StringValue(*jsonv.Metadata.ServerCert)
	}

	if jsonv.Metadata.ServerKey != nil {
		data.ServerKey = types.StringValue(*jsonv.Metadata.ServerKey)
	}

	return &data
}

func (r *AwsKubernetesStaged) toJson(data any) any {
	json := awsKubernetesStagedApi{}

	datav, ok := data.(*awsKubernetesStagedModel)
	if !ok {
		return nil
	}

	json.Item.Connectivity.ConnectivityType = &datav.ConnectivityType
	json.Item.Hosting.HostingType = &datav.HostingType
	json.Item.Hosting.ClusterArn = &datav.ClusterArn

	if !datav.ClusterEndpoint.IsNull() && !datav.ClusterEndpoint.IsUnknown() {
		clusterEndpoint := datav.ClusterEndpoint.ValueString()
		json.Item.ClusterEndpoint = &clusterEndpoint
	}

	if !datav.CertificateAuthority.IsNull() && !datav.CertificateAuthority.IsUnknown() {
		certificateAuthority := datav.CertificateAuthority.ValueString()
		json.Item.CertificateAuthority = &certificateAuthority
	}

	return &json
}

func (r *AwsKubernetesStaged) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json awsKubernetesStagedApi
	var data awsKubernetesStagedModel

	var inputData awsKubernetesStagedModel
	req.Config.Get(ctx, &inputData)
	inputJson, ok := r.toJson(&inputData).(*awsKubernetesStagedApi)
	if !ok {
		return
	}

	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, inputJson.Item)
}

func (r *AwsKubernetesStaged) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var json awsKubernetesStagedApi
	var data awsKubernetesStagedModel
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &json, &data)
}

func (r *AwsKubernetesStaged) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var json awsKubernetesStagedApi
	var data awsKubernetesStagedModel

	var inputData awsKubernetesStagedModel
	req.Config.Get(ctx, &inputData)
	inputJson, ok := r.toJson(&inputData).(*awsKubernetesStagedApi)
	if !ok {
		return
	}

	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, inputJson.Item)
}

func (r *AwsKubernetesStaged) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data awsKubernetesStagedModel
	r.installer.Delete(ctx, &resp.Diagnostics, &resp.State, &data)
}

func (r *AwsKubernetesStaged) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
