# Google Secret Manager requires Google Cloud IAM management installed for the same project first.
# See p0_gcp_iam_write for that resource.
resource "p0_gcp_secret_manager" "example" {
  project = p0_gcp_iam_write.example.project
}
