# Installs GCP CloudSQL JIT access via GCP IAM auth (PostgreSQL only; MySQL
# unsupported), reached through a Cloud Run connector with direct VPC access.
# Requires the root p0_gcp install plus p0_gcp_iam_write on the same project
# (its custom-role and grant sub-chain: see the p0_gcp_iam_write example).

resource "p0_gcp" "example" {
  organization_id = "123456789012"
}

locals {
  # Staged and final CloudSQL installs must target the same project.
  project = "my-project-id"
  region  = "us-west1"
}

resource "p0_gcp_iam_write" "example" {
  project    = local.project
  depends_on = [p0_gcp.example]
}

# APIs the connector and CloudSQL require.
resource "google_project_service" "enable_services" {
  for_each = toset([
    "compute.googleapis.com",
    "iam.googleapis.com",
    "run.googleapis.com",
    "servicenetworking.googleapis.com",
    "sqladmin.googleapis.com",
  ])
  service            = each.key
  project            = local.project
  disable_on_destroy = false
}

# VPC/subnet the instances live on; the connector gets direct VPC egress here.
# Restricted orgs may need to grant the Cloud Run service agent VPC access:
# https://docs.cloud.google.com/run/docs/configuring/vpc-direct-vpc#direct-vpc-iam
resource "google_compute_network" "example" {
  name                    = "p0-cloudsql-vpc"
  project                 = local.project
  auto_create_subnetworks = false
}

resource "google_compute_subnetwork" "example" {
  name          = "p0-cloudsql-subnet"
  project       = local.project
  region        = local.region
  network       = google_compute_network.example.id
  ip_cidr_range = "10.0.0.0/24"
}

# Stage the install to get the connector identifiers the Cloud Run deploy needs.
resource "p0_gcp_cloudsql_staged" "example" {
  id         = google_compute_network.example.name
  project_id = local.project
  subnetwork = google_compute_subnetwork.example.name
  depends_on = [p0_gcp_iam_write.example]
}

# p0-security/p0-connector/google: deploys the connector image, creates its
# runtime service account, and grants P0's SA the Cloud Run invoker role.
module "gcp_cloudsql_vpc" {
  source  = "p0-security/p0-connector/google"
  version = "0.0.3"

  project_id                     = local.project
  service                        = "cloudsql"
  connector_name                 = p0_gcp_cloudsql_staged.example.connector_service_name
  connector_service_account_name = split("@", p0_gcp_cloudsql_staged.example.connector_service_account)[0]
  region                         = p0_gcp_cloudsql_staged.example.region
  vpc_network                    = google_compute_network.example.name
  vpc_subnetwork                 = google_compute_subnetwork.example.name
  invoker_service_account_email  = p0_gcp.example.service_account_email

  depends_on = [google_project_service.enable_services]
}

# Needs cloudsql.admin, not cloudsql.client, to provision IAM DB users via the
# CloudSQL Admin API (client lacks cloudsql.users.create).
resource "google_project_iam_member" "connector_cloudsql_admin" {
  project    = local.project
  role       = "roles/cloudsql.admin"
  member     = "serviceAccount:${p0_gcp_cloudsql_staged.example.connector_service_account}"
  depends_on = [module.gcp_cloudsql_vpc]
}

# Lets the connector log in to CloudSQL as this SA via IAM auth.
resource "google_project_iam_member" "connector_instance_user" {
  project    = local.project
  role       = "roles/cloudsql.instanceUser"
  member     = "serviceAccount:${p0_gcp_cloudsql_staged.example.connector_service_account}"
  depends_on = [module.gcp_cloudsql_vpc]
}

# IAM authentication must be on so P0 can grant JIT access, and the instance
# must be reachable from the connector's VPC.
resource "google_sql_database_instance" "example" {
  name                = "p0-cloudsql-example"
  project             = local.project
  region              = local.region
  database_version    = "POSTGRES_15"
  deletion_protection = false

  settings {
    tier = "db-custom-1-3840"

    ip_configuration {
      ipv4_enabled    = false
      private_network = google_compute_network.example.id
    }

    database_flags {
      name  = "cloudsql.iam_authentication"
      value = "on"
    }
  }
}

# Register the connector's SA as an IAM database user on the instance.
resource "google_sql_user" "connector" {
  name       = trimsuffix(p0_gcp_cloudsql_staged.example.connector_service_account, ".gserviceaccount.com")
  instance   = google_sql_database_instance.example.name
  project    = local.project
  type       = "CLOUD_IAM_SERVICE_ACCOUNT"
  depends_on = [module.gcp_cloudsql_vpc]
}

# Completes the install; creating it verifies the connector is reachable.
resource "p0_gcp_cloudsql" "example" {
  id         = p0_gcp_cloudsql_staged.example.id
  project_id = p0_gcp_cloudsql_staged.example.project_id
  subnetwork = p0_gcp_cloudsql_staged.example.subnetwork
  depends_on = [
    module.gcp_cloudsql_vpc,
    google_project_iam_member.connector_cloudsql_admin,
    google_project_iam_member.connector_instance_user,
  ]
}
