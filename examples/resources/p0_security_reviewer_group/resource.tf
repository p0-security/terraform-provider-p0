# Grants the P0 "Security Reviewer" role to every member of a directory group.
resource "p0_security_reviewer_group" "example" {
  group = "security-team"
}
