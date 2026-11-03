# Access bundles need enable_preview_resources in the provider block:
#
# provider "p0" {
#   org                      = "my-org"
#   enable_preview_resources = true
# }
#
# The gcloud and aws integrations must be installed in your P0 app, and the
# group must exist in an installed directory integration.
resource "p0_access_bundle" "deploy" {
  label       = "Terraform deploy"
  description = "Everything needed to run the production Terraform deploy"

  requestor = {
    type   = "group"
    effect = "keep"
    groups = [{
      directory = "workspace"
      id        = "04d34og81j3pxk1"
      label     = "eng@example.com"
    }]
  }

  approval = [{
    type   = "group"
    effect = "keep"
    groups = [{
      directory = "workspace"
      id        = "00xvir7l2dtpb3m"
      label     = "approvers@example.com"
    }]
    options = {
      require_reason = true
    }
  }]

  items = [
    # A project-level role.
    {
      type   = "gcloud"
      access = "role"
      args = jsonencode({
        project = "my-project"
        name    = ["roles/editor"]
      })
    },
    # A role on a single bucket. The locator is itself a JSON string, so it is
    # encoded once inside args, and args is encoded again.
    {
      type   = "gcloud"
      access = "resource"
      args = jsonencode({
        project  = "my-project"
        accesses = ["roles/storage.objectViewer"]
        locator = jsonencode({
          fullName = "//storage.googleapis.com/my-bucket"
          name     = "my-bucket"
          service  = "storage"
          type     = "bucket"
        })
      })
    },
    # An AWS IAM Identity Center permission set.
    {
      type   = "aws"
      access = "permission-set"
      args = jsonencode({
        account = "123456789012"
        name    = "arn:aws:sso:::permissionSet/ssoins-1234567890abcdef/ps-1234567890abcdef"
      })
    },
  ]
}
