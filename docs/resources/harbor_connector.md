---
page_title: "p0_harbor_connector Resource - p0"
subcategory: ""
description: |-
  A Harbor connector installation.
  The connector is how P0 reaches your Harbor instances. You deploy it in your own AWS account or Google Cloud project, and one connector can serve several Harbor instances. Once it is installed, add each instance with p0_harbor_instance.
  Important: Before creating this resource you must stage the installation with p0_harbor_connector_staged and deploy the connector. Creating this resource has P0 check the connector where hosting says it runs. If the check fails, the apply fails with P0's message. IAM changes can take a few seconds to take effect, so if the first check is denied access, apply again.
  id and hosting must match the staged resource. P0 refuses to install a connector anywhere but where it was staged, and changing either replaces the connector.
  Destroying this resource returns the connector to its staged state, which p0_harbor_connector_staged still owns. P0 refuses that while a Harbor instance uses the connector, so remove each p0_harbor_instance that uses it first. Terraform does so in the same apply when the instance refers to this resource, as the examples do.
  Prerequisites:
  
  P0's Harbor connector, deployed where hosting says. Set its ALLOWED_EMAIL_DOMAIN_PATTERN environment variable to a regular expression that an email's whole domain must match for its user to receive access, such as example[.]com. The pattern is set on the connector alone, and isn't an attribute of this resource: P0 doesn't set it.
  For AWS Lambda, the image p0security/p0-connector-harbor. Lambda runs images only from Amazon ECR, so copy the image from Docker Hub into ECR first. p0_aws_iam_write must be installed for the account the connector runs in. Grant that installation's role lambda:InvokeFunction on the connector's function.
  For Google Cloud Run, the image p0security/p0-connector-harbor-gcloud, running as the staged resource's hosting.connector_service_account. p0_gcp must be installed. Grant its service account roles/run.invoker and roles/run.viewer on the connector's service, and pass the same address to the connector as its INVOKER_SA_EMAIL environment variable.
  If P0 stops finding the connector: while Harbor isn't enabled for your organization, or after the connector is removed in P0, every refresh of this resource fails, and so do plan, destroy and import. Terraform keeps the resource in state rather than stage the connector again. If Harbor should be enabled, contact support@p0.dev. To plan or apply other changes meanwhile, run Terraform with -refresh=false. If the connector was removed in P0, remove the resource from state with terraform state rm <address>, and the next apply stages it again.
  Note: This integration is currently in preview.
---

# p0_harbor_connector (Resource)

A Harbor connector installation.

The connector is how P0 reaches your Harbor instances. You deploy it in your own AWS account or Google Cloud project, and one connector can serve several Harbor instances. Once it is installed, add each instance with `p0_harbor_instance`.

**Important:** Before creating this resource you must stage the installation with `p0_harbor_connector_staged` and deploy the connector. Creating this resource has P0 check the connector where `hosting` says it runs. If the check fails, the apply fails with P0's message. IAM changes can take a few seconds to take effect, so if the first check is denied access, apply again.

`id` and `hosting` must match the staged resource. P0 refuses to install a connector anywhere but where it was staged, and changing either replaces the connector.

Destroying this resource returns the connector to its staged state, which `p0_harbor_connector_staged` still owns. P0 refuses that while a Harbor instance uses the connector, so remove each `p0_harbor_instance` that uses it first. Terraform does so in the same apply when the instance refers to this resource, as the examples do.

**Prerequisites:**

- P0's Harbor connector, deployed where `hosting` says. Set its `ALLOWED_EMAIL_DOMAIN_PATTERN` environment variable to a regular expression that an email's whole domain must match for its user to receive access, such as `example[.]com`. The pattern is set on the connector alone, and isn't an attribute of this resource: P0 doesn't set it.

- For AWS Lambda, the image `p0security/p0-connector-harbor`. Lambda runs images only from Amazon ECR, so copy the image from Docker Hub into ECR first. `p0_aws_iam_write` must be installed for the account the connector runs in. Grant that installation's role `lambda:InvokeFunction` on the connector's function.

- For Google Cloud Run, the image `p0security/p0-connector-harbor-gcloud`, running as the staged resource's `hosting.connector_service_account`. `p0_gcp` must be installed. Grant its service account `roles/run.invoker` and `roles/run.viewer` on the connector's service, and pass the same address to the connector as its `INVOKER_SA_EMAIL` environment variable.

**If P0 stops finding the connector:** while Harbor isn't enabled for your organization, or after the connector is removed in P0, every refresh of this resource fails, and so do `plan`, `destroy` and `import`. Terraform keeps the resource in state rather than stage the connector again. If Harbor should be enabled, contact support@p0.dev. To plan or apply other changes meanwhile, run Terraform with `-refresh=false`. If the connector was removed in P0, remove the resource from state with `terraform state rm <address>`, and the next apply stages it again.

**Note:** This integration is currently in preview.

## Example Usage

### Google Cloud Run

```terraform
# Installs P0's Harbor connector on Google Cloud Run. For AWS Lambda, see aws.tf
# in this directory.
#
# Full chain: p0_gcp (see its example) -> p0_harbor_connector_staged -> the
# connector's service account and Cloud Run service -> roles/run.invoker and
# roles/run.viewer for P0's service account -> p0_harbor_connector.

terraform {
  # Reading hosting.connector_service_account while the staged resource is
  # still planned needs Terraform 1.2.
  required_version = ">= 1.2"
}

resource "p0_harbor_connector_staged" "example" {
  id = "prod"

  hosting = {
    type             = "gcp"
    project_id       = "my-project-id"
    connector_name   = "p0-harbor-prod"
    connector_region = "us-west1"
  }

  depends_on = [p0_gcp.example]
}

locals {
  harbor = p0_harbor_connector_staged.example.hosting
}

resource "google_project_service" "harbor" {
  for_each = toset([
    "iam.googleapis.com",
    "run.googleapis.com",
  ])
  project            = local.harbor.project_id
  service            = each.key
  disable_on_destroy = false
}

# The identity the connector runs as: the service account P0 generated when the
# connector was staged.
resource "google_service_account" "harbor" {
  project      = local.harbor.project_id
  account_id   = split("@", local.harbor.connector_service_account)[0]
  display_name = "P0 Harbor connector"

  depends_on = [google_project_service.harbor]
}

resource "google_cloud_run_v2_service" "harbor" {
  project             = local.harbor.project_id
  name                = local.harbor.connector_name
  location            = local.harbor.connector_region
  deletion_protection = false
  # P0 calls the connector from outside Google Cloud; IAM, not origin, gates access.
  ingress = "INGRESS_TRAFFIC_ALL"

  template {
    service_account = google_service_account.harbor.email

    containers {
      # P0's connector image, pinned by its tag and digest. Use the tag and
      # digest of the release you want to run.
      image = "docker.io/p0security/p0-connector-harbor-gcloud:sha-1234567@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

      # The connector accepts calls from this identity alone, and refuses every
      # call while it is unset.
      env {
        name  = "INVOKER_SA_EMAIL"
        value = p0_gcp.example.service_account_email
      }

      # A regular expression that an email's whole domain must match for its
      # user to receive access through this connector. It is set here alone,
      # and isn't an attribute of p0_harbor_connector: P0 doesn't set it.
      env {
        name  = "ALLOWED_EMAIL_DOMAIN_PATTERN"
        value = "example[.]com"
      }
    }
  }

  depends_on = [google_project_service.harbor]
}

resource "google_cloud_run_v2_service_iam_member" "harbor_invoker" {
  project  = local.harbor.project_id
  location = google_cloud_run_v2_service.harbor.location
  name     = google_cloud_run_v2_service.harbor.name
  role     = "roles/run.invoker"
  member   = "serviceAccount:${p0_gcp.example.service_account_email}"
}

# P0 reads the service to get its invocation URL while it installs. The invoker
# role gives run.routes.invoke only, so it cannot do this read.
resource "google_cloud_run_v2_service_iam_member" "harbor_viewer" {
  project  = local.harbor.project_id
  location = google_cloud_run_v2_service.harbor.location
  name     = google_cloud_run_v2_service.harbor.name
  role     = "roles/run.viewer"
  member   = "serviceAccount:${p0_gcp.example.service_account_email}"
}

# Completes the install. Creating it has P0 check the connector, so it waits
# until P0 can invoke the connector and read its service.
resource "p0_harbor_connector" "example" {
  id = p0_harbor_connector_staged.example.id

  # The same hosting as the staged resource, field by field. The staged
  # resource's hosting can't be assigned whole, because it also holds the
  # attributes that P0 sets.
  hosting = {
    type             = local.harbor.type
    project_id       = local.harbor.project_id
    connector_name   = local.harbor.connector_name
    connector_region = local.harbor.connector_region
  }

  depends_on = [
    google_cloud_run_v2_service_iam_member.harbor_invoker,
    google_cloud_run_v2_service_iam_member.harbor_viewer,
  ]
}
```

### AWS Lambda

```terraform
# The AWS Lambda variant of the p0_harbor_connector example in this directory.
#
# Full chain: p0_aws_iam_write (see its example) -> p0_harbor_connector_staged
# -> the connector's ECR repository, role and Lambda function ->
# lambda:InvokeFunction for P0's role -> p0_harbor_connector.
#
# Applying this copies P0's connector image from Docker Hub into an ECR
# repository, which Lambda requires. Run terraform where Docker and the AWS CLI
# are installed and the AWS CLI is signed in to account 123456789012.

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
      version = "~> 3.0"
    }
  }
}

locals {
  harbor_aws = {
    # Where P0 invokes the connector.
    account_id       = "123456789012"
    connector_name   = "p0-harbor-prod"
    connector_region = "us-east-1"
    # How many days CloudWatch keeps the connector's logs.
    log_retention_days = 30
    # P0's connector image: its tag on Docker Hub, and the digest that tag must
    # resolve to. Use the tag and digest of the release you want to run.
    image_tag    = "sha-1234567"
    image_digest = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
  }
}

resource "p0_harbor_connector_staged" "aws_example" {
  id = "prod-aws"

  hosting = {
    type             = "aws"
    account_id       = local.harbor_aws.account_id
    connector_name   = local.harbor_aws.connector_name
    connector_region = local.harbor_aws.connector_region
  }

  depends_on = [p0_aws_iam_write.example]
}

# The connector's name as P0 staged it. The resources below take it from here, so
# none is created until staging succeeds.
locals {
  harbor_aws_staged = p0_harbor_connector_staged.aws_example.hosting
}

# P0 derives the connector's ARN from the account, region and name above, so the
# connector must be deployed exactly there. Add the credential settings your
# other AWS providers use, such as profile or assume_role, if they have any.
provider "aws" {
  alias               = "harbor"
  region              = local.harbor_aws.connector_region
  allowed_account_ids = [local.harbor_aws.account_id]
}

data "docker_registry_image" "harbor" {
  name = "p0security/p0-connector-harbor:${local.harbor_aws.image_tag}"

  lifecycle {
    postcondition {
      condition     = self.sha256_digest == local.harbor_aws.image_digest
      error_message = "The connector image's tag doesn't resolve to the digest pinned here."
    }
  }
}

resource "aws_ecr_repository" "harbor" {
  provider = aws.harbor

  name = local.harbor_aws_staged.connector_name
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
resource "terraform_data" "harbor_image" {
  triggers_replace = {
    digest         = data.docker_registry_image.harbor.sha256_digest
    repository_url = aws_ecr_repository.harbor.repository_url
    tag            = local.harbor_aws.image_tag
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
      REGION   = local.harbor_aws.connector_region
      REGISTRY = split("/", aws_ecr_repository.harbor.repository_url)[0]
      SOURCE   = "p0security/p0-connector-harbor@${local.harbor_aws.image_digest}"
      TARGET   = "${aws_ecr_repository.harbor.repository_url}:${local.harbor_aws.image_tag}-${self.id}"
    }
  }
}

# Pulling one platform and pushing it gives ECR that image's own manifest, not
# the tag's multi-platform one, so the digest there can differ from the one
# pinned above. Lambda needs ECR's.
data "aws_ecr_image" "harbor" {
  provider = aws.harbor

  repository_name = aws_ecr_repository.harbor.name
  image_tag       = "${local.harbor_aws.image_tag}-${terraform_data.harbor_image.id}"
}

# The identity the connector runs as, named after the connector.
resource "aws_iam_role" "harbor" {
  provider = aws.harbor

  name = local.harbor_aws_staged.connector_name
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
resource "aws_cloudwatch_log_group" "harbor" {
  provider = aws.harbor

  name              = "/aws/lambda/${local.harbor_aws_staged.connector_name}"
  retention_in_days = local.harbor_aws.log_retention_days
}

# The connector writes its own logs, and no others.
resource "aws_iam_role_policy" "harbor_logs" {
  provider = aws.harbor

  name = "WriteConnectorLogs"
  role = aws_iam_role.harbor.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = ["logs:CreateLogStream", "logs:PutLogEvents"]
      Resource = "${aws_cloudwatch_log_group.harbor.arn}:*"
    }]
  })
}

# No vpc_config: the connector calls your Harbor instances over the internet.
resource "aws_lambda_function" "harbor" {
  provider = aws.harbor

  function_name = local.harbor_aws_staged.connector_name
  description   = "P0 Harbor connector"
  role          = aws_iam_role.harbor.arn
  package_type  = "Image"
  image_uri     = "${aws_ecr_repository.harbor.repository_url}@${data.aws_ecr_image.harbor.image_digest}"
  architectures = ["x86_64"]
  memory_size   = 512
  timeout       = 60

  environment {
    variables = {
      # A regular expression that an email's whole domain must match for
      # its user to receive access through this connector. It is set here
      # alone, and isn't an attribute of p0_harbor_connector: P0 doesn't set
      # it.
      ALLOWED_EMAIL_DOMAIN_PATTERN = "example[.]com"
    }
  }

  # The connector's role can't create a log group, so its own must exist before
  # it runs.
  depends_on = [aws_cloudwatch_log_group.harbor]
}

# P0 invokes the connector as its role in this account, and needs nothing else
# on it. The role is the one the p0_aws_iam_write example creates.
resource "aws_iam_role_policy" "harbor_invoke" {
  provider = aws.harbor

  name = "${local.harbor_aws_staged.connector_name}-invoke"
  role = aws_iam_role.p0_iam_manager.name
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = "lambda:InvokeFunction"
      Resource = aws_lambda_function.harbor.arn
    }]
  })
}

# Completes the install. Creating it has P0 check the connector, so it waits
# until P0 can invoke the connector. IAM changes can take a few seconds to take
# effect, so if the first check is denied access, apply again.
resource "p0_harbor_connector" "aws_example" {
  id = p0_harbor_connector_staged.aws_example.id

  # The same hosting as the staged resource, field by field. The staged
  # resource's hosting can't be assigned whole, because it also holds the
  # attributes that P0 sets.
  hosting = {
    type             = local.harbor_aws_staged.type
    account_id       = local.harbor_aws_staged.account_id
    connector_name   = aws_lambda_function.harbor.function_name
    connector_region = local.harbor_aws_staged.connector_region
  }

  depends_on = [aws_iam_role_policy.harbor_invoke]
}
```

<!-- schema generated by tfplugindocs -->
## Schema

### Required

- `hosting` (Attributes) Where the connector runs. Must match the `p0_harbor_connector_staged` resource. (see [below for nested schema](#nestedatt--hosting))
- `id` (String) The `id` of the `p0_harbor_connector_staged` resource being finalized

### Read-Only

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
