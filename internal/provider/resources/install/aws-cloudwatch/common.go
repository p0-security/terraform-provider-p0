package installawscloudwatch

import (
	"context"
	"fmt"
	"net/url"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/p0-security/terraform-provider-p0/internal"
	"github.com/p0-security/terraform-provider-p0/internal/common"
	installaws "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/aws"
	logsource "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/log-source"
)

// The AWS CloudWatch Logs log source, as the app defines it in
// packages/integrations/aws-cloudwatch/shared/src/components.ts.
const (
	AwsCloudWatchKey = "aws-cloudwatch"
	sourceComponent  = "source"
	label            = "AWS CloudWatch Logs"
)

// The options of the app's `cloudtrail` select. The cloudtrail block's coverage
// is the option's type, and an unset block is `none`.
const (
	cloudtrailNone       = "none"
	coverageAccount      = "account"
	coverageOrganization = "organization"
)

const cloudtrailExampleNote = "See `cloudtrail.tf` in the `p0_aws_cloudwatch_logs` examples for an organization trail that delivers to CloudWatch Logs."

// An AWS region, such as us-west-2.
var awsRegionRegex = regexp.MustCompile(`^[a-z]+(?:-[a-z]+)+-\d+$`)

// A region outside AWS's commercial partition: GovCloud, China, the ISO regions
// or the European Sovereign Cloud. P0 invokes the connector by an `arn:aws:`
// ARN, so it can't run anywhere else.
var awsOtherPartitionRegionRegex = regexp.MustCompile(`^(?:us-gov-|cn-|[a-z]+-iso[a-z]*-|eusc-)`)

// The app's rule for a CloudWatch log group name.
var logGroupNameRegex = regexp.MustCompile(`^[\w\-/.#]{1,512}$`)

// An item's ID is its name in P0, which may be anything that doesn't start or
// end with whitespace.
var itemIdRegex = regexp.MustCompile(`^\S(?:.{0,253}\S)?$`)

// GetId escapes the item's ID, which the installer puts in a URL path. The
// installer gives the escaped ID back to FromJson, which reads it with
// idFromPath.
func getId(data any) *string {
	model, ok := data.(*sourceModel)
	if !ok {
		return nil
	}
	escaped := url.PathEscape(model.Id.ValueString())
	return &escaped
}

func idFromPath(diags *diag.Diagnostics, id string) string {
	unescaped, err := url.PathUnescape(id)
	if err != nil {
		diags.AddError("Bad item ID", fmt.Sprintf("Could not read the item ID %q: %s", id, err))
	}
	return unescaped
}

func idValidator() validator.String {
	return stringvalidator.RegexMatches(itemIdRegex, "Use a name of 1 to 255 characters that doesn't start or end with whitespace, such as 123456789012:us-west-2")
}

func accountIdValidator() validator.String {
	return stringvalidator.RegexMatches(installaws.AwsAccountIdRegex, "AWS account IDs should consist of 12 numeric digits")
}

// Rejects a value that isn't an AWS region, or a region outside AWS's
// commercial partition.
func commercialRegionValidator() validator.String {
	return commercialRegion{}
}

type commercialRegion struct{}

func (commercialRegion) Description(context.Context) string {
	return "an AWS region in AWS's commercial partition, such as us-west-2"
}

func (v commercialRegion) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (commercialRegion) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	region := req.ConfigValue.ValueString()
	switch {
	case !awsRegionRegex.MatchString(region):
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid AWS region", fmt.Sprintf("Enter an AWS region, such as us-west-2, got: %q", region))
	case awsOtherPartitionRegionRegex.MatchString(region):
		resp.Diagnostics.AddAttributeError(req.Path, "Unsupported AWS region", fmt.Sprintf("The connector runs only in commercial AWS regions, e.g. us-east-1, got: %q", region))
	}
}

// The attributes of both resources, which share one item.
func sourceAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"id": logsource.FixedAttribute(
			"The log source's name in P0. Any name works; `<account ID>:<region>`, such as `123456789012:us-west-2`, keeps it unique, since P0 allows one log source per account and region.",
			idValidator(),
		),
		"account_id": logsource.FixedAttribute(
			"The AWS account whose CloudWatch Logs P0 reads, and where the connector runs",
			accountIdValidator(),
		),
		"region": logsource.FixedAttribute(
			"The AWS region whose CloudWatch Logs P0 reads, and where the connector runs. P0 supports only commercial AWS regions.",
			commercialRegionValidator(),
		),
		"cloudtrail": schema.SingleNestedAttribute{
			Optional: true,
			MarkdownDescription: "The CloudTrail trail that delivers to CloudWatch Logs in this account and region. Leave it unset if there is none: P0 then reads only RDS logs here. P0 fixes the trail when it stages the log source, since the connector's read policy names its log group, so changing it replaces the log source. " +
				cloudtrailExampleNote,
			Attributes: map[string]schema.Attribute{
				"log_group_name": schema.StringAttribute{
					Required:            true,
					MarkdownDescription: "The CloudWatch Logs group in this account and region that the trail delivers to, such as `aws-cloudtrail-logs`",
					Validators: []validator.String{
						stringvalidator.RegexMatches(logGroupNameRegex, "Enter the CloudWatch log group your trail delivers to, such as aws-cloudtrail-logs: 1 to 512 letters, digits and _-/.#"),
					},
				},
				"coverage": schema.StringAttribute{
					Required:            true,
					MarkdownDescription: "Which accounts the trail logs: `account` for a trail that logs this AWS account's events, or `organization` for an organization trail that logs every account in your AWS organization. P0 allows one `organization` log source, and one `account` log source per account.",
					Validators: []validator.String{
						stringvalidator.OneOf(coverageAccount, coverageOrganization),
					},
				},
			},
			PlanModifiers: []planmodifier.Object{
				objectplanmodifier.RequiresReplace(),
			},
		},
		"connector_name":   logsource.GeneratedAttribute("The name of the connector's Lambda function, which P0 invokes. The example names the connector's ECR repository and log group after it, and its execution role `<connector_name>-<region>`."),
		"invoke_role_name": logsource.GeneratedAttribute("The name of the IAM role P0 assumes to invoke the connector, and nothing else"),
		"invoke_trust_policy": logsource.GeneratedAttribute(
			"The trust policy of the invoke role, as JSON. It lets P0's own service account, and no other, assume the role.",
		),
		"execution_policy": logsource.GeneratedAttribute(
			"The policy the connector's execution role needs, as JSON: Logs Insights on the CloudTrail log group and on RDS's log groups in this account and region, and reading the trails' settings",
		),
		"state": common.StateAttribute,
	}
}

type cloudtrailModel struct {
	LogGroupName types.String `tfsdk:"log_group_name"`
	Coverage     types.String `tfsdk:"coverage"`
}

type sourceModel struct {
	Id                types.String     `tfsdk:"id"`
	AccountId         types.String     `tfsdk:"account_id"`
	Region            types.String     `tfsdk:"region"`
	Cloudtrail        *cloudtrailModel `tfsdk:"cloudtrail"`
	ConnectorName     types.String     `tfsdk:"connector_name"`
	InvokeRoleName    types.String     `tfsdk:"invoke_role_name"`
	InvokeTrustPolicy types.String     `tfsdk:"invoke_trust_policy"`
	ExecutionPolicy   types.String     `tfsdk:"execution_policy"`
	State             types.String     `tfsdk:"state"`
}

// The app's `cloudtrail` select, which P0 stores as an object.
type cloudtrailJson struct {
	Type         string  `json:"type"`
	LogGroupName *string `json:"logGroupName,omitempty"`
}

// P0 also stores the service account it picked for the item, which the trust
// policy carries, and `warnings` from the last install check. The provider
// neither models nor sends them.
type sourceJson struct {
	AccountId  *string         `json:"accountId,omitempty"`
	Region     *string         `json:"region,omitempty"`
	Cloudtrail *cloudtrailJson `json:"cloudtrail,omitempty"`
	// P0 generates these.
	ConnectorName     *string `json:"connectorName,omitempty"`
	InvokeRoleName    *string `json:"invokeRoleName,omitempty"`
	InvokeTrustPolicy *string `json:"invokeTrustPolicy,omitempty"`
	ExecutionPolicy   *string `json:"executionPolicy,omitempty"`
	State             *string `json:"state,omitempty"`
}

type sourceApi = logsource.ItemApi[sourceJson]

func sourceFromJson(_ context.Context, diags *diag.Diagnostics, id string, json any) any {
	item, ok := json.(*sourceJson)
	if !ok {
		return nil
	}
	model := &sourceModel{
		Id:                types.StringValue(idFromPath(diags, id)),
		AccountId:         types.StringPointerValue(item.AccountId),
		Region:            types.StringPointerValue(item.Region),
		ConnectorName:     types.StringPointerValue(item.ConnectorName),
		InvokeRoleName:    types.StringPointerValue(item.InvokeRoleName),
		InvokeTrustPolicy: types.StringPointerValue(item.InvokeTrustPolicy),
		ExecutionPolicy:   types.StringPointerValue(item.ExecutionPolicy),
		State:             types.StringPointerValue(item.State),
	}
	// An item without a trail is stored as `none`, or with no cloudtrail at all
	// when it was staged without one.
	if item.Cloudtrail != nil && item.Cloudtrail.Type != cloudtrailNone {
		model.Cloudtrail = &cloudtrailModel{
			LogGroupName: types.StringPointerValue(item.Cloudtrail.LogGroupName),
			Coverage:     types.StringValue(item.Cloudtrail.Type),
		}
	}
	return model
}

// P0 generates the connector's names and policies and sets the state, so only
// where the item reads is sent. An unset cloudtrail block is sent as `none`.
func sourceToJson(data any) any {
	model, ok := data.(*sourceModel)
	if !ok {
		return nil
	}
	cloudtrail := &cloudtrailJson{Type: cloudtrailNone}
	if model.Cloudtrail != nil {
		cloudtrail = &cloudtrailJson{
			Type:         model.Cloudtrail.Coverage.ValueString(),
			LogGroupName: model.Cloudtrail.LogGroupName.ValueStringPointer(),
		}
	}
	return &sourceJson{
		AccountId:  model.AccountId.ValueStringPointer(),
		Region:     model.Region.ValueStringPointer(),
		Cloudtrail: cloudtrail,
	}
}

// The staged and installed resources share one item, and so one model.
func newInstaller(data *internal.P0ProviderData) *common.Install {
	return &common.Install{
		Integration:           AwsCloudWatchKey,
		Component:             sourceComponent,
		ProviderData:          data,
		GetId:                 getId,
		GetItemJson:           logsource.GetItemJson[sourceJson],
		FromJson:              sourceFromJson,
		ToJson:                sourceToJson,
		DescribeCheckError:    logsource.DescribeCheckError(label, "log source"),
		DescribeWriteNotFound: logsource.DescribeWriteNotFound(label, "log source"),
	}
}
