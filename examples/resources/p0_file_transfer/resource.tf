module "file_transfer_bucket" {
  source  = "p0-security/p0-file-transfer/aws"
  version = "1.0.0"
}

# File transfer requires AWS SSH to be installed for the same account first.
resource "p0_ssh_aws" "example" {
  account_id = module.file_transfer_bucket.account_id
}

resource "p0_file_transfer" "example" {
  account_id    = module.file_transfer_bucket.account_id
  bucket_name   = module.file_transfer_bucket.bucket_name
  region        = module.file_transfer_bucket.region
  aws_partition = module.file_transfer_bucket.aws_partition
  depends_on    = [module.file_transfer_bucket]
}
