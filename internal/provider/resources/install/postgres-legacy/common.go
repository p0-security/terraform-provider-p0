package installpostgreslegacy

import (
	"regexp"

	installresources "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install"
)

// PostgresLegacyKey is the P0 integration key for the legacy (pre-connector)
// PostgreSQL integration. Note that this differs from the directory/resource
// naming convention: the backend retains the original "pg" integration key
// for backwards compatibility with existing customer configs, even though
// the backend package that implements it is named "postgres-legacy".
const PostgresLegacyKey = "pg"

// All installable postgres-legacy components.
var Components = []string{installresources.AccessManagement}

var ComponentIdRegex = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9-]*$`)
var CloudSqlInstanceIdRegex = regexp.MustCompile(`^[^:]+$`)
var AwsAccountIdRegex = regexp.MustCompile(`^\d{12}$`)
var PortRegex = regexp.MustCompile(`^\d{1,5}$`)

const PostgresLegacyDefaultPort = "5432"
