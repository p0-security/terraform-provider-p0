package installk8s

import (
	"regexp"

	installresources "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install"
)

const (
	K8s       = "k8s"
	Inventory = "inventory"
)

// All installable AWS components.
var Components = []string{installresources.Kubernetes, Inventory}
var AwsAccountIdRegex = regexp.MustCompile(`^\d{12}$`)
