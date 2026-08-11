resource "p0_agentic_identity_provider" "example" {
  id                   = "github-actions"
  issuer               = "https://token.actions.githubusercontent.com"
  audience_pattern     = "https://github.com/my-org/*"
  subject_pattern      = "repo:my-org/*"
  dynamic_registration = true
}
