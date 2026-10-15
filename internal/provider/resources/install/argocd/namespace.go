package installargocd

import "regexp"

// P0's rule for the Kubernetes namespace that an instance's argocd-rbac-cm is in, and
// its message for one that breaks it (packages/integrations/argocd/shared/src/namespace.ts
// in the app): a namespace's name, a DNS-1123 label. The app also takes a blank one,
// which the provider refuses: see namespaceRules.

// Up to 63 lowercase letters, digits and hyphens, starting and ending with a letter or
// digit.
var namespaceRegex = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

// Where Argo CD's install manifests put it, and so argocd-rbac-cm. P0 stores no
// namespace for an instance that leaves it out, and the app reads that as this one
// (`toNamespace` in namespace.ts).
const defaultNamespace = "argocd"

const blankNamespace = "Leave namespace out, rather than empty, for " + defaultNamespace + ", Argo CD's default."

const invalidNamespace = "Enter the Kubernetes namespace Argo CD runs in, such as argocd: up to 63 lowercase letters, digits and hyphens, " +
	"starting and ending with a letter or digit."
