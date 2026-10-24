package installawssqs

import (
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

const (
	AwsSqsIntegration  = "aws-sqs"
	AuditLogsComponent = "audit-log"
	// The role P0 assumes in the queue's account. P0's staging and development
	// environments add a suffix (for example "Stage").
	RoleName = "P0RoleIamEventSender"
	docsUrl  = "https://docs.p0.dev/integrations/siem-integrations/amazon-sqs-setup"
)

var queueArnRegex = regexp.MustCompile(`^arn:(aws|aws-us-gov):sqs:[a-z0-9-]+:\d{12}:[A-Za-z0-9_-]{1,75}(\.fifo)?$`)
var kmsKeyArnRegex = regexp.MustCompile(`^arn:(aws|aws-us-gov):kms:[a-z0-9-]+:\d{12}:key/[A-Za-z0-9-]+$`)

var idAttribute = schema.StringAttribute{
	Required:            true,
	MarkdownDescription: "The ARN of the SQS queue P0 sends audit events to, in the form `arn:aws:sqs:<region>:<account-id>:<queue-name>`",
	Validators: []validator.String{
		stringvalidator.RegexMatches(queueArnRegex, "must be an SQS queue ARN, in the form arn:aws:sqs:<region>:<account-id>:<queue-name>"),
	},
	PlanModifiers: []planmodifier.String{
		stringplanmodifier.RequiresReplace(),
	},
}

var kmsKeyArnAttribute = schema.StringAttribute{
	Optional:            true,
	MarkdownDescription: "The ARN of the customer managed KMS key that encrypts the queue. Omit it for queues that use SQS-managed encryption or the AWS managed key (`alias/aws/sqs`).",
	Validators: []validator.String{
		stringvalidator.RegexMatches(kmsKeyArnRegex, "must be a KMS key ARN, in the form arn:aws:kms:<region>:<account-id>:key/<key-id>"),
	},
	PlanModifiers: []planmodifier.String{
		stringplanmodifier.RequiresReplace(),
	},
}

// The item P0 stores for a queue. serviceAccountId and serviceAccountEmail are set
// by P0 when the queue is staged, so they are never sent.
type auditLogsJson struct {
	State               *string `json:"state,omitempty"`
	KmsKeyArn           *string `json:"kmsKeyArn,omitempty"`
	ServiceAccountId    *string `json:"serviceAccountId,omitempty"`
	ServiceAccountEmail *string `json:"serviceAccountEmail,omitempty"`
}

type auditLogsApi struct {
	Item *auditLogsJson `json:"item"`
}

type auditLogsInputJson struct {
	KmsKeyArn *string `json:"kmsKeyArn,omitempty"`
}
