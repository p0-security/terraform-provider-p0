---
page_title: "p0_pingfederate_aws_iam_federation_app Resource - p0"
subcategory: ""
description: |-
  A PingFederate SP connection that signs users in to AWS. A p0_aws_iam_federation resource has an AWS account trust it.
  Its role source sets how PingFederate gets each user's AWS roles. Set exactly one of attribute_source or scim. P0 fixes id, pingfederate_url, the role source and every scim field when it adds the app, so changing one replaces the installation. An attribute source's entity_id, issuer, client_id and jwks change in place. If the app is already added in P0, import it rather than creating it: P0 refuses to add an app twice. If P0 hasn't finished installing the app, for example because you imported it before its checks passed, Terraform plans an update, and applying it finishes the install.
  Attribute source
  PingFederate reads each user's roles from P0 at each sign-in, through a REST API data store. P0 calls nothing in your network and holds no secret: PingFederate's own OAuth server issues the data store's access token, and P0 checks it against issuer, client_id, the public keys, and the audience that P0 sets for the app. A grant or a revoke takes effect at the user's next sign-in.
  Creating this resource doesn't call PingFederate, so a mistake in PingFederate's setup shows as a refused AWS sign-in. Set up PingFederate with the values this resource gives:
  
  Under Access Token Management, add a JSON Web Tokens instance that uses the centralized signing key. Set its Issuer Claim Value to issuer, and its Audience Claim Value to attribute_source.audience, which P0 sets when it adds the app.
  Map the instance for the client_credentials context, and add an OAuth client whose client ID is client_id, with the client credentials grant, that issues only this instance's tokens. Keep the instance's Client ID Claim Name as client_id.
  Add a REST API data store whose base URL is attribute_source.roles_url. Its authentication method is OAuth 2.0 Bearer Token with Client Credentials, the client above, and PingFederate's own token endpoint, <pingfederate_url>/as/token.oauth2. Map the local attribute roles to /roles.
  Create an SP connection to the partner urn:amazon:webservices. Add the data store as an attribute source with the resource path /apps/<id>/roles?email=${mail}, and fulfill https://aws.amazon.com/SAML/Attributes/Role from its roles. Map the user's mail to https://aws.amazon.com/SAML/Attributes/RoleSessionName and https://aws.amazon.com/SAML/Attributes/SourceIdentity, with the URI name format. Its subject uses the persistent NameID format. In its signature policy, select Always sign the SAML Assertion: AWS refuses a response that signs only the SAML response.
  SCIM
  P0's connector writes each user's AWS roles to Active Directory through PingFederate's SCIM inbound provisioning, and signs in to it with HTTP Basic credentials. P0 never holds the SCIM password. You store it in your own secret manager, in the connector's cloud, and the connector reads it on every call.
  Important: Set up PingFederate, store the SCIM password and let the connector read it before Terraform creates this resource. Do that in an earlier apply, or in the same apply with this resource depending on the connector's read grant. Creating this resource has P0 validate the install through the connector: the connector reads the secret, searches PingFederate's SCIM users by email, checks the role attribute in the custom SCIM schema, and reads PingFederate's SAML metadata for AWS. If a check fails, the apply fails with P0's message. IAM changes can take a few seconds to propagate, so if the connector is denied the secret right after you grant it, apply again. To rotate the SCIM password, store the new one as a new version of the same secret, which needs no change here.
  Prerequisites:
  
  A PingFederate (AWS IAM Federation) connector, installed with p0_pingfederate_aws_iam_federation_connector. The connector must reach PingFederate's runtime at pingfederate_url, including /pf-scim. The SCIM password must be in the connector's cloud: AWS Secrets Manager for a connector on AWS Lambda, or Google Secret Manager for one on Cloud Run. It can be in another account or project.
  A multi-valued string attribute on users in the Active Directory schema, named by scim.role_attribute, that P0 adds each user's AWS roles to. The account of PingFederate's Active Directory data store must be able to write it.
  A PingFederate IdP connection with only Inbound Provisioning, on SCIM 2.0, whose user repository is the Active Directory data store. Add scim.role_attribute to its custom SCIM schema as a multi-valued, read-write string. In Write Users, map the Active Directory attribute to the SCIM attribute of the same name. In Read Users, map emails.work.value to mail, and scim.role_attribute to itself. A SCIM search must return each user's id. The connection uses HTTP Basic authentication with the user name scim.scim_username. Store its password outside Terraform, as the secret's whole value, in the account and region or the project that scim.vault names. Storing it outside Terraform keeps it out of Terraform's state.
  A PingFederate SP connection to the partner urn:amazon:webservices. It maps scim.role_attribute to https://aws.amazon.com/SAML/Attributes/Role, and the user's mail to https://aws.amazon.com/SAML/Attributes/RoleSessionName and https://aws.amazon.com/SAML/Attributes/SourceIdentity, with the URI name format. Its subject uses the persistent NameID format. In its signature policy, select Always sign the SAML Assertion: AWS refuses a response that signs only the SAML response.
  Read access to that one secret, and no other, for the connector's identity (its connector_identity). P0 names the secret in each call, so a call could point the connector at any other secret it can read.
  On AWS, give the connector's role secretsmanager:GetSecretValue on arn:aws:secretsmanager:<secrets_region>:<account_id>:secret:<name>-??????. Secrets Manager adds a hyphen and six random characters to the name in a secret's ARN, and ?????? matches those, so the grant covers this one secret. If a customer-managed KMS key encrypts the secret, also give the role kms:Decrypt on that key, with the condition that kms:ViaService is secretsmanager.<secrets_region>.amazonaws.com. A secret outside the connector's account needs a customer-managed key, because no other account can use the default aws/secretsmanager key. In the secret's account, the secret's resource policy and the key's policy must also allow the connector's role.
  On Google Cloud, give the connector's service account roles/secretmanager.secretAccessor and roles/secretmanager.viewer on the secret itself, not on its project.
  P0's secret manager integration for where the secret is: AWS Secrets Manager (p0_aws_secrets_manager) for the AWS account, or Google Secret Manager for the Google Cloud project.
  Note: This integration is currently in preview.
  For the AWS install steps, see Setting up AWS IAM management https://docs.p0.dev/integrations/resource-integrations/aws/installation-methods.
---

# p0_pingfederate_aws_iam_federation_app (Resource)

A PingFederate SP connection that signs users in to AWS. A `p0_aws_iam_federation` resource has an AWS account trust it.

Its role source sets how PingFederate gets each user's AWS roles. Set exactly one of `attribute_source` or `scim`. P0 fixes `id`, `pingfederate_url`, the role source and every `scim` field when it adds the app, so changing one replaces the installation. An attribute source's `entity_id`, `issuer`, `client_id` and `jwks` change in place. If the app is already added in P0, import it rather than creating it: P0 refuses to add an app twice. If P0 hasn't finished installing the app, for example because you imported it before its checks passed, Terraform plans an update, and applying it finishes the install.

## Attribute source

PingFederate reads each user's roles from P0 at each sign-in, through a REST API data store. P0 calls nothing in your network and holds no secret: PingFederate's own OAuth server issues the data store's access token, and P0 checks it against `issuer`, `client_id`, the public keys, and the audience that P0 sets for the app. A grant or a revoke takes effect at the user's next sign-in.

Creating this resource doesn't call PingFederate, so a mistake in PingFederate's setup shows as a refused AWS sign-in. Set up PingFederate with the values this resource gives:

- Under **Access Token Management**, add a **JSON Web Tokens** instance that uses the centralized signing key. Set its **Issuer Claim Value** to `issuer`, and its **Audience Claim Value** to `attribute_source.audience`, which P0 sets when it adds the app.

- Map the instance for the `client_credentials` context, and add an OAuth client whose client ID is `client_id`, with the client credentials grant, that issues only this instance's tokens. Keep the instance's **Client ID Claim Name** as `client_id`.

- Add a **REST API** data store whose base URL is `attribute_source.roles_url`. Its authentication method is **OAuth 2.0 Bearer Token** with **Client Credentials**, the client above, and PingFederate's own token endpoint, `<pingfederate_url>/as/token.oauth2`. Map the local attribute `roles` to `/roles`.

- Create an SP connection to the partner `urn:amazon:webservices`. Add the data store as an attribute source with the resource path `/apps/<id>/roles?email=${mail}`, and fulfill `https://aws.amazon.com/SAML/Attributes/Role` from its `roles`. Map the user's `mail` to `https://aws.amazon.com/SAML/Attributes/RoleSessionName` and `https://aws.amazon.com/SAML/Attributes/SourceIdentity`, with the URI name format. Its subject uses the persistent NameID format. In its signature policy, select **Always sign the SAML Assertion**: AWS refuses a response that signs only the SAML response.

## SCIM

P0's connector writes each user's AWS roles to Active Directory through PingFederate's SCIM inbound provisioning, and signs in to it with HTTP Basic credentials. P0 never holds the SCIM password. You store it in your own secret manager, in the connector's cloud, and the connector reads it on every call.

**Important:** Set up PingFederate, store the SCIM password and let the connector read it before Terraform creates this resource. Do that in an earlier apply, or in the same apply with this resource depending on the connector's read grant. Creating this resource has P0 validate the install through the connector: the connector reads the secret, searches PingFederate's SCIM users by email, checks the role attribute in the custom SCIM schema, and reads PingFederate's SAML metadata for AWS. If a check fails, the apply fails with P0's message. IAM changes can take a few seconds to propagate, so if the connector is denied the secret right after you grant it, apply again. To rotate the SCIM password, store the new one as a new version of the same secret, which needs no change here.

**Prerequisites:**

- A PingFederate (AWS IAM Federation) connector, installed with `p0_pingfederate_aws_iam_federation_connector`. The connector must reach PingFederate's runtime at `pingfederate_url`, including `/pf-scim`. The SCIM password must be in the connector's cloud: AWS Secrets Manager for a connector on AWS Lambda, or Google Secret Manager for one on Cloud Run. It can be in another account or project.

- A multi-valued string attribute on users in the Active Directory schema, named by `scim.role_attribute`, that P0 adds each user's AWS roles to. The account of PingFederate's Active Directory data store must be able to write it.

- A PingFederate IdP connection with only Inbound Provisioning, on SCIM 2.0, whose user repository is the Active Directory data store. Add `scim.role_attribute` to its custom SCIM schema as a multi-valued, read-write string. In Write Users, map the Active Directory attribute to the SCIM attribute of the same name. In Read Users, map `emails.work.value` to `mail`, and `scim.role_attribute` to itself. A SCIM search must return each user's `id`. The connection uses HTTP Basic authentication with the user name `scim.scim_username`. Store its password outside Terraform, as the secret's whole value, in the account and region or the project that `scim.vault` names. Storing it outside Terraform keeps it out of Terraform's state.

- A PingFederate SP connection to the partner `urn:amazon:webservices`. It maps `scim.role_attribute` to `https://aws.amazon.com/SAML/Attributes/Role`, and the user's `mail` to `https://aws.amazon.com/SAML/Attributes/RoleSessionName` and `https://aws.amazon.com/SAML/Attributes/SourceIdentity`, with the URI name format. Its subject uses the persistent NameID format. In its signature policy, select **Always sign the SAML Assertion**: AWS refuses a response that signs only the SAML response.

- Read access to that one secret, and no other, for the connector's identity (its `connector_identity`). P0 names the secret in each call, so a call could point the connector at any other secret it can read.

  On AWS, give the connector's role `secretsmanager:GetSecretValue` on `arn:aws:secretsmanager:<secrets_region>:<account_id>:secret:<name>-??????`. Secrets Manager adds a hyphen and six random characters to the name in a secret's ARN, and `??????` matches those, so the grant covers this one secret. If a customer-managed KMS key encrypts the secret, also give the role `kms:Decrypt` on that key, with the condition that `kms:ViaService` is `secretsmanager.<secrets_region>.amazonaws.com`. A secret outside the connector's account needs a customer-managed key, because no other account can use the default `aws/secretsmanager` key. In the secret's account, the secret's resource policy and the key's policy must also allow the connector's role.

  On Google Cloud, give the connector's service account `roles/secretmanager.secretAccessor` and `roles/secretmanager.viewer` on the secret itself, not on its project.

- P0's secret manager integration for where the secret is: AWS Secrets Manager (`p0_aws_secrets_manager`) for the AWS account, or Google Secret Manager for the Google Cloud project.

**Note:** This integration is currently in preview.

For the AWS install steps, see [Setting up AWS IAM management](https://docs.p0.dev/integrations/resource-integrations/aws/installation-methods).

## Example Usage

### Attribute source

```terraform
# Adds the PingFederate app acme-sso, whose PingFederate reads each user's AWS
# roles from P0 at sign-in. It needs no connector and no secret.
#
# P0 sets the token audience when it adds the app, so set up PingFederate
# after the first apply, with this app's outputs:
#
# - Under Access Token Management, add a JSON Web Tokens instance that uses
#   the centralized signing key. Set its Issuer Claim Value to the issuer
#   below, and its Audience Claim Value to the
#   pingfederate_attribute_source_audience output.
# - Map the instance for the client_credentials context, and add an OAuth
#   client with the client ID below and the client credentials grant, that
#   issues only its tokens. Keep the instance's Client ID Claim Name as
#   client_id.
# - Add a REST API data store whose base URL is the
#   pingfederate_attribute_source_roles_url output. Authenticate with OAuth 2.0
#   Bearer Token and Client Credentials, the client above and PingFederate's
#   token endpoint, https://sso.example.com:9031/as/token.oauth2. Map the local
#   attribute roles to /roles.
# - On the SP connection to urn:amazon:webservices, add the data store as an
#   attribute source with the resource path /apps/acme-sso/roles?email=${mail},
#   and fulfill https://aws.amazon.com/SAML/Attributes/Role from its roles.
resource "p0_pingfederate_aws_iam_federation_app" "attribute_source_example" {
  id               = "acme-sso"
  pingfederate_url = "https://sso.example.com:9031"

  attribute_source = {
    # PingFederate's System > Server > Protocol Settings > Federation Info.
    entity_id = "urn:acme:pingfederate"
    issuer    = "urn:acme:pingfederate:p0"
    client_id = "p0-roles"
    # Unset, P0 reads https://sso.example.com:9031/pf/JWKS. To paste the keys
    # instead, and paste them again after each key rotation:
    # jwks = trimspace(file("${path.module}/pingfederate-jwks.json"))
  }
}

output "pingfederate_attribute_source_audience" {
  value = p0_pingfederate_aws_iam_federation_app.attribute_source_example.attribute_source.audience
}

output "pingfederate_attribute_source_roles_url" {
  value = p0_pingfederate_aws_iam_federation_app.attribute_source_example.attribute_source.roles_url
}
```

### SCIM, with a connector on AWS

```terraform
# Adds the PingFederate app acme-aws, whose roles P0's connector writes through
# SCIM, and whose SCIM password is in AWS Secrets Manager, in the account of the connector on AWS Lambda that the
# p0_pingfederate_aws_iam_federation_connector example installs. For a connector on
# Cloud Run and a SCIM password in Google Secret Manager, see gcp.tf in this
# directory.
#
# Before you apply this:
#
# - In Active Directory, add the multi-valued string attribute awsRole on
#   users, which P0 adds each user's AWS roles to, and let the account of
#   PingFederate's Active Directory data store write it.
# - In PingFederate, create an IdP connection with only Inbound Provisioning,
#   on SCIM 2.0, on the Active Directory data store. Add awsRole to its custom
#   SCIM schema as a multi-valued, read-write string, and map it to the Active
#   Directory attribute in Write Users and Read Users. In Read Users, also map
#   emails.work.value to mail. Use HTTP Basic authentication with the user name
#   p0-scim, and copy its password to the file pingfederate-scim-password.txt.
# - Create an SP connection to urn:amazon:webservices. Map awsRole to
#   https://aws.amazon.com/SAML/Attributes/Role, and mail to
#   https://aws.amazon.com/SAML/Attributes/RoleSessionName and
#   https://aws.amazon.com/SAML/Attributes/SourceIdentity, with the URI name
#   format. Use the persistent NameID format for the subject, and select
#   "Always sign the SAML Assertion".
#
# The example doesn't create the secret, which keeps the SCIM password out of
# Terraform's state. Creating p0_pingfederate_aws_iam_federation_app has the
# connector read it, so store it before then, as the secret's whole value.
# With the AWS CLI signed in to the secret's account, run this where
# pingfederate-scim-password.txt is:
#
#   aws secretsmanager create-secret --name pingfederate/acme/p0 \
#     --secret-string file://pingfederate-scim-password.txt --region us-west-2
#
# If the secret exists, store the SCIM password as a new version instead,
# which is also how to rotate it:
#
#   aws secretsmanager put-secret-value --secret-id pingfederate/acme/p0 \
#     --secret-string file://pingfederate-scim-password.txt --region us-west-2
#
# Then delete pingfederate-scim-password.txt.
#
# The grant is what P0's PingFederate (AWS IAM Federation) installer generates for
# the app, with p0_pingfederate_aws_iam_federation_app after it.

locals {
  p0_pingfederate_aws_iam_federation_app_acme_aws = {
    # The connector's AWS account, which its role is in.
    connector_account_id = "123456789012"
    # The secret that holds the SCIM password: its name, and the
    # account and region it's in. P0 has the connector read the secret by this
    # name there. A new secret replaces the installation. To rotate the SCIM
    # password, add a new version to this secret instead.
    scim_password_name = "pingfederate/acme/p0"
    secrets_account_id = "123456789012"
    secrets_region     = "us-west-2"
    # The ARN of the KMS key that encrypts the secret, if that's a
    # customer-managed key. The default aws/secretsmanager key needs nothing
    # for a secret in the connector's account, but can't be used from another
    # one. For a secret in another account, set this to its customer-managed
    # key, and in that account allow the connector's role
    # secretsmanager:GetSecretValue in the secret's resource policy and
    # kms:Decrypt through Secrets Manager in the key's policy.
    kms_key_arn = null
  }
}

# IAM is global, so the region only has to be one you use. Add the credential
# settings your other AWS providers use, such as profile or assume_role, if
# they have any.
provider "aws" {
  alias               = "p0_pingfederate_aws_iam_federation_app_acme_aws"
  region              = local.p0_pingfederate_aws_iam_federation_app_acme_aws.secrets_region
  allowed_account_ids = [local.p0_pingfederate_aws_iam_federation_app_acme_aws.connector_account_id]
}

data "aws_partition" "p0_pingfederate_aws_iam_federation_app_acme_aws" {
  provider = aws.p0_pingfederate_aws_iam_federation_app_acme_aws
}

# The connector reads the app's SCIM password, and no other secret.
# The policy goes on the connector's role, which P0 names in
# connector_identity. Each app adds a policy of its own to that role, and a
# role's inline policies share a limit of 10,240 characters: past a few dozen
# apps, deploy another connector for the rest.
resource "aws_iam_role_policy" "p0_pingfederate_aws_iam_federation_app_acme_aws_secret" {
  provider = aws.p0_pingfederate_aws_iam_federation_app_acme_aws

  name = "ReadPingFederateScimPassword-acme-aws"
  role = p0_pingfederate_aws_iam_federation_connector.example.connector_identity.role_name
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = concat(
      [{
        Sid    = "ReadScimPassword"
        Effect = "Allow"
        Action = "secretsmanager:GetSecretValue"
        # P0 gives the connector the secret's partial ARN: its ARN without the
        # hyphen and six random characters that end it. "??????" matches
        # those, so the connector gets this one secret and no other.
        Resource = join(":", [
          "arn:aws:secretsmanager",
          local.p0_pingfederate_aws_iam_federation_app_acme_aws.secrets_region,
          local.p0_pingfederate_aws_iam_federation_app_acme_aws.secrets_account_id,
          "secret",
          "${local.p0_pingfederate_aws_iam_federation_app_acme_aws.scim_password_name}-??????",
        ])
      }],
      local.p0_pingfederate_aws_iam_federation_app_acme_aws.kms_key_arn == null ? [] : [{
        Sid      = "DecryptScimPassword"
        Effect   = "Allow"
        Action   = "kms:Decrypt"
        Resource = local.p0_pingfederate_aws_iam_federation_app_acme_aws.kms_key_arn
        Condition = {
          StringEquals = {
            "kms:ViaService" = "secretsmanager.${local.p0_pingfederate_aws_iam_federation_app_acme_aws.secrets_region}.${data.aws_partition.p0_pingfederate_aws_iam_federation_app_acme_aws.dns_suffix}"
          }
        }
      }],
    )
  })

  # A secret in another account can't use the default aws/secretsmanager key.
  lifecycle {
    precondition {
      condition     = local.p0_pingfederate_aws_iam_federation_app_acme_aws.secrets_account_id == local.p0_pingfederate_aws_iam_federation_app_acme_aws.connector_account_id || local.p0_pingfederate_aws_iam_federation_app_acme_aws.kms_key_arn != null
      error_message = "The SCIM password is in another AWS account, so it must be encrypted with a customer-managed KMS key. Set kms_key_arn to that key's ARN, and allow the connector's role in the secret's resource policy and in the key's policy."
    }
  }
}

# Adds the app. Creating it has P0 validate the install through the
# connector, so it waits until the connector can read the SCIM password. IAM
# changes can take a few seconds to propagate, so if the connector is denied the
# secret, apply again.
resource "p0_pingfederate_aws_iam_federation_app" "example" {
  id               = "acme-aws"
  pingfederate_url = "https://sso.example.com:9031"

  scim = {
    connector      = p0_pingfederate_aws_iam_federation_connector.example.id
    role_attribute = "awsRole"
    scim_username  = "p0-scim"

    vault = {
      type           = "aws-sm"
      account_id     = local.p0_pingfederate_aws_iam_federation_app_acme_aws.secrets_account_id
      secrets_region = local.p0_pingfederate_aws_iam_federation_app_acme_aws.secrets_region
    }
    scim_password_name = local.p0_pingfederate_aws_iam_federation_app_acme_aws.scim_password_name
  }

  depends_on = [aws_iam_role_policy.p0_pingfederate_aws_iam_federation_app_acme_aws_secret]
}

# The app's SAML metadata, which each AWS account's SAML provider trusts (see
# the p0_aws_iam_federation example).
output "pingfederate_saml_metadata_url" {
  value = p0_pingfederate_aws_iam_federation_app.example.saml_metadata_url
}
```

### SCIM, with a connector on Google Cloud

```terraform
# The Google Cloud variant of the p0_pingfederate_aws_iam_federation_app example in
# this directory: the app's connector runs on Cloud Run (see gcp.tf in the
# p0_pingfederate_aws_iam_federation_connector example), and its SCIM password is in
# Google Secret Manager.
#
# Before you apply this, set up Active Directory and PingFederate, as the example in
# resource.tf says, and store the SCIM password as the secret
# pingfederate-acme-gcp-p0. The example doesn't create the secret, which keeps the
# SCIM password out of Terraform's state. Run this where
# pingfederate-scim-password.txt is:
#
#   gcloud secrets create pingfederate-acme-gcp-p0 --project my-secrets-project-id \
#     --data-file=pingfederate-scim-password.txt
#
# If the secret exists, store the SCIM password as a new version instead,
# which is also how to rotate it:
#
#   gcloud secrets versions add pingfederate-acme-gcp-p0 --project my-secrets-project-id \
#     --data-file=pingfederate-scim-password.txt
#
# Then delete pingfederate-scim-password.txt.

locals {
  p0_pingfederate_aws_iam_federation_app_acme_gcp = {
    # The secret that holds the SCIM password, and its project,
    # which needn't be the connector's. A new secret replaces the installation.
    # To rotate the SCIM password, add a new version to this secret instead.
    scim_password_project = "my-secrets-project-id"
    scim_password_id      = "pingfederate-acme-gcp-p0"
  }
}

# The connector reads the SCIM password's value. Each binding adds one member
# to the secret's IAM policy, and replaces nothing. Both are on the secret and
# not on its project, so the connector can read no other secret.
resource "google_secret_manager_secret_iam_member" "p0_pingfederate_aws_iam_federation_app_acme_gcp_accessor" {
  project   = local.p0_pingfederate_aws_iam_federation_app_acme_gcp.scim_password_project
  secret_id = local.p0_pingfederate_aws_iam_federation_app_acme_gcp.scim_password_id
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${p0_pingfederate_aws_iam_federation_connector.gcp_example.connector_identity.service_account_email}"
}

# The accessor role gives secretmanager.versions.access only. The connector also
# reads the version it got the value from, which needs
# secretmanager.versions.get.
resource "google_secret_manager_secret_iam_member" "p0_pingfederate_aws_iam_federation_app_acme_gcp_viewer" {
  project   = local.p0_pingfederate_aws_iam_federation_app_acme_gcp.scim_password_project
  secret_id = local.p0_pingfederate_aws_iam_federation_app_acme_gcp.scim_password_id
  role      = "roles/secretmanager.viewer"
  member    = "serviceAccount:${p0_pingfederate_aws_iam_federation_connector.gcp_example.connector_identity.service_account_email}"
}

# Adds the app. Creating it has P0 validate the install through the
# connector, so it waits until the connector can read the SCIM password.
resource "p0_pingfederate_aws_iam_federation_app" "gcp_example" {
  id               = "acme-gcp"
  pingfederate_url = "https://sso.example.com:9031"

  scim = {
    connector      = p0_pingfederate_aws_iam_federation_connector.gcp_example.id
    role_attribute = "awsRole"
    scim_username  = "p0-scim"

    vault = {
      type       = "gcp-sm"
      project_id = local.p0_pingfederate_aws_iam_federation_app_acme_gcp.scim_password_project
    }
    scim_password_name = local.p0_pingfederate_aws_iam_federation_app_acme_gcp.scim_password_id
    # PingFederate's TLS certificate is from an internal CA. P0 removes the
    # whitespace around the value, so trim the file's contents.
    ca_certificate = trimspace(file("${path.module}/pingfederate-ca.pem"))
  }

  depends_on = [
    google_secret_manager_secret_iam_member.p0_pingfederate_aws_iam_federation_app_acme_gcp_accessor,
    google_secret_manager_secret_iam_member.p0_pingfederate_aws_iam_federation_app_acme_gcp_viewer,
  ]
}
```

<!-- schema generated by tfplugindocs -->
## Schema

### Required

- `id` (String) A name you choose for the app, such as `acme-aws`. Use lowercase letters, digits and hyphens only, starting and ending with a letter or digit, up to 63 characters.
- `pingfederate_url` (String) PingFederate's runtime base URL, such as `https://sso.example.com:9031`. P0 reads the SAML metadata of its SP connection to AWS from it. With `scim`, P0 also calls its SCIM endpoint there.

### Optional

- `attribute_source` (Attributes) PingFederate reads each user's AWS roles from P0 at each sign-in, through a REST API data store. Set this or `scim`, not both. (see [below for nested schema](#nestedatt--attribute_source))
- `scim` (Attributes) P0's connector writes each user's AWS roles to Active Directory through PingFederate's SCIM endpoint. Set this or `attribute_source`, not both. (see [below for nested schema](#nestedatt--scim))

### Read-Only

- `saml_metadata_url` (String) The URL of the app's SAML metadata. Create each AWS account's SAML provider from it.
- `state` (String) This item's install progress in the P0 application:
	- 'stage': The item has been staged for installation
	- 'configure': The item is available to be added to P0, and may be configured
	- 'installed': The item is fully installed

<a id="nestedatt--attribute_source"></a>
### Nested Schema for `attribute_source`

Required:

- `client_id` (String) The client ID of the OAuth client that the REST API data store gets its tokens with. P0 refuses the tokens of other clients.
- `entity_id` (String) PingFederate's SAML entity ID, under System > Server > Protocol Settings > Federation Info. Each AWS account's SAML provider trusts it.
- `issuer` (String) The Issuer Claim Value of the JWT access token manager that signs the REST API data store's tokens

Optional:

- `jwks` (String) The JSON of PingFederate's public keys, from `<pingfederate_url>/pf/JWKS`, for P0 to check the data store's tokens with. Paste it again when PingFederate rotates its signing key. Leave it unset to have P0 read `<pingfederate_url>/pf/JWKS` itself, which needs no change on a rotation, but needs P0 to reach PingFederate's runtime. P0 removes the whitespace around the value, so remove it here too, for example with `trimspace(file("jwks.json"))`.

Read-Only:

- `audience` (String) The Audience Claim Value to set on the JWT access token manager. P0 sets it for the app.
- `roles_url` (String) The base URL of the REST API data store: P0's roles route for your organization


<a id="nestedatt--scim"></a>
### Nested Schema for `scim`

Required:

- `connector` (String) The connector that acts on this app: the `id` of a `p0_pingfederate_aws_iam_federation_connector` resource. Requires that connector to be installed. It must reach PingFederate's runtime, including `/pf-scim`.
- `role_attribute` (String) The multi-valued attribute in the SCIM connection's custom schema that holds each user's AWS roles, such as `awsRole`. Active Directory has an attribute of the same name. The SP connection maps it to AWS's Role attribute.
- `scim_password_name` (String) The name of the secret that holds the SCIM connection's password, such as `pingfederate/acme/p0` in AWS Secrets Manager or `pingfederate-acme-p0` in Google Secret Manager: the name alone, not the secret's ARN or its full resource name. In AWS Secrets Manager the name can't end in a hyphen and six letters or digits, like `-AbCdEf`, because Secrets Manager can't find a secret by such a name. To rotate the SCIM password, store the new one as a new version of the same secret, which needs no change here.
- `scim_username` (String) The user name of the HTTP Basic credentials on PingFederate's SCIM IdP connection, which writes to Active Directory
- `vault` (Attributes) The secret manager that holds the SCIM password. Your connector reads the secret, and P0 never does. It must be in the connector's cloud, and can be in another account or project there. (see [below for nested schema](#nestedatt--scim--vault))

Optional:

- `ca_certificate` (String) PEM certificates of the CA that issued PingFederate's TLS certificate, if it isn't a public CA. P0 removes the whitespace around the value, so remove it here too, for example with `trimspace(file("ca.pem"))`.

<a id="nestedatt--scim--vault"></a>
### Nested Schema for `scim.vault`

Required:

- `type` (String) The secret manager: either `aws-sm` (AWS Secrets Manager) or `gcp-sm` (Google Secret Manager)

Optional:

- `account_id` (String) The ID of the AWS account that holds the SCIM password (in the P0 app, one of your AWS Secrets Manager installations). Required for, and only valid with, `aws-sm`.
- `project_id` (String) The ID of the Google Cloud project that holds the SCIM password (in the P0 app, one of your Google Secret Manager installations). Required for, and only valid with, `gcp-sm`.
- `secrets_region` (String) The AWS region the SCIM password is stored in, such as `us-west-2`. Required for, and only valid with, `aws-sm`.
