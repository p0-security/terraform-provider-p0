terraform {
  required_providers {
    okta = {
      source  = "okta/okta"
      version = ">= 4.8.0"
    }
  }
}

# P0 requires the following settings:
# - "joinAllRoles" must be enabled to allow direct assignment of roles to users
# - "webSSOAllowedClient" must be set to the client ID of the OIDC app that will be used for login
# - The SourceIdentity custom SAML attribute must be set to user.login
# - "Managed by P0" user schema property of boolean type

# To import: terraform import "module.okta_aws_federation.okta_app_saml.aws_account_federation" {applicationId}
resource "okta_app_saml" "aws_account_federation" {
  preconfigured_app = "amazon_aws"
  label             = var.app_name
  status            = "ACTIVE"
  saml_version      = "2.0"
  enduser_note      = var.enduser_note
  app_settings_json = <<JSON
    {
      "appFilter": "okta",
      "groupFilter": "aws_(?{{accountid}}\\d+)_(?{{role}}[a-zA-Z0-9+=,.@\\-_]+)",
      "useGroupMapping": false,
      "joinAllRoles": true,
      "identityProviderArn": "arn:aws:iam::${var.aws_account_id}:saml-provider/${var.aws_saml_identity_provider_name}",
      "sessionDuration": 3600,
      "roleValuePattern": "arn:aws:iam::$${accountid}:saml-provider/OKTA,arn:aws:iam::$${accountid}:role/$${role}",
      "awsEnvironmentType": "aws.amazon",
      "webSSOAllowedClient": "${var.login_app_client_id}",
      "loginURL": "https://console.aws.amazon.com/ec2/home"
    }
  JSON
  attribute_statements {
    type      = "EXPRESSION"
    name      = "https://aws.amazon.com/SAML/Attributes/SourceIdentity"
    namespace = "urn:oasis:names:tc:SAML:2.0:attrname-format:unspecified"
    values    = ["user.login"]
  }
  # ################################### #
  #    WARNING! MANUAL EDIT REQUIRED!   #
  # ################################### #
  # The Provisioning settings are not configurable via API (or Terraform).
  # Make sure to "Enable API Integration" in the "Provisioning" tab in the Okta Admin Console manually.
}

# To import: terraform import "module.okta_aws_federation.okta_app_user_schema_property.managed_by_p0_property" {applicationId}/managedByP0
resource "okta_app_user_schema_property" "managed_by_p0_property" {
  app_id      = okta_app_saml.aws_account_federation.id
  index       = "managedByP0"
  title       = "Managed by P0"
  type        = "boolean"
  required    = false
  master      = "OKTA"
  scope       = "NONE" # "NONE" means this property can be set at the group level
  permissions = "READ_WRITE"
}
