resource "p0_routing_rules" "example" {
  rule {
    requestor = {
      type      = "group"
      directory = "okta"
      id        = "00abcdefghijklmno697"
      label     = "AWS Developers"
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
