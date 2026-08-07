locals {
  # The Entra ID (Azure AD) tenant ID
  tenant_id = "12345678-1234-1234-1234-123456789012"
}

provider "azuread" {
  tenant_id = local.tenant_id
}

# ── Step 1: P0 Azure root integration ─────────────────────────────────────────
# Required before the iam-assessment component can be installed.
resource "p0_azure" "example" {
  directory_id = local.tenant_id
}

# ── Step 2: App Registration ───────────────────────────────────────────────────
# See examples/resources/p0_entra_app/ for the full app registration setup
# (Graph permissions, federated credential, and the p0_entra_app resource
# itself). Unlike iam_write, iam_assessment is read-only and does not require
# a Function App or the P0 Security Perimeter.
resource "azuread_application_registration" "p0" {
  display_name = "p0-security-entra-integration"
}

resource "azuread_service_principal" "p0" {
  client_id  = azuread_application_registration.p0.client_id
  depends_on = [azuread_application_registration.p0]
}

# Federated credential — lets P0's service account obtain Azure tokens
# without a client secret.
resource "azuread_application_federated_identity_credential" "p0" {
  depends_on     = [p0_azure.example, azuread_application_registration.p0]
  application_id = azuread_application_registration.p0.id
  display_name   = "P0Integration"
  description    = "P0 service account credential"
  issuer         = p0_azure.example.credential_info.issuer
  subject        = p0_azure.example.service_account_id
  audiences      = p0_azure.example.credential_info.audiences
}

# Register the App Registration with P0. See examples/resources/p0_entra_app/
# for the full recommended Graph permission set.
resource "p0_entra_app" "example" {
  depends_on = [
    azuread_application_federated_identity_credential.p0,
  ]
  client_id = azuread_application_registration.p0.client_id
}

# ── Step 3: P0 Entra ID IAM Assessment ────────────────────────────────────────
resource "p0_entra_id_iam_assessment" "example" {
  depends_on = [
    p0_azure.example,
    p0_entra_app.example,
  ]

  tenant_id          = local.tenant_id
  client_id          = azuread_application_registration.p0.client_id
  sovereign_cloud_id = "AzureCloud"
  email_field        = "userPrincipalName"
}
