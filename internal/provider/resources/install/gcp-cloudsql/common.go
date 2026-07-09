package installgcpcloudsql

import (
	installresources "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install"
)

const GcpCloudSqlKey = "gcp-cloudsql"

// All installable GCP CloudSQL components.
var Components = []string{installresources.IamWrite}
