# Grants the P0 "Owner" role to a user. Owners can add integrations and alter
# organization settings.
resource "p0_owner_user" "example" {
  email = "owner@example.com"
}
