# Published module: provisions the hardened S3 bucket P0 uses to broker file transfers; exposes account_id and bucket_name.
module "file_transfer_bucket" {
  source  = "p0-security/p0-file-transfer/aws"
  version = "~> 0.1.1"
}

# File transfer requires AWS SSH installed for the same account first.
resource "p0_ssh_aws" "example" {
  account_id = module.file_transfer_bucket.account_id
}

resource "p0_file_transfer" "example" {
  account_id  = module.file_transfer_bucket.account_id
  bucket_name = module.file_transfer_bucket.bucket_name

  # P0 enforces this ordering: AWS SSH must be installed before file transfer.
  depends_on = [p0_ssh_aws.example]
}
