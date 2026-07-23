# Registers forwarding of P0 audit logs to Datadog Logs.
# Only one per org; a second disables forwarding org-wide.
resource "p0_datadog_audit_logs" "example" {
  identifier = "example"

  # Must be https://http-intake.logs.<site> for your Datadog site, and match the API key's region.
  intake_url = "https://http-intake.logs.datadoghq.com"

  # API key for the intake region, from Datadog (Org Settings > API Keys). P0 stores it
  # encrypted and exposes only its SHA-256 hash via the computed api_key_hash attribute.
  api_key_cleartext = sensitive("your-datadog-api-key")

  # Optional; defaults to "p0" when omitted.
  service = "p0"
}
