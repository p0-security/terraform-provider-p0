# Registers a shared drive with P0. Before P0 can grant access in it, a manager
# of the drive must add the connector's service account
# (p0_google_drive_connector.example.connector_service_account) to the drive
# with the Manager role, in Google Drive under "Manage members".
resource "p0_google_drive_shared_drive" "engineering" {
  id         = "engineering"
  drive_url  = "https://drive.google.com/drive/folders/0AbCdEfGhIjKlMnOpQr"
  project_id = p0_google_drive_connector.example.project_id
}
