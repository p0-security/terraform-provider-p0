package installaws

import "regexp"

var Aws = "aws"

var IamWrite = "iam-write"
var Inventory = "inventory"

// All installable AWS components.
var Components = []string{IamWrite, Inventory}

var AwsAccountIdRegex = regexp.MustCompile(`^\d{12}$`)
var AwsIdpPattern = regexp.MustCompile(`^[\w.-/]+$`)
var OktaAppIdRegex = regexp.MustCompile(`^0o\w+$`)
