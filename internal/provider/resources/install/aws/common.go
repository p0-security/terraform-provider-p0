package installaws

import (
	"regexp"

	installresources "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install"
)

const (
	Aws       = "aws"
	Inventory = "inventory"
)

// All installable AWS components.
var Components = []string{installresources.IamWrite, Inventory}

var AwsAccountIdRegex = regexp.MustCompile(`^\d{12}$`)
var AwsIdpPattern = regexp.MustCompile(`^[\w.-/]+$`)
var OktaAppIdRegex = regexp.MustCompile(`^0o\w+$`)

const AwsLabelMarkdownDescription = "The AWS account's alias (if available)"
