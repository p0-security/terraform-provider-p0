# Grants the P0 "Assessment User" role to a user. Assessment Users can run,
# manage, and view IAM assessments.
resource "p0_assessment_user" "example" {
  email = "assessor@example.com"
}
