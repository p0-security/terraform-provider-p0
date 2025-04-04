# Module which deploys the P0 AWS IAM Management Integration (incl. SSH)

terraform {
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "6.16.0"
    }
    p0 = {
      source  = "p0-security/p0"
      version = "0.14.0"
    }
  }
}

resource "p0_gcp" "org" {
  organization_id = var.gcp_organization_id
}

# Deploy the P0 GCP Security Perimeter
module "p0_gcp_security_perimeter" {
  source                = "../p0_gcp_security_perimeter"
  gcp_project_id        = var.gcp_project_id
  location              = var.location
  p0_project_id         = var.p0_project_id
  service_account_email = p0_gcp.org.service_account_email
  depends_on            = [p0_gcp.org]
}

# Stages the installation of the P0 GCP IAM Writer
# To import: terraform import module.p0_gcp_iam_management.p0_gcp_iam_write_staged.iam_write_staged p0-demo
resource "p0_gcp_iam_write_staged" "iam_write_staged" {
  project    = var.gcp_project_id
  depends_on = [p0_gcp.org]
}

# This custom role is required for P0 to manage IAM grants in your project
# To import: terraform import module.p0_gcp_iam_management.google_project_iam_custom_role.iam-manager-role projects/p0-demo/roles/p0IamManager
resource "google_project_iam_custom_role" "iam-manager-role" {
  project     = var.gcp_project_id
  role_id     = p0_gcp_iam_write_staged.iam_write_staged.custom_role.id
  title       = p0_gcp_iam_write_staged.iam_write_staged.custom_role.name
  description = "Role used by P0 to manage access to your GCP project"
  permissions = p0_gcp_iam_write_staged.iam_write_staged.permissions
}

# Grants the P0 IAM Manager role to the P0 Security Perimeter service account
resource "google_project_iam_member" "iam-manager-role-binding" {
  project = var.gcp_project_id
  role    = google_project_iam_custom_role.iam-manager-role.id
  member  = "serviceAccount:${p0_gcp.org.service_account_email}"
}

# Grants the Security Reviewer role to the P0 Security Perimeter service account
resource "google_project_iam_member" "security_reviewer_role_binding" {
  project = var.gcp_project_id
  role    = "roles/iam.securityReviewer"
  member  = "serviceAccount:${p0_gcp.org.service_account_email}"
}

# IAM Role for P0 IAM Writer
# To import: terraform import module.p0_gcp_iam_management.google_project_iam_custom_role.iam_writer_role projects/p0-demo/roles/p0IamWriter
resource "google_project_iam_custom_role" "iam_writer_role" {
  role_id     = "p0IamWriter"
  title       = "P0 IAM Writer"
  description = "Role used by p0 security perimeter service account to manage iam in p0-demo"
  project     = "p0-demo"
  stage       = "GA"

  permissions = [
    "bigquery.datasets.get",
    "bigquery.datasets.update",
    "iam.serviceAccounts.get",
    "resourcemanager.projects.get"
  ]
}

# Grants the IAM Writer role to the P0 Security Perimeter service account
resource "google_project_iam_member" "iam_writer_role_binding" {
  project = var.gcp_project_id
  role    = "projects/p0-demo/roles/p0IamWriter"
  member  = "serviceAccount:${p0_gcp.org.service_account_email}"
}

# Grants the Security Admin role to the P0 Security Perimeter service account
resource "google_project_iam_member" "security_admin_role_binding" {
  project = var.gcp_project_id
  role    = "roles/iam.securityAdmin"
  member  = "serviceAccount:${p0_gcp.org.service_account_email}"
}

# Finalizes the installation of the P0 GCP IAM Writer
resource "p0_gcp_iam_write" "iam_write" {
  project    = var.gcp_project_id
  depends_on = [
    p0_gcp_iam_write_staged.iam_write_staged,
    google_project_iam_custom_role.iam-manager-role,
    google_project_iam_member.iam-manager-role-binding,
    google_project_iam_member.security_reviewer_role_binding,
    google_project_iam_custom_role.iam_writer_role,
    google_project_iam_member.iam_writer_role_binding,
    google_project_iam_member.security_admin_role_binding,
    module.p0_gcp_security_perimeter
  ]
}

# Installs the GCP SSH integration
resource "p0_ssh_gcp" "ssh" {
  project_id      = var.gcp_project_id
  group_key       = var.gcp_group_key
  is_sudo_enabled = var.gcp_is_sudo_enabled
  depends_on      = [p0_gcp_iam_write.iam_write]
}
