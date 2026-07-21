# Installing SSH on Google Cloud lets P0 manage just-in-time access to your
# Compute Engine instances. P0 grants access via OS Login and brokers the SSH
# connection through Identity-Aware Proxy (IAP) tunneling.
#
# Prerequisites:
#   - The root `p0_gcp` organization install (below). See examples/resources/p0_gcp
#     for the full root-install pattern.
#   - The Google Cloud IAM-management install (`p0_gcp_iam_write`) for the project.
#     The SSH install is layered on top of the project's `gcloud` iam-write
#     integration and fails to configure unless that integration is already
#     installed for the same project. The `p0_gcp_iam_write_staged` ->
#     custom role + role bindings -> `p0_gcp_iam_write` chain below reproduces the
#     pattern from examples/resources/p0_gcp_iam_write.

resource "p0_gcp" "example" {
  organization_id = "123456789012"
}

locals {
  # The Google Cloud project whose instances P0 manages SSH access to.
  project_id = "my-project-id"
}

# --- Prerequisite: the project must be connected to P0 via the GCP
# IAM-management install. p0_ssh_gcp is keyed to this project's `gcloud`
# iam-write integration, so this chain (p0_gcp_iam_write_staged -> custom role +
# bindings -> p0_gcp_iam_write) is the P0 prerequisite for the SSH install. ---

resource "p0_gcp_iam_write_staged" "example" {
  project    = local.project_id
  depends_on = [p0_gcp.example]
}

# This custom role is required for P0 to manage IAM grants in your project.
resource "google_project_iam_custom_role" "example" {
  project     = local.project_id
  role_id     = p0_gcp_iam_write_staged.example.custom_role.id
  title       = p0_gcp_iam_write_staged.example.custom_role.name
  description = "Integration role for P0 IAM management integration"
  permissions = p0_gcp_iam_write_staged.example.permissions
}

resource "google_project_iam_member" "example_custom_role" {
  project = local.project_id
  role    = google_project_iam_custom_role.example.id
  member  = "serviceAccount:${p0_gcp.example.service_account_email}"
}

# The predefined role is required for P0 to grant resource-level access.
resource "google_project_iam_member" "example_predefined_role" {
  project = local.project_id
  role    = p0_gcp_iam_write_staged.example.predefined_role
  member  = "serviceAccount:${p0_gcp.example.service_account_email}"
}

# Completes the GCP IAM-management integration. Must be installed _after_ the P0
# service account is granted the roles above.
resource "p0_gcp_iam_write" "example" {
  project = local.project_id
  depends_on = [
    google_project_iam_member.example_custom_role,
    google_project_iam_member.example_predefined_role,
  ]
}

# An example Compute Engine instance managed by this integration. P0 grants SSH
# via OS Login, so instances must have OS Login enabled.
resource "google_compute_instance" "example" {
  name         = "p0-ssh-example"
  project      = local.project_id
  machine_type = "e2-micro"
  zone         = "us-west1-a"

  boot_disk {
    initialize_params {
      image = "debian-cloud/debian-12"
    }
  }

  network_interface {
    network = "default"
  }

  metadata = {
    enable-oslogin = "TRUE"
  }

  # `group_key` (below) groups instances by the value of this label, so access
  # can be requested to all instances sharing a label value in one request. The
  # label KEY here ("p0-group") is the part after the "/" in group_key.
  labels = {
    p0-group = "dev-servers"
  }
}

# P0 brokers SSH through IAP tunneling. Allow SSH from the IAP forwarding range.
resource "google_compute_firewall" "allow_iap_ssh" {
  name    = "p0-allow-iap-ssh"
  project = local.project_id
  network = "default"

  allow {
    protocol = "tcp"
    ports    = ["22"]
  }

  source_ranges = ["35.235.240.0/20"]
}

resource "p0_ssh_gcp" "example" {
  project_id = local.project_id

  # Google Cloud group keys must be formatted as "<project_id_or_org_id>/<key>".
  # Instances are matched either by a resource-manager tag with this full key, or
  # by an instance label named by the part after the "/" (here, "p0-group").
  group_key       = "${local.project_id}/p0-group"
  is_sudo_enabled = true

  depends_on = [
    p0_gcp_iam_write.example,
    google_compute_instance.example,
  ]
}
