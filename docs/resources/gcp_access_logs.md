---
# generated by https://github.com/hashicorp/terraform-plugin-docs
page_title: "p0_gcp_access_logs Resource - p0"
subcategory: ""
description: |-
  An installation of P0, on a single Google Cloud project, for access-log collection,
  which enhances IAM assessment.
  To use this resource, you must also:
  install the p0_gcp_iam_assessment resource, andgrant P0 the ability to create logging sinks in your project.
  Use the read-only attributes defined on p0_gcp to create the requisite Google Cloud infrastructure.
  P0 recommends defining this infrastructure according to the example usage pattern.
---

# p0_gcp_access_logs (Resource)

An installation of P0, on a single Google Cloud project, for access-log collection,
which enhances IAM assessment.

To use this resource, you must also:
- install the `p0_gcp_iam_assessment` resource, and
- grant P0 the ability to create logging sinks in your project.

Use the read-only attributes defined on `p0_gcp` to create the requisite Google Cloud infrastructure.

P0 recommends defining this infrastructure according to the example usage pattern.

## Example Usage

```terraform
resource "p0_gcp" "example" {
  organization_id = "123456789012"
}

locals {
  project = "my_project_id"
}

# Follow instructions for creating Terraform for IAM assessment in p0_gcp_iam_assessment documentation
# ...

resource "p0_gcp_iam_assessment" "example" {
  project    = locals.project
  depends_on = [google_project_iam_member.example]
}

resource "google_project_iam_audit_config" "example" {
  project = locals.project
  service = "allServices"
  audit_log_config {
    log_type = "ADMIN_READ"
  }
  audit_log_config {
    log_type = "DATA_READ"
  }
  audit_log_config {
    log_type = "DATA_WRITE"
  }
}

resource "google_project_iam_custom_role" "example" {
  project     = locals.project
  role_id     = p0_gcp.example.access_logs.custom_role.id
  title       = p0_gcp.example.access_logs.custom_role.name
  permissions = p0_gcp.example.access_logs.permissions
}

# Grants the logging service account permission to write to the access-logging Pub/Sub topic
resource "google_project_iam_member" "example" {
  project = locals.project
  role    = google_project_iam_custom_role.example.name
  member  = "serviceAccount:${p0_gcp.example.service_account_email}"
}

# Finish the P0 access-logs installation
resource "p0_gcp_access_logs" "example" {
  project = locals.project
  depends_on = [
    google_project_iam_audit_config.example,
    google_project_iam_member.example
  ]
}
```

<!-- schema generated by tfplugindocs -->
## Schema

### Required

- `project` (String) The ID of the Google Cloud project to manage with P0

### Read-Only

- `state` (String) This item's install progress in the P0 application:
	- 'stage': The item has been staged for installation
	- 'configure': The item is available to be added to P0, and may be configured
	- 'installed': The item is fully installed
