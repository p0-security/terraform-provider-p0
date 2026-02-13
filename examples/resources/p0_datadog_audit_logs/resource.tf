resource "p0_datadog_audit_logs" "example" {
  identifier        = "example"
  intake_url        = "https://http-intake.logs.datadoghq.com"
  api_key_cleartext = sensitive("your-datadog-api-key")
  service           = "p0"
}
