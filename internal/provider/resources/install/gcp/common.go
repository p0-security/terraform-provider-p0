package installgcp

import (
	"regexp"

	installresources "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install"
)

const (
	AccessLogs         = "access-logs"
	GcpKey             = "gcloud"
	SharingRestriction = "sharing-restriction"
)

var ProjectComponents = []string{
	AccessLogs,
	installresources.IamAssessment,
	installresources.IamWrite,
	SharingRestriction,
}

var GcpProjectIdRegex = regexp.MustCompile(`^[\w-]+$`)
var GcpOrganizationIdRegex = regexp.MustCompile(`^[\d]+$`)
