# Module which deploys the P0 Splunk HEC Integration

terraform {
  required_providers {
    p0 = {
      source  = "registry.terraform.io/p0-security/p0"
      version = "0.13.0"
    }
  }
}

resource "p0_splunk_audit_logs" "token" {
  token_id            = var.token_id
  index               = var.index
  hec_token_cleartext = var.hec_token_cleartext
  hec_endpoint        = var.hec_endpoint
}

