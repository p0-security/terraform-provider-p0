# Grants the P0 "Security Reviewer" role to a user. Security Reviewers can
# review policies and user access, and may be configured as approvers for access
# requests.
resource "p0_security_reviewer_user" "example" {
  email = "reviewer@example.com"
}
