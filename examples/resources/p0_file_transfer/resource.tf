# File transfer requires AWS SSH to be installed for the same account first.
resource "p0_ssh_aws" "example" {
  account_id = "123456789012"
}

resource "p0_file_transfer" "example" {
  account_id    = p0_ssh_aws.example.account_id
  bucket_name   = "my-file-transfer-bucket"
  region        = "us-east-1"
  aws_partition = "aws"
}
