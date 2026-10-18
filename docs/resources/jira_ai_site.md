---
page_title: "p0_jira_ai_site Resource - p0"
subcategory: ""
description: |-
  A Jira Cloud site whose projects P0 can give agents access to.
  P0 reaches the site through a Jira connector, which you install with p0_jira_ai_connector. The connector manages project roles as an Atlassian service account that administers the site's projects, with that account's OAuth client. The client's secret stays in your own Google Secret Manager: P0 never reads it, and only the connector does. When an agent is granted access to a project, P0 has the connector add one of the site's agent accounts (p0_jira_ai_pool_account) to the project role read_role or write_role names, and remove it when the access ends.
  Important: Install the connector first: P0 adds a site only for a connector that is installed. Creating this resource has P0 look up the site's cloud ID from Atlassian, then check through the connector that the admin service account signs in and holds Administer Jira.
  A site's attributes can't change once it is created, so changing any of them replaces the site. To rotate the admin client's secret, add a new version to the same secret, which needs no change here.
  Warning: Replacing or destroying this resource removes the site from P0. P0 still revokes the site's open access afterwards, through the site's connector, its grants in the vault project and its admin secret, so keep all three in place until that access has ended.
  A site that already exists in P0, such as one added in the P0 app, isn't created again: the apply fails, and says to import it with terraform import. An imported site that P0 hasn't finished installing plans an update, and applying it finishes the install.
  Prerequisites:
  
  An Atlassian service account with Administer Jira, the global Jira admin permission, and an OAuth client. Store the client's secret, as the secret's whole value, in the Google Cloud project that vault names, outside Terraform, which keeps it out of Terraform's state.
  Google Secret Manager installed in P0 for that project. This provider has no resource for it, so install it in the P0 app.
  Grants in that project for the connector's service account, hosting.connector_service_account of p0_jira_ai_connector: read on the admin client's secret and on the site's agent account secrets, which start with p0_install_jira-ai_<id>_, and create, read, delete and add versions to the site's token secrets, which start with p0_access-token_jira-ai_<id>_. P0's Jira installer generates them, as in the example.
  Note: This integration is currently in beta.
---

# p0_jira_ai_site (Resource)

A Jira Cloud site whose projects P0 can give agents access to.

P0 reaches the site through a Jira connector, which you install with `p0_jira_ai_connector`. The connector manages project roles as an Atlassian service account that administers the site's projects, with that account's OAuth client. The client's secret stays in your own Google Secret Manager: P0 never reads it, and only the connector does. When an agent is granted access to a project, P0 has the connector add one of the site's agent accounts (`p0_jira_ai_pool_account`) to the project role `read_role` or `write_role` names, and remove it when the access ends.

**Important:** Install the connector first: P0 adds a site only for a connector that is installed. Creating this resource has P0 look up the site's cloud ID from Atlassian, then check through the connector that the admin service account signs in and holds Administer Jira.

A site's attributes can't change once it is created, so changing any of them replaces the site. To rotate the admin client's secret, add a new version to the same secret, which needs no change here.

**Warning:** Replacing or destroying this resource removes the site from P0. P0 still revokes the site's open access afterwards, through the site's connector, its grants in the vault project and its admin secret, so keep all three in place until that access has ended.

A site that already exists in P0, such as one added in the P0 app, isn't created again: the apply fails, and says to import it with `terraform import`. An imported site that P0 hasn't finished installing plans an update, and applying it finishes the install.

**Prerequisites:**

- An Atlassian service account with Administer Jira, the global Jira admin permission, and an OAuth client. Store the client's secret, as the secret's whole value, in the Google Cloud project that `vault` names, outside Terraform, which keeps it out of Terraform's state.

- Google Secret Manager installed in P0 for that project. This provider has no resource for it, so install it in the P0 app.

- Grants in that project for the connector's service account, `hosting.connector_service_account` of `p0_jira_ai_connector`: read on the admin client's secret and on the site's agent account secrets, which start with `p0_install_jira-ai_<id>_`, and create, read, delete and add versions to the site's token secrets, which start with `p0_access-token_jira-ai_<id>_`. P0's Jira installer generates them, as in the example.

**Note:** This integration is currently in beta.

## Example Usage

```terraform
# Adds a Jira Cloud site, reached through an installed connector. The admin
# client's secret must already be in Google Secret Manager, in the vault's
# project. See gcp.tf in this directory for the grants the connector needs.
resource "p0_jira_ai_site" "example" {
  id        = "acme"
  connector = p0_jira_ai_connector.example.id
  site_url  = "https://acme.atlassian.net"

  vault = {
    type       = "gcp-sm"
    project_id = "my-secrets-project-id"
  }

  admin_client_id   = "AbCdEfGhIjKlMnOpQrStUvWxYz012345"
  admin_secret_name = "jira-acme-admin-client-secret"
  read_role         = "P0 agent read"
  write_role        = "P0 agent write"
}
```

### Google Cloud

```terraform
# Adds the Jira Cloud site acme, and lets its connector (see gcp.tf in the
# p0_jira_ai_connector example) use the site's secrets in its vault project,
# and no others.
#
# The grants are what P0's Jira installer generates for the site. They need
# Terraform's google provider at 6.0 or later.
#
# Before you apply this, create the admin service account's OAuth client in
# Atlassian, and store its secret as the secret jira-acme-admin-client-secret.
# The example doesn't create the secret, which keeps the client's secret out of
# Terraform's state. To store it, read it without showing it:
#
#   read -rs JIRA_ADMIN_CLIENT_SECRET
#
# Then create the secret:
#
#   printf %s "$JIRA_ADMIN_CLIENT_SECRET" | gcloud secrets create jira-acme-admin-client-secret \
#     --project my-secrets-project-id --data-file=-
#
# To rotate it, add a new version to the same secret, which needs no change
# here.

locals {
  p0_jira_ai_site_acme = {
    # The site's ID, which the secret prefixes and custom role IDs below carry.
    id = "acme"
    # The project that holds the site's secrets: one of your Google Secret
    # Manager installations in P0.
    vault_project_id = "my-secrets-project-id"
    admin_secret     = "jira-acme-admin-client-secret"
    # The service account the site's connector runs as.
    connector_service_account = p0_jira_ai_connector.example.hosting.connector_service_account
  }
}

locals {
  # Each agent account's client secret starts with this, and each request's
  # token secret with the other. A site's ID holds no underscore, so the
  # underscore after it ends the prefix: no other site's secrets match.
  p0_jira_ai_site_acme_pool_secret_prefix  = "p0_install_jira-ai_${local.p0_jira_ai_site_acme.id}_"
  p0_jira_ai_site_acme_token_secret_prefix = "p0_access-token_jira-ai_${local.p0_jira_ai_site_acme.id}_"
  # Custom role IDs take no hyphen.
  p0_jira_ai_site_acme_role_id = replace(local.p0_jira_ai_site_acme.id, "-", "_")
}

# A condition matches on the resource name, which carries the project number
# and not the project ID.
data "google_project" "p0_jira_ai_site_acme" {
  project_id = local.p0_jira_ai_site_acme.vault_project_id
}

# The connector reads the admin client's secret. The accessor role reads a
# value. The viewer role reads the version's metadata, which the connector also
# reads, and can change nothing.
resource "google_secret_manager_secret_iam_member" "p0_jira_ai_site_acme_admin_accessor" {
  project   = local.p0_jira_ai_site_acme.vault_project_id
  secret_id = local.p0_jira_ai_site_acme.admin_secret
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${local.p0_jira_ai_site_acme.connector_service_account}"
}

resource "google_secret_manager_secret_iam_member" "p0_jira_ai_site_acme_admin_viewer" {
  project   = local.p0_jira_ai_site_acme.vault_project_id
  secret_id = local.p0_jira_ai_site_acme.admin_secret
  role      = "roles/secretmanager.viewer"
  member    = "serviceAccount:${local.p0_jira_ai_site_acme.connector_service_account}"
}

# The connector reads each agent account's client secret on this site.
resource "google_project_iam_member" "p0_jira_ai_site_acme_pool_accessor" {
  project = local.p0_jira_ai_site_acme.vault_project_id
  role    = "roles/secretmanager.secretAccessor"
  member  = "serviceAccount:${local.p0_jira_ai_site_acme.connector_service_account}"

  condition {
    title      = "P0 Jira site acme agent account secrets"
    expression = "resource.name.startsWith('projects/${data.google_project.p0_jira_ai_site_acme.number}/secrets/${local.p0_jira_ai_site_acme_pool_secret_prefix}')"
  }
}

resource "google_project_iam_member" "p0_jira_ai_site_acme_pool_viewer" {
  project = local.p0_jira_ai_site_acme.vault_project_id
  role    = "roles/secretmanager.viewer"
  member  = "serviceAccount:${local.p0_jira_ai_site_acme.connector_service_account}"

  condition {
    title      = "P0 Jira site acme agent account secrets"
    expression = "resource.name.startsWith('projects/${data.google_project.p0_jira_ai_site_acme.number}/secrets/${local.p0_jira_ai_site_acme_pool_secret_prefix}')"
  }
}

# Google checks secretmanager.secrets.create on the project, so this binding
# cannot carry the condition that the management binding below does.
resource "google_project_iam_custom_role" "p0_jira_ai_site_acme_token_create" {
  project     = local.p0_jira_ai_site_acme.vault_project_id
  role_id     = "p0JiraAi_${local.p0_jira_ai_site_acme_role_id}_create"
  title       = "P0 Jira site acme token secret creation"
  permissions = ["secretmanager.secrets.create"]
}

resource "google_project_iam_member" "p0_jira_ai_site_acme_token_create" {
  project = local.p0_jira_ai_site_acme.vault_project_id
  role    = google_project_iam_custom_role.p0_jira_ai_site_acme_token_create.name
  member  = "serviceAccount:${local.p0_jira_ai_site_acme.connector_service_account}"
}

resource "google_project_iam_custom_role" "p0_jira_ai_site_acme_token_manage" {
  project = local.p0_jira_ai_site_acme.vault_project_id
  role_id = "p0JiraAi_${local.p0_jira_ai_site_acme_role_id}_manage"
  title   = "P0 Jira site acme token secret management"
  permissions = [
    "secretmanager.secrets.delete",
    "secretmanager.secrets.get",
    "secretmanager.versions.add",
  ]
}

resource "google_project_iam_member" "p0_jira_ai_site_acme_token_manage" {
  project = local.p0_jira_ai_site_acme.vault_project_id
  role    = google_project_iam_custom_role.p0_jira_ai_site_acme_token_manage.name
  member  = "serviceAccount:${local.p0_jira_ai_site_acme.connector_service_account}"

  condition {
    title      = "P0 Jira site acme token secrets"
    expression = "resource.name.startsWith('projects/${data.google_project.p0_jira_ai_site_acme.number}/secrets/${local.p0_jira_ai_site_acme_token_secret_prefix}')"
  }
}

# Adds the site. P0 looks up its cloud ID from Atlassian. The first agent
# account's install check uses the admin client, so the grants come first.
resource "p0_jira_ai_site" "acme" {
  id        = local.p0_jira_ai_site_acme.id
  connector = p0_jira_ai_connector.example.id
  site_url  = "https://acme.atlassian.net"

  vault = {
    type       = "gcp-sm"
    project_id = local.p0_jira_ai_site_acme.vault_project_id
  }

  admin_client_id   = "AbCdEfGhIjKlMnOpQrStUvWxYz012345"
  admin_secret_name = local.p0_jira_ai_site_acme.admin_secret
  # The project roles P0 adds an agent account to, as your site names them.
  read_role  = "P0 agent read"
  write_role = "P0 agent write"

  depends_on = [
    google_secret_manager_secret_iam_member.p0_jira_ai_site_acme_admin_accessor,
    google_secret_manager_secret_iam_member.p0_jira_ai_site_acme_admin_viewer,
    google_project_iam_member.p0_jira_ai_site_acme_pool_accessor,
    google_project_iam_member.p0_jira_ai_site_acme_pool_viewer,
    google_project_iam_member.p0_jira_ai_site_acme_token_create,
    google_project_iam_member.p0_jira_ai_site_acme_token_manage,
  ]
}
```

<!-- schema generated by tfplugindocs -->
## Schema

### Required

- `admin_client_id` (String) The client ID of the OAuth client of an Atlassian service account that administers the site's projects.
- `admin_secret_name` (String) The name of the secret, in `vault`, that holds the admin client's secret as its whole value: the name alone, without its project or a version. Up to 255 letters, digits, underscores and hyphens. P0 never reads it. Only your connector does.
- `connector` (String) The `id` of the `p0_jira_ai_connector` that reaches this site. The connector must be installed.
- `id` (String) A name you choose for the site, such as `acme`. Agent accounts refer to the site by it, and a P0 policy's `site` filter matches it. Use lowercase letters, digits and hyphens, starting and ending with a letter or digit, up to 48 characters.
- `read_role` (String) The name of the project role P0 adds an agent account to for read access, such as `P0 agent read`. Use a role nothing else relies on.
- `site_url` (String) The address of your Jira Cloud site, such as `https://acme.atlassian.net`, with no path.
- `vault` (Attributes) The secret manager that holds the site's secrets: the admin client's secret, and each agent account's client secret. P0 never reads them. Only the connector does, as its service account, so it must be Google Secret Manager. (see [below for nested schema](#nestedatt--vault))
- `write_role` (String) The name of the project role P0 adds an agent account to for write access, such as `P0 agent write`. Use a role nothing else relies on.

### Read-Only

- `cloud_id` (String) Atlassian's ID for the site, which P0 looks up from the site's address when it adds the site.
- `state` (String) This item's install progress in the P0 application:
	- 'stage': The item has been staged for installation
	- 'configure': The item is available to be added to P0, and may be configured
	- 'installed': The item is fully installed

<a id="nestedatt--vault"></a>
### Nested Schema for `vault`

Required:

- `project_id` (String) The ID of the Google Cloud project that holds the secrets: one of your Google Secret Manager installations in P0. This provider has no resource for that installation, so install Google Secret Manager in the P0 app first.
- `type` (String) The secret manager. Only `gcp-sm` (Google Secret Manager) is supported.
