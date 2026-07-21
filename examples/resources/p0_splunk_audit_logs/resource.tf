# Forwards P0 audit logs to Splunk via an HTTP Event Collector (HEC).
# The values below come from an HEC token created in your Splunk deployment
# (Settings > Data Inputs > HTTP Event Collector > New Token). Because the Splunk
# deployment is not managed by this provider, these are supplied as literals.
# Only one p0_splunk_audit_logs resource is supported per P0 organization;
# creating more than one disables Splunk event forwarding for the entire organization.
resource "p0_splunk_audit_logs" "example" {
  # A user-chosen name/ID for the HEC token. Rotating the token replaces this resource.
  token_id = "p0-audit-logs"

  # The HEC token value from Splunk (a UUID). Sensitive; rotating it replaces this resource.
  hec_token_cleartext = sensitive("12345678-1234-1234-1234-123456789012")

  # Your Splunk HEC endpoint (HEC listens on port 8088 by default). Must begin with "https:".
  hec_endpoint = "https://splunk.example.com:8088"

  # Optional: the Splunk index the HEC token writes events to.
  # Omit to use the token's default index.
  index = "p0_audit_logs"
}
