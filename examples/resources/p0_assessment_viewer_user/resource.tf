# Grants the P0 "Assessment Viewer" role to a user. Assessment Viewers can view
# IAM assessments.
resource "p0_assessment_viewer_user" "example" {
  email = "auditor@example.com"
}
