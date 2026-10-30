package installredshiftai

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/p0-security/terraform-provider-p0/internal"
	"github.com/p0-security/terraform-provider-p0/internal/common"
)

var _ resource.Resource = &Cluster{}
var _ resource.ResourceWithConfigure = &Cluster{}
var _ resource.ResourceWithImportState = &Cluster{}
var _ resource.ResourceWithModifyPlan = &Cluster{}

type Cluster struct {
	installer *common.Install
}

func NewCluster() resource.Resource {
	return &Cluster{}
}

func (*Cluster) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_redshift_ai_cluster"
}

func (*Cluster) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attributes := clusterStagedAttributes()
	attributes["database"] = schema.StringAttribute{
		Required:            true,
		MarkdownDescription: "The database that the connector's admin statements connect to, such as `dev`. The connector finds the cluster's other databases from it. Changing it updates the cluster in place, and P0 checks the install again.",
		Validators:          []validator.String{databaseValidator()},
	}
	attributes["object_owner"] = schema.StringAttribute{
		Optional:            true,
		MarkdownDescription: "The database user that takes over each table and view an agent created once its access ends, such as `etl_owner` or `IAMR:etl-role`. When unset, the connector's own user takes them over. P0 checks that the user exists. Changing it updates the cluster in place, and P0 checks the install again.",
		Validators:          []validator.String{objectOwnerValidator()},
	}
	attributes["arn"] = generatedAttribute("The ARN of the provisioned cluster or Serverless workgroup, which P0 finds when it configures the cluster")

	resp.Schema = schema.Schema{
		MarkdownDescription: `A Redshift (AI) cluster installation: one provisioned cluster or Serverless workgroup.

**Important:** Before creating this resource you must stage the cluster with ` + "`p0_redshift_ai_cluster_staged`" + `, and set up the following:

- An inline policy on the connector's role that lets it sign in to the cluster, run statements through the Redshift Data API, and look the cluster up.
- The connector's database user, ` + "`IAMR:p0-connector-redshift-ai-aws`" + `, with the ` + "`sys:secadmin`" + ` role and a ` + "`p0_connector_admin`" + ` role that may alter tables and drop schemas, functions and procedures. A superuser creates them with the setup SQL that P0's installer shows.

Every apply that creates or changes this resource has P0 check the install: that the connector can find the cluster and sign in, that its user holds those roles, and that ` + "`object_owner`" + ` names an existing user. If a check fails, the apply fails with P0's message.

A cluster that P0 hasn't finished installing, such as one imported before its install check passed, plans an update, and applying it finishes the install.

` + betaNote + `

` + docsNote,
		Attributes: attributes,
	}
}

func (r *Cluster) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.installer = newInstaller[clusterJson](internal.Configure(&req, resp), clusterComponent, clusterFromJson, clusterToJson)
}

func clusterFromJson(_ context.Context, diags *diag.Diagnostics, id string, json any) any {
	item, ok := json.(*clusterJson)
	if !ok {
		return nil
	}
	model := &clusterModel{
		clusterStagedModel: newClusterStagedModel(diags, id, item),
		Database:           types.StringPointerValue(item.Database),
		ObjectOwner:        types.StringPointerValue(item.ObjectOwner),
		Arn:                types.StringPointerValue(item.Arn),
	}
	// clusterToJson clears an unset object owner with an empty string.
	if model.ObjectOwner.ValueString() == "" {
		model.ObjectOwner = types.StringNull()
	}
	return model
}

// P0 merges what it gets into the item, so an unset object owner is sent as an
// empty string, which clears it. P0 sets the ARN, so it is never sent.
func clusterToJson(data any) any {
	model, ok := data.(*clusterModel)
	if !ok {
		return nil
	}
	json := model.toJson()
	json.Database = model.Database.ValueStringPointer()
	objectOwner := model.ObjectOwner.ValueString()
	json.ObjectOwner = &objectOwner
	return json
}

func (r *Cluster) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	stageAndInstall(ctx, r.installer, &resp.Diagnostics, &req.Plan, &resp.State, &clusterApi{}, &clusterModel{})
}

func (r *Cluster) Read(ctx context.Context, _ resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &clusterApi{}, &clusterModel{})
}

func (*Cluster) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	planFinishingInstall(ctx, req, resp)
}

// Only database and object_owner change in place. P0 checks them on the
// configure step, which is all an installed cluster gets, and saves nothing if
// the check fails: the cluster stays installed with its current values, and
// the next plan shows the update again. A staged cluster is verified first,
// and a failed verify saves nothing either.
func (r *Cluster) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	updateFromInstallState(ctx, r.installer, req, resp, &clusterApi{}, &clusterModel{})
}

// Returns the cluster to the "stage" state, so that the staged resource
// deletes it.
func (r *Cluster) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Rollback(ctx, &resp.Diagnostics, &req.State, &clusterModel{})
}

func (r *Cluster) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importById(ctx, req, resp, clusterIdValidator())
}
