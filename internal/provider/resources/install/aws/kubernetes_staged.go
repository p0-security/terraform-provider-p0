// Copyright (c) 2024 P0 Security, Inc
// SPDX-License-Identifier: MPL-2.0

package installaws

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
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
		Label        *string       `json:"label"`
		State        *string       `json:"state"`
		Namespace    *string       `json:"namespace"`
		Region       *string       `json:"region"`
		AccountId    *string       `json:"accountId"`
		AwsPartition *AwsPartition `json:"awsPartition"`
	} `json:"item"`
	Metadata struct {
		ServiceAccountId string `json:"serviceAccountId"`
		Manifest         string `json:"manifest"`
		Namespace        string `json:"namespace"`
	} `json:"metadata"`
}

type awsKubernetesStagedModel struct {
	Id               string       `tfsdk:"id"`
	AccountId        types.String `tfsdk:"account_id"`
	Partition        types.String `tfsdk:"partition"`
	Region           types.String `tfsdk:"region"`
	Namespace        types.String `tfsdk:"namespace"`
	Label            types.String `tfsdk:"label"`
	ServiceAccountId types.String `tfsdk:"service_account_id"`
	Manifests        types.Object `tfsdk:"manifests"`
}

func (r *AwsKubernetesStaged) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_aws_kubernetes_staged"
}

func (r *AwsKubernetesStaged) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A staged AWS EKS (Kubernetes) installation. Staged resources are used to generate Kubernetes manifests.

**Important** Before using this resource, please read the instructions for the 'aws_kubernetes' resource.
`,
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
			"partition": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("aws"),
				MarkdownDescription: `The AWS partition (aws or aws-us-gov). Defaults to aws if not specified.`,
				Validators: []validator.String{
					stringvalidator.RegexMatches(AwsPartitionRegex, "AWS partition must be one of: aws, aws-us-gov."),
				},
			},
			"region": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: `The AWS region where the EKS cluster is located`,
			},
			"namespace": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("p0-security"),
				MarkdownDescription: `The Kubernetes namespace where P0 resources will be deployed. Defaults to p0-security if not specified.`,
			},
			"label": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: `The cluster's display label`,
			},
			"service_account_id": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("p0-service-account"),
				MarkdownDescription: `The audience ID of the service account to include in this cluster's P0 service account configuration`,
			},
			"manifests": schema.SingleNestedAttribute{
				Computed:            true,
				MarkdownDescription: `Kubernetes manifests that should be applied to the cluster`,
				Attributes: map[string]schema.Attribute{
					"manifest": schema.StringAttribute{
						Computed:            true,
						MarkdownDescription: `The combined Kubernetes YAML manifest that should be applied to the cluster`,
					},
					"namespace": schema.StringAttribute{
						Computed:            true,
						MarkdownDescription: `The namespace where the manifest should be applied`,
					},
				},
			},
		},
	}
}

func (r *AwsKubernetesStaged) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	providerData := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:  Aws,
		Component:    installresources.Kubernetes,
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
	if jsonv.Item.Label != nil {
		data.Label = types.StringValue(*jsonv.Item.Label)
	}

	if jsonv.Item.AccountId != nil {
		data.AccountId = types.StringValue(*jsonv.Item.AccountId)
	}

	if jsonv.Item.AwsPartition != nil && jsonv.Item.AwsPartition.Type != nil {
		data.Partition = types.StringValue(*jsonv.Item.AwsPartition.Type)
	}

	if jsonv.Item.Region != nil {
		data.Region = types.StringValue(*jsonv.Item.Region)
	}

	if jsonv.Item.Namespace != nil {
		data.Namespace = types.StringValue(*jsonv.Item.Namespace)
	}

	data.ServiceAccountId = types.StringValue(jsonv.Metadata.ServiceAccountId)

	manifests, objErr := types.ObjectValue(
		map[string]attr.Type{
			"manifest":  types.StringType,
			"namespace": types.StringType,
		},
		map[string]attr.Value{
			"manifest":  types.StringValue(jsonv.Metadata.Manifest),
			"namespace": types.StringValue(jsonv.Metadata.Namespace),
		},
	)
	if objErr.HasError() {
		diags.Append(objErr...)
		return nil
	}
	data.Manifests = manifests

	return &data
}

func (r *AwsKubernetesStaged) toJson(data any) any {
	json := awsKubernetesStagedApi{}

	datav, ok := data.(*awsKubernetesStagedModel)
	if !ok {
		return nil
	}

	if !datav.Label.IsNull() && !datav.Label.IsUnknown() {
		label := datav.Label.ValueString()
		json.Item.Label = &label
	}

	if !datav.AccountId.IsNull() && !datav.AccountId.IsUnknown() {
		accountId := datav.AccountId.ValueString()
		json.Item.AccountId = &accountId
	}

	if !datav.Partition.IsNull() && !datav.Partition.IsUnknown() {
		partition := datav.Partition.ValueString()
		json.Item.AwsPartition = &AwsPartition{Type: &partition}
	}

	if !datav.Region.IsNull() && !datav.Region.IsUnknown() {
		region := datav.Region.ValueString()
		json.Item.Region = &region
	}

	if !datav.Namespace.IsNull() && !datav.Namespace.IsUnknown() {
		namespace := datav.Namespace.ValueString()
		json.Item.Namespace = &namespace
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
