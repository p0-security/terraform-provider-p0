// Copyright (c) 2024 P0 Security, Inc
// SPDX-License-Identifier: MPL-2.0

package installk8s

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
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

type awsKubernetesModel struct {
	Id                   string                `tfsdk:"id"`
	Token                basetypes.StringValue `tfsdk:"token"`
	PublicJwk            basetypes.StringValue `tfsdk:"public_jwk"`
	ConnectivityType     basetypes.StringValue `tfsdk:"connectivity_type"`
	HostingType          basetypes.StringValue `tfsdk:"hosting_type"`
	ClusterArn           basetypes.StringValue `tfsdk:"cluster_arn"`
	ClusterEndpoint      basetypes.StringValue `tfsdk:"cluster_endpoint"`
	CertificateAuthority basetypes.StringValue `tfsdk:"certificate_authority"`
	State                basetypes.StringValue `tfsdk:"state"`
}

type awsKubernetesApi struct {
	Connectivity struct {
		ConnectivityType *string `json:"type"`
		PublicJwk        *string `json:"publicJwk"`
	} `json:"connectivity"`
	Token struct {
		ClearText *string `json:"clearText"`
	} `json:"token"`
	Hosting struct {
		HostingType *string `json:"type"`
		ClusterArn  *string `json:"arn"`
	} `json:"hosting"`
	ClusterEndpoint      *string `json:"endpoint"`
	CertificateAuthority *string `json:"ca"`
	State                string  `json:"state"`
}

func (r *AwsKubernetes) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_kubernetes"
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
			"token": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: `The value of the p0-service-account-secret`,
			},
			"public_jwk": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: `The public JWK token of the braekhus service`,
			},
			"connectivity_type": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: `The connectivity type for the cluster (e.g., 'public', 'proxy')`,
			},
			"hosting_type": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: `The hosting type for the cluster (e.g., 'eks')`,
			},
			"cluster_arn": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: `The ARN of the EKS cluster`,
			},
			"cluster_endpoint": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: `The EKS API server endpoint for the cluster`,
			},
			"certificate_authority": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: `The base-64 encoded certificate authority for the cluster`,
			},
			"state": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: common.StateMarkdownDescription,
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
	return inner
}

func (r *AwsKubernetes) fromJson(ctx context.Context, diags *diag.Diagnostics, id string, json any) any {
	data := awsKubernetesModel{}
	jsonv, ok := json.(*awsKubernetesApi)
	if !ok {
		return nil
	}

	data.Id = id

	data.Token = types.StringNull()
	if jsonv.Token.ClearText != nil {
		data.Token = types.StringValue(*jsonv.Token.ClearText)
	}

	data.PublicJwk = types.StringNull()
	if jsonv.Connectivity.PublicJwk != nil {
		data.PublicJwk = types.StringValue(*jsonv.Connectivity.PublicJwk)
	}

	data.ConnectivityType = types.StringNull()
	if jsonv.Connectivity.ConnectivityType != nil {
		data.ConnectivityType = types.StringValue(*jsonv.Connectivity.ConnectivityType)
	}

	data.HostingType = types.StringNull()
	if jsonv.Hosting.HostingType != nil {
		data.HostingType = types.StringValue(*jsonv.Hosting.HostingType)
	}

	data.ClusterArn = types.StringNull()
	if jsonv.Hosting.ClusterArn != nil {
		data.ClusterArn = types.StringValue(*jsonv.Hosting.ClusterArn)
	}

	data.ClusterEndpoint = types.StringNull()
	if jsonv.ClusterEndpoint != nil {
		data.ClusterEndpoint = types.StringValue(*jsonv.ClusterEndpoint)
	}

	data.CertificateAuthority = types.StringNull()
	if jsonv.CertificateAuthority != nil {
		data.CertificateAuthority = types.StringValue(*jsonv.CertificateAuthority)
	}

	data.State = types.StringValue(jsonv.State)

	return &data
}

func (r *AwsKubernetes) toJson(data any) any {
	json := awsKubernetesApi{}

	datav, ok := data.(*awsKubernetesModel)
	if !ok {
		return nil
	}

	if !datav.Token.IsNull() && !datav.Token.IsUnknown() {
		token := datav.Token.ValueString()
		json.Token.ClearText = &token
	}

	if !datav.PublicJwk.IsNull() && !datav.PublicJwk.IsUnknown() {
		publicJwk := datav.PublicJwk.ValueString()
		json.Connectivity.PublicJwk = &publicJwk
	}

	if !datav.ConnectivityType.IsNull() && !datav.ConnectivityType.IsUnknown() {
		connectivityType := datav.ConnectivityType.ValueString()
		json.Connectivity.ConnectivityType = &connectivityType
	}

	if !datav.HostingType.IsNull() && !datav.HostingType.IsUnknown() {
		hostingType := datav.HostingType.ValueString()
		json.Hosting.HostingType = &hostingType
	}

	if !datav.ClusterArn.IsNull() && !datav.ClusterArn.IsUnknown() {
		clusterArn := datav.ClusterArn.ValueString()
		json.Hosting.ClusterArn = &clusterArn
	}

	if !datav.ClusterEndpoint.IsNull() && !datav.ClusterEndpoint.IsUnknown() {
		clusterEndpoint := datav.ClusterEndpoint.ValueString()
		json.ClusterEndpoint = &clusterEndpoint
	}

	if !datav.CertificateAuthority.IsNull() && !datav.CertificateAuthority.IsUnknown() {
		certificateAuthority := datav.CertificateAuthority.ValueString()
		json.CertificateAuthority = &certificateAuthority
	}

	// can omit state here as it's filled by the backend

	return &json
}

func (r *AwsKubernetes) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
