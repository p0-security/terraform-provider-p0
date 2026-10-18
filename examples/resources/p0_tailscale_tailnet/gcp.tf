# The Google Cloud variant of the p0_tailscale_tailnet example: the tailnet's
# connector runs on Cloud Run (see the p0_tailscale_connector example's Google
# Cloud Run variant, gcp.tf), and the tailnet is in posture mode, for a policy
# file that a GitOps pipeline or tailscale_acl owns.
#
# The federated identity trusts the Google ID tokens of the connector's service
# account. A Google ID token's subject is the service account's unique numeric
# ID, not its email, so the data source looks the ID up.

locals {
  p0_tailscale_tailnet_t5678cntrl = {
    tailnet_id = "T5678CNTRL"
    # The scopes the connector needs in posture mode, where it never writes the
    # policy file.
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

# Posture mode is for a policy file that nothing edits in the admin console. This
# turns on "prevent edits in the admin console", which P0 checks when it
# validates the tailnet.
resource "tailscale_tailnet_settings" "p0_gcp" {
  provider = tailscale.p0_tailscale_tailnet_t5678cntrl

  acls_externally_managed_on = true
}

# The policy file, which tailscale_acl replaces whole on every apply, so keep
# all of the tailnet's rules here. It defines the P0 posture that the posture
# destination in the p0_tailscale_destination example names, and gates a grant
# on it with srcPosture: a device gets the grant while P0 sets
# custom:p0_prod_db on it.
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
  # The policy file gates grants on P0 postures, and P0 only sets device
  # attributes. Its destinations are postures (see the p0_tailscale_destination
  # example).
  mode = "posture"

  depends_on = [tailscale_tailnet_settings.p0_gcp]
}
