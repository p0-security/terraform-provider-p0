variable "token_id" {
  description = "The token ID"
  type        = string
}

variable "hec_token_cleartext" {
  description = "The Splunk HEC token cleartext"
  type        = string
  sensitive   = true
}

variable "hec_endpoint" {
  description = "The Splunk HEC endpoint"
  type        = string
}

variable "index" {
  description = "The Splunk HEC index to use"
  type        = string
}