terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "5.42.0"
    }
  }
}

data "aws_caller_identity" "current" {}

locals {
  account_id = data.aws_caller_identity.current.account_id
  tags = {
    managed-by = "terraform"
    used-by    = "P0Security"
  }
}

# This SSM command document is executed by P0 to manage the sudoers file and grant / revoke sudo
# access to a user. The document is created by the customer, P0 is not allowed to create documents
# that it can execute because that is a privilege escalation path.
# To import: terraform import "module.aws_p0_ssm_documents.aws_ssm_document.p0_manage_sudo_access" P0ManageUserSudoAccess
resource "aws_ssm_document" "p0_manage_sudo_access" {
  name            = "P0ProvisionUserAccess"
  document_format = "YAML"
  document_type   = "Command"
  target_type     = "/AWS::EC2::Instance"

  content = <<DOC
schemaVersion: "2.2"
description: "Grant/revoke password-less sudo access, add/remove an authorized ssh key, or create a user"
parameters:
  UserName:
    type: "String"
    description: "User name"
    allowedPattern: "^[a-z][-a-z0-9_]*$"
  Action:
    type: "String"
    description: "'grant' or 'revoke'"
    allowedValues:
    - grant
    - revoke
  RequestId:
    type: "String"
    description: "P0 access request identifier"
    allowedPattern: "^[a-zA-Z0-9]*$"
  PublicKey:
    type: "String"
    description: "SSH public key"
    allowedPattern: "^[^'\n]*$"
    default: "N/A"
  Sudo:
    type: "String"
    description: "Whether to grant sudo access"
    allowedValues:
    - "false"
    - "true"
    default: "false"
mainSteps:
- precondition:
    StringEquals:
      - platformType
      - Linux
  action: aws:runShellScript
  name: InvokeLinuxScript
  inputs:
    runCommand:
      - |
        #!/bin/bash
        set -e

        ExitWithFailure() {
          MESSAGE="$1"
          (>&2 "echo" "$MESSAGE")
          exit 1
        }

        EnsureUserExists() {
          local COMMAND CREATE_HOME_ARGUMENT
          local USERNAME="$1"

          if [ -f /usr/sbin/useradd ]; then
            COMMAND='/usr/sbin/useradd'
            CREATE_HOME_ARGUMENT='--create-home'
          elif [ -f /usr/sbin/adduser ]; then
            COMMAND='/usr/sbin/adduser'
            CREATE_HOME_ARGUMENT=''
          else
            ExitWithFailure 'Cannot create user: neither of the required commands adduser or useradd exist.'
          fi

          id "$USERNAME" &>/dev/null || $COMMAND "$USERNAME" "$CREATE_HOME_ARGUMENT" || ExitWithFailure 'Failed to create the specified user.'
        }

        EnsureLineInFile() {
          local LINE="$1"
          local FILE="$2"
      
          if ! grep -qF "$LINE" "$FILE"; then
              echo "$LINE" | sudo tee -a "$FILE" >/dev/null
          fi
        }

        EnsureContentInFile() {
          local CONTENT="$1"
          local REQUEST_ID="$2"
          local FILE_PATH="$3"
          local PERMISSION="$4"
          local COMMENT="# RequestID: $REQUEST_ID"
      
          sudo mkdir -p "$(dirname "$FILE_PATH")"

          if [ ! -e "$FILE_PATH" ]; then
            sudo touch "$FILE_PATH"
            sudo chmod "$PERMISSION" "$FILE_PATH"
          fi
      
          if ! (grep -qF "$COMMENT" "$FILE_PATH" && grep -qF "$CONTENT" "$FILE_PATH"); then
              echo "$COMMENT" | sudo tee -a "$FILE_PATH" >/dev/null
              echo "$CONTENT" | sudo tee -a "$FILE_PATH" >/dev/null
          fi
        }

        RemoveContentFromFile() {
          local REQUEST_ID="$1"
          local FILE_PATH="$2"
          local COMMENT="# RequestID: $REQUEST_ID"
        
          if [ -f "$FILE_PATH" ]; then
              sudo sed -i "/^$COMMENT$/,/^$/d" "$FILE_PATH"
          fi
        }
      
        if [ '{{ Action }}' = "grant" ]
        then
          EnsureUserExists '{{ UserName }}'
          if [ -n '{{ Sudo }}' ] && [ '{{ Sudo }}' = "true" ]; then
            EnsureContentInFile '{{ UserName }} ALL=(ALL) NOPASSWD: ALL' '{{ RequestId }}' "/etc/sudoers-p0" "440"
            EnsureLineInFile "#include sudoers-p0" /etc/sudoers
          fi
          if [ -n '{{ PublicKey }}' ] && [ '{{ PublicKey }}' != "N/A" ]; then
            EnsureContentInFile '{{ PublicKey }}' '{{ RequestId }}' '/home/{{ UserName }}/.ssh/authorized_keys' "600"
            sudo chown '{{ UserName }}' '/home/{{ UserName }}/.ssh/authorized_keys'
          fi
        else
          RemoveContentFromFile '{{ RequestId }}' "/etc/sudoers-p0"
          RemoveContentFromFile '{{ RequestId }}' '/home/{{ UserName }}/.ssh/authorized_keys'
        fi
DOC
  tags    = local.tags
}
