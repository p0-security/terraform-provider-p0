resource "p0_splunk_audit_logs" "example" {
  token_id            = "token_id"
  hec_token_cleartext = sensitive("12345678-1234-1234-1234-123456789012")
  hec_endpoint        = "https://www.example.com"
}