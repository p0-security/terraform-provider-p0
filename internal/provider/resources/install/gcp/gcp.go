package installgcp

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/p0-security/terraform-provider-p0/internal"
	installresources "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &Gcp{}
var _ resource.ResourceWithImportState = &Gcp{}
var _ resource.ResourceWithConfigure = &Gcp{}

func NewGcp() resource.Resource {
	return &Gcp{}
}

type Gcp struct {
	installer *installresources.RootInstall
}

type gcpModel struct {
	OrganizationId      types.String `tfsdk:"organization_id"`
	ServiceAccountEmail types.String `tfsdk:"service_account_email"`
	AccessLogs          types.Object `tfsdk:"access_logs"`
	OrgWidePolicy       types.Object `tfsdk:"org_wide_policy"`
}

type gcpAccessLogsLoggingMetadata struct {
	Filter string `json:"filter" tfsdk:"filter"`
	SinkId string `json:"sinkId" tfsdk:"sink_id"`
	Role   string `json:"role" tfsdk:"role"`
}

type gcpAccessLogsPubSubMetadata struct {
	TopicId string `json:"topicId" tfsdk:"topic_id"`
}

type gcpAccessLogsMetadata struct {
	Logging        gcpAccessLogsLoggingMetadata `json:"logging" tfsdk:"logging"`
	PredefinedRole string                       `json:"predefinedRole" tfsdk:"predefined_role"`
	PubSub         gcpAccessLogsPubSubMetadata  `json:"pubSub" tfsdk:"pub_sub"`
}

type gcpApi struct {
	Config struct {
		Root struct {
			Singleton struct {
				OrganizationId      string  `json:"organizationId"`
				ServiceAccountEmail *string `json:"serviceAccountEmail"`
			} `json:"_"`
		} `json:"root"`
	} `json:"config"`
	Metadata struct {
		AccessLogs    gcpAccessLogsMetadata  `json:"access-logs"`
		OrgWidePolicy gcpPermissionsMetadata `json:"org-wide-policy"`
	} `json:"metadata"`
}

func (r *Gcp) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_gcp"
}

func (r *Gcp) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A Google Cloud installation.`,
		Attributes: map[string]schema.Attribute{
			"organization_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: `The Google Cloud organization ID`,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.RegexMatches(GcpOrganizationIdRegex, "GCP organization IDs should be numeric"),
				},
			},
			"service_account_email": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: `The identity that P0 uses to communicate with your Google Cloud organization`,
			},
			"access_logs": schema.SingleNestedAttribute{
				Computed:            true,
				MarkdownDescription: `Read-only attributes used to configure infrastructure and IAM grants for access-logs integrations`,
				Attributes: map[string]schema.Attribute{
					"logging": schema.SingleNestedAttribute{
						Computed:            true,
						MarkdownDescription: `Describes expected Cloud Logging infrastructure`,
						Attributes: map[string]schema.Attribute{
							"filter": schema.StringAttribute{
								Computed:            true,
								MarkdownDescription: `Logs should be directed to a logging sink with this filter`,
							},
							"role": schema.StringAttribute{
								Computed:            true,
								MarkdownDescription: `The project's logging service account should have this predefined role`,
							},
							"sink_id": schema.StringAttribute{
								Computed:            true,
								MarkdownDescription: `Logs should be directed to a logging sink with this ID`,
							},
						},
					},
					"predefined_role": predefinedRole,
					"pub_sub": schema.SingleNestedAttribute{
						Computed:            true,
						MarkdownDescription: `Describes expected Pub/Sub infrastructure`,
						Attributes: map[string]schema.Attribute{
							"topic_id": schema.StringAttribute{
								Computed:            true,
								MarkdownDescription: `Logs should be directed to a Pub/Sub topic with this ID`,
							},
						},
					},
				},
			},
			"org_wide_policy": schema.SingleNestedAttribute{
				Computed:            true,
				MarkdownDescription: `Read-only attributes used to configure IAM grants for org-wide policy-read installation`,
				Attributes: map[string]schema.Attribute{
					"permissions": permissions("org-wide policy-read installation"),
					"custom_role": customRole,
				},
			},
		},
	}
}

func (r *Gcp) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	providerData := internal.Configure(&req, resp)
	r.installer = &installresources.RootInstall{
		Integration:  GcpKey,
		ProviderData: providerData,
		FromJson:     r.fromJson,
		ToJson:       r.toJson,
	}
}

func (r *Gcp) fromJson(ctx context.Context, diags *diag.Diagnostics, json any) any {
	data := gcpModel{}

	jsonv, ok := json.(*gcpApi)
	if !ok {
		return nil
	}

	root := jsonv.Config.Root.Singleton

	data.OrganizationId = types.StringValue(root.OrganizationId)
	data.ServiceAccountEmail = types.StringPointerValue(root.ServiceAccountEmail)

	metadata := jsonv.Metadata

	accessLogs, alDiags := types.ObjectValueFrom(ctx, map[string]attr.Type{
		"logging": types.ObjectType{
			AttrTypes: map[string]attr.Type{"filter": types.StringType, "role": types.StringType, "sink_id": types.StringType},
		},
		"predefined_role": types.StringType,
		"pub_sub": types.ObjectType{
			AttrTypes: map[string]attr.Type{"topic_id": types.StringType},
		},
	}, metadata.AccessLogs)
	if alDiags.HasError() {
		diags.Append(alDiags...)
		return nil
	}
	data.AccessLogs = accessLogs

	orgWidePolicy, owDiags := types.ObjectValueFrom(ctx, map[string]attr.Type{
		"custom_role": types.ObjectType{
			AttrTypes: map[string]attr.Type{
				"id":   types.StringType,
				"name": types.StringType,
			},
		},
		"permissions": types.ListType{
			ElemType: types.StringType,
		},
	}, metadata.OrgWidePolicy)
	if owDiags.HasError() {
		diags.Append(owDiags...)
		return nil
	}
	data.OrgWidePolicy = orgWidePolicy

	return &data
}

func (r *Gcp) toJson(data any) any {
	json := gcpApi{}

	datav, ok := data.(*gcpModel)
	if !ok {
		return nil
	}

	json.Config.Root.Singleton.OrganizationId = datav.OrganizationId.ValueString()

	return &json.Config
}

func (r *Gcp) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json gcpApi
	var data gcpModel
	r.installer.Create(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data)
}

func (r *Gcp) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var json gcpApi
	var data gcpModel
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &json, &data)
}

func (r *Gcp) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError("Not Updateable", "Modifying P0's GCP integration forces replacement")
}

func (r *Gcp) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data gcpModel
	r.installer.Delete(ctx, &resp.Diagnostics, &resp.State, &data)
}

func (r *Gcp) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("organization_id"), req, resp)
}
