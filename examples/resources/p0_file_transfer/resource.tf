# The customer-owned S3 bucket that P0 uses to broker fast file transfers.
# This published module provisions the bucket and its hardening (public-access
# block, encryption, ownership controls, and lifecycle rules) and exposes the
# account_id and bucket_name consumed below.
module "file_transfer_bucket" {
  source  = "p0-security/p0-file-transfer/aws"
  version = "~> 0.1.1"
}

# File transfer requires AWS SSH to be installed for the same account first.
resource "p0_ssh_aws" "example" {
  account_id = module.file_transfer_bucket.account_id
}

resource "p0_file_transfer" "example" {
  account_id  = module.file_transfer_bucket.account_id
  bucket_name = module.file_transfer_bucket.bucket_name

  # AWS SSH must already be installed for this account before file transfer
  # can be requested; the P0 backend enforces this ordering.
  depends_on = [p0_ssh_aws.example]
}
