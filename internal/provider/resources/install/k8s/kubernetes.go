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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
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

type awsKubernetesItemStruct struct {
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

type awsKubernetesApi struct {
	Item awsKubernetesItemStruct `json:"item"`
}

func (r *AwsKubernetes) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_kubernetes"
}

func (r *AwsKubernetes) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A fully installed K8s integration. This resource provides final configuration values for the installation and verifies integration setup. 

**Disclaimer**: This resource currently only supports installation against an AWS EKS cluster. Support for Azure, GCP, and self-hosted clusters will
be added in a future release.

**Important**: This resource only completes the final steps of the installation process, and assumes that a corresponding 'p0_kubernetes_staged' resource has already
been provisioned. Before using this resource, please read the instructions for the 'p0_kubernetes_staged' resource.`,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: `The display name of the cluster`,
			},
			"token": schema.StringAttribute{
				Required:            true,
				Sensitive:           true,
				MarkdownDescription: `The value of the p0-service-account-secret`,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"public_jwk": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: `The public JWK token of the braekhus proxy service`,
			},
			"connectivity_type": schema.StringAttribute{
				Optional: true,
				Computed: true,
				Default:  stringdefault.StaticString("proxy"),
				MarkdownDescription: `One of:
	- 'proxy' (default): The integration will connect to the cluster via P0's proxy service. 
	- 'public': The integration will connect to the cluster via the public internet`,
				Validators: []validator.String{
					stringvalidator.OneOf("public", "proxy"),
				},
			},
			"hosting_type": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("aws"),
				MarkdownDescription: `The hosting type for the cluster`,
			},
			"cluster_arn": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: `The ARN of the cluster`,
			},
			"cluster_endpoint": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: `The server API endpoint of the cluster`,
			},
			"certificate_authority": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: `The base-64 encoded certificate authority of the cluster`,
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
	return &inner.Item
}

func (r *AwsKubernetes) fromJson(ctx context.Context, diags *diag.Diagnostics, id string, json any) any {
	data := awsKubernetesModel{}

	// json is actually a pointer to the Item field from awsKubernetesApi
	jsonv, ok := json.(*awsKubernetesItemStruct)
	if !ok {
		return nil
	}

	data.Id = id

	// Token is write-only and not returned by the API - don't read it from the response
	// Terraform will use the value from the plan/config instead

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
	// Request format: fields at top level (no 'item' wrapper)
	type awsKubernetesRequest struct {
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
	}

	json := awsKubernetesRequest{}

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

	// Save the token from the plan since it's write-only and won't be in the API response
	plannedToken := data.Token

	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data)

	if resp.Diagnostics.HasError() {
		return
	}

	// Restore the token value from the plan
	var stateData awsKubernetesModel
	resp.State.Get(ctx, &stateData)
	stateData.Token = plannedToken
	resp.State.Set(ctx, &stateData)
}

func (r *AwsKubernetes) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var json awsKubernetesApi
	var data awsKubernetesModel

	// Save the token from current state since it's write-only and won't be in the API response
	var currentState awsKubernetesModel
	req.State.Get(ctx, &currentState)
	currentToken := currentState.Token

	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &json, &data)

	if resp.Diagnostics.HasError() {
		return
	}

	// Restore the token value from the previous state
	var stateData awsKubernetesModel
	resp.State.Get(ctx, &stateData)
	stateData.Token = currentToken
	resp.State.Set(ctx, &stateData)
}

func (r *AwsKubernetes) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var json awsKubernetesApi
	var data awsKubernetesModel
	req.Config.Get(ctx, &data)

	// Save the token from the plan since it's write-only and won't be in the API response
	plannedToken := data.Token

	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data)

	if resp.Diagnostics.HasError() {
		return
	}

	// Restore the token value from the plan
	var stateData awsKubernetesModel
	resp.State.Get(ctx, &stateData)
	stateData.Token = plannedToken
	resp.State.Set(ctx, &stateData)
}

func (r *AwsKubernetes) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data awsKubernetesModel
	r.installer.Rollback(ctx, &resp.Diagnostics, &resp.State, &data)
}

func (r *AwsKubernetes) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
