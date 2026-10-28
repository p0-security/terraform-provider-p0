# The Google Cloud variant of the p0_tailscale_tailnet example: the tailnet's
# connector runs on Cloud Run (see the p0_tailscale_connector example's Google
# Cloud Run variant, gcp.tf), and tailscale_acl owns the tailnet's policy file,
# which gates grants on P0 postures.
#
# The federated identity trusts the Google ID tokens of the connector's service
# account. A Google ID token's subject is the service account's unique numeric
# ID, not its email, so the data source looks the ID up.

locals {
  p0_tailscale_tailnet_t5678cntrl = {
    tailnet_id = "T5678CNTRL"
    # The scopes the connector needs, which never let it write the policy file.
    scopes = [
      "policy_file:read",
      "users:read",
      "devices:core:read",
      "devices:posture_attributes",
      "devices:routes:read",
      "services:read",
    ]
  }
}

provider "tailscale" {
  alias   = "p0_tailscale_tailnet_t5678cntrl"
  tailnet = local.p0_tailscale_tailnet_t5678cntrl.tailnet_id
}

data "google_service_account" "p0_tailscale_connector_gcp" {
  account_id = p0_tailscale_connector.gcp_example.connector_identity.service_account_email
}

# Leave audience unset: P0 uses Tailscale's default, api.tailscale.com/<client ID>.
resource "tailscale_federated_identity" "p0_gcp" {
  provider = tailscale.p0_tailscale_tailnet_t5678cntrl

  description = "P0 connector prod-gcp"
  issuer      = "https://accounts.google.com"
  subject     = data.google_service_account.p0_tailscale_connector_gcp.unique_id
  scopes      = local.p0_tailscale_tailnet_t5678cntrl.scopes
}

# tailscale_acl owns the policy file, so this turns on "prevent edits in the
# admin console" to keep edits made there from being lost on the next apply.
resource "tailscale_tailnet_settings" "p0_gcp" {
  provider = tailscale.p0_tailscale_tailnet_t5678cntrl

  acls_externally_managed_on = true
}

# The policy file, which tailscale_acl replaces whole on every apply, so keep
# all of the tailnet's rules here. It defines a P0 posture that requestors can
# ask for, and gates a grant on it with srcPosture: a device gets the grant
# while P0 sets custom:p0_prod_db on it.
resource "tailscale_acl" "gitops" {
  provider = tailscale.p0_tailscale_tailnet_t5678cntrl

  overwrite_existing_content = true
  acl = jsonencode({
    tagOwners = {
      "tag:prod-db" = ["autogroup:admin"]
    }
    postures = {
      "posture:p0_prod_db" = ["custom:p0_prod_db == true"]
    }
    grants = [{
      src        = ["autogroup:member"]
      dst        = ["tag:prod-db"]
      ip         = ["tcp:5432"]
      srcPosture = ["posture:p0_prod_db"]
    }]
  })
}

# Adds the tailnet. Creating it has P0 validate the install through the
# connector: the connector exchanges its own ID token for a Tailscale token.
resource "p0_tailscale_tailnet" "gcp_example" {
  id        = local.p0_tailscale_tailnet_t5678cntrl.tailnet_id
  connector = p0_tailscale_connector.gcp_example.id

  credential = {
    type      = "federated"
    client_id = tailscale_federated_identity.p0_gcp.id
  }

  # P0 counts the P0 postures the policy file defines when it validates the
  # tailnet.
  depends_on = [tailscale_acl.gitops]
}
