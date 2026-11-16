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

// An aws-iam-federation item's ID (FEDERATION_ID_PATTERN in the app).
var AwsIamFederationIdRegex = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)

var OktaAppIdRegex = regexp.MustCompile(`^0o\w+$`)

const AwsLabelMarkdownDescription = "The AWS account's alias (if available)"
