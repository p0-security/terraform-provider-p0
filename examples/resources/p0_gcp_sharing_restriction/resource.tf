resource "p0_gcp" "example" {
  organization_id = "123456789012"
}

locals {
  project = "my-project-id"
}

# Sharing restriction requires a completed p0_gcp_iam_write; that chain is reproduced below (see its docs).
resource "p0_gcp_iam_write_staged" "example" {
  project    = local.project
  depends_on = [p0_gcp.example]
}

# Custom role required for P0 to manage IAM grants in your project.
resource "google_project_iam_custom_role" "example" {
  project     = local.project
  role_id     = p0_gcp_iam_write_staged.example.custom_role.id
  title       = p0_gcp_iam_write_staged.example.custom_role.name
  description = "Integration role for P0 IAM management integration"
  permissions = p0_gcp_iam_write_staged.example.permissions
}

resource "google_project_iam_member" "example_custom_role" {
  project = local.project
  role    = google_project_iam_custom_role.example.id
  member  = "serviceAccount:${p0_gcp.example.service_account_email}"
}

# Predefined role required for P0 to grant resource-level access.
resource "google_project_iam_member" "example_predefined_role" {
  project = local.project
  role    = p0_gcp_iam_write_staged.example.predefined_role
  member  = "serviceAccount:${p0_gcp.example.service_account_email}"
}

# p0_gcp_iam_write fails validation unless installed after the grants above.
resource "p0_gcp_iam_write" "example" {
  project = local.project
  depends_on = [
    google_project_iam_member.example_custom_role,
    google_project_iam_member.example_predefined_role
  ]
}

# Restrict P0 grants to your domain.
resource "google_org_policy_policy" "example" {
  name   = "projects/${local.project}/policies/iam.allowedPolicyMemberDomains"
  parent = "projects/${local.project}"

  spec {
    rules {
      values {
        allowed_values = [
          "is:principalSet://iam.googleapis.com/organizations/${p0_gcp.example.organization_id}"
        ]
      }
    }
  }
}

# P0 verifies the restriction with a live self-grant test that intermittently
# fails until the org-policy change propagates (~1 min); wait before installing.
resource "time_sleep" "wait_for_org_policy" {
  depends_on      = [google_org_policy_policy.example]
  create_duration = "120s"
}

resource "p0_gcp_sharing_restriction" "example" {
  project = p0_gcp_iam_write.example.project
  depends_on = [
    time_sleep.wait_for_org_policy
  ]
}
