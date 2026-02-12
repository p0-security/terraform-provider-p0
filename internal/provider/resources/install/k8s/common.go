package installk8s

import (
	installresources "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install"
)

const (
	K8s       = "k8s"
	Inventory = "inventory"
)

// All installable Kubernetes components.
var Components = []string{installresources.Kubernetes, Inventory}
