package installargocd

import "regexp"

// P0's rule for the Kubernetes namespace that an instance's argocd-rbac-cm is in, and
// its message for one that breaks it (packages/integrations/argocd/shared/src/namespace.ts
// in the app): a namespace's name, a DNS-1123 label. An instance that leaves it out
// is in defaultNamespace, which P0 fills in when it reads the instance.

// Up to 63 lowercase letters, digits and hyphens, starting and ending with a letter or
// digit.
var namespaceRegex = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

// Where Argo CD's install manifests put it, and so argocd-rbac-cm.
const defaultNamespace = "argocd"

const invalidNamespace = "Enter the Kubernetes namespace Argo CD runs in, such as argocd: up to 63 lowercase letters, digits and hyphens, " +
	"starting and ending with a letter or digit."
