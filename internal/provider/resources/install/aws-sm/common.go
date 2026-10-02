package installawssm

import "regexp"

const AwsSmKey = "aws-sm"

// An AWS region, such as us-west-2.
var AwsRegionRegex = regexp.MustCompile(`^[a-z]+(?:-[a-z]+)+-\d+$`)
