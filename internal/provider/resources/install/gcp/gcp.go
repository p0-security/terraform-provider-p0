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
	"github.com/p0-security/terraform-provider-p0/internal/common"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &Gcp{}
var _ resource.ResourceWithImportState = &Gcp{}
var _ resource.ResourceWithConfigure = &Gcp{}

func NewGcp() resource.Resource {
	return &Gcp{}
}

type Gcp struct {
	installer *common.RootInstall
}

type gcpModel struct {
	OrganizationId      types.String `tfsdk:"organization_id"`
	ServiceAccountEmail types.String `tfsdk:"service_account_email"`
	AccessLogs          types.Object `tfsdk:"access_logs"`
	IamAssessment       types.Object `tfsdk:"iam_assessment"`
	OrgWidePolicy       types.Object `tfsdk:"org_wide_policy"`
}

type gcpAccessLogsMetadata struct {
	Permissions []string        `json:"requiredPermissions" tfsdk:"permissions"`
	CustomRole  gcpRoleMetadata `json:"customRole" tfsdk:"custom_role"`
}

type gcpIamAssessmentMetadata struct {
	ProjectPermissions      []string `json:"requiredPermissions" tfsdk:"project"`
	OrganizationPermissions []string `json:"orgLevelPermissions" tfsdk:"organization"`
}

type gcpConfig struct {
	Root struct {
		Singleton struct {
			OrganizationId string `json:"organizationId"`
		} `json:"_"`
	} `json:"root"`
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
		AccessLogs    gcpAccessLogsMetadata    `json:"access-logs"`
		IamAssessment gcpIamAssessmentMetadata `json:"iam-assessment"`
		OrgWidePolicy gcpPermissionsMetadata   `json:"org-wide-policy"`
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
					"permissions": permissions("access logging"),
					"custom_role": customRole,
				},
			},
			"iam_assessment": schema.SingleNestedAttribute{
				Computed:            true,
				MarkdownDescription: `Read-only attributes used to configure IAM grants for IAM-assessment integrations`,
				Attributes: map[string]schema.Attribute{
					"permissions": schema.SingleNestedAttribute{
						Computed:            true,
						MarkdownDescription: `Permissions that must be granted to P0's service account`,
						Attributes: map[string]schema.Attribute{
							"project": schema.ListAttribute{
								Computed:            true,
								ElementType:         types.StringType,
								MarkdownDescription: `Permissions required for project-level IAM-assessment installs`,
							},
							"organization": schema.ListAttribute{
								Computed:            true,
								ElementType:         types.StringType,
								MarkdownDescription: `Permissions, in addition to 'project' permissions, required for organization-level IAM-assessment installs`,
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
	r.installer = &common.RootInstall{
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
		"permissions": types.ListType{ElemType: types.StringType},
		"custom_role": types.ObjectType{AttrTypes: map[string]attr.Type{
			"id":   types.StringType,
			"name": types.StringType,
		}},
	}, metadata.AccessLogs)
	if alDiags.HasError() {
		diags.Append(alDiags...)
		return nil
	}
	data.AccessLogs = accessLogs

	iamAssessmentPermissionsType := map[string]attr.Type{
		"project":      types.ListType{ElemType: types.StringType},
		"organization": types.ListType{ElemType: types.StringType},
	}
	iamAssessmentPermissions, iapDiags := types.ObjectValueFrom(
		ctx, iamAssessmentPermissionsType, metadata.IamAssessment,
	)
	if iapDiags.HasError() {
		diags.Append(iapDiags...)
		return nil
	}
	iamAssessment, iaDiags := types.ObjectValue(
		map[string]attr.Type{"permissions": types.ObjectType{
			AttrTypes: iamAssessmentPermissionsType,
		}},
		map[string]attr.Value{"permissions": iamAssessmentPermissions},
	)
	if iaDiags.HasError() {
		diags.Append(iaDiags...)
		return nil
	}
	data.IamAssessment = iamAssessment

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
	json := gcpConfig{}

	datav, ok := data.(*gcpModel)
	if !ok {
		return nil
	}

	json.Root.Singleton.OrganizationId = datav.OrganizationId.ValueString()

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
