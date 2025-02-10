data "aws_caller_identity" "current" {}

locals {
  role_name   = "P0RoleIamManager"
  policy_name = "P0RoleIamManagerPolicy"
  account_id  = data.aws_caller_identity.current.account_id
  tags = {
    managed-by = "terraform"
    used-by    = "P0Security"
  }
}

# To import: terraform import "module.aws_p0_install.aws_iam_role.p0_iam_role" P0RoleIamManager
resource "aws_iam_role" "p0_iam_role" {
  name = local.role_name

  assume_role_policy = <<EOF
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {
        "Federated": "accounts.google.com"
      },
      "Action": "sts:AssumeRoleWithWebIdentity",
      "Condition": {
        "StringEquals": {
          "accounts.google.com:aud": "${var.gcp_service_account_id}"
        }
      }
    }
  ]
}
EOF

  inline_policy {
    name = local.policy_name

    policy = <<EOF
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "P0CanReadItsOwnRoleForValidation",
      "Effect": "Allow",
      "Action": [
        "iam:GetRole",
        "iam:GetRolePolicy"
      ],
      "Resource": "arn:aws:iam::${local.account_id}:role/${local.role_name}"
    },
    {
      "Sid": "P0CanReadP0ProvisionUserAccessDocumentForValidation",
      "Effect": "Allow",
      "Action": ["ssm:GetDocument"],
      "Resource": "arn:aws:ssm:*:${local.account_id}:document/P0ProvisionUserAccess"
    },
    {
      "Sid": "P0CanReadAccountInformation",
      "Effect": "Allow",
      "Action": [
        "account:ListRegions",
        "iam:ListAccountAliases",
        "iam:GetSAMLProvider"
      ],
      "Resource": "*",
      "Condition": {
        "StringEquals": {
          "aws:ResourceAccount": "${local.account_id}"
        }
      }
    },
    {
      "Sid": "P0CanCreateP0ManagedPolicies",
      "Effect": "Allow",
      "Action": [
        "iam:CreatePolicy",
        "iam:TagPolicy"
      ],
      "Resource": "arn:aws:iam::${local.account_id}:policy/P0Policy*",
      "Condition": {
        "StringEquals": {
          "aws:RequestTag/P0Security": "Managed by P0"
        }
      }
    },
    {
      "Sid": "P0CanChangeP0ManagedPolicies",
      "Effect": "Allow",
      "Action": [
        "iam:CreatePolicyVersion",
        "iam:DeletePolicy",
        "iam:DeletePolicyVersion",
        "iam:GetPolicy",
        "iam:GetPolicyVersion",
        "iam:ListPolicyVersions"
      ],
      "Resource": "arn:aws:iam::${local.account_id}:policy/P0Policy*"
    },
    {
      "Sid": "P0CanListP0ManagedRoles",
      "Effect": "Allow",
      "Action": [
        "iam:ListRoles"
      ],
      "Resource": "arn:aws:iam::${local.account_id}:role/p0-grants/*"
    },
    {
      "Sid": "P0CanChangeP0ManagedRoles",
      "Effect": "Allow",
      "Action": [
        "iam:DeleteRolePolicy",
        "iam:GetRole",
        "iam:GetRolePolicy",
        "iam:ListAttachedRolePolicies",
        "iam:ListRolePolicies",
        "iam:ListRoles",
        "iam:PutRolePolicy"
      ],
      "Resource": "arn:aws:iam::${local.account_id}:role/p0-grants/P0GrantsRole*"
    },
    {
      "Sid": "P0CanAttachP0ManagedPoliciesToP0ManagedRoles",
      "Effect": "Allow",
      "Action": [
        "iam:AttachRolePolicy",
        "iam:DetachRolePolicy"
      ],
      "Resource": "arn:aws:iam::${local.account_id}:role/p0-grants/P0GrantsRole*",
      "Condition": {
        "StringLike": {
          "iam:PolicyARN": "arn:aws:iam::${local.account_id}:policy/P0Policy*"
        }
      }
    },
    {
      "Sid": "P0CanManageSshAccess",
      "Effect": "Allow",
      "Action": [
        "ec2:DescribeTags",
        "ssm:DescribeInstanceInformation",
        "ssm:DescribeSessions",
        "ssm:GetCommandInvocation",
        "ssm:TerminateSession"
      ],
      "Resource": "*",
      "Condition": {
        "StringEquals": {
          "aws:ResourceAccount": "${local.account_id}"
        }
      }
    },
    {
      "Sid": "P0CanProvisionUserForSshAccess",
      "Effect": "Allow",
      "Action": "ssm:SendCommand",
      "Resource": [
        "arn:aws:ec2:*:${local.account_id}:instance/*",
        "arn:aws:ssm:*:${local.account_id}:document/P0ProvisionUserAccess"
      ]
    },
    {
      "Sid": "P0CanNotAlterItsOwnRole",
      "Effect": "Deny",
      "Action": [
        "iam:AttachRole*",
        "iam:DeleteRole*",
        "iam:DetachRole*",
        "iam:PutRole*",
        "iam:TagRole",
        "iam:UntagRole",
        "iam:UpdateRole*"
      ],
      "Resource": "arn:aws:iam::${local.account_id}:role/${local.role_name}"
    },
    {
      "Sid": "P0CannotModifySSMDocuments",
      "Effect": "Deny",
      "Action": [
        "ssm:CreateDocument",
        "ssm:UpdateDocument",
        "ssm:DeleteDocument"
      ],
      "Resource": "*"
    },
    {
      "Sid": "P0CanNotAssumeRoles",
      "Effect": "Deny",
      "Action": "sts:AssumeRole",
      "Resource": "*"
    }
  ]
}
EOF
  }

  tags = local.tags
}
