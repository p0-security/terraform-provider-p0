# Grants the P0 "Assessment Viewer" role to every member of a directory group.
resource "p0_assessment_viewer_group" "example" {
  group = "auditors"
}
