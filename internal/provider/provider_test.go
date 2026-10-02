// Copyright (c) HashiCorp, Inc. and P0 Security, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"net/http"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/p0-security/terraform-provider-p0/internal"
)

// testAccProtoV6ProviderFactories are used to instantiate a provider during
// acceptance testing. The factory function will be invoked for every Terraform
// CLI command executed to create a provider server to which the CLI can
// reattach.
var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"p0": providerserver.NewProtocol6WithError(New("test")()),
}

// Acceptance tests install into the P0 organization P0_ORG, served at P0_HOST
// (https://api.p0.app by default), with the API token P0_API_TOKEN. Use an
// organization set aside for tests: they create and delete real installs.
const (
	accOrgEnv   = "P0_ORG"
	accHostEnv  = "P0_HOST"
	accTokenEnv = "P0_API_TOKEN"
)

// testAccPreCheck skips an acceptance test that has no P0 organization to run
// against, rather than failing it, because CI sets TF_ACC without one.
func testAccPreCheck(t *testing.T) {
	if os.Getenv(accOrgEnv) == "" || os.Getenv(accTokenEnv) == "" {
		t.Skipf("Set %s and %s to run acceptance tests against a P0 organization", accOrgEnv, accTokenEnv)
	}
}

func testAccHost() string {
	if host := os.Getenv(accHostEnv); host != "" {
		return host
	}
	return defaultHost
}

// testAccProviderConfig configures the provider for the acceptance-test
// organization. The provider reads the API token from P0_API_TOKEN itself.
func testAccProviderConfig() string {
	return fmt.Sprintf(`
provider "p0" {
  org  = %q
  host = %q
}
`, os.Getenv(accOrgEnv), testAccHost())
}

// testAccClient calls the acceptance-test organization's API directly, for
// checks that Terraform can't make, such as that a destroyed install is gone.
func testAccClient() *internal.P0ProviderData {
	return &internal.P0ProviderData{
		BaseUrl:        orgUrl(testAccHost(), os.Getenv(accOrgEnv)),
		Authentication: "Bearer " + os.Getenv(accTokenEnv),
		Client:         http.DefaultClient,
	}
}
