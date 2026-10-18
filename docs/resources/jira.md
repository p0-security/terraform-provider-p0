---
page_title: "p0_jira Resource - p0"
subcategory: ""
description: |-
  A Jira installation for one Jira Cloud site.
  Installing Jira lets P0 grant your users just-in-time roles on the site's company-managed projects. P0 acts through a connector that you deploy in your own AWS account or Google Cloud project, one for each site. The connector signs in to Jira as an Atlassian service account, with an OAuth 2.0 credential whose client secret stays in your secret manager: P0 never reads it, and only the connector does.
  Important: Create the service account's OAuth credential and store its client secret before you apply this resource, and deploy the connector first: in an earlier apply, or in the same one with this resource depending on the connector's access grants, as the examples do. Creating this resource has P0 check the site through the connector: that P0 can invoke the connector, that the connector can read the client secret and sign in to the site with it, and that the service account holds the Jira permissions P0 uses. If a check fails, the apply fails with P0's message.
  Changing site, hosting, vault, client_secret_name or domain_pattern replaces the installation. To rotate the client secret, add a new version to the same secret, which needs no change here. Changing client_id updates the installation in place, and P0 checks the site again. If the check fails, the apply fails, and an installed site keeps its current client ID. An installation that P0 hasn't finished, such as one imported before its checks passed, plans an update, and applying it finishes the install.
  domain_pattern is also the connector's DOMAIN_PATTERN environment variable, and P0 checks that the two are the same whenever it checks the site. To change the pattern, change both in one apply: the connector is deployed with the new pattern, and the installation is replaced with it.
  Warning: Replacing or destroying this resource removes the site from P0, and P0 can't revoke access on a site it no longer has. Before you change an attribute that replaces the installation, or destroy this resource, revoke the site's open access in P0 or wait for it to expire.
  P0 checks the site's host, where the connector runs, the vault, the secret's name and the domain pattern when it creates the installation. A plan checks all of them by P0's rules, except whether domain_pattern compiles and is safe to match, which waits for P0 at apply. The connector and the secret must be in the same cloud, and on AWS in commercial regions: Jira doesn't support AWS GovCloud, China, ISO or European Sovereign Cloud regions.
  Prerequisites:
  
  An Atlassian service account in the organization that owns the site, with access to Jira on the site. Add it to a group that holds the Jira global permissions Administer Jira, to change project roles, and Browse users and groups, to find people by their email. Give it an OAuth 2.0 credential with the scopes manage:jira-configuration, manage:jira-project, read:jira-user and read:jira-work, and store the credential's client secret as a secret in the secret manager that vault names.
  Warning: Administer Jira lets the service account change every project's roles, and the site's configuration.
  P0's Jira connector, deployed where hosting says, with its DOMAIN_PATTERN environment variable set to domain_pattern. P0's Jira installer generates Terraform that deploys the connector with its image pinned, as in the examples.
  Grant the connector read access to the client secret alone, never through a wildcard that matches other secrets, or at the account or project level. P0 names the secret in each call, so any other secret the connector can read is one a call could point it at.
  For AWS Lambda, the image p0security/p0-connector-jira, running as an IAM role named hosting.connector_name. Lambda runs images only from Amazon ECR, so copy the image from Docker Hub into ECR first. Give the role secretsmanager:GetSecretValue on arn:aws:secretsmanager:<secrets_region>:<account_id>:secret:<client_secret_name>-??????, with the vault's region and account, and kms:Decrypt on the secret's key if a customer-managed KMS key encrypts it. A secret in an account other than the connector's needs a customer-managed key, and the secret's resource policy and the key's policy must allow the connector's role. p0_aws_iam_write must be installed for the account the connector runs in. Grant that installation's role lambda:InvokeFunction on the connector's function.
  For Google Cloud Run, the image p0security/p0-connector-jira-gcloud, running as hosting.connector_service_account: the service account whose ID is hosting.connector_name, in the connector's project. Give it roles/secretmanager.secretAccessor and roles/secretmanager.viewer on the secret alone. p0_gcp must be installed. Grant its service account roles/run.invoker and roles/run.viewer on the connector's service, and pass that service account's email to the connector as its INVOKER_SA_EMAIL environment variable.
  Note: This integration is in beta.
  For the full install steps, see Jira https://docs.p0.dev/integrations/resource-integrations/jira.
---

# p0_jira (Resource)

A Jira installation for one Jira Cloud site.

Installing Jira lets P0 grant your users just-in-time roles on the site's company-managed projects. P0 acts through a connector that you deploy in your own AWS account or Google Cloud project, one for each site. The connector signs in to Jira as an Atlassian service account, with an OAuth 2.0 credential whose client secret stays in your secret manager: P0 never reads it, and only the connector does.

**Important:** Create the service account's OAuth credential and store its client secret before you apply this resource, and deploy the connector first: in an earlier apply, or in the same one with this resource depending on the connector's access grants, as the examples do. Creating this resource has P0 check the site through the connector: that P0 can invoke the connector, that the connector can read the client secret and sign in to the site with it, and that the service account holds the Jira permissions P0 uses. If a check fails, the apply fails with P0's message.

Changing `site`, `hosting`, `vault`, `client_secret_name` or `domain_pattern` replaces the installation. To rotate the client secret, add a new version to the same secret, which needs no change here. Changing `client_id` updates the installation in place, and P0 checks the site again. If the check fails, the apply fails, and an installed site keeps its current client ID. An installation that P0 hasn't finished, such as one imported before its checks passed, plans an update, and applying it finishes the install.

`domain_pattern` is also the connector's `DOMAIN_PATTERN` environment variable, and P0 checks that the two are the same whenever it checks the site. To change the pattern, change both in one apply: the connector is deployed with the new pattern, and the installation is replaced with it.

**Warning:** Replacing or destroying this resource removes the site from P0, and P0 can't revoke access on a site it no longer has. Before you change an attribute that replaces the installation, or destroy this resource, revoke the site's open access in P0 or wait for it to expire.

P0 checks the site's host, where the connector runs, the vault, the secret's name and the domain pattern when it creates the installation. A plan checks all of them by P0's rules, except whether `domain_pattern` compiles and is safe to match, which waits for P0 at apply. The connector and the secret must be in the same cloud, and on AWS in commercial regions: Jira doesn't support AWS GovCloud, China, ISO or European Sovereign Cloud regions.

**Prerequisites:**

- An Atlassian service account in the organization that owns the site, with access to Jira on the site. Add it to a group that holds the Jira global permissions Administer Jira, to change project roles, and Browse users and groups, to find people by their email. Give it an OAuth 2.0 credential with the scopes `manage:jira-configuration`, `manage:jira-project`, `read:jira-user` and `read:jira-work`, and store the credential's client secret as a secret in the secret manager that `vault` names.

  **Warning:** Administer Jira lets the service account change every project's roles, and the site's configuration.

- P0's Jira connector, deployed where `hosting` says, with its `DOMAIN_PATTERN` environment variable set to `domain_pattern`. P0's Jira installer generates Terraform that deploys the connector with its image pinned, as in the examples.

  Grant the connector read access to the client secret alone, never through a wildcard that matches other secrets, or at the account or project level. P0 names the secret in each call, so any other secret the connector can read is one a call could point it at.

- For AWS Lambda, the image `p0security/p0-connector-jira`, running as an IAM role named `hosting.connector_name`. Lambda runs images only from Amazon ECR, so copy the image from Docker Hub into ECR first. Give the role `secretsmanager:GetSecretValue` on `arn:aws:secretsmanager:<secrets_region>:<account_id>:secret:<client_secret_name>-??????`, with the vault's region and account, and `kms:Decrypt` on the secret's key if a customer-managed KMS key encrypts it. A secret in an account other than the connector's needs a customer-managed key, and the secret's resource policy and the key's policy must allow the connector's role. `p0_aws_iam_write` must be installed for the account the connector runs in. Grant that installation's role `lambda:InvokeFunction` on the connector's function.

- For Google Cloud Run, the image `p0security/p0-connector-jira-gcloud`, running as `hosting.connector_service_account`: the service account whose ID is `hosting.connector_name`, in the connector's project. Give it `roles/secretmanager.secretAccessor` and `roles/secretmanager.viewer` on the secret alone. `p0_gcp` must be installed. Grant its service account `roles/run.invoker` and `roles/run.viewer` on the connector's service, and pass that service account's email to the connector as its `INVOKER_SA_EMAIL` environment variable.

**Note:** This integration is in beta.

For the full install steps, see [Jira](https://docs.p0.dev/integrations/resource-integrations/jira).

## Example Usage

### AWS Lambda

```terraform
# Installs Jira for the site acme.atlassian.net, with P0's Jira connector on AWS
# Lambda and the service account's OAuth client secret in AWS Secrets Manager.
# For Google Cloud Run, see gcp.tf in this directory.
#
# Before you apply this, create the site's Atlassian service account and its
# OAuth credential, store the credential's client secret in Secrets Manager as
# p0/jira/acme, and install p0_aws_iam_write for the account the connector runs
# in. This resource's page lists what the service account needs. The example
# doesn't create the secret, which keeps the client secret out of Terraform's
# state.
#
# Creating p0_jira has the connector read the client secret, so store the
# client secret before you apply. To store it, with the AWS CLI signed in to the secret's
# account:
#
#   aws secretsmanager create-secret \
#     --name p0/jira/acme \
#     --secret-string file://client-secret.txt --region us-east-1
#
# client-secret.txt holds the credential's client secret, and us-east-1 is the
# secret's region, secrets_region below. To encrypt the secret with a
# customer-managed KMS key, which a secret in another account needs, add
# --kms-key-id with the key's ARN to create-secret.
#
# The connector's Terraform follows what P0's Jira installer generates for the
# site, with p0_jira in place of its output, and names the secret by its account
# and name where the installer uses a partial ARN. The installer pins the
# current connector image.

# P0's Jira connector for the site acme.atlassian.net, on AWS Lambda.
#
# Applying this copies P0's connector image from Docker Hub into an ECR
# repository, which Lambda requires. Run terraform where Docker and the AWS CLI
# are installed and the AWS CLI is signed in to account 123456789012.
#
# Other sites can share this configuration, but each needs its own connector
# name: the connector's role, repository and log group are named after it, and
# these names are unique in an AWS account.
#
# The connector reads the site's client secret from AWS Secrets Manager in
# this account. It needs no secret to exist yet: until the secret does, the
# connector just has nothing to read.
# This doesn't create the secret, and never reads or holds its value.

terraform {
  # terraform_data needs Terraform 1.4.
  required_version = ">= 1.4"

  required_providers {
    p0 = {
      source = "p0-security/p0"
    }
    aws = {
      source = "hashicorp/aws"
    }
    docker = {
      source  = "kreuzwerker/docker"
      version = ">= 3.3, < 5.0"
    }
  }
}

locals {
  p0_jira_site_acme = {
    # Where P0 invokes the connector. P0's installer has the same values.
    account_id       = "123456789012"
    connector_name   = "p0-jira-site-acme"
    connector_region = "us-west-2"
    # The connector gives Jira access only to users whose whole email domain
    # matches this. It's also p0_jira's domain_pattern, which P0 checks against
    # the connector's. A new pattern replaces the site in P0.
    domain_pattern = "(.+\\.)?example\\.com"
    # How many days CloudWatch keeps the connector's logs. Change it if you
    # need to.
    log_retention_days = 30
    # The role P0 assumes in this account, which invokes the connector.
    p0_role_name = "P0RoleIamManager"
    # P0's connector image: its tag on Docker Hub, and the digest that tag must
    # resolve to. These are placeholders, for an image that doesn't exist. Use
    # the pinned values in the Terraform that P0's installer shows, or the
    # image's published tag and digest.
    image_tag    = "sha-1234567"
    image_digest = "sha256:abababababababababababababababababababababababababababababababab"
    # The secret that holds the client secret: its name, and the account and
    # region it's in. A new secret replaces the site in P0. To rotate the
    # client secret, add a new version to this secret instead.
    client_secret_name = "p0/jira/acme"
    secrets_account_id = "123456789012"
    secrets_region     = "us-east-1"
    # The ARN of the KMS key that encrypts the secret, if that's a
    # customer-managed key: the key's own ARN, not an alias. The default
    # aws/secretsmanager key needs nothing for a secret in this account.
    kms_key_arn = null
  }
}

# P0 derives the connector's ARN from the account, region and name above, so the
# connector must be deployed exactly there. Add the credential settings your
# other AWS providers use, such as profile or assume_role, if they have any.
provider "aws" {
  alias               = "p0_jira_site_acme"
  region              = local.p0_jira_site_acme.connector_region
  allowed_account_ids = [local.p0_jira_site_acme.account_id]
}

data "aws_partition" "p0_jira_site_acme" {
  provider = aws.p0_jira_site_acme
}

# Stops the plan if the tag has been moved to another image since P0 pinned
# it, so that only the image P0 pinned is deployed.
data "docker_registry_image" "p0_jira_site_acme" {
  name = "p0security/p0-connector-jira:${local.p0_jira_site_acme.image_tag}"

  lifecycle {
    postcondition {
      condition     = self.sha256_digest == local.p0_jira_site_acme.image_digest
      error_message = "The connector image's tag doesn't resolve to the digest pinned here. Copy the current Terraform from P0's Jira installer, or contact support@p0.dev."
    }
  }
}

resource "aws_ecr_repository" "p0_jira_site_acme" {
  provider = aws.p0_jira_site_acme

  name = local.p0_jira_site_acme.connector_name
  # Tags can't be overwritten, so no one who can push to this repository can
  # replace the image that runs as the connector.
  image_tag_mutability = "IMMUTABLE"
  force_delete         = true

  image_scanning_configuration {
    scan_on_push = true
  }
}

# Copies the pinned image from Docker Hub into the ECR repository, tagged with
# the image's tag and this copy's id. ECR's tags can't be overwritten, so each
# copy needs a tag of its own: copying the image again, to roll back or after
# an interrupted apply, pushes a new one.
resource "terraform_data" "p0_jira_site_acme_image" {
  triggers_replace = {
    digest         = data.docker_registry_image.p0_jira_site_acme.sha256_digest
    repository_url = aws_ecr_repository.p0_jira_site_acme.repository_url
    tag            = local.p0_jira_site_acme.image_tag
  }

  provisioner "local-exec" {
    # Signs Docker in to ECR in a configuration directory of its own, which it
    # deletes at the end. So the login doesn't outlast the copy, connectors
    # copied at the same time don't share one, and your own Docker logins are
    # left alone. The Docker daemon is still the one your Docker context
    # points at.
    command = <<-EOT
      set -e
      if [ -z "$DOCKER_HOST" ]; then
        DOCKER_HOST="$(docker context inspect --format '{{ .Endpoints.docker.Host }}')"
        export DOCKER_HOST
      fi
      DOCKER_CONFIG="$(mktemp -d)"
      export DOCKER_CONFIG
      trap 'rm -rf "$DOCKER_CONFIG"' EXIT
      aws ecr get-login-password --region "$REGION" |
        docker login --username AWS --password-stdin "$REGISTRY"
      docker pull --platform linux/amd64 "$SOURCE"
      docker tag "$SOURCE" "$TARGET"
      docker push "$TARGET"
    EOT

    environment = {
      REGION   = local.p0_jira_site_acme.connector_region
      REGISTRY = split("/", aws_ecr_repository.p0_jira_site_acme.repository_url)[0]
      SOURCE   = "p0security/p0-connector-jira@${local.p0_jira_site_acme.image_digest}"
      TARGET   = "${aws_ecr_repository.p0_jira_site_acme.repository_url}:${local.p0_jira_site_acme.image_tag}-${self.id}"
    }
  }
}

# Pulling one platform and pushing it gives ECR that image's own manifest, not
# the tag's multi-platform one, so the digest there can differ from the one
# pinned above. Lambda needs ECR's.
data "aws_ecr_image" "p0_jira_site_acme" {
  provider = aws.p0_jira_site_acme

  repository_name = aws_ecr_repository.p0_jira_site_acme.name
  image_tag       = "${local.p0_jira_site_acme.image_tag}-${terraform_data.p0_jira_site_acme_image.id}"
}

# The identity the connector runs as, named after the connector.
resource "aws_iam_role" "p0_jira_site_acme" {
  provider = aws.p0_jira_site_acme

  name = local.p0_jira_site_acme.connector_name
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Service = "lambda.amazonaws.com" }
      Action    = "sts:AssumeRole"
    }]
  })
}

# The log group Lambda writes the connector's logs to.
resource "aws_cloudwatch_log_group" "p0_jira_site_acme" {
  provider = aws.p0_jira_site_acme

  name              = "/aws/lambda/${local.p0_jira_site_acme.connector_name}"
  retention_in_days = local.p0_jira_site_acme.log_retention_days
}

# The connector writes its own logs, and no others.
resource "aws_iam_role_policy" "p0_jira_site_acme_logs" {
  provider = aws.p0_jira_site_acme

  name = "WriteConnectorLogs"
  role = aws_iam_role.p0_jira_site_acme.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = ["logs:CreateLogStream", "logs:PutLogEvents"]
      Resource = "${aws_cloudwatch_log_group.p0_jira_site_acme.arn}:*"
    }]
  })
}

# The connector reads the site's client secret, and nothing else.
resource "aws_iam_role_policy" "p0_jira_site_acme_read" {
  provider = aws.p0_jira_site_acme

  name = "ReadJiraClientSecret"
  role = aws_iam_role.p0_jira_site_acme.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = concat(
      [{
        Sid    = "ReadSecret"
        Effect = "Allow"
        Action = "secretsmanager:GetSecretValue"
        # "??????" matches the six random characters that end the secret's
        # ARN, so the connector gets this one secret and no other.
        Resource = "arn:aws:secretsmanager:${local.p0_jira_site_acme.secrets_region}:${local.p0_jira_site_acme.secrets_account_id}:secret:${local.p0_jira_site_acme.client_secret_name}-??????"
      }],
      local.p0_jira_site_acme.kms_key_arn == null ? [] : [{
        Sid      = "DecryptSecret"
        Effect   = "Allow"
        Action   = "kms:Decrypt"
        Resource = local.p0_jira_site_acme.kms_key_arn
        Condition = {
          StringEquals = {
            "kms:ViaService" = "secretsmanager.${local.p0_jira_site_acme.secrets_region}.${data.aws_partition.p0_jira_site_acme.dns_suffix}"
          }
        }
      }],
    )
  })

  lifecycle {
    # A key in the secret's region, by its ARN. It may be in another account.
    precondition {
      condition     = local.p0_jira_site_acme.kms_key_arn == null || can(regex("^arn:aws:kms:${local.p0_jira_site_acme.secrets_region}:[0-9]{12}:key/(mrk-)?[0-9a-f-]+$", local.p0_jira_site_acme.kms_key_arn))
      error_message = "Set kms_key_arn to the ARN of a KMS key in the secret's region, such as arn:aws:kms:<region>:<account>:key/<key id>. An alias, or a key's ID alone, won't do: the policy needs the key's ARN."
    }
  }
}

# No vpc_config: the connector calls Atlassian over the internet.
resource "aws_lambda_function" "p0_jira_site_acme" {
  provider = aws.p0_jira_site_acme

  function_name = local.p0_jira_site_acme.connector_name
  description   = "P0 Jira connector for acme.atlassian.net"
  role          = aws_iam_role.p0_jira_site_acme.arn
  package_type  = "Image"
  image_uri     = "${aws_ecr_repository.p0_jira_site_acme.repository_url}@${data.aws_ecr_image.p0_jira_site_acme.image_digest}"
  architectures = ["x86_64"]
  memory_size   = 512
  timeout       = 60

  environment {
    variables = {
      DOMAIN_PATTERN = local.p0_jira_site_acme.domain_pattern
    }
  }

  # The connector's role can't create a log group, so its own must exist before
  # it runs.
  depends_on = [aws_cloudwatch_log_group.p0_jira_site_acme]
}

# P0 invokes the connector as its role in this account, and needs nothing else
# on it.
resource "aws_iam_role_policy" "p0_jira_site_acme_invoke" {
  provider = aws.p0_jira_site_acme

  name = "${local.p0_jira_site_acme.connector_name}-invoke"
  role = local.p0_jira_site_acme.p0_role_name
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = "lambda:InvokeFunction"
      Resource = aws_lambda_function.p0_jira_site_acme.arn
    }]
  })
}

# Installs Jira for the site. Creating it has P0 check the site through the
# connector, so it waits until P0 can invoke the connector and the connector
# can read the client secret. IAM changes can take a few seconds to take
# effect, so if the first check is denied access, apply again.
resource "p0_jira" "example" {
  site = "acme.atlassian.net"

  hosting = {
    type             = "aws"
    account_id       = local.p0_jira_site_acme.account_id
    connector_name   = aws_lambda_function.p0_jira_site_acme.function_name
    connector_region = local.p0_jira_site_acme.connector_region
  }

  vault = {
    type           = "aws-sm"
    account_id     = local.p0_jira_site_acme.secrets_account_id
    secrets_region = local.p0_jira_site_acme.secrets_region
  }
  client_secret_name = local.p0_jira_site_acme.client_secret_name
  domain_pattern     = local.p0_jira_site_acme.domain_pattern

  # The client ID of the service account's OAuth credential. A new credential
  # updates the site in place.
  client_id = "AbCdEfGhIjKlMnOpQrStUvWxYz012345"

  depends_on = [
    aws_iam_role_policy.p0_jira_site_acme_invoke,
    aws_iam_role_policy.p0_jira_site_acme_read,
  ]
}
```

### Google Cloud Run

```terraform
# The Google Cloud Run variant of the p0_jira example in resource.tf, for the
# site acme-dev.atlassian.net, with the service account's OAuth client secret in
# Google Secret Manager.
#
# Before you apply this, create the site's Atlassian service account and its
# OAuth credential, store the credential's client secret in Secret Manager as
# jira-acme-dev in the project secrets-project, and install p0_gcp (see its
# example). To store the secret, with the gcloud CLI signed in:
#
#   gcloud secrets create jira-acme-dev \
#     --project secrets-project --data-file=client-secret.txt
#
# The connector's Terraform is what P0's Jira installer generates for the site,
# with p0_jira in place of its output. Copy it from the installer, which pins
# the current connector image.

# P0's Jira connector for the site acme-dev.atlassian.net, on Google Cloud Run.
#
# It needs Terraform's google provider at 6.0 or later, and the Service Usage
# API enabled in project jira-host,
# which lets Terraform enable the IAM and Cloud Run APIs. If the first apply
# fails right after it enables them, apply again: a newly enabled API can take
# a few minutes to answer.
#
# Cloud Run pulls P0's connector image from Docker Hub. Terraform reads the
# image's digest from Docker Hub on every plan, so the machine that runs
# Terraform has to reach it. It needs no Docker.
#
# Other sites can share this configuration, but each needs its own connector
# name: the connector's service and service account are named after it.
#
# The secret has to exist already, holding the client secret. This doesn't
# create it, and never reads or holds its value.

terraform {
  # The image's digest check needs Terraform 1.2.
  required_version = ">= 1.2"

  # If your configuration already has a required_providers block, such as the
  # one in resource.tf, add these to it, and delete this one.
  required_providers {
    p0 = {
      source = "p0-security/p0"
    }
    google = {
      source  = "hashicorp/google"
      version = ">= 6.0"
    }
    docker = {
      source  = "kreuzwerker/docker"
      version = ">= 3.3, < 5.0"
    }
  }
}

locals {
  p0_jira_site_acme_dev = {
    # Where P0 invokes the connector. P0's installer has the same values.
    project_id       = "jira-host"
    connector_name   = "p0-jira-site-acme-dev"
    connector_region = "us-west1"
    # P0's own service account, from the p0_gcp example. The connector accepts
    # calls from it alone.
    invoker_email = p0_gcp.example.service_account_email
    # The connector gives Jira access only to users whose whole email domain
    # matches this. It's also p0_jira's domain_pattern, which P0 checks against
    # the connector's. A new pattern replaces the site in P0.
    domain_pattern = "example\\.com"
    # The secret that holds the client secret, and the project it's in. A new
    # secret replaces the site in P0. To rotate the client secret, add a new
    # version to this secret instead.
    secrets_project_id = "secrets-project"
    secret_name        = "jira-acme-dev"
    # P0's connector image: its tag on Docker Hub, and the digest that tag must
    # resolve to. These are placeholders, for an image that doesn't exist. Use
    # the pinned values in the Terraform that P0's installer shows, or the
    # image's published tag and digest.
    image_tag    = "sha-1234567"
    image_digest = "sha256:abababababababababababababababababababababababababababababababab"
  }
}

# Reads the image's digest from Docker Hub. It never talks to a Docker daemon,
# so none has to be running.
provider "docker" {
  alias                       = "p0_jira_site_acme_dev"
  disable_docker_daemon_check = true
}

# Stops the plan if the tag has been moved to another image since P0 pinned
# it, so that only the image P0 pinned is deployed.
data "docker_registry_image" "p0_jira_site_acme_dev" {
  provider = docker.p0_jira_site_acme_dev

  name = "p0security/p0-connector-jira-gcloud:${local.p0_jira_site_acme_dev.image_tag}"

  lifecycle {
    postcondition {
      condition     = self.sha256_digest == local.p0_jira_site_acme_dev.image_digest
      error_message = "The connector image's tag doesn't resolve to the digest pinned here. Copy the current Terraform from P0's Jira installer, or contact support@p0.dev."
    }
  }
}

# The IAM and Cloud Run APIs. Never disabled on destroy: other services in the
# project may use them.
resource "google_project_service" "p0_jira_site_acme_dev" {
  for_each = toset(["iam.googleapis.com", "run.googleapis.com"])

  project            = local.p0_jira_site_acme_dev.project_id
  service            = each.key
  disable_on_destroy = false
}

# The identity the connector runs as: the service account whose ID is the
# connector's name, which P0 names as the site's
# hosting.connector_service_account. Its only role is read on the site's
# secret, below.
#
# Don't destroy and recreate it. Google treats a recreated account as a new
# one, even under the same name: it doesn't get the old account's roles.
# https://cloud.google.com/iam/docs/service-accounts-delete-undelete
resource "google_service_account" "p0_jira_site_acme_dev" {
  project      = local.p0_jira_site_acme_dev.project_id
  account_id   = local.p0_jira_site_acme_dev.connector_name
  display_name = "P0 Jira site connector acme-dev"

  depends_on = [google_project_service.p0_jira_site_acme_dev]
}

# The connector reads the secret's current value, on this secret alone, never
# on its project.
resource "google_secret_manager_secret_iam_member" "p0_jira_site_acme_dev_accessor" {
  project   = local.p0_jira_site_acme_dev.secrets_project_id
  secret_id = local.p0_jira_site_acme_dev.secret_name
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${google_service_account.p0_jira_site_acme_dev.email}"
}

# The accessor role gives secretmanager.versions.access only. P0's secret
# client reads the latest version's value and then that version's metadata
# (secretmanager.versions.get), which the accessor role doesn't cover. The
# viewer role does, and can change nothing: on this secret it reads the
# secret's metadata, its versions' metadata and its IAM policy, never a value.
resource "google_secret_manager_secret_iam_member" "p0_jira_site_acme_dev_secret_viewer" {
  project   = local.p0_jira_site_acme_dev.secrets_project_id
  secret_id = local.p0_jira_site_acme_dev.secret_name
  role      = "roles/secretmanager.viewer"
  member    = "serviceAccount:${google_service_account.p0_jira_site_acme_dev.email}"
}

resource "google_cloud_run_v2_service" "p0_jira_site_acme_dev" {
  project             = local.p0_jira_site_acme_dev.project_id
  name                = local.p0_jira_site_acme_dev.connector_name
  location            = local.p0_jira_site_acme_dev.connector_region
  deletion_protection = false
  # P0 calls the connector from outside Google Cloud; IAM, not origin, gates
  # access.
  ingress = "INGRESS_TRAFFIC_ALL"

  template {
    service_account = google_service_account.p0_jira_site_acme_dev.email

    containers {
      image = "docker.io/p0security/p0-connector-jira-gcloud:${local.p0_jira_site_acme_dev.image_tag}@${data.docker_registry_image.p0_jira_site_acme_dev.sha256_digest}"

      # The connector checks every caller's token against this, and refuses
      # every call while it's unset, so it is not redundant with the
      # roles/run.invoker grant.
      env {
        name  = "INVOKER_SA_EMAIL"
        value = local.p0_jira_site_acme_dev.invoker_email
      }

      env {
        name  = "DOMAIN_PATTERN"
        value = local.p0_jira_site_acme_dev.domain_pattern
      }
    }
  }

  depends_on = [google_project_service.p0_jira_site_acme_dev]
}

# P0 invokes the connector as its own service account. Nothing here lets
# anyone else invoke it.
resource "google_cloud_run_v2_service_iam_member" "p0_jira_site_acme_dev_invoker" {
  project  = local.p0_jira_site_acme_dev.project_id
  location = google_cloud_run_v2_service.p0_jira_site_acme_dev.location
  name     = google_cloud_run_v2_service.p0_jira_site_acme_dev.name
  role     = "roles/run.invoker"
  member   = "serviceAccount:${local.p0_jira_site_acme_dev.invoker_email}"
}

# P0 reads the service to get its invocation URL when it installs. The invoker
# role gives run.routes.invoke only, so it cannot do this read.
resource "google_cloud_run_v2_service_iam_member" "p0_jira_site_acme_dev_viewer" {
  project  = local.p0_jira_site_acme_dev.project_id
  location = google_cloud_run_v2_service.p0_jira_site_acme_dev.location
  name     = google_cloud_run_v2_service.p0_jira_site_acme_dev.name
  role     = "roles/run.viewer"
  member   = "serviceAccount:${local.p0_jira_site_acme_dev.invoker_email}"
}

# Installs Jira for the site. Creating it has P0 check the site through the
# connector, so it waits until P0 can invoke the connector and read its service,
# and the connector can read the client secret. A new grant can take a minute or
# two to take effect, so if the first check is denied access, apply again.
resource "p0_jira" "gcp_example" {
  site = "acme-dev.atlassian.net"

  hosting = {
    type             = "gcp"
    project_id       = local.p0_jira_site_acme_dev.project_id
    connector_name   = google_cloud_run_v2_service.p0_jira_site_acme_dev.name
    connector_region = google_cloud_run_v2_service.p0_jira_site_acme_dev.location
  }

  vault = {
    type       = "gcp-sm"
    project_id = local.p0_jira_site_acme_dev.secrets_project_id
  }
  client_secret_name = local.p0_jira_site_acme_dev.secret_name
  domain_pattern     = local.p0_jira_site_acme_dev.domain_pattern

  # The client ID of the service account's OAuth credential. A new credential
  # updates the site in place.
  client_id = "AbCdEfGhIjKlMnOpQrStUvWxYz012345"

  depends_on = [
    google_cloud_run_v2_service_iam_member.p0_jira_site_acme_dev_invoker,
    google_cloud_run_v2_service_iam_member.p0_jira_site_acme_dev_viewer,
    google_secret_manager_secret_iam_member.p0_jira_site_acme_dev_accessor,
    google_secret_manager_secret_iam_member.p0_jira_site_acme_dev_secret_viewer,
  ]
}
```

<!-- schema generated by tfplugindocs -->
## Schema

### Required

- `client_id` (String) The client ID of the service account's OAuth 2.0 credential. Changing it updates the installation in place, and P0 checks the site again.
- `client_secret_name` (String) The name of the secret that holds the service account's OAuth client secret, in `vault`, such as `p0/jira/acme`: the name alone, never an ARN or a resource name. For `aws-sm`, up to 512 letters, digits and `/_+=.@-`, and, when the vault's account isn't the connector's, not ending in a hyphen and six letters or digits, like `-AbCdEf`. For `gcp-sm`, up to 255 letters, digits, underscores and hyphens. Changing it replaces the installation. To rotate the client secret, add a new version to the same secret instead.
- `domain_pattern` (String) A regular expression that the connector matches against the whole email domain of every user it grants to, such as `example\.com`. The connector refuses any other user. It is the connector's `DOMAIN_PATTERN` environment variable too, and P0 checks that the two are the same. P0 refuses a pattern that doesn't compile, or one it can't show won't backtrack catastrophically: allow subdomains with `(.+\.)?example\.com` rather than `(.+\.)*example\.com`. Changing it replaces the installation.
- `hosting` (Attributes) Where the site's connector is deployed, and how P0 addresses it: AWS Lambda or Google Cloud Run. (see [below for nested schema](#nestedatt--hosting))
- `site` (String) The site's host, in lowercase, such as `acme.atlassian.net`: `<name>.atlassian.net`, without `https://` or anything after the host.
- `vault` (Attributes) The secret manager that holds the site's OAuth client secret, as installed in P0. P0 never reads the secret. The connector does, so the secret manager must be in the connector's cloud: AWS Secrets Manager for a connector on AWS Lambda, Google Secret Manager for one on Google Cloud Run. (see [below for nested schema](#nestedatt--vault))

### Read-Only

- `cloud_id` (String) The site's Atlassian cloud ID, which P0 finds through the connector when it checks the site
- `state` (String) This item's install progress in the P0 application:
	- 'stage': The item has been staged for installation
	- 'configure': The item is available to be added to P0, and may be configured
	- 'installed': The item is fully installed

<a id="nestedatt--hosting"></a>
### Nested Schema for `hosting`

Required:

- `connector_name` (String) The name of the Lambda function or Cloud Run service hosting the connector
- `connector_region` (String) The region the connector is deployed in (e.g. `us-east-1` on AWS, or `us-central1` on Google Cloud)
- `type` (String) The connector's hosting: either `aws` (Lambda) or `gcp` (Cloud Run)

Optional:

- `account_id` (String) The AWS account ID in which the connector's Lambda function is deployed. Required for, and only valid with, `aws` hosting.
- `project_id` (String) The Google Cloud project ID in which the connector's Cloud Run service is deployed. Required for, and only valid with, `gcp` hosting.

Read-Only:

- `connector_service_account` (String) The email of the service account the connector must run as, generated by P0. Only populated for `gcp` hosting.
- `connector_service_uri` (String) The connector's invocation URL, resolved by P0 during install. Only populated for `gcp` hosting.


<a id="nestedatt--vault"></a>
### Nested Schema for `vault`

Required:

- `type` (String) The secret manager: `aws-sm` (AWS Secrets Manager) or `gcp-sm` (Google Secret Manager). It must match `hosting.type`: `aws-sm` with `aws`, `gcp-sm` with `gcp`.

Optional:

- `account_id` (String) The ID of the AWS account that holds the secret: one of your AWS Secrets Manager installations in P0. It may be another account than the connector's. Required for, and only valid with, `aws-sm`.
- `project_id` (String) The ID of the Google Cloud project that holds the secret: one of your Google Secret Manager installations in P0. This provider has no resource for that installation, so install Google Secret Manager in the P0 app first. Required for, and only valid with, `gcp-sm`.
- `secrets_region` (String) The AWS region the secret is stored in, such as `us-west-2`. Required for, and only valid with, `aws-sm`.
