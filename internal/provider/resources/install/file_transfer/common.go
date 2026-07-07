package installfiletransfer

import (
	"regexp"

	installresources "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install"
)

const FileTransferKey = "file-transfer"

// All installable file-transfer components.
var Components = []string{installresources.IamWrite}

// A DNS-style S3 bucket name: 3-63 characters, lowercase alphanumerics plus dots and
// hyphens, beginning and ending with an alphanumeric. Mirrors the validation in the P0 app.
var BucketNameRegex = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$`)
