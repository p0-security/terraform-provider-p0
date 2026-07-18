# AWS RDS example
resource "p0_postgres_legacy" "rds_example" {
  id            = "production-db"
  database_name = "app_production"
  install_type = {
    type        = "rds"
    account     = p0_aws_iam_write.production.id
    region      = "us-east-1"
    resource_id = "db-ABCDEFGHIJKLMNOPQRSTUVWXYZ"
    instance    = "production-db"
    hostname    = "production-db.xxxxxxxxxx.us-east-1.rds.amazonaws.com"
    port        = "5432"
    connectivity = {
      type = "public"
    }
  }
}

# GCP Cloud SQL example
resource "p0_postgres_legacy" "cloudsql_example" {
  id            = "staging-db"
  database_name = "app_staging"
  install_type = {
    type        = "cloud-sql"
    project_id  = p0_gcp_iam_write.staging.id
    region      = "us-central1"
    instance_id = "staging-db"
  }
}
