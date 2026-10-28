# Adds the tailnet T1234CNTRL through the connector on AWS Lambda that the
# p0_tailscale_connector example installs. The tailnet needs a Tailscale Premium
# or Enterprise plan, and a policy file that gates grants on P0 postures, as the
# Google Cloud example's does. The connector signs in with a Tailscale federated
# identity that trusts the connector's role, so no secret is stored anywhere. For
# a connector on Cloud Run, see the Google Cloud example (gcp.tf). For an OAuth
# client whose secret is in your secret manager, see the OAuth client example
# (oauth-client.tf).
#
# Tailscale's own Terraform provider creates the federated identity, signed in to
# the same tailnet with a credential of yours that may create trust credentials.
# The connector's own Terraform turns on AWS outbound identity federation and
# lets the connector's role mint the identity tokens.
#
# This example goes in the same configuration as the p0_tailscale_connector
# example, and uses its required_providers block. If your configuration already
# has a required_providers block, add these to it:
#   p0 = {
#     source  = "p0-security/p0"
#     version = "~> 0.86"
#   }
#   tailscale = {
#     source  = "tailscale/tailscale"
#     version = "~> 0.29.2"
#   }

locals {
  p0_tailscale_tailnet_t1234cntrl = {
    # The Tailnet ID from Tailscale's Settings, General, not the tailnet's DNS
    # name.
    tailnet_id = "T1234CNTRL"
    # The scopes the connector needs, which never let it write the policy file.
    # A federated identity's tokens carry all of them.
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

# Set TAILSCALE_OAUTH_CLIENT_ID and TAILSCALE_OAUTH_CLIENT_SECRET, or the
# provider's other credential settings, for your own credential.
provider "tailscale" {
  tailnet = local.p0_tailscale_tailnet_t1234cntrl.tailnet_id
}

# Trusts the AWS identity tokens that the connector's role mints. Leave audience
# unset: P0 uses Tailscale's default, api.tailscale.com/<client ID>.
resource "tailscale_federated_identity" "p0" {
  description = "P0 connector prod"
  issuer      = aws_iam_outbound_web_identity_federation.p0_tailscale_connector_prod.issuer_identifier
  subject     = p0_tailscale_connector.example.connector_identity.role_arn
  scopes      = local.p0_tailscale_tailnet_t1234cntrl.scopes
}

# Adds the tailnet. Creating it has P0 validate the install through the
# connector, so it waits until the connector's role may mint the token. IAM
# changes can take a few seconds to propagate, so if the connector is denied,
# apply again.
resource "p0_tailscale_tailnet" "example" {
  id        = local.p0_tailscale_tailnet_t1234cntrl.tailnet_id
  connector = p0_tailscale_connector.example.id

  credential = {
    type      = "federated"
    client_id = tailscale_federated_identity.p0.id
  }

  depends_on = [aws_iam_role_policy.p0_tailscale_connector_prod_identity_token]
}

# What P0 found on the tailnet when it validated it.
output "tailscale_discovery" {
  value = p0_tailscale_tailnet.example.discovery
}
