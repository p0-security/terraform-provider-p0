# SSH on Google Cloud = JIT access to Compute Engine instances. P0 grants access
# via OS Login and brokers SSH through Identity-Aware Proxy (IAP) tunneling.
#
# Prerequisites: the root p0_gcp org install (below; see examples/resources/p0_gcp)
# and p0_gcp_iam_write for the project — SSH layers on the project's gcloud
# iam-write integration and fails unless it's already installed for the same
# project. The chain below reproduces examples/resources/p0_gcp_iam_write.

resource "p0_gcp" "example" {
  organization_id = "123456789012"
}

locals {
  # Google Cloud project whose instances P0 manages SSH access to.
  project_id = "my-project-id"
}

resource "p0_gcp_iam_write_staged" "example" {
  project    = local.project_id
  depends_on = [p0_gcp.example]
}

# Custom role required for P0 to manage IAM grants in your project.
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

# Predefined role required for P0 to grant resource-level access.
resource "google_project_iam_member" "example_predefined_role" {
  project = local.project_id
  role    = p0_gcp_iam_write_staged.example.predefined_role
  member  = "serviceAccount:${p0_gcp.example.service_account_email}"
}

# Completes the GCP IAM-management integration; install after the grants above.
resource "p0_gcp_iam_write" "example" {
  project = local.project_id
  depends_on = [
    google_project_iam_member.example_custom_role,
    google_project_iam_member.example_predefined_role,
  ]
}

# Example instance. P0 grants SSH via OS Login, so instances must enable OS Login.
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

  # group_key groups instances by this label's VALUE; the label KEY ("p0-group")
  # is the part after the "/" in group_key.
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

  # group_key must be "<project_id_or_org_id>/<key>". Matched by a resource-manager
  # tag with the full key, or an instance label named by the part after the "/".
  group_key       = "${local.project_id}/p0-group"
  is_sudo_enabled = true

  depends_on = [
    p0_gcp_iam_write.example,
    google_compute_instance.example,
  ]
}
