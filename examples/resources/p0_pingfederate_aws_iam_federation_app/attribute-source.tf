# Adds the PingFederate app acme-sso, whose PingFederate reads each user's AWS
# roles from P0 at sign-in. It needs no connector and no secret.
#
# P0 sets the token audience when it adds the app, so set up PingFederate
# after the first apply, with this app's outputs:
#
# - Under Access Token Management, add a JSON Web Tokens instance that uses
#   the centralized signing key. Set its Issuer Claim Value to the issuer
#   below, and its Audience Claim Value to the
#   pingfederate_attribute_source_audience output.
# - Map the instance for the client_credentials context, and add an OAuth
#   client with the client credentials grant that issues only its tokens.
# - Add a REST API data store whose base URL is the
#   pingfederate_attribute_source_roles_url output. Authenticate with OAuth 2.0
#   Bearer Token and Client Credentials, the client above and PingFederate's
#   token endpoint, https://sso.example.com:9031/as/token.oauth2. Map the local
#   attribute roles to /roles.
# - On the SP connection to urn:amazon:webservices, add the data store as an
#   attribute source with the resource path /apps/acme-sso/roles?email=${mail},
#   and fulfill https://aws.amazon.com/SAML/Attributes/Role from its roles.
resource "p0_pingfederate_aws_iam_federation_app" "attribute_source_example" {
  id               = "acme-sso"
  pingfederate_url = "https://sso.example.com:9031"

  attribute_source = {
    # PingFederate's System > Server > Protocol Settings > Federation Info.
    entity_id = "urn:acme:pingfederate"
    issuer    = "urn:acme:pingfederate:p0"
    # Unset, P0 reads https://sso.example.com:9031/pf/JWKS. To paste the keys
    # instead, and paste them again after each key rotation:
    # jwks = trimspace(file("${path.module}/pingfederate-jwks.json"))
  }
}

output "pingfederate_attribute_source_audience" {
  value = p0_pingfederate_aws_iam_federation_app.attribute_source_example.attribute_source.audience
}

output "pingfederate_attribute_source_roles_url" {
  value = p0_pingfederate_aws_iam_federation_app.attribute_source_example.attribute_source.roles_url
}
