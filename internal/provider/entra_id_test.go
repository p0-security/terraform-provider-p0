package provider

import (
	"fmt"
	"net/http"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

const (
	entraTenantId    = "11111111-2222-3333-4444-555555555555"
	entraClientId    = "66666666-7777-8888-9999-000000000000"
	entraSaId        = "104857600000000000001"
	entraSaEmail     = "p0-entra@p0-gcp-project.iam.gserviceaccount.com"
	entraTenantLabel = "Acme Corp"
	entraStaged      = "p0_entra_id_staged.example"
	entraFinal       = "p0_entra_id.example"
)

type entraFake struct {
	*fakeP0
	// When set, configure fails the install check with this message.
	failedCheck string
}

// Adds what P0's Entra ID installer does: staging an iam-write item makes a base item
// with the service account, and configure checks the tenant and labels the item with
// the tenant's name.
func newEntraFake(t *testing.T) *entraFake {
	f := &entraFake{fakeP0: newFakeP0(t)}
	f.normalize = func(key fakeItemKey, item map[string]any) {
		if key.integration != "entra-id" || key.component != "iam-write" {
			return
		}
		baseKey := fakeItemKey{"entra-id", "base", key.id}
		if _, ok := f.items[baseKey]; !ok && item["state"] == "stage" {
			f.items[baseKey] = map[string]any{
				"serviceAccountId":    entraSaId,
				"serviceAccountEmail": entraSaEmail,
				"state":               "installed",
			}
		}
		if item["state"] == "installed" {
			item["label"] = entraTenantLabel
		}
	}
	f.refuse = func(step fakeStep, _ map[fakeItemKey]map[string]any) (int, string) {
		if step.key.integration != "entra-id" || step.nextState != "installed" || f.failedCheck == "" {
			return 0, ""
		}
		return http.StatusBadRequest, f.failedCheck
	}
	return f
}

func (f *entraFake) iamWrite() map[string]any {
	return f.item("entra-id", "iam-write", entraTenantId)
}

func (f *entraFake) base() map[string]any {
	return f.item("entra-id", "base", entraTenantId)
}

// The option P0 holds in an item's field, as {"type": option}, the way the app writes
// it. Anything else, such as a bare string, reads as no option.
func option(item map[string]any, field string) any {
	value, ok := item[field].(map[string]any)
	if !ok {
		return nil
	}
	return value["type"]
}

func entraStagedConfig(f *entraFake, cloud string) string {
	cloudLine := ""
	if cloud != "" {
		cloudLine = fmt.Sprintf("  sovereign_cloud_id = %q\n", cloud)
	}
	return providerConfig(f.fakeP0) + fmt.Sprintf(`
resource "p0_entra_id_staged" "example" {
  tenant_id = %q
%s}
`, entraTenantId, cloudLine)
}

// The staged and the final resource. An empty cloud or email field leaves it out.
func entraConfig(f *entraFake, cloud string, emailField string) string {
	emailLine := ""
	if emailField != "" {
		emailLine = fmt.Sprintf("  email_field = %q\n", emailField)
	}
	return entraStagedConfig(f, cloud) + fmt.Sprintf(`
resource "p0_entra_id" "example" {
  tenant_id          = p0_entra_id_staged.example.tenant_id
  sovereign_cloud_id = p0_entra_id_staged.example.sovereign_cloud_id
  client_id          = %q
%s}
`, entraClientId, emailLine)
}

func checkEntraDestroyed(f *entraFake) resource.TestCheckFunc {
	return func(*terraform.State) error {
		if item := f.iamWrite(); item != nil {
			return fmt.Errorf("the Entra ID install was not removed from P0: %v", item)
		}
		if item := f.base(); item != nil {
			return fmt.Errorf("the Entra ID service account was not removed from P0: %v", item)
		}
		return nil
	}
}

func TestEntraId(t *testing.T) {
	f := newEntraFake(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkEntraDestroyed(f),
		Steps: []resource.TestStep{
			// Staging sends the default cloud and reads the service account P0 assigns.
			{
				Config: entraStagedConfig(f, ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(entraStaged, "state", "stage"),
					resource.TestCheckResourceAttr(entraStaged, "sovereign_cloud_id", "AzureCloud"),
					resource.TestCheckResourceAttr(entraStaged, "service_account_id", entraSaId),
					resource.TestCheckResourceAttr(entraStaged, "service_account_email", entraSaEmail),
					func(*terraform.State) error {
						if item := f.iamWrite(); option(item, "sovereignCloudId") != "AzureCloud" {
							return fmt.Errorf("P0 item does not hold the cloud: %v", item)
						}
						return nil
					},
				),
			},
			// The framework re-plans after the step and fails on a non-empty plan, so this
			// also checks that the label P0 sets leaves no diff.
			{
				Config: entraConfig(f, "", ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(entraFinal, "state", "installed"),
					resource.TestCheckResourceAttr(entraFinal, "client_id", entraClientId),
					resource.TestCheckResourceAttr(entraFinal, "email_field", "userPrincipalName"),
					resource.TestCheckResourceAttr(entraFinal, "label", entraTenantLabel),
					func(*terraform.State) error {
						item := f.iamWrite()
						if item["state"] != "installed" || item["clientId"] != entraClientId ||
							option(item, "emailField") != "userPrincipalName" || option(item, "sovereignCloudId") != "AzureCloud" {
							return fmt.Errorf("P0 item is not installed with the caller app: %v", item)
						}
						return nil
					},
				),
			},
			{
				ResourceName:                         entraStaged,
				ImportState:                          true,
				ImportStateId:                        entraTenantId,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "tenant_id",
				// The staged resource's state is from before the install was completed.
				ImportStateVerifyIgnore: []string{"state"},
			},
			{
				ResourceName:                         entraFinal,
				ImportState:                          true,
				ImportStateId:                        entraTenantId,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "tenant_id",
			},
			// The email field and the caller app have no install step, so a change updates
			// the install in place.
			{
				Config: entraConfig(f, "", "mail"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(entraStaged, plancheck.ResourceActionNoop),
						plancheck.ExpectResourceAction(entraFinal, plancheck.ResourceActionUpdate),
					},
				},
				Check: func(*terraform.State) error {
					if item := f.iamWrite(); item["state"] != "installed" || option(item, "emailField") != "mail" {
						return fmt.Errorf("P0 item does not hold the new email field: %v", item)
					}
					return nil
				},
			},
			// P0 fixes the cloud when it stages the tenant, so a new one replaces the install.
			{
				Config: entraConfig(f, "AzureUSGovernment", "mail"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(entraStaged, plancheck.ResourceActionReplace),
						plancheck.ExpectResourceAction(entraFinal, plancheck.ResourceActionReplace),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(entraStaged, "sovereign_cloud_id", "AzureUSGovernment"),
					resource.TestCheckResourceAttr(entraStaged, "service_account_id", entraSaId),
					resource.TestCheckResourceAttr(entraFinal, "state", "installed"),
					func(*terraform.State) error {
						if item := f.iamWrite(); option(item, "sovereignCloudId") != "AzureUSGovernment" || item["state"] != "installed" {
							return fmt.Errorf("P0 item does not hold the new cloud: %v", item)
						}
						return nil
					},
				),
			},
		},
	})
}

// Destroying only the final resource returns its item to "stage", so the staged
// resource still owns it.
func TestEntraIdRollback(t *testing.T) {
	f := newEntraFake(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkEntraDestroyed(f),
		Steps: []resource.TestStep{
			{Config: entraConfig(f, "", "")},
			{
				Config: entraStagedConfig(f, ""),
				Check: func(*terraform.State) error {
					if item := f.iamWrite(); item == nil || item["state"] != "stage" {
						return fmt.Errorf("P0 item was not returned to stage: %v", item)
					}
					if f.base() == nil {
						return fmt.Errorf("the rollback removed the Entra ID service account")
					}
					return nil
				},
			},
		},
	})
}

// A check that fails, as when the caller app has no admin consent yet, fails the
// apply with P0's message. The next apply checks again.
func TestEntraIdFailedCheck(t *testing.T) {
	f := newEntraFake(t)
	f.failedCheck = "Failed to connect to Entra ID: Insufficient privileges to complete the operation."

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkEntraDestroyed(f),
		Steps: []resource.TestStep{
			{
				Config:      entraConfig(f, "", ""),
				ExpectError: harborError("Entra ID install check failed", fmt.Sprintf("Entra tenant %q", entraTenantId), f.failedCheck),
			},
			{
				PreConfig: func() { f.failedCheck = "" },
				Config:    entraConfig(f, "", ""),
				Check:     resource.TestCheckResourceAttr(entraFinal, "state", "installed"),
			},
		},
	})
}

// A staged tenant whose service account P0 no longer has fails the refresh with how
// to go on.
func TestEntraIdMissingServiceAccount(t *testing.T) {
	f := newEntraFake(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkEntraDestroyed(f),
		Steps: []resource.TestStep{
			{Config: entraStagedConfig(f, "")},
			{
				PreConfig: func() {
					f.mu.Lock()
					defer f.mu.Unlock()
					delete(f.items, fakeItemKey{"entra-id", "base", entraTenantId})
				},
				Config:      entraStagedConfig(f, ""),
				ExpectError: harborError("Entra ID service account missing", "Destroy p0_entra_id_staged"),
			},
		},
	})
}

func TestEntraIdValidation(t *testing.T) {
	f := newEntraFake(t)
	cases := []struct {
		name   string
		config string
		error  string
	}{
		{
			name: "a tenant ID that isn't a UUID",
			config: providerConfig(f.fakeP0) + `
resource "p0_entra_id_staged" "example" {
  tenant_id = "acme.onmicrosoft.com"
}
`,
			error: "Entra tenant ID must be a valid UUID",
		},
		{
			name: "an unknown cloud",
			config: providerConfig(f.fakeP0) + fmt.Sprintf(`
resource "p0_entra_id_staged" "example" {
  tenant_id          = %q
  sovereign_cloud_id = "AzureChinaCloud"
}
`, entraTenantId),
			error: "AzureCloud",
		},
		{
			name: "a client ID that isn't a UUID",
			config: providerConfig(f.fakeP0) + fmt.Sprintf(`
resource "p0_entra_id" "example" {
  tenant_id = %q
  client_id = "p0-security-azure-integration"
}
`, entraTenantId),
			error: "Entra application client ID must be a valid UUID",
		},
		{
			name: "an unknown email field",
			config: providerConfig(f.fakeP0) + fmt.Sprintf(`
resource "p0_entra_id" "example" {
  tenant_id   = %q
  client_id   = %q
  email_field = "email"
}
`, entraTenantId, entraClientId),
			error: "userPrincipalName",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{
						Config:      tc.config,
						PlanOnly:    true,
						ExpectError: regexp.MustCompile(regexp.QuoteMeta(tc.error)),
					},
				},
			})
		})
	}
}
