package installrds

import (
	"regexp"

	installresources "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install"
)

const RdsKey = "aws-rds"

// All installable RDS components.
var Components = []string{installresources.IamWrite}

var AwsAccountIdRegex = regexp.MustCompile(`^\d{12}$`)
var AwsRegionRegex = regexp.MustCompile(`^[a-z]{2}-[a-z]+-\d{1}$`)
var AwsVpcIdRegex = regexp.MustCompile(`^vpc-[a-f0-9]{8,17}$`)

const AwsLabelMarkdownDescription = "The AWS account's alias (if available)"
