package installredshiftai

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/p0-security/terraform-provider-p0/internal"
	"github.com/p0-security/terraform-provider-p0/internal/common"
)

var _ resource.Resource = &ClusterStaged{}
var _ resource.ResourceWithConfigure = &ClusterStaged{}
var _ resource.ResourceWithImportState = &ClusterStaged{}

type ClusterStaged struct {
	installer *common.Install
}

func NewClusterStaged() resource.Resource {
	return &ClusterStaged{}
}

func (*ClusterStaged) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_redshift_ai_cluster_staged"
}

func (*ClusterStaged) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A staged Redshift (AI) cluster: one provisioned cluster or Serverless workgroup that the connector in its AWS account manages.

Staging records the cluster in P0 and checks that its AWS account can take the IAM role P0 creates for each agent's request. Then let the connector's role into the cluster, have a superuser run the setup SQL that creates the connector's database user, and create a ` + "`p0_redshift_ai_cluster`" + ` resource with the same ` + "`id`" + ` to complete the installation.

**Prerequisites:** The connector in the cluster's AWS account must be installed (see the ` + "`p0_redshift_ai_connector`" + ` resource), and so must P0's AWS IAM management (see ` + "`p0_aws_iam_write`" + `) and AWS OIDC for that account.

` + betaNote + `

` + docsNote,
		Attributes: clusterStagedAttributes(),
	}
}

func (r *ClusterStaged) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.installer = newInstaller[clusterJson](internal.Configure(&req, resp), clusterComponent, clusterStagedFromJson, clusterStagedToJson)
}

func clusterStagedFromJson(_ context.Context, diags *diag.Diagnostics, id string, json any) any {
	item, ok := json.(*clusterJson)
	if !ok {
		return nil
	}
	model := newClusterStagedModel(diags, id, item)
	return &model
}

func clusterStagedToJson(data any) any {
	model, ok := data.(*clusterStagedModel)
	if !ok {
		return nil
	}
	return model.toJson()
}

func (r *ClusterStaged) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	ensureConfigAndStage(ctx, r.installer, &resp.Diagnostics, &req.Plan, &resp.State, &clusterApi{}, &clusterStagedModel{})
}

func (r *ClusterStaged) Read(ctx context.Context, _ resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &clusterApi{}, &clusterStagedModel{})
}

// Every attribute that can be set requires replacement, so Terraform never
// updates a staged cluster in place. Staging it again would also return an
// installed cluster to the "stage" state.
func (r *ClusterStaged) Update(context.Context, resource.UpdateRequest, *resource.UpdateResponse) {
}

func (r *ClusterStaged) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Delete(ctx, &resp.Diagnostics, &req.State, &clusterStagedModel{})
}

func (r *ClusterStaged) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importById(ctx, req, resp, clusterIdValidator())
}
