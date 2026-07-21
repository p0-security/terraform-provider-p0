# Registers forwarding of P0 audit logs to Datadog Logs.
# Only one p0_datadog_audit_logs resource is supported per P0 organization;
# creating more than one disables Datadog log forwarding for the entire organization.
resource "p0_datadog_audit_logs" "example" {
  identifier = "example"

  # The intake URL must match ^https://http-intake.logs.<site>, where <site> is
  # the Datadog site your account belongs to (e.g. datadoghq.com, datadoghq.eu,
  # us5.datadoghq.com). It must be the same region as the API key below.
  intake_url = "https://http-intake.logs.datadoghq.com"

  # A Datadog API key valid for the intake region above. This is the one
  # vendor-side dependency: create the key in your Datadog account (Organization
  # Settings > API Keys) and supply its cleartext value here. P0 stores the key
  # encrypted in a secret store and exposes only its SHA-256 hash via the
  # computed api_key_hash attribute.
  api_key_cleartext = sensitive("your-datadog-api-key")

  # Optional service name for log attribution; defaults to "p0" when omitted.
  service = "p0"
}
