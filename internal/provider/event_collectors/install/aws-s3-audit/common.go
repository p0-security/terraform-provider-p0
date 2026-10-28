package installawss3audit

import (
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const (
	AwsS3AuditIntegration = "aws-s3-audit"
	AuditLogsComponent    = "audit-log"
	// The role P0 assumes in the bucket's account. P0's staging and development
	// environments add a suffix (for example "Stage").
	RoleName = "P0RoleIamS3Writer"
	docsUrl  = "https://docs.p0.dev/integrations/siem-integrations/amazon-s3-audit-setup"
)

// S3 general purpose bucket names: 3-63 lowercase letters, digits, dots and
// hyphens, starting and ending with a letter or digit. P0 also rejects ".." and
// IP-address names.
var bucketNameRegex = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$`)
var accountIdRegex = regexp.MustCompile(`^\d{12}$`)
var regionRegex = regexp.MustCompile(`^[a-z]{2}(-gov)?-[a-z]+-\d+$`)
var partitionRegex = regexp.MustCompile(`^(aws|aws-us-gov)$`)

var idAttribute = schema.StringAttribute{
	Required:            true,
	MarkdownDescription: "The name of the S3 bucket P0 writes audit files to. The name only, not an ARN or `s3://` URL.",
	Validators: []validator.String{
		stringvalidator.RegexMatches(bucketNameRegex, "must be an S3 bucket name: 3 to 63 lowercase letters, numbers, dots or hyphens"),
	},
	PlanModifiers: []planmodifier.String{
		stringplanmodifier.RequiresReplace(),
	},
}

var accountIdAttribute = schema.StringAttribute{
	Required:            true,
	MarkdownDescription: "The ID of the AWS account that owns the bucket. P0's AWS integration must be installed for this account.",
	Validators: []validator.String{
		stringvalidator.RegexMatches(accountIdRegex, "AWS account IDs should consist of 12 numeric digits"),
	},
	PlanModifiers: []planmodifier.String{
		stringplanmodifier.RequiresReplace(),
	},
}

var regionAttribute = schema.StringAttribute{
	Required:            true,
	MarkdownDescription: "The AWS region the bucket is in, for example `us-west-2`",
	Validators: []validator.String{
		stringvalidator.RegexMatches(regionRegex, "must be an AWS region code, for example us-west-2"),
	},
	PlanModifiers: []planmodifier.String{
		stringplanmodifier.RequiresReplace(),
	},
}

var partitionAttribute = schema.StringAttribute{
	Optional:            true,
	Computed:            true,
	Default:             stringdefault.StaticString("aws"),
	MarkdownDescription: "The AWS partition the account is in, `aws` or `aws-us-gov`. Defaults to `aws`.",
	Validators: []validator.String{
		stringvalidator.RegexMatches(partitionRegex, "AWS partition must be one of: aws, aws-us-gov."),
	},
	PlanModifiers: []planmodifier.String{
		stringplanmodifier.RequiresReplace(),
	},
}

type awsPartitionJson struct {
	Type *string `json:"type"`
}

// The item P0 stores for a bucket. serviceAccountId and serviceAccountEmail are
// set by P0 when the bucket is staged, so they are never sent.
type auditLogsJson struct {
	State               *string           `json:"state,omitempty"`
	AccountId           *string           `json:"accountId,omitempty"`
	Region              *string           `json:"region,omitempty"`
	AwsPartition        *awsPartitionJson `json:"awsPartition,omitempty"`
	ServiceAccountId    *string           `json:"serviceAccountId,omitempty"`
	ServiceAccountEmail *string           `json:"serviceAccountEmail,omitempty"`
}

type auditLogsApi struct {
	Item *auditLogsJson `json:"item"`
}

type auditLogsInputJson struct {
	AccountId    *string           `json:"accountId,omitempty"`
	Region       *string           `json:"region,omitempty"`
	AwsPartition *awsPartitionJson `json:"awsPartition,omitempty"`
}

// The bucket fields both resources send, so P0 can find the AWS integration's
// service account and check the role at every install step.
type bucketFields struct {
	AccountId types.String
	Region    types.String
	Partition types.String
}

func (f bucketFields) toJson() *auditLogsInputJson {
	return &auditLogsInputJson{
		AccountId:    f.AccountId.ValueStringPointer(),
		Region:       f.Region.ValueStringPointer(),
		AwsPartition: &awsPartitionJson{Type: f.Partition.ValueStringPointer()},
	}
}

func bucketFieldsFromJson(json *auditLogsJson) bucketFields {
	partition := types.StringValue("aws")
	if json.AwsPartition != nil && json.AwsPartition.Type != nil {
		partition = types.StringPointerValue(json.AwsPartition.Type)
	}
	return bucketFields{
		AccountId: types.StringPointerValue(json.AccountId),
		Region:    types.StringPointerValue(json.Region),
		Partition: partition,
	}
}
