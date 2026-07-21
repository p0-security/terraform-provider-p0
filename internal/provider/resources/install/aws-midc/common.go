package installawsmidc

import (
	installresources "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install"
)

const AwsMidcKey = "aws-midc"

// All installable AWS Identity Center (merged) components.
var Components = []string{installresources.Identity}
