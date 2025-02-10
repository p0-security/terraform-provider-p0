data "aws_caller_identity" "current" {}

locals {
  account_id = data.aws_caller_identity.current.account_id
  # Tag resource created by Terraform with the "managed-by"="terraform" tag
  tags = {
    managed-by = "terraform"
    used-by    = "P0Security"
  }
}

data "aws_iam_policy_document" "p0_grants_role_trust_policy" {
  statement {
    effect = "Allow"

    principals {
      type        = "Federated"
      identifiers = ["arn:aws:iam::${local.account_id}:saml-provider/${var.saml_identity_provider_name}"]
    }

    actions = [
      "sts:AssumeRoleWithSAML"
    ]

    condition {
      test     = "StringEquals"
      variable = "SAML:aud"
      values   = ["https://signin.aws.amazon.com/saml"]
    }
  }

  statement {
    effect = "Allow"

    principals {
      type        = "Federated"
      identifiers = ["arn:aws:iam::${local.account_id}:saml-provider/${var.saml_identity_provider_name}"]
    }

    actions = ["sts:SetSourceIdentity"]
  }
}

# To import: terraform import "module.aws_p0_roles.aws_iam_role.p0_grants_roles[0]" {roleName}
resource "aws_iam_role" "p0_grants_roles" {
  count              = var.role_count
  name               = format("P0GrantsRole%s", count.index)
  path               = "/p0-grants/"
  assume_role_policy = data.aws_iam_policy_document.p0_grants_role_trust_policy.json

  tags = local.tags

  # This is a default policy that applies to all P0 grant roles.
  # It allows any user to resume and terminate their own sessions.
  inline_policy {
    name = "P0PolicySharedSSH"

    policy = <<EOF
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Action": ["ssm:TerminateSession"],
      "Effect": "Allow",
      "Resource": "arn:aws:ssm:*:${local.account_id}:session/*",
      "Condition": {
        "StringLike": {
          "ssm:resourceTag/aws:ssmmessages:session-id": ["$${aws:userid}*"]
        }
      }
    },
    {
      "Action": ["ssm:ResumeSession"],
      "Effect": "Allow",
      "Resource": ["arn:aws:ssm:*:${local.account_id}:session/$${aws:username}-*"]
    }
  ]
}
EOF
  }
}
