# Forwards P0 audit logs to Splunk via an HTTP Event Collector (HEC); token comes from
# Splunk (Settings > Data Inputs > HTTP Event Collector). Only one per org; a second disables forwarding org-wide.
resource "p0_splunk_audit_logs" "example" {
  # User-chosen name for the HEC token.
  token_id = "p0-audit-logs"

  # HEC token value (a UUID) from Splunk; rotating the token replaces this resource.
  hec_token_cleartext = sensitive("12345678-1234-1234-1234-123456789012")

  # Must begin with "https:"; HEC uses port 8088 by default.
  hec_endpoint = "https://splunk.example.com:8088"

  # Optional; omit to use the token's default index.
  index = "p0_audit_logs"
}
