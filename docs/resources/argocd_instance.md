---
page_title: "p0_argocd_instance Resource - p0"
subcategory: ""
description: |-
  An Argo CD instance that P0 can grant roles on.
  P0 reaches the instance through an Argo CD connector, which you install with p0_argocd_connector. The connector calls the instance's API as the Argo CD local account p0-connector, whose API token stays in your own secret manager: P0 never reads it, and only the connector does. One connector can serve several instances, and each instance has its own token and secret.
  P0 grants a role as a line in the policy.p0.csv key of the instance's argocd-rbac-cm, which it writes through the Argo CD Application that managing_app names. P0 owns that key: leave it out of Git.
  Important: Install the connector first, and grant its identity read access to the token's secret: in an earlier apply, or in the same one with this resource depending on the grant, as the examples do. Creating this resource has P0 check the instance through the connector. If the check fails, the apply fails with P0's message.
  An instance's attributes can't change once it is created. Changing id, connector, vault or secret_name replaces the instance. A plan refuses a change to an installed instance's url, managing_app or namespace: P0 revokes access where it granted it, so for another instance, Application or namespace, add a new p0_argocd_instance with its own id, and remove this one once its open access has ended. To rotate the API token, generate a new one and add it as a new version of the same secret, which needs no change here.
  Warning: Replacing or destroying this resource removes the instance from P0. P0 can't revoke access it granted on an instance it no longer has, so those lines stay in policy.p0.csv until you remove them there. Before you change an attribute or destroy this resource, revoke the instance's open access in P0 or wait for it to expire.
  An instance that already exists in P0, such as one added in the P0 app, isn't created again: the apply fails, and says to import it with terraform import. An imported instance that P0 hasn't finished installing plans an update, and applying it finishes the install. If that install can't finish, fix what P0's message names and apply again, or start over: terraform apply -replace=<address> deletes the instance and creates it again, and terraform state rm <address> stops managing it.
  The email domains that may receive access are set on the connector, as its ALLOWED_EMAIL_DOMAIN_PATTERN environment variable, and aren't an attribute of this resource.
  Prerequisites:
  
  Argo CD 3.0 or later, served at the root of its address. An Argo CD served under a root path (server.rootpath) isn't supported.
  An Argo CD Application that manages argocd-rbac-cm. Without one, P0 can't write grants.
  The local account p0-connector, with accounts.p0-connector: apiKey in argocd-cm, and these lines in argocd-rbac-cm's policy.csv, with <project>/<application> the Application that manages it and <namespace> its namespace:
  
  p, role:p0-connector, applications, get, <project>/<application>, allow
  p, role:p0-connector, applications, update//ConfigMap/<namespace>/argocd-rbac-cm, <project>/<application>, allow
  p, role:p0-connector, projects, get, *, allow
  g, p0-connector, role:p0-connector
  
  argocd-rbac-cm's scopes must include email, since P0 grants roles to users by their email.
  A secret that holds the account's API token as JSON, {"token": "<api token>"}, in the secret manager that vault names. Generate the token with argocd account generate-token --account p0-connector.
  For aws-sm, p0_aws_secrets_manager installed for the account that holds the secret. Give the connector's role secretsmanager:GetSecretValue on the secret alone, and kms:Decrypt on its key if a customer-managed KMS key encrypts it. A secret in an account other than the connector's needs a customer-managed key, and the secret's resource policy and the key's policy must allow the connector's role.
  For gcp-sm, Google Secret Manager installed in P0 for the project that holds the secret. This provider has no resource for it, so install it in the P0 app. Give the connector's service account roles/secretmanager.secretAccessor and roles/secretmanager.viewer on the secret alone.
  Note: This integration is currently in beta.
  For the full install steps, see Installing Argo CD https://docs.p0.dev/integrations/resource-integrations/argocd/installation.
---

# p0_argocd_instance (Resource)

An Argo CD instance that P0 can grant roles on.

P0 reaches the instance through an Argo CD connector, which you install with `p0_argocd_connector`. The connector calls the instance's API as the Argo CD local account `p0-connector`, whose API token stays in your own secret manager: P0 never reads it, and only the connector does. One connector can serve several instances, and each instance has its own token and secret.

P0 grants a role as a line in the `policy.p0.csv` key of the instance's `argocd-rbac-cm`, which it writes through the Argo CD Application that `managing_app` names. P0 owns that key: leave it out of Git.

**Important:** Install the connector first, and grant its identity read access to the token's secret: in an earlier apply, or in the same one with this resource depending on the grant, as the examples do. Creating this resource has P0 check the instance through the connector. If the check fails, the apply fails with P0's message.

An instance's attributes can't change once it is created. Changing `id`, `connector`, `vault` or `secret_name` replaces the instance. A plan refuses a change to an installed instance's `url`, `managing_app` or `namespace`: P0 revokes access where it granted it, so for another instance, Application or namespace, add a new `p0_argocd_instance` with its own `id`, and remove this one once its open access has ended. To rotate the API token, generate a new one and add it as a new version of the same secret, which needs no change here.

**Warning:** Replacing or destroying this resource removes the instance from P0. P0 can't revoke access it granted on an instance it no longer has, so those lines stay in `policy.p0.csv` until you remove them there. Before you change an attribute or destroy this resource, revoke the instance's open access in P0 or wait for it to expire.

An instance that already exists in P0, such as one added in the P0 app, isn't created again: the apply fails, and says to import it with `terraform import`. An imported instance that P0 hasn't finished installing plans an update, and applying it finishes the install. If that install can't finish, fix what P0's message names and apply again, or start over: `terraform apply -replace=<address>` deletes the instance and creates it again, and `terraform state rm <address>` stops managing it.

The email domains that may receive access are set on the connector, as its `ALLOWED_EMAIL_DOMAIN_PATTERN` environment variable, and aren't an attribute of this resource.

**Prerequisites:**

- Argo CD 3.0 or later, served at the root of its address. An Argo CD served under a root path (`server.rootpath`) isn't supported.

- An Argo CD Application that manages `argocd-rbac-cm`. Without one, P0 can't write grants.

- The local account `p0-connector`, with `accounts.p0-connector: apiKey` in `argocd-cm`, and these lines in `argocd-rbac-cm`'s `policy.csv`, with `<project>/<application>` the Application that manages it and `<namespace>` its namespace:

  ```
  p, role:p0-connector, applications, get, <project>/<application>, allow
  p, role:p0-connector, applications, update//ConfigMap/<namespace>/argocd-rbac-cm, <project>/<application>, allow
  p, role:p0-connector, projects, get, *, allow
  g, p0-connector, role:p0-connector
  ```

  `argocd-rbac-cm`'s `scopes` must include `email`, since P0 grants roles to users by their email.

- A secret that holds the account's API token as JSON, `{"token": "<api token>"}`, in the secret manager that `vault` names. Generate the token with `argocd account generate-token --account p0-connector`.

- For `aws-sm`, `p0_aws_secrets_manager` installed for the account that holds the secret. Give the connector's role `secretsmanager:GetSecretValue` on the secret alone, and `kms:Decrypt` on its key if a customer-managed KMS key encrypts it. A secret in an account other than the connector's needs a customer-managed key, and the secret's resource policy and the key's policy must allow the connector's role.

- For `gcp-sm`, Google Secret Manager installed in P0 for the project that holds the secret. This provider has no resource for it, so install it in the P0 app. Give the connector's service account `roles/secretmanager.secretAccessor` and `roles/secretmanager.viewer` on the secret alone.

**Note:** This integration is currently in beta.

For the full install steps, see [Installing Argo CD](https://docs.p0.dev/integrations/resource-integrations/argocd/installation).

## Example Usage

### Google Cloud

```terraform
# Adds an Argo CD instance that P0's Argo CD connector on Google Cloud Run
# reaches, with the p0-connector account's API token in Google Secret Manager.
# For a connector on AWS Lambda, see aws.tf in this directory.
#
# Builds on the p0_argocd_connector example, which deploys and installs the
# connector. Google Secret Manager must already be installed in P0 for the
# secret's project: this provider has no resource for it, so install it in the
# P0 app.
#
# Before you apply this, create the p0-connector account in Argo CD, as this
# resource's page describes, and store its API token as the secret
# argocd-prod-token, in the JSON {"token": "<api token>"}. The example doesn't
# create the secret, which keeps the token out of Terraform's state. To store
# it, with the argocd CLI signed in as an admin and gcloud signed in to the
# secret's project:
#
#   argocd account generate-token --account p0-connector |
#     jq -Rc '{token: .}' |
#     gcloud secrets create argocd-prod-token --project my-secrets-project \
#       --data-file -

locals {
  argocd_instance = {
    # The instance's address: https and the host, with nothing after it.
    url = "https://argocd.example.com"
    # The Application that manages argocd-rbac-cm, as <project>/<application>.
    managing_app = "infra/argocd"
    # The project that holds the secret, where Google Secret Manager is
    # installed in P0.
    secrets_project_id = "my-secrets-project"
    secret_name        = "argocd-prod-token"
  }
}

# The connector reads the API token's secret, and no other. It reads the
# secret's latest version and then that version's metadata, and the accessor
# role covers only the first.
resource "google_secret_manager_secret_iam_member" "argocd_accessor" {
  project   = local.argocd_instance.secrets_project_id
  secret_id = local.argocd_instance.secret_name
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${google_service_account.argocd.email}"
}

resource "google_secret_manager_secret_iam_member" "argocd_viewer" {
  project   = local.argocd_instance.secrets_project_id
  secret_id = local.argocd_instance.secret_name
  role      = "roles/secretmanager.viewer"
  member    = "serviceAccount:${google_service_account.argocd.email}"
}

# Creating it has P0 check the instance through the connector, so it waits
# until the connector is installed and the grants exist. IAM changes can take a
# few seconds to take effect, so if the first check is denied access, apply
# again.
resource "p0_argocd_instance" "example" {
  id           = "prod"
  connector    = p0_argocd_connector.example.id
  url          = local.argocd_instance.url
  managing_app = local.argocd_instance.managing_app

  vault = {
    type       = "gcp-sm"
    project_id = local.argocd_instance.secrets_project_id
  }
  secret_name = local.argocd_instance.secret_name

  depends_on = [
    google_secret_manager_secret_iam_member.argocd_accessor,
    google_secret_manager_secret_iam_member.argocd_viewer,
  ]
}
```

### AWS

```terraform
# The AWS variant of the p0_argocd_instance example in this directory: an Argo
# CD instance that P0's Argo CD connector on AWS Lambda reaches, with the
# p0-connector account's API token in AWS Secrets Manager.
#
# Builds on aws.tf in the p0_argocd_connector example, which deploys and
# installs the connector, and on the p0_aws_secrets_manager example, which
# installs AWS Secrets Manager in P0 for the secret's account.
#
# Before you apply this, create the p0-connector account in Argo CD, as this
# resource's page describes, and store its API token as the secret
# argocd/prod/token, in the JSON {"token": "<api token>"}. To store it, with the
# argocd CLI signed in as an admin and the AWS CLI signed in to the secret's
# account:
#
#   argocd account generate-token --account p0-connector |
#     jq -Rc '{token: .}' |
#     aws secretsmanager create-secret --name argocd/prod/token \
#       --secret-string file:///dev/stdin --region us-east-1

locals {
  argocd_instance_aws = {
    url            = "https://argocd.example.com"
    managing_app   = "infra/argocd"
    secrets_region = "us-east-1"
    secret_name    = "argocd/prod/token"
    # The ARN of the customer-managed KMS key that encrypts the secret, or null
    # if the secret uses the default aws/secretsmanager key. A secret in an
    # account other than the connector's needs a customer-managed key.
    kms_key_arn = null
  }
}

data "aws_partition" "argocd" {
  provider = aws.argocd
}

# The connector reads the API token's secret, and no other.
resource "aws_iam_role_policy" "argocd_token_secret" {
  provider = aws.argocd

  name = "p0-argocd-prod-aws-read"
  role = aws_iam_role.argocd.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = concat(
      [{
        Sid    = "ReadSecret"
        Effect = "Allow"
        Action = "secretsmanager:GetSecretValue"
        # Secrets Manager adds a hyphen and six random characters to the name
        # in the secret's ARN, and "??????" matches those, so the grant gets
        # this one secret and no other.
        Resource = join(":", [
          "arn:aws:secretsmanager",
          local.argocd_instance_aws.secrets_region,
          p0_aws_secrets_manager.example.account_id,
          "secret",
          "${local.argocd_instance_aws.secret_name}-??????",
        ])
      }],
      local.argocd_instance_aws.kms_key_arn == null ? [] : [{
        Sid      = "DecryptSecret"
        Effect   = "Allow"
        Action   = "kms:Decrypt"
        Resource = local.argocd_instance_aws.kms_key_arn
        Condition = {
          StringEquals = {
            "kms:ViaService" = "secretsmanager.${local.argocd_instance_aws.secrets_region}.${data.aws_partition.argocd.dns_suffix}"
          }
        }
      }],
    )
  })
}

# Creating it has P0 check the instance through the connector. IAM changes can
# take a few seconds to take effect, so if the first check is denied access,
# apply again.
resource "p0_argocd_instance" "aws_example" {
  id           = "prod-aws"
  connector    = p0_argocd_connector.aws_example.id
  url          = local.argocd_instance_aws.url
  managing_app = local.argocd_instance_aws.managing_app

  vault = {
    type           = "aws-sm"
    account_id     = p0_aws_secrets_manager.example.account_id
    secrets_region = local.argocd_instance_aws.secrets_region
  }
  secret_name = local.argocd_instance_aws.secret_name

  depends_on = [aws_iam_role_policy.argocd_token_secret]
}
```

<!-- schema generated by tfplugindocs -->
## Schema

### Required

- `connector` (String) The `id` of the `p0_argocd_connector` that reaches this instance. The connector must be installed, and run in the same cloud as `vault`: on AWS Lambda for `aws-sm`, on Google Cloud Run for `gcp-sm`.
- `id` (String) A name you choose for the instance, such as `prod`: lowercase letters, digits and hyphens, starting and ending with a letter or digit, up to 63 characters
- `managing_app` (String) The Argo CD Application that manages `argocd-rbac-cm`, as `<project>/<application>`, such as `default/argocd`: each name lowercase letters, digits, hyphens and dots, up to 253 characters. The connector writes grants through it, and the `p0-connector` account's policy names it. It can't change: for another Application, add a new `p0_argocd_instance` with its own `id`.
- `secret_name` (String) The name of the secret that holds the API token, in `vault`: the name alone, never an ARN or a resource name. For `aws-sm`, up to 512 letters, digits and `/_+=.@-`, and, when the vault's account isn't the connector's, not ending in a hyphen and six letters or digits, like `-AbCdEf`. For `gcp-sm`, up to 255 letters, digits, underscores and hyphens.
- `url` (String) The address your Argo CD instance is served at, such as `https://argocd.example.com`: `https://` and the host, with a port if it isn't 443, in lowercase and with nothing after it. Leave out a trailing slash, any path, and any username or password. P0 stores the address in this spelling alone, and the connector sends the API token to it. It can't change: for another address, add a new `p0_argocd_instance` with its own `id`.
- `vault` (Attributes) The secret manager that holds the p0-connector account's API token, as installed in P0. P0 never reads the secret. The connector does, so the secret manager must be in the connector's cloud: AWS Secrets Manager for a connector on AWS Lambda, Google Secret Manager for one on Google Cloud Run. (see [below for nested schema](#nestedatt--vault))

### Optional

- `namespace` (String) The Kubernetes namespace `argocd-rbac-cm` is in: up to 63 lowercase letters, digits and hyphens, starting and ending with a letter or digit. Leave it out, rather than empty, for `argocd`, Argo CD's default. It can't change: for another namespace, add a new `p0_argocd_instance` with its own `id`.

### Read-Only

- `state` (String) This item's install progress in the P0 application:
	- 'stage': The item has been staged for installation
	- 'configure': The item is available to be added to P0, and may be configured
	- 'installed': The item is fully installed

<a id="nestedatt--vault"></a>
### Nested Schema for `vault`

Required:

- `type` (String) The secret manager: `aws-sm` (AWS Secrets Manager) or `gcp-sm` (Google Secret Manager)

Optional:

- `account_id` (String) The ID of the AWS account that holds the secret: one of your AWS Secrets Manager installations in P0 (`p0_aws_secrets_manager`). Required for, and only valid with, `aws-sm`.
- `project_id` (String) The ID of the Google Cloud project that holds the secret: one of your Google Secret Manager installations in P0. This provider has no resource for that installation, so install Google Secret Manager in the P0 app first. Required for, and only valid with, `gcp-sm`.
- `secrets_region` (String) The AWS region the secret is stored in, such as `us-west-2`. Required for, and only valid with, `aws-sm`.
