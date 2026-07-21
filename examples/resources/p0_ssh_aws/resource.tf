locals {
  account_id = "123456789012"

  # p0_ssh_aws groups instances by the VALUE of this tag key: any instances that
  # share a value for this tag can be requested together in a single request.
  group_tag_key = "P0Group"
}

# --- Prerequisite: the AWS account must already be connected to P0 via the AWS
# IAM-management install. p0_ssh_aws has no _staged sibling of its own, so this
# chain (p0_aws_iam_write_staged -> P0 role -> p0_aws_iam_write) is the only P0
# prerequisite for the SSH install below. ---

# Stage the AWS account. This computes the role name, trust policy, and inline
# policy that P0 requires to manage IAM in the account.
resource "p0_aws_iam_write_staged" "example" {
  id = local.account_id
}

# The IAM role that P0 assumes to manage grants in your account. Its name and
# trust policy come from the staged resource's computed outputs.
resource "aws_iam_role" "p0_iam_manager" {
  name               = p0_aws_iam_write_staged.example.role.name
  assume_role_policy = p0_aws_iam_write_staged.example.role.trust_policy
}

# The inline policy that grants the role its IAM-management permissions.
resource "aws_iam_role_policy" "p0_iam_manager" {
  name   = p0_aws_iam_write_staged.example.role.inline_policy_name
  role   = aws_iam_role.p0_iam_manager.name
  policy = p0_aws_iam_write_staged.example.role.inline_policy
}

# Complete the AWS integration. Must be installed _after_ P0's role and inline
# policy exist.
resource "p0_aws_iam_write" "example" {
  id         = p0_aws_iam_write_staged.example.id
  depends_on = [aws_iam_role_policy.p0_iam_manager]

  login = {
    type = "iam"
    identity = {
      type = "email"
    }
  }
}

# --- Target infrastructure: an SSM-ready EC2 instance. P0 AWS SSH sessions run
# over AWS Systems Manager Session Manager, so the instances P0 brokers access to
# must be managed by SSM (SSM agent running + an instance profile granting
# AmazonSSMManagedInstanceCore + outbound HTTPS to the SSM endpoints).
#
# The per-instance IAM instance profile below is the simplest way to make a
# single instance SSM-managed. To cover every instance in an account without a
# per-instance profile, use account-wide Default Host Management Configuration
# instead: create the AWSSystemsManagerDefaultEC2InstanceManagementRole role
# (attach the AWS-managed AmazonSSMManagedEC2InstanceDefaultPolicy, and guard it
# with a prevent_destroy lifecycle) and enable DHMC per region. Instances in
# private subnets without internet egress additionally need SSM/EC2messages/
# SSMmessages VPC endpoints. See the aws_systems_manager module in
# p0-security/p0-terraform-install for the full DHMC + VPC-endpoint pattern. ---

# Amazon Linux 2023 ships the SSM agent preinstalled.
data "aws_ami" "al2023" {
  most_recent = true
  owners      = ["amazon"]

  filter {
    name   = "name"
    values = ["al2023-ami-*-x86_64"]
  }
}

# EC2-trust role for the instance, so it can register with SSM.
resource "aws_iam_role" "ssm_instance" {
  name = "p0-ssm-instance-example"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Action    = "sts:AssumeRole"
      Effect    = "Allow"
      Principal = { Service = "ec2.amazonaws.com" }
    }]
  })
}

resource "aws_iam_role_policy_attachment" "ssm_core" {
  role       = aws_iam_role.ssm_instance.name
  policy_arn = "arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore"
}

resource "aws_iam_instance_profile" "ssm_instance" {
  name = "p0-ssm-instance-example"
  role = aws_iam_role.ssm_instance.name
}

# The instance needs outbound HTTPS (443) to the SSM endpoints, via either
# default egress to the internet or SSM/EC2messages/SSMmessages VPC endpoints.
resource "aws_instance" "example" {
  ami                  = data.aws_ami.al2023.id
  instance_type        = "t3.micro"
  iam_instance_profile = aws_iam_instance_profile.ssm_instance.name

  # Instances that share this tag's value are grouped together for access
  # requests (see group_key on p0_ssh_aws below).
  tags = {
    (local.group_tag_key) = "dev-servers"
  }
}

# --- SSM command documents P0 requires. These must be created by you (P0 is not
# permitted to create documents it can execute, to avoid a privilege-escalation
# path) and must exist in EVERY region where P0 brokers access. P0 verifies the
# document content at install time, so it must match the YAML shipped in
# p0-security/p0-terraform-install (aws_p0_ssm_documents module) exactly. ---

# Grants/revokes password-less sudo, manages authorized SSH keys, and provisions
# the requesting user. Required whenever is_sudo_enabled = true (and for user
# provisioning generally).
resource "aws_ssm_document" "p0_provision_user_access" {
  name            = "P0ProvisionUserAccess"
  document_format = "YAML"
  document_type   = "Command"
  target_type     = "/AWS::EC2::Instance"
  content         = file("${path.module}/p0-provision-user-access.yaml")
}

# Retrieves SSH host keys from a target instance at session-establishment time.
resource "aws_ssm_document" "p0_get_ssh_host_keys" {
  name            = "P0GetSshHostKeys"
  document_format = "YAML"
  document_type   = "Command"
  target_type     = "/AWS::EC2::Instance"
  content         = file("${path.module}/p0-get-ssh-host-keys.yaml")
}

# --- The SSH install itself. ---
resource "p0_ssh_aws" "example" {
  account_id      = local.account_id
  group_key       = local.group_tag_key
  is_sudo_enabled = true

  # Requires the AWS integration (p0_aws_iam_write) to be installed first, and
  # the P0 SSM command documents to exist (in every enabled region).
  depends_on = [
    p0_aws_iam_write.example,
    aws_ssm_document.p0_provision_user_access,
    aws_ssm_document.p0_get_ssh_host_keys,
  ]
}
