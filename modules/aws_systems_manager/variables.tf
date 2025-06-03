# A map of region name to variables for that region
variable "regional_aws" {
  type = map(object({
    enabled_vpcs = set(string)
  }))
}
