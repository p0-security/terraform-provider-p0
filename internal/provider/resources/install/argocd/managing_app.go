package installargocd

import (
	"regexp"
	"strings"
)

// P0's rule for the Application that manages an instance's argocd-rbac-cm, and its
// message for one that breaks it (packages/integrations/argocd/shared/src/managing-app.ts
// in the app): `<project>/<application>`, how Argo CD's RBAC names an Application, and
// so how the customer's p0-connector policy lines name it. Each name is a Kubernetes
// object's name. An Application in another namespace than Argo CD's own
// (`<project>/<namespace>/<application>`) isn't supported.

// A Kubernetes object's name (a DNS-1123 subdomain): dot-separated labels of lowercase
// letters, digits and hyphens, each starting and ending with a letter or digit.
var objectNameRegex = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]*[a-z0-9])?)*$`)

// Kubernetes caps an object's name at 253 characters.
const objectNameMaxLength = 253

const invalidManagingApp = "Enter the Argo CD Application that manages argocd-rbac-cm as <project>/<application>, such as default/argocd. " +
	"Both names are lowercase letters, digits, hyphens and dots."

func isObjectName(name string) bool {
	return len(name) <= objectNameMaxLength && objectNameRegex.MatchString(name)
}

// Reports whether app names an Application as `<project>/<application>`. Unlike P0,
// it doesn't ignore whitespace around app: validateField refuses that on its own.
func isManagingApp(app string) bool {
	project, application, ok := strings.Cut(app, "/")
	return ok && isObjectName(project) && isObjectName(application)
}
