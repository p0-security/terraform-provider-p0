# Import an existing bundle by its ID, then let Terraform write its config:
#
#   terraform plan -generate-config-out=bundles.tf
import {
  to = p0_access_bundle.deploy
  id = "dsy9eYJIUoyBM1RJuvUx"
}
