---
page_title: "p0_tailscale_connector Resource - p0"
subcategory: ""
description: |-
  A Tailscale connector installation.
  The connector is how P0 grants network access on your tailnets. You deploy it in your own AWS account or Google Cloud project, where it calls only the Tailscale API, and only it holds a tailnet's Tailscale credential. One connector can serve many tailnets: register each with a p0_tailscale_tailnet resource that names this connector. P0 grants access by setting posture attributes on devices and never writes your policy file. Posture attributes need a Tailscale Premium or Enterprise plan on each tailnet.
  Important: Before you create this resource, stage the installation with p0_tailscale_connector_staged and deploy the connector. Creating this resource has P0 check that it can invoke the connector, and on Google Cloud that it can read the Cloud Run service's URL. If the check fails, the apply fails with P0's message. IAM changes can take a few seconds to propagate, so if the first check is denied access, apply again.
  P0 fixes where a connector runs when it adds the connector, so changing id or hosting replaces it. Replacing it revokes nothing, and revokes on its tailnets fail until each tailnet's credential trusts the new connector's identity, so change hosting only when none of the connector's tailnets has active grants: revoke them first. If P0 hasn't finished installing the connector, for example because you imported it before its check passed, Terraform plans an update, and applying it finishes the install.
  Destroying this resource returns the connector to its staged state, which p0_tailscale_connector_staged still owns. P0 can't grant or revoke access through a connector that isn't installed, so revoke the grants on the tailnets that name the connector, destroy those tailnets, then the connector. Destroying a tailnet revokes nothing.
  Prerequisites:
  
  P0's Tailscale connector, deployed where hosting says and running as the identity that connector_identity names. P0's Tailscale installer generates Terraform that deploys the connector with its image pinned, as in the example.
  For AWS Lambda, the image p0security/p0-connector-tailscale. Lambda runs images only from Amazon ECR, so copy the image from Docker Hub into ECR first. Install p0_aws_iam_write for the account the connector runs in, and grant that installation's role lambda:InvokeFunction on the connector's function. For a tailnet with a federated identity, also turn on AWS outbound identity federation for the account, and let the connector's role call sts:GetWebIdentityToken for Tailscale's audiences, api.tailscale.com/*, for 300 seconds or less.
  For Google Cloud Run, the image p0security/p0-connector-tailscale-gcloud. Install p0_gcp, and grant its service account roles/run.invoker and roles/run.viewer on the connector's service, and pass the same address to the connector as its INVOKER_SA_EMAIL environment variable.
  Note: This integration is currently in beta.
  For the full install steps, see Tailscale https://docs.p0.dev/integrations/resource-integrations/tailscale.
---

# p0_tailscale_connector (Resource)

A Tailscale connector installation.

The connector is how P0 grants network access on your tailnets. You deploy it in your own AWS account or Google Cloud project, where it calls only the Tailscale API, and only it holds a tailnet's Tailscale credential. One connector can serve many tailnets: register each with a `p0_tailscale_tailnet` resource that names this connector. P0 grants access by setting posture attributes on devices and never writes your policy file. Posture attributes need a Tailscale Premium or Enterprise plan on each tailnet.

**Important:** Before you create this resource, stage the installation with `p0_tailscale_connector_staged` and deploy the connector. Creating this resource has P0 check that it can invoke the connector, and on Google Cloud that it can read the Cloud Run service's URL. If the check fails, the apply fails with P0's message. IAM changes can take a few seconds to propagate, so if the first check is denied access, apply again.

P0 fixes where a connector runs when it adds the connector, so changing `id` or `hosting` replaces it. Replacing it revokes nothing, and revokes on its tailnets fail until each tailnet's credential trusts the new connector's identity, so change `hosting` only when none of the connector's tailnets has active grants: revoke them first. If P0 hasn't finished installing the connector, for example because you imported it before its check passed, Terraform plans an update, and applying it finishes the install.

Destroying this resource returns the connector to its staged state, which `p0_tailscale_connector_staged` still owns. P0 can't grant or revoke access through a connector that isn't installed, so revoke the grants on the tailnets that name the connector, destroy those tailnets, then the connector. Destroying a tailnet revokes nothing.

**Prerequisites:**

- P0's Tailscale connector, deployed where `hosting` says and running as the identity that `connector_identity` names. P0's Tailscale installer generates Terraform that deploys the connector with its image pinned, as in the example.

- For AWS Lambda, the image `p0security/p0-connector-tailscale`. Lambda runs images only from Amazon ECR, so copy the image from Docker Hub into ECR first. Install `p0_aws_iam_write` for the account the connector runs in, and grant that installation's role `lambda:InvokeFunction` on the connector's function. For a tailnet with a federated identity, also turn on AWS outbound identity federation for the account, and let the connector's role call `sts:GetWebIdentityToken` for Tailscale's audiences, `api.tailscale.com/*`, for 300 seconds or less.

- For Google Cloud Run, the image `p0security/p0-connector-tailscale-gcloud`. Install `p0_gcp`, and grant its service account `roles/run.invoker` and `roles/run.viewer` on the connector's service, and pass the same address to the connector as its `INVOKER_SA_EMAIL` environment variable.

**Note:** This integration is currently in beta.

For the full install steps, see [Tailscale](https://docs.p0.dev/integrations/resource-integrations/tailscale).

## Example Usage

### AWS Lambda

```terraform
# Installs P0's Tailscale connector prod on AWS Lambda. Full chain:
# p0_aws_iam_write (see its example) -> p0_tailscale_connector_staged -> the
# connector's Lambda function -> p0_tailscale_connector -> p0_tailscale_tailnet.
#
# This example hosts the connector on AWS Lambda. For Google Cloud Run, see the
# Google Cloud Run example (gcp.tf).
#
# The connector's Terraform is what P0's Tailscale installer generates for the
# connector, with p0_tailscale_connector_staged before it and
# p0_tailscale_connector in place of its output. Copy it from the installer,
# which pins the current connector image.
#
# Applying this copies P0's connector image from Docker Hub into an ECR
# repository, which Lambda requires. Run terraform where Docker and the AWS CLI
# are installed and the AWS CLI is signed in to account 123456789012.
#
# One connector serves every tailnet that names it in P0. Each tailnet's own
# Terraform trusts this connector, or lets it read that tailnet's OAuth client
# secret (see the p0_tailscale_tailnet example). This configuration grants it
# no credential.

terraform {
  # terraform_data needs Terraform 1.4.
  required_version = ">= 1.4"

  required_providers {
    # 0.86.0 is the first release whose p0_tailscale_tailnet is posture-only.
    p0 = {
      source  = "p0-security/p0"
      version = "~> 0.86"
    }
    aws = {
      source = "hashicorp/aws"
    }
    docker = {
      source  = "kreuzwerker/docker"
      version = "~> 3.0"
    }
    # For the p0_tailscale_tailnet example, which goes in the same
    # configuration.
    tailscale = {
      source  = "tailscale/tailscale"
      version = "~> 0.29.2"
    }
  }
}

locals {
  p0_tailscale_connector_prod = {
    # Where P0 invokes the connector. P0 derives the connector's ARN from the
    # account, region and name, so deploy the connector exactly there. P0 fixes
    # these when it adds the connector.
    account_id       = "123456789012"
    connector_name   = "p0-tailscale-prod"
    connector_region = "us-west-2"
    # How many days CloudWatch keeps the connector's logs.
    log_retention_days = 30
    # The role P0 assumes in this account, which invokes the connector.
    p0_role_name = "P0RoleIamManager"
    # P0's connector image: its tag on Docker Hub, and the digest that tag must
    # resolve to. These pin the first posture-only build. For a later one, use
    # the pinned values in the Terraform that P0's installer shows.
    image_tag    = "sha-19655ef"
    image_digest = "sha256:072afd15933f4650d03f530b955b2bce9218bb38f5d04050ef4fbc5fda7f85a0"
  }
}

# Tells P0 where the connector will run, and names the role it must run as.
resource "p0_tailscale_connector_staged" "example" {
  id = "prod"

  hosting = {
    type             = "aws"
    account_id       = local.p0_tailscale_connector_prod.account_id
    connector_name   = local.p0_tailscale_connector_prod.connector_name
    connector_region = local.p0_tailscale_connector_prod.connector_region
  }
}

# Add the credential settings your other AWS providers use, such as profile or
# assume_role, if they have any.
provider "aws" {
  alias               = "p0_tailscale_connector_prod"
  region              = local.p0_tailscale_connector_prod.connector_region
  allowed_account_ids = [local.p0_tailscale_connector_prod.account_id]
}

data "docker_registry_image" "p0_tailscale_connector_prod" {
  name = "p0security/p0-connector-tailscale:${local.p0_tailscale_connector_prod.image_tag}"

  lifecycle {
    postcondition {
      condition     = self.sha256_digest == local.p0_tailscale_connector_prod.image_digest
      error_message = "The connector image's tag doesn't resolve to the digest pinned here. Copy the current Terraform from P0's Tailscale installer, or contact support@p0.dev."
    }
  }
}

resource "aws_ecr_repository" "p0_tailscale_connector_prod" {
  provider = aws.p0_tailscale_connector_prod

  name = local.p0_tailscale_connector_prod.connector_name
  # Tags can't be overwritten, so no one who can push to this repository can
  # replace the image that runs with access to your tailnets.
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
resource "terraform_data" "p0_tailscale_connector_prod_image" {
  triggers_replace = {
    digest         = data.docker_registry_image.p0_tailscale_connector_prod.sha256_digest
    repository_url = aws_ecr_repository.p0_tailscale_connector_prod.repository_url
    tag            = local.p0_tailscale_connector_prod.image_tag
  }

  provisioner "local-exec" {
    # Signs Docker out of ECR at the end, so its login doesn't outlast the copy.
    # A failed sign-out doesn't fail the copy once the image is pushed.
    command = <<-EOT
      set -e
      trap 'docker logout "$REGISTRY" || true' EXIT
      aws ecr get-login-password --region "$REGION" |
        docker login --username AWS --password-stdin "$REGISTRY"
      docker pull --platform linux/amd64 "$SOURCE"
      docker tag "$SOURCE" "$TARGET"
      docker push "$TARGET"
    EOT

    environment = {
      REGION   = local.p0_tailscale_connector_prod.connector_region
      REGISTRY = split("/", aws_ecr_repository.p0_tailscale_connector_prod.repository_url)[0]
      SOURCE   = "p0security/p0-connector-tailscale@${local.p0_tailscale_connector_prod.image_digest}"
      TARGET   = "${aws_ecr_repository.p0_tailscale_connector_prod.repository_url}:${local.p0_tailscale_connector_prod.image_tag}-${self.id}"
    }
  }
}

# Pulling one platform and pushing it gives ECR that image's own manifest, not
# the tag's multi-platform one, so the digest there can differ from the one
# pinned above. Lambda needs ECR's.
data "aws_ecr_image" "p0_tailscale_connector_prod" {
  provider = aws.p0_tailscale_connector_prod

  repository_name = aws_ecr_repository.p0_tailscale_connector_prod.name
  image_tag       = "${local.p0_tailscale_connector_prod.image_tag}-${terraform_data.p0_tailscale_connector_prod_image.id}"
}

# Lets the connector mint AWS identity tokens, which a Tailscale federated
# identity accepts in place of a secret. This is a setting of the whole AWS
# account: destroying this resource turns outbound identity federation off for
# everything in it. If it is already on, import it rather than create it. To
# check, run this with AWS CLI 2.32.0 or later:
#   aws iam get-outbound-web-identity-federation-info
# If it's on, the output shows "JwtVendingEnabled": true and the issuer URL as
# IssuerIdentifier. If it's off, the command fails with FeatureDisabled. Import
# it with:
#   terraform import aws_iam_outbound_web_identity_federation.p0_tailscale_connector_prod 123456789012
# To remove the connector but leave federation on, replace this block with a
# removed block (Terraform 1.7+) before you destroy the rest. Terraform then
# forgets the setting without turning it off:
#   removed {
#     from = aws_iam_outbound_web_identity_federation.p0_tailscale_connector_prod
#     lifecycle {
#       destroy = false
#     }
#   }
resource "aws_iam_outbound_web_identity_federation" "p0_tailscale_connector_prod" {
  provider = aws.p0_tailscale_connector_prod
}

# The connector's role, which P0 names after the connector and expects by
# exactly that name. Each tailnet's federated identity names it as its subject,
# and each OAuth client's Terraform adds a policy to it that lets the connector
# read that client's secret.
resource "aws_iam_role" "p0_tailscale_connector_prod" {
  provider = aws.p0_tailscale_connector_prod

  name = p0_tailscale_connector_staged.example.connector_identity.role_name
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
resource "aws_cloudwatch_log_group" "p0_tailscale_connector_prod" {
  provider = aws.p0_tailscale_connector_prod

  name              = "/aws/lambda/${local.p0_tailscale_connector_prod.connector_name}"
  retention_in_days = local.p0_tailscale_connector_prod.log_retention_days
}

# The connector writes its own logs, and no others.
resource "aws_iam_role_policy" "p0_tailscale_connector_prod_logs" {
  provider = aws.p0_tailscale_connector_prod

  name = "WriteConnectorLogs"
  role = aws_iam_role.p0_tailscale_connector_prod.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = ["logs:CreateLogStream", "logs:PutLogEvents"]
      Resource = "${aws_cloudwatch_log_group.p0_tailscale_connector_prod.arn}:*"
    }]
  })
}

# Only short-lived tokens for Tailscale. A federated identity's audience is
# api.tailscale.com/<its client ID>, and the connector serves tailnets added
# after it's deployed, so this allows any Tailscale client ID.
resource "aws_iam_role_policy" "p0_tailscale_connector_prod_identity_token" {
  provider = aws.p0_tailscale_connector_prod

  name = "MintTailscaleIdentityToken"
  role = aws_iam_role.p0_tailscale_connector_prod.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = "sts:GetWebIdentityToken"
      Resource = "*"
      Condition = {
        "ForAllValues:StringLike" = { "sts:IdentityTokenAudience" = ["api.tailscale.com/*"] }
        NumericLessThanEquals     = { "sts:DurationSeconds" = 300 }
      }
    }]
  })
}

# No vpc_config: the connector calls the Tailscale API over the internet.
resource "aws_lambda_function" "p0_tailscale_connector_prod" {
  provider = aws.p0_tailscale_connector_prod

  function_name = local.p0_tailscale_connector_prod.connector_name
  description   = "P0 Tailscale connector prod"
  role          = aws_iam_role.p0_tailscale_connector_prod.arn
  package_type  = "Image"
  image_uri     = "${aws_ecr_repository.p0_tailscale_connector_prod.repository_url}@${data.aws_ecr_image.p0_tailscale_connector_prod.image_digest}"
  architectures = ["x86_64"]
  memory_size   = 512
  timeout       = 60

  # The connector's role can't create a log group, so its own must exist before
  # it runs.
  depends_on = [aws_cloudwatch_log_group.p0_tailscale_connector_prod]
}

# P0 invokes the connector as its role in this account, and needs nothing else
# on it.
resource "aws_iam_role_policy" "p0_tailscale_connector_prod_invoke" {
  provider = aws.p0_tailscale_connector_prod

  name = "${local.p0_tailscale_connector_prod.connector_name}-invoke"
  role = local.p0_tailscale_connector_prod.p0_role_name
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = "lambda:InvokeFunction"
      Resource = aws_lambda_function.p0_tailscale_connector_prod.arn
    }]
  })
}

# Completes the install. Creating it has P0 check that it can invoke the
# connector, so it waits until P0's role may. IAM changes can take a few seconds
# to propagate, so if the first check is denied access, apply again.
resource "p0_tailscale_connector" "example" {
  id      = p0_tailscale_connector_staged.example.id
  hosting = p0_tailscale_connector_staged.example.hosting

  depends_on = [aws_iam_role_policy.p0_tailscale_connector_prod_invoke]
}
```

### Google Cloud Run

```terraform
# The Google Cloud Run variant of the p0_tailscale_connector example in this
# directory.
#
# Full chain: p0_gcp (P0's service account) -> p0_tailscale_connector_staged ->
# the connector's Cloud Run service -> p0_tailscale_connector ->
# p0_tailscale_tailnet.
#
# Enable these APIs in project my-project-id first:
#   - iam.googleapis.com
#   - run.googleapis.com
#   - secretmanager.googleapis.com, for a tailnet with an OAuth client
#
# One connector serves every tailnet that names it in P0. Each tailnet's own
# Terraform trusts this connector, or lets it read that tailnet's OAuth client
# secret (see gcp.tf in the p0_tailscale_tailnet example). This configuration
# grants it no credential.

resource "p0_gcp" "gcp_example" {
  organization_id = "123456789012"
}

locals {
  p0_tailscale_connector_gcp = {
    # Where P0 invokes the connector. P0 fixes these when it adds the connector.
    project_id       = "my-project-id"
    connector_name   = "p0-tailscale-prod"
    connector_region = "us-west1"
    # P0's connector image: the digest of its tag on Docker Hub. This pins the
    # first posture-only build, sha-19655ef. For a later one, use the pinned
    # value in the Terraform that P0's installer shows.
    image_digest = "sha256:6f75ba4338bcc45fced7144fc49c10417d38ae02d02a146b18cd34e107590007"
  }
}

# Tells P0 where the connector will run, and names the service account it must
# run as.
resource "p0_tailscale_connector_staged" "gcp_example" {
  id = "prod-gcp"

  hosting = {
    type             = "gcp"
    project_id       = local.p0_tailscale_connector_gcp.project_id
    connector_name   = local.p0_tailscale_connector_gcp.connector_name
    connector_region = local.p0_tailscale_connector_gcp.connector_region
  }

  depends_on = [p0_gcp.gcp_example]
}

# The connector's service account, which P0 names after the connector and
# expects by exactly that name. Each tailnet's federated identity trusts it, or
# the tailnet's Terraform lets it read the OAuth client's secret.
resource "google_service_account" "p0_tailscale_connector_gcp" {
  project      = local.p0_tailscale_connector_gcp.project_id
  account_id   = p0_tailscale_connector_staged.gcp_example.connector_identity.service_account_id
  display_name = "P0 Tailscale connector prod-gcp"
}

resource "google_cloud_run_v2_service" "p0_tailscale_connector_gcp" {
  project             = local.p0_tailscale_connector_gcp.project_id
  name                = local.p0_tailscale_connector_gcp.connector_name
  location            = local.p0_tailscale_connector_gcp.connector_region
  deletion_protection = false
  # No VPC: P0 calls the connector from outside Google Cloud, and the connector
  # calls the Tailscale API over the internet. IAM, not the caller's network,
  # gates access.
  ingress = "INGRESS_TRAFFIC_ALL"

  template {
    service_account = google_service_account.p0_tailscale_connector_gcp.email

    containers {
      image = "docker.io/p0security/p0-connector-tailscale-gcloud@${local.p0_tailscale_connector_gcp.image_digest}"

      # The connector checks the caller's OIDC token against this address, on
      # top of the roles/run.invoker grant.
      env {
        name  = "INVOKER_SA_EMAIL"
        value = p0_gcp.gcp_example.service_account_email
      }
    }
  }
}

resource "google_cloud_run_v2_service_iam_member" "p0_tailscale_connector_gcp_invoker" {
  project  = local.p0_tailscale_connector_gcp.project_id
  location = local.p0_tailscale_connector_gcp.connector_region
  name     = google_cloud_run_v2_service.p0_tailscale_connector_gcp.name
  role     = "roles/run.invoker"
  member   = "serviceAccount:${p0_gcp.gcp_example.service_account_email}"
}

# P0 reads the service to get the URL it invokes when it installs. The invoker
# role gives run.routes.invoke only, which doesn't cover that read.
resource "google_cloud_run_v2_service_iam_member" "p0_tailscale_connector_gcp_viewer" {
  project  = local.p0_tailscale_connector_gcp.project_id
  location = local.p0_tailscale_connector_gcp.connector_region
  name     = google_cloud_run_v2_service.p0_tailscale_connector_gcp.name
  role     = "roles/run.viewer"
  member   = "serviceAccount:${p0_gcp.gcp_example.service_account_email}"
}

# Completes the install. Creating it has P0 read the service's URL and check
# that it can invoke the connector, so it waits until P0's service account may
# do both. P0 looks the URL up itself, so read it from
# hosting.connector_service_uri rather than setting it.
resource "p0_tailscale_connector" "gcp_example" {
  id      = p0_tailscale_connector_staged.gcp_example.id
  hosting = p0_tailscale_connector_staged.gcp_example.hosting

  depends_on = [
    google_cloud_run_v2_service_iam_member.p0_tailscale_connector_gcp_invoker,
    google_cloud_run_v2_service_iam_member.p0_tailscale_connector_gcp_viewer,
  ]
}
```

<!-- schema generated by tfplugindocs -->
## Schema

### Required

- `hosting` (Attributes) Where the connector runs. Must match the `p0_tailscale_connector_staged` resource, whose `hosting` this attribute can take whole. Changing it replaces the connector, which revokes nothing, and revokes on the connector's tailnets fail until each tailnet's credential trusts the new identity. Change it only when none of those tailnets has active grants: revoke them first. (see [below for nested schema](#nestedatt--hosting))
- `id` (String) The `id` of the `p0_tailscale_connector_staged` resource whose install this finishes

### Read-Only

- `connector_identity` (Attributes) The identity the connector must run as: an IAM role on AWS, or a service account on Google Cloud, named after the connector. P0 doesn't look the identity up, so deploy the connector with exactly this one. Each tailnet that names the connector trusts this identity, as its federated identity's subject, or lets it read its OAuth client's secret. (see [below for nested schema](#nestedatt--connector_identity))
- `state` (String) This item's install progress in the P0 application:
	- 'stage': The item has been staged for installation
	- 'configure': The item is available to be added to P0, and may be configured
	- 'installed': The item is fully installed

<a id="nestedatt--hosting"></a>
### Nested Schema for `hosting`

Required:

- `connector_name` (String) The name of the Lambda function or Cloud Run service that runs the connector
- `connector_region` (String) The region the connector runs in, such as `us-east-1` on AWS or `us-central1` on Google Cloud
- `type` (String) The connector's hosting: either `aws` (Lambda) or `gcp` (Cloud Run)

Optional:

- `account_id` (String) The ID of the AWS account the connector's Lambda function runs in. Required for, and only valid with, `aws` hosting.
- `project_id` (String) The ID of the Google Cloud project the connector's Cloud Run service runs in. Required for, and only valid with, `gcp` hosting.

Read-Only:

- `connector_service_uri` (String) The URL P0 invokes the connector at, which P0 looks up when it installs the connector. Only populated for `gcp` hosting.


<a id="nestedatt--connector_identity"></a>
### Nested Schema for `connector_identity`

Read-Only:

- `account_id` (String) The ID of the AWS account the role is in. Only populated for `aws`.
- `project_id` (String) The ID of the Google Cloud project the service account is in. Only populated for `gcp`.
- `role_arn` (String) The ARN of the IAM role the connector's Lambda function runs as. Only populated for `aws`.
- `role_name` (String) The name of the IAM role the connector's Lambda function runs as. Only populated for `aws`.
- `service_account_email` (String) The email of the service account the connector's Cloud Run service runs as. Only populated for `gcp`.
- `service_account_id` (String) The account ID of the service account the connector's Cloud Run service runs as: the part of its email before the `@`. Only populated for `gcp`.
- `type` (String) The identity's cloud: either `aws` (an IAM role) or `gcp` (a service account)
