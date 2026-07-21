# Prerequisites (must already exist in the P0 app before these rules resolve):
# - the referenced Okta directory group, exposed via an installed Okta directory
#   listing integration (see the p0_okta_directory_listing example),
# - the "aws" integration (see the p0_aws_iam_write example), and
# - the PagerDuty integration, connected in the P0 app (not managed via Terraform).
resource "p0_routing_rules" "example" {
  rule {
    name = "AWS developers on-call access"
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
}
