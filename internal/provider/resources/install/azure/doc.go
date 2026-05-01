// Package installazure implements Terraform resources for installing P0 on Microsoft Azure.
//
// Several components use a two-step pattern: a "staged" resource returns metadata from P0 that
// you use to create Azure-side resources, then the main resource completes registration.
//
// Worked Terraform examples in this repository:
//   - examples/resources/p0_azure_app_staged/ — create the Azure AD app and federated credential
//     using p0_azure_app_staged outputs (app_name, credential_info), then p0_azure_app.
//   - examples/resources/p0_azure_bastion_host_staged/ and examples/resources/p0_azure_bastion_host/ —
//     read custom_role from p0_azure_bastion_host_staged, define the Azure custom role and Bastion,
//     then register with p0_azure_bastion_host.
package installazure
