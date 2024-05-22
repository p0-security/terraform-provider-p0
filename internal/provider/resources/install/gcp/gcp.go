package installgcp

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
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

type gcpRoleMetadata struct {
	Id   string `json:"id" tfsdk:"id"`
	Name string `json:"name" tfsdk:"name"`
}

type gcpAccessLogsMetadata struct {
	Logging struct {
		Filter string `json:"filter" tfsdk:"filter"`
		SinkId string `json:"sinkId" tfsdk:"sink_id"`
		Role   string `json:"role" tfsdk:"role"`
	} `json:"logging" tfsdk:"logging"`
	PredefinedRole string `json:"predefinedRole" tfsdk:"predefined_role"`
	PubSub         struct {
		TopicId string `json:"topicId" tfsdk:"topic_id"`
	} `json:"pubSub" tfsdk:"pub_sub"`
}

type gcpPermissionsMetadata struct {
	Permissions []string        `json:"requiredPermissions" tfsdk:"permissions"`
	Role        gcpRoleMetadata `json:"role" tfsdk:"custom_role"`
}

type gcpPermissionsMetadataWithPredefinedRole struct {
	PredefinedRole string          `json:"predefinedRole" tfsdk:"predefined_role"`
	Permissions    []string        `json:"requiredPermissions" tfsdk:"permissions"`
	CustomRole     gcpRoleMetadata `json:"role" tfsdk:"custom_role"`
}

type gcpModel struct {
	OrganizationId      basetypes.StringValue                     `tfsdk:"organization_id"`
	ServiceAccountEmail basetypes.StringValue                     `tfsdk:"service_account_email"`
	AccessLogs          *gcpAccessLogsMetadata                    `tfsdk:"access_logs"`
	IamAssessment       *gcpPermissionsMetadata                   `tfsdk:"iam_assessment"`
	IamWrite            *gcpPermissionsMetadataWithPredefinedRole `tfsdk:"iam_write"`
	OrgWidePolicy       *gcpPermissionsMetadata                   `tfsdk:"org_wide_policy"`
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
		AccessLogs    gcpAccessLogsMetadata                    `json:"access-logs"`
		IamAssessment gcpPermissionsMetadata                   `json:"iam-assessment"`
		IamWrite      gcpPermissionsMetadataWithPredefinedRole `json:"iam-write"`
		OrgWidePolicy gcpPermissionsMetadata                   `json:"org-wide-policy"`
	} `json:"metadata"`
}

func (r *Gcp) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_gcp"
}

func (r *Gcp) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	customRole := schema.SingleNestedAttribute{
		Computed:            true,
		MarkdownDescription: `Describes the custom role that should be created and assigned to P0's service account`,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: `The custom role expected identifier`,
			},
			"name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: `The custom role's expected title`,
			},
		},
	}
	predefinedRole := schema.StringAttribute{
		Computed:            true,
		MarkdownDescription: `The predefined role that should be granted to P0, in order to install projects for IAM management`,
	}
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
			"iam_assessment": schema.SingleNestedAttribute{
				Computed:            true,
				MarkdownDescription: `Read-only attributes used to configure IAM grants for IAM-assessment integrations`,
				Attributes: map[string]schema.Attribute{
					"permissions": schema.ListAttribute{
						ElementType: types.StringType,
						Computed:    true,
						MarkdownDescription: `Permissions that should be granted to P0 via the custom role, described in the 'role' attribute,
in order to install projects for IAM assessment`,
					},
					"custom_role": customRole,
				},
			},
			"iam_write": schema.SingleNestedAttribute{
				Computed:            true,
				MarkdownDescription: `Read-only attributes used to configure IAM grants for IAM-management integrations`,
				Attributes: map[string]schema.Attribute{
					"permissions": schema.ListAttribute{
						ElementType:         types.StringType,
						Computed:            true,
						MarkdownDescription: `Permissions that should be granted to P0 via the custom role, described in the 'role' attribute, in order to install projects for IAM management`,
					},
					"predefined_role": predefinedRole,
					"custom_role":     customRole,
				},
			},
			"org_wide_policy": schema.SingleNestedAttribute{
				Computed:            true,
				MarkdownDescription: `Read-only attributes used to configure IAM grants for org-wide policy-read installation`,
				Attributes: map[string]schema.Attribute{
					"permissions": schema.ListAttribute{
						ElementType: types.StringType,
						Computed:    true,
						MarkdownDescription: `Permissions that should be granted to P0 via the custom role, described in the 'role' attribute,
in order to install projects for org-wide policy-read installation`,
					},
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

func (r *Gcp) fromJson(json any) any {
	data := gcpModel{}

	jsonv, ok := json.(*gcpApi)
	if !ok {
		return nil
	}

	root := jsonv.Config.Root.Singleton

	data.OrganizationId = types.StringValue(root.OrganizationId)
	data.ServiceAccountEmail = types.StringPointerValue(root.ServiceAccountEmail)

	metadata := jsonv.Metadata

	data.AccessLogs = &metadata.AccessLogs
	data.IamAssessment = &metadata.IamAssessment
	data.IamWrite = &metadata.IamWrite
	data.OrgWidePolicy = &metadata.OrgWidePolicy

	return &data
}

func (r *Gcp) toJson(data any) any {
	json := gcpApi{}

	datav, ok := data.(*gcpModel)
	if !ok {
		return nil
	}

	json.Config.Root.Singleton.OrganizationId = datav.OrganizationId.ValueString()
	json.Config.Root.Singleton.ServiceAccountEmail = datav.OrganizationId.ValueStringPointer()

	return &json
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
