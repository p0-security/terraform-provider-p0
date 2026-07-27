# Grants the P0 "Owner" role to every member of a directory group.
resource "p0_owner_group" "example" {
  group = "eng-admins"
}
