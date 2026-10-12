package installaws

import (
	"regexp"

	installresources "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install"
)

const Aws = "aws"

// All installable AWS components.
var Components = []string{installresources.IamWrite, installresources.Inventory}

var AwsAccountIdRegex = regexp.MustCompile(`^\d{12}$`)
var AwsPartitionRegex = regexp.MustCompile(`^(aws|aws-us-gov)$`)
var AwsIdpPattern = regexp.MustCompile(`^[\w.-/]+$`)
var OktaAppIdRegex = regexp.MustCompile(`^0o\w+$`)

const AwsLabelMarkdownDescription = "The AWS account's alias (if available)"
