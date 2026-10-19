---
page_title: "p0_jira_ai_pool_account Resource - p0"
subcategory: ""
description: |-
  An agent account: an Atlassian service account that P0 leases to one agent session at a time on a Jira site. When an agent is granted access to a project, P0 has the site's connector add the account to the project's role, and remove it when the access ends. Add one for each agent session the site should serve at once.
  P0 never holds the account's API key. When P0 adds the account it names the secret that holds it, secret_name, in the site's vault. Create that secret and store the key in it, outside Terraform, which keeps it out of Terraform's state. Only the connector reads it, under the grant the site's Terraform gives it.
  Important: Install the site first, in an earlier apply or in the same one with this resource referring to it, as the example does. Creating this resource makes P0 look up the account's Atlassian ID through the site's connector, as the site's admin service account. That also checks the site's admin client. If the check fails, the apply fails with P0's message.
  An agent account's attributes can't change once it is created, so changing any of them replaces the account. To rotate the API key, add a new version to the same secret, which needs no change here.
  An agent account that already exists in P0, such as one added in the P0 app, isn't created again: the apply fails, and says to import it with terraform import. An imported account that P0 hasn't finished installing plans an update, and applying it finishes the install.
  Prerequisite: an Atlassian service account that is a member of the site, with an API key, and no project roles of its own: P0 gives it each role an agent is granted.
  Note: This integration is currently in beta.
---

# p0_jira_ai_pool_account (Resource)

An agent account: an Atlassian service account that P0 leases to one agent session at a time on a Jira site. When an agent is granted access to a project, P0 has the site's connector add the account to the project's role, and remove it when the access ends. Add one for each agent session the site should serve at once.

P0 never holds the account's API key. When P0 adds the account it names the secret that holds it, `secret_name`, in the site's vault. Create that secret and store the key in it, outside Terraform, which keeps it out of Terraform's state. Only the connector reads it, under the grant the site's Terraform gives it.

**Important:** Install the site first, in an earlier apply or in the same one with this resource referring to it, as the example does. Creating this resource makes P0 look up the account's Atlassian ID through the site's connector, as the site's admin service account. That also checks the site's admin client. If the check fails, the apply fails with P0's message.

An agent account's attributes can't change once it is created, so changing any of them replaces the account. To rotate the API key, add a new version to the same secret, which needs no change here.

An agent account that already exists in P0, such as one added in the P0 app, isn't created again: the apply fails, and says to import it with `terraform import`. An imported account that P0 hasn't finished installing plans an update, and applying it finishes the install.

**Prerequisite:** an Atlassian service account that is a member of the site, with an API key, and no project roles of its own: P0 gives it each role an agent is granted.

**Note:** This integration is currently in beta.

## Example Usage

```terraform
# Adds an Atlassian service account that P0 leases to agent sessions on a
# site. P0 names the secret its API key goes in, as secret_name.
# See gcp.tf in this directory for creating that secret.
resource "p0_jira_ai_pool_account" "example" {
  id    = "agent-1"
  site  = p0_jira_ai_site.example.id
  email = "p0-agent-1@serviceaccount.atlassian.com"
}
```

### Google Cloud

```terraform
# Adds two agent accounts on the site acme (see gcp.tf in the p0_jira_ai_site
# example), and creates the secret each one's API key goes in, in
# the site's vault project. The site's grants already let its connector read
# these secrets, by their prefix.
#
# Creating an account has P0 look its Atlassian ID up through the site's
# connector, so the site must be installed, and its admin service account must
# be able to see the account.

locals {
  # Each account's email, by the account's ID.
  p0_jira_ai_pool_accounts = {
    "agent-1" = "p0-agent-1@serviceaccount.atlassian.com"
    "agent-2" = "p0-agent-2@serviceaccount.atlassian.com"
  }
}

resource "p0_jira_ai_pool_account" "acme" {
  for_each = local.p0_jira_ai_pool_accounts

  id    = each.key
  site  = p0_jira_ai_site.acme.id
  email = each.value
}

# The secret P0 names for each account, secret_name on the account. Its name is
# fixed by the site's and the account's IDs, so it's written out here rather than
# read from secret_name: replacing an account would otherwise make the name
# unknown at plan time and replace the secret, deleting the stored API key.
#
# This creates the secret alone, with no value, which keeps the API key out of
# Terraform's state. Store it as the secret's first version:
#
#   read -rs JIRA_API_KEY
#   printf %s "$JIRA_API_KEY" | gcloud secrets versions add p0_install_jira-ai_acme_agent-1_api-key \
#     --project my-secrets-project-id --data-file=-
resource "google_secret_manager_secret" "p0_jira_ai_pool_account_acme" {
  for_each = local.p0_jira_ai_pool_accounts

  project   = p0_jira_ai_site.acme.vault.project_id
  secret_id = "p0_install_jira-ai_${p0_jira_ai_site.acme.id}_${each.key}_api-key"

  replication {
    auto {}
  }

  lifecycle {
    # Stops the apply if P0 ever names the secret differently.
    postcondition {
      condition     = self.secret_id == p0_jira_ai_pool_account.acme[each.key].secret_name
      error_message = "P0 names this account's secret ${p0_jira_ai_pool_account.acme[each.key].secret_name}. Rename this secret to match."
    }
  }
}
```

<!-- schema generated by tfplugindocs -->
## Schema

### Required

- `email` (String) The service account's email, as Atlassian shows it.
- `id` (String) A name you choose for the account, such as `agent-1`. Use lowercase letters, digits and hyphens, starting and ending with a letter or digit, up to 63 characters.
- `site` (String) The `id` of the `p0_jira_ai_site` this account works on. The site must be installed.

### Read-Only

- `account_id` (String) Atlassian's ID for the account, which P0 looks up through the site's connector when it checks the install.
- `secret_name` (String) The name of the secret in the site's vault that holds the account's API key. P0 names it when it adds the account: `p0_install_jira-ai_<site>_<id>_api-key`. Create it, and store the key as its whole value.
- `state` (String) This item's install progress in the P0 application:
	- 'stage': The item has been staged for installation
	- 'configure': The item is available to be added to P0, and may be configured
	- 'installed': The item is fully installed
