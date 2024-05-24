
resource "p0_gcp" "example" {
  organization_id = "123456789012"
}

# Follow instructions for creating Terraform for IAM management in p0_gcp_iam_write documentation
# ...

resource "p0_gcp_iam_write" "example" {
  project = locals.project
  depends_on = [
    google_project_iam_member.example_custom_role,
    google_project_iam_member.example_predefined_role
  ]
}

# Prevents P0 from assigning grants outside your domain
resource "google_org_policy_policy" "example" {
  name   = "projects/my_project/policies/iam.allowedPolicyMemberDomains"
  parent = "projects/my_project"

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

# Finish the P0 sharing-restriction installation
resource "p0_gcp_sharing_restriction" "example" {
  project = p0_gcp_iam_write.example.project
  depends_on = [
    google_org_policy_policy.example
  ]
}
