# Must pre-exist in the P0 app: the Okta group (via an installed Okta directory
# listing integration, see p0_okta_directory_listing), the "aws" integration (see
# p0_aws_iam_write), and the PagerDuty integration (connected in-app, not via Terraform).
resource "p0_routing_rule" "example" {
  name = "okta-aws-developers-oncall"
  requestor = {
    type   = "group"
    effect = "keep"
    groups = [{
      directory = "okta"
      id        = "00abcdefghijklmno697"
      label     = "AWS Developers"
    }]
  }
  resource = {
    type    = "integration"
    service = "aws"
    filters = {
      "tag" = {
        effect  = "keep"
        key     = "p0_grantable"
        pattern = "1|true"
      }
    }
  }
  approval = [{
    type        = "auto"
    integration = "pagerduty"
    options = {
      require_reason = true
    }
  }]
}
