package installapp

import (
	"regexp"

	installresources "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install"
)

const AppKey = "app"

// All installable custom-application components.
var Components = []string{installresources.IamWrite}

const (
	AwsHosting = "aws"
	GcpHosting = "gcp"
)

// A Lambda function name or a Cloud Run service name. This is the Lambda constraint,
// the looser of the two, because the schema validator cannot see which hosting the
// name belongs to; ValidateConfig narrows it to CloudRunServiceNameRegex for `gcp`.
var ConnectorNameRegex = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9-_]{0,63}$`)

// A Cloud Run service name: at most 49 characters of lowercase letters, digits and
// hyphens, starting with a letter and not ending with a hyphen.
var CloudRunServiceNameRegex = regexp.MustCompile(`^[a-z]([a-z0-9-]{0,47}[a-z0-9])?$`)

// An AWS or Google Cloud region, e.g. "us-east-1" (AWS separates the trailing
// number with a hyphen) or "us-central1" (Google Cloud does not). Google Cloud's
// numbering runs into two digits, e.g. "europe-west12".
var ConnectorRegionRegex = regexp.MustCompile(`^[a-z]{2,}(-[a-z]+)+-?\d{1,2}$`)
