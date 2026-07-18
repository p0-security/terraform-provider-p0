package installpostgreslegacy

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/p0-security/terraform-provider-p0/internal"
	"github.com/p0-security/terraform-provider-p0/internal/common"
	installresources "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &PostgresLegacy{}
var _ resource.ResourceWithConfigure = &PostgresLegacy{}
var _ resource.ResourceWithImportState = &PostgresLegacy{}

type PostgresLegacy struct {
	installer *common.Install
}

func NewPostgresLegacy() resource.Resource {
	return &PostgresLegacy{}
}

type postgresLegacyConnectivityModel struct {
	Type              types.String `tfsdk:"type"`
	SecurityPerimeter types.String `tfsdk:"security_perimeter"`
	SecurityGroupId   types.String `tfsdk:"security_group_id"`
}

type postgresLegacyConnectivityJson struct {
	Type              string  `json:"type"`
	SecurityPerimeter *string `json:"securityPerimeter,omitempty"`
	SecurityGroupId   *string `json:"securityGroupId,omitempty"`
}

type postgresLegacyInstallTypeModel struct {
	Type         types.String                     `tfsdk:"type"`
	Region       types.String                     `tfsdk:"region"`
	ProjectId    types.String                     `tfsdk:"project_id"`
	InstanceId   types.String                     `tfsdk:"instance_id"`
	Account      types.String                     `tfsdk:"account"`
	ResourceId   types.String                     `tfsdk:"resource_id"`
	Instance     types.String                     `tfsdk:"instance"`
	Hostname     types.String                     `tfsdk:"hostname"`
	Port         types.String                     `tfsdk:"port"`
	Connectivity *postgresLegacyConnectivityModel `tfsdk:"connectivity"`
}

type postgresLegacyInstallTypeJson struct {
	Type         string                          `json:"type"`
	Region       string                          `json:"region"`
	ProjectId    *string                         `json:"projectId,omitempty"`
	InstanceId   *string                         `json:"instanceId,omitempty"`
	Account      *string                         `json:"account,omitempty"`
	ResourceId   *string                         `json:"resourceId,omitempty"`
	Instance     *string                         `json:"instance,omitempty"`
	Hostname     *string                         `json:"hostname,omitempty"`
	Port         *string                         `json:"port,omitempty"`
	Connectivity *postgresLegacyConnectivityJson `json:"connectivity,omitempty"`
}

type postgresLegacyAccessManagementModel struct {
	Id           types.String                    `tfsdk:"id"`
	Label        types.String                    `tfsdk:"label"`
	DatabaseName types.String                    `tfsdk:"database_name"`
	InstallType  *postgresLegacyInstallTypeModel `tfsdk:"install_type"`
	State        types.String                    `tfsdk:"state"`
}

type postgresLegacyAccessManagementJson struct {
	Label        *string                        `json:"label,omitempty"`
	DatabaseName string                         `json:"databaseName"`
	InstallType  *postgresLegacyInstallTypeJson `json:"installType"`
	State        string                         `json:"state"`
}

type postgresLegacyAccessManagementApi struct {
	Item *postgresLegacyAccessManagementJson `json:"item"`
}

func (*PostgresLegacy) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_postgres_legacy"
}

func (*PostgresLegacy) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A legacy PostgreSQL installation, managed via direct IAM-authenticated connections (no connector deployment required).

**Note:** This is the original PostgreSQL integration (backend integration key ` + "`pg`" + `). For new installations that use P0's Lambda/Cloud Run connector architecture, see ` + "`p0_postgres`" + ` instead.

Each ` + "`p0_postgres_legacy`" + ` resource manages access to a single database on a single instance. Create one resource per database.`,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: `A unique identifier for this PostgreSQL installation (can be any string, e.g., "production-db" or "staging-postgres")`,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.RegexMatches(ComponentIdRegex, "Identifier must start with a letter and contain only alphanumeric characters and hyphens"),
				},
			},
			"label": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: `A display label for this installation (defaults to the id if not provided)`,
			},
			"database_name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: `The name of the database to manage. To manage more than one database on the same instance, create an additional 'p0_postgres_legacy' resource for each.`,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"state": common.StateAttribute,
			"install_type": schema.SingleNestedAttribute{
				Required:            true,
				MarkdownDescription: `How the PostgreSQL database is hosted or managed`,
				PlanModifiers: []planmodifier.Object{
					objectplanmodifier.RequiresReplace(),
				},
				Attributes: map[string]schema.Attribute{
					"type": schema.StringAttribute{
						Required:            true,
						MarkdownDescription: `One of 'cloud-sql' (Google Cloud SQL) or 'rds' (AWS RDS)`,
						Validators: []validator.String{
							stringvalidator.AnyWithAllWarnings(
								stringvalidator.All(
									stringvalidator.OneOf("cloud-sql"),
									stringvalidator.AlsoRequires(
										path.MatchRelative().AtParent().AtName("project_id"),
										path.MatchRelative().AtParent().AtName("instance_id"),
									),
									stringvalidator.ConflictsWith(
										path.MatchRelative().AtParent().AtName("account"),
										path.MatchRelative().AtParent().AtName("resource_id"),
										path.MatchRelative().AtParent().AtName("instance"),
										path.MatchRelative().AtParent().AtName("hostname"),
										path.MatchRelative().AtParent().AtName("port"),
										path.MatchRelative().AtParent().AtName("connectivity"),
									),
								),
								stringvalidator.All(
									stringvalidator.OneOf("rds"),
									stringvalidator.AlsoRequires(
										path.MatchRelative().AtParent().AtName("account"),
										path.MatchRelative().AtParent().AtName("resource_id"),
										path.MatchRelative().AtParent().AtName("instance"),
										path.MatchRelative().AtParent().AtName("hostname"),
									),
									stringvalidator.ConflictsWith(
										path.MatchRelative().AtParent().AtName("project_id"),
										path.MatchRelative().AtParent().AtName("instance_id"),
									),
								),
							),
						},
					},
					"region": schema.StringAttribute{
						Required:            true,
						MarkdownDescription: `The GCP or AWS region where the database instance is located`,
					},
					"project_id": schema.StringAttribute{
						Optional:            true,
						MarkdownDescription: `(cloud-sql only) The GCP project ID, which must reference an already-installed 'p0_gcp_iam_write' resource`,
					},
					"instance_id": schema.StringAttribute{
						Optional:            true,
						MarkdownDescription: `(cloud-sql only) The ID of the CloudSQL instance hosting the database (must have public IP access enabled). This is the bare instance ID, not the full "project:region:instance" connection name.`,
						Validators: []validator.String{
							stringvalidator.RegexMatches(CloudSqlInstanceIdRegex, `Must not contain colons; enter only the instance ID, not the full connection name`),
						},
					},
					"account": schema.StringAttribute{
						Optional:            true,
						MarkdownDescription: `(rds only) The AWS account ID, which must reference an already-installed 'p0_aws_iam_write' resource`,
						Validators: []validator.String{
							stringvalidator.RegexMatches(AwsAccountIdRegex, "AWS account IDs should consist of 12 numeric digits"),
						},
					},
					"resource_id": schema.StringAttribute{
						Optional:            true,
						MarkdownDescription: `(rds only) The resource ID of the RDS instance (typically starts with 'db-')`,
					},
					"instance": schema.StringAttribute{
						Optional:            true,
						MarkdownDescription: `(rds only) The RDS instance identifier, as assigned by AWS`,
					},
					"hostname": schema.StringAttribute{
						Optional:            true,
						MarkdownDescription: `(rds only) The public hostname/endpoint of the RDS instance`,
					},
					"port": schema.StringAttribute{
						Optional: true,
						Computed: true,
						// No schema-level default: a static default would also apply to
						// cloud-sql plans, whose API items never carry a port, producing
						// an inconsistent result after apply. The rds default is applied
						// in toJson instead.
						MarkdownDescription: `(rds only) The port on which the PostgreSQL instance is listening (defaults to 5432)`,
						Validators: []validator.String{
							stringvalidator.RegexMatches(PortRegex, "Must be a valid port number (1-65535)"),
						},
					},
					"connectivity": schema.SingleNestedAttribute{
						Optional:            true,
						Computed:            true,
						MarkdownDescription: `(rds only) How P0 will connect to the RDS database (defaults to 'public')`,
						Attributes: map[string]schema.Attribute{
							"type": schema.StringAttribute{
								Required:            true,
								MarkdownDescription: `One of 'public' (connect to the RDS instance's public endpoint) or 'private' (connect via an AWS Security Perimeter)`,
								Validators: []validator.String{
									stringvalidator.AnyWithAllWarnings(
										stringvalidator.All(
											stringvalidator.OneOf("public"),
											stringvalidator.ConflictsWith(
												path.MatchRelative().AtParent().AtName("security_perimeter"),
												path.MatchRelative().AtParent().AtName("security_group_id"),
											),
										),
										stringvalidator.All(
											stringvalidator.OneOf("private"),
											stringvalidator.AlsoRequires(
												path.MatchRelative().AtParent().AtName("security_perimeter"),
												path.MatchRelative().AtParent().AtName("security_group_id"),
											),
										),
									),
								},
							},
							"security_perimeter": schema.StringAttribute{
								Optional:            true,
								MarkdownDescription: `(private only) The ID of an already-installed AWS 'rds-security-perimeter' component item`,
							},
							"security_group_id": schema.StringAttribute{
								Optional:            true,
								MarkdownDescription: `(private only) An AWS Security Group assigned to the RDS instance`,
							},
						},
					},
				},
			},
		},
	}
}

func (r *PostgresLegacy) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:  PostgresLegacyKey,
		Component:    installresources.AccessManagement,
		ProviderData: data,
		GetId:        r.getId,
		GetItemJson:  r.getItemJson,
		FromJson:     r.fromJson,
		ToJson:       r.toJson,
	}
}

func (r *PostgresLegacy) getId(data any) *string {
	model, ok := data.(*postgresLegacyAccessManagementModel)
	if !ok {
		return nil
	}
	str := model.Id.ValueString()
	return &str
}

func (r *PostgresLegacy) getItemJson(json any) any {
	inner, ok := json.(*postgresLegacyAccessManagementApi)
	if !ok {
		return nil
	}
	return inner.Item
}

func (r *PostgresLegacy) fromJson(ctx context.Context, diags *diag.Diagnostics, id string, json any) any {
	data := postgresLegacyAccessManagementModel{}
	jsonv, ok := json.(*postgresLegacyAccessManagementJson)
	if !ok {
		return nil
	}

	data.Id = types.StringValue(id)
	data.State = types.StringValue(jsonv.State)
	data.DatabaseName = types.StringValue(jsonv.DatabaseName)
	data.Label = types.StringPointerValue(jsonv.Label)

	if jsonv.InstallType != nil {
		installType := postgresLegacyInstallTypeModel{
			Type:       types.StringValue(jsonv.InstallType.Type),
			Region:     types.StringValue(jsonv.InstallType.Region),
			ProjectId:  types.StringPointerValue(jsonv.InstallType.ProjectId),
			InstanceId: types.StringPointerValue(jsonv.InstallType.InstanceId),
			Account:    types.StringPointerValue(jsonv.InstallType.Account),
			ResourceId: types.StringPointerValue(jsonv.InstallType.ResourceId),
			Instance:   types.StringPointerValue(jsonv.InstallType.Instance),
			Hostname:   types.StringPointerValue(jsonv.InstallType.Hostname),
			Port:       types.StringPointerValue(jsonv.InstallType.Port),
		}

		if jsonv.InstallType.Connectivity != nil {
			installType.Connectivity = &postgresLegacyConnectivityModel{
				Type:              types.StringValue(jsonv.InstallType.Connectivity.Type),
				SecurityPerimeter: types.StringPointerValue(jsonv.InstallType.Connectivity.SecurityPerimeter),
				SecurityGroupId:   types.StringPointerValue(jsonv.InstallType.Connectivity.SecurityGroupId),
			}
		}

		data.InstallType = &installType
	}

	return &data
}

func (r *PostgresLegacy) toJson(data any) any {
	json := postgresLegacyAccessManagementJson{}

	datav, ok := data.(*postgresLegacyAccessManagementModel)
	if !ok {
		return nil
	}

	json.DatabaseName = datav.DatabaseName.ValueString()

	if !datav.Label.IsNull() && !datav.Label.IsUnknown() {
		label := datav.Label.ValueString()
		json.Label = &label
	}

	if datav.InstallType != nil {
		installType := postgresLegacyInstallTypeJson{
			Type:   datav.InstallType.Type.ValueString(),
			Region: datav.InstallType.Region.ValueString(),
		}

		switch installType.Type {
		case "cloud-sql":
			installType.ProjectId = datav.InstallType.ProjectId.ValueStringPointer()
			installType.InstanceId = datav.InstallType.InstanceId.ValueStringPointer()
		case "rds":
			installType.Account = datav.InstallType.Account.ValueStringPointer()
			installType.ResourceId = datav.InstallType.ResourceId.ValueStringPointer()
			installType.Instance = datav.InstallType.Instance.ValueStringPointer()
			installType.Hostname = datav.InstallType.Hostname.ValueStringPointer()

			// Default the port here rather than in the schema so that cloud-sql
			// plans (which never carry a port) are not assigned one. Sending the
			// default also keeps the stored value a string; the backend's own
			// fallback would store it as a number.
			port := PostgresLegacyDefaultPort
			if !datav.InstallType.Port.IsNull() && !datav.InstallType.Port.IsUnknown() {
				port = datav.InstallType.Port.ValueString()
			}
			installType.Port = &port

			if datav.InstallType.Connectivity != nil {
				connectivity := postgresLegacyConnectivityJson{
					Type: datav.InstallType.Connectivity.Type.ValueString(),
				}
				if connectivity.Type == "private" {
					connectivity.SecurityPerimeter = datav.InstallType.Connectivity.SecurityPerimeter.ValueStringPointer()
					connectivity.SecurityGroupId = datav.InstallType.Connectivity.SecurityGroupId.ValueStringPointer()
				}
				installType.Connectivity = &connectivity
			}
		}

		json.InstallType = &installType
	}

	return &json
}

func (r *PostgresLegacy) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json postgresLegacyAccessManagementApi
	var data postgresLegacyAccessManagementModel

	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)

	inputJson := r.toJson(&data)

	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, inputJson)
	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data)
}

func (r *PostgresLegacy) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &postgresLegacyAccessManagementApi{}, &postgresLegacyAccessManagementModel{})
}

func (r *PostgresLegacy) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &postgresLegacyAccessManagementApi{}, &postgresLegacyAccessManagementModel{})
}

func (r *PostgresLegacy) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Rollback(ctx, &resp.Diagnostics, &req.State, &postgresLegacyAccessManagementModel{})
}

func (r *PostgresLegacy) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
