# Adds two agent accounts on the site acme (see gcp.tf in the p0_jira_ai_site
# example), and creates the secret each one's OAuth client secret goes in, in
# the site's vault project. The site's grants already let its connector read
# these secrets, by their prefix.
#
# Creating an account has P0 look its Atlassian ID up through the site's
# connector, so the site must be installed, and its admin service account must
# be able to see the account.

locals {
  # Each account's email and OAuth client ID, by the account's ID.
  p0_jira_ai_pool_accounts = {
    "agent-1" = {
      email     = "p0-agent-1@serviceaccount.atlassian.com"
      client_id = "ZyXwVuTsRqPoNmLkJiHgFeDcBa543210"
    }
    "agent-2" = {
      email     = "p0-agent-2@serviceaccount.atlassian.com"
      client_id = "QwErTyUiOpAsDfGhJkLzXcVbNm678901"
    }
  }
}

resource "p0_jira_ai_pool_account" "acme" {
  for_each = local.p0_jira_ai_pool_accounts

  id        = each.key
  site      = p0_jira_ai_site.acme.id
  email     = each.value.email
  client_id = each.value.client_id
}

# The secret P0 names for each account, secret_name on the account. Its name is
# fixed by the site's and the account's IDs, so it's written out here rather than
# read from secret_name: replacing an account would otherwise make the name
# unknown at plan time and replace the secret, deleting the stored client secret.
#
# This creates the secret alone, with no value, which keeps the client's secret
# out of Terraform's state. Store it as the secret's first version:
#
#   read -rs JIRA_CLIENT_SECRET
#   printf %s "$JIRA_CLIENT_SECRET" | gcloud secrets versions add p0_install_jira-ai_acme_agent-1_oauth-client \
#     --project my-secrets-project-id --data-file=-
resource "google_secret_manager_secret" "p0_jira_ai_pool_account_acme" {
  for_each = local.p0_jira_ai_pool_accounts

  project   = p0_jira_ai_site.acme.vault.project_id
  secret_id = "p0_install_jira-ai_${p0_jira_ai_site.acme.id}_${each.key}_oauth-client"

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
