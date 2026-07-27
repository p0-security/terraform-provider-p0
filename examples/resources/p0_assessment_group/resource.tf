# Grants the P0 "Assessment User" role to every member of a directory group.
resource "p0_assessment_group" "example" {
  group = "iam-assessors"
}
