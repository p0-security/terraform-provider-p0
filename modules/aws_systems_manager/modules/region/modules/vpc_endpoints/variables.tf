variable "vpc_id" {
  description = "The VPC ID to enable the Systems Manager VPC endpoints in"
}

variable "tags" {
  description = "Tags to apply to the VPC endpoints"
  type        = map(string)
}
