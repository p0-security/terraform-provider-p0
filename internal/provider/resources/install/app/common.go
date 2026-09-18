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

// A Lambda function name or a Cloud Run service name. Cloud Run is the stricter of the
// two (lowercase letters, digits, and hyphens), and this accepts either.
var ConnectorNameRegex = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9-_]{0,63}$`)

// An AWS or Google Cloud region, e.g. "us-east-1" (AWS separates the trailing
// number with a hyphen) or "us-central1" (Google Cloud does not).
var ConnectorRegionRegex = regexp.MustCompile(`^[a-z]{2,}(-[a-z]+)+-?\d$`)
