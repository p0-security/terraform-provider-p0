resource "p0_gcp" "example" {
  organization_id = "123456789012"
}

locals {
  project = "my-project-id"
}

# The sharing-restriction install requires a completed `p0_gcp_iam_write`
# install; the blocks below reproduce that installation chain (see the
# `p0_gcp_iam_write` documentation for details)
resource "p0_gcp_iam_write_staged" "example" {
  project    = local.project
  depends_on = [p0_gcp.example]
}

# This custom role is required for P0 to manage IAM grants in your project
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

# The predefined role is required for P0 to grant resource-level access in your project
resource "google_project_iam_member" "example_predefined_role" {
  project = local.project
  role    = p0_gcp_iam_write_staged.example.predefined_role
  member  = "serviceAccount:${p0_gcp.example.service_account_email}"
}

# The `p0_gcp_iam_write` resource will fail to validate unless it is installed
# _after_ the P0 service account is granted the above roles
resource "p0_gcp_iam_write" "example" {
  project = local.project
  depends_on = [
    google_project_iam_member.example_custom_role,
    google_project_iam_member.example_predefined_role
  ]
}

# Prevents P0 from assigning grants outside your domain
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

# Org-policy changes take up to a minute to propagate. P0 verifies the sharing
# restriction with a live self-grant test, which intermittently fails if the
# domain-restricted sharing policy is not yet in effect. Wait for propagation
# before completing the install.
resource "time_sleep" "wait_for_org_policy" {
  depends_on      = [google_org_policy_policy.example]
  create_duration = "120s"
}

# Finish the P0 sharing-restriction installation
resource "p0_gcp_sharing_restriction" "example" {
  project = p0_gcp_iam_write.example.project
  depends_on = [
    time_sleep.wait_for_org_policy
  ]
}
