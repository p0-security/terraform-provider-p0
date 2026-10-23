package provider

import (
	"fmt"
	"net/http"
	"slices"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

const (
	lookerId          = "acme"
	lookerProject     = "agents-project"
	lookerUrl         = "https://acme.looker.app"
	lookerAdminSecret = "p0_install_looker_acme_admin-key"
	lookerStaged      = "p0_looker_ai_staged.example"
	lookerFinal       = "p0_looker_ai.example"
)

// The elements that P0 fixes when it stages an item (step "new" in the app).
var lookerFixedFields = []string{"instanceUrl", "adminKeySecret"}

type lookerFake struct {
	*fakeP0
	// When set, verify fails the install check with this message.
	failedCheck string
	// Each write to a Looker install that P0 accepted or refused, in order.
	steps []fakeStep
}

// Adds the fields P0's installer does: the connector's identifiers when it is staged,
// and its invocation URL once it is verified. Verify and configure refuse a change to a
// fixed field, in the install framework's words.
func newLookerFake(t *testing.T) *lookerFake {
	f := &lookerFake{fakeP0: newFakeP0(t)}
	f.normalize = func(key fakeItemKey, item map[string]any) {
		secretManager, ok := item["secretManager"].(map[string]any)
		if key.integration != "looker-ai" || key.component != "credential" || !ok {
			return
		}
		secretManager["connectorRegion"] = "us-west1"
		secretManager["connectorServiceName"] = "p0-connector-looker-" + key.id
		secretManager["connectorServiceAccount"] = fmt.Sprintf("p0-connector-looker-%s@%s.iam.gserviceaccount.com", key.id, secretManager["install"])
		if item["state"] != "stage" {
			secretManager["connectorServiceUri"] = "https://p0-connector-looker-" + key.id + "-abc123-uw.a.run.app"
		}
	}
	f.refuse = func(step fakeStep, _ map[fakeItemKey]map[string]any) (int, string) {
		if step.key.integration != "looker-ai" {
			return 0, ""
		}
		f.steps = append(f.steps, step)
		if step.nextState == "stage" {
			return 0, ""
		}
		for _, field := range lookerFixedFields {
			if sent, ok := step.body[field]; ok && sent != step.previous[field] {
				return http.StatusUnprocessableEntity, fmt.Sprintf(
					"'%s' can only be altered on initial installation. Create a new installation to change this field.", field,
				)
			}
		}
		if step.nextState == "configure" && f.failedCheck != "" {
			return http.StatusUnprocessableEntity, f.failedCheck
		}
		return 0, ""
	}
	return f
}

func (f *lookerFake) credential() map[string]any {
	return f.item("looker-ai", "credential", lookerId)
}

// The states that the writes since the step at index from moved the install to,
// accepted or not.
func (f *lookerFake) stepsSince(from int) []string {
	f.mu.Lock()
	defer f.mu.Unlock()

	states := []string{}
	for _, step := range f.steps[from:] {
		states = append(states, step.nextState)
	}
	return states
}

func (f *lookerFake) stepCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()

	return len(f.steps)
}

func lookerStagedConfig(f *lookerFake) string {
	return lookerStagedConfigWith(f, lookerUrl, lookerAdminSecret)
}

func lookerStagedConfigWith(f *lookerFake, url string, secret string) string {
	return providerConfig(f.fakeP0) + fmt.Sprintf(`
resource "p0_looker_ai_staged" "example" {
  id               = %q
  instance_url     = %q
  admin_key_secret = %q

  secret_manager = {
    type       = "gcp-sm"
    project_id = %q
  }
}
`, lookerId, url, secret, lookerProject)
}

// The staged and the final resource, as a customer's Terraform installs them. An
// empty folder leaves content_folder_id out.
func lookerConfig(f *lookerFake, url string, folder string) string {
	return lookerConfigWith(f, url, lookerAdminSecret, folder)
}

func lookerConfigWith(f *lookerFake, url string, secret string, folder string) string {
	folderLine := ""
	if folder != "" {
		folderLine = fmt.Sprintf("  content_folder_id = %q\n", folder)
	}
	return lookerStagedConfigWith(f, url, secret) + `
resource "p0_looker_ai" "example" {
  id               = p0_looker_ai_staged.example.id
  instance_url     = p0_looker_ai_staged.example.instance_url
  admin_key_secret = p0_looker_ai_staged.example.admin_key_secret
` + folderLine + fmt.Sprintf(`
  # Terraform before 1.3 can't read a nested attribute in configuration, so
  # these are literals. The id above orders this after staging.
  secret_manager = {
    type       = "gcp-sm"
    project_id = %q
  }
}
`, lookerProject)
}

func checkLookerDestroyed(f *lookerFake) resource.TestCheckFunc {
	return func(*terraform.State) error {
		if item := f.credential(); item != nil {
			return fmt.Errorf("the Looker install was not removed from P0: %v", item)
		}
		return nil
	}
}

func TestLookerAi(t *testing.T) {
	f := newLookerFake(t)
	otherUrl := "https://acme-prod.eu.looker.app"
	otherSecret := "looker-admin-key"

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkLookerDestroyed(f),
		Steps: []resource.TestStep{
			// Staging alone gets the connector's identifiers, and no invocation URL yet.
			{
				Config: lookerStagedConfig(f),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(lookerStaged, "state", "stage"),
					resource.TestCheckResourceAttr(lookerStaged, "secret_manager.connector_region", "us-west1"),
					resource.TestCheckResourceAttr(lookerStaged, "secret_manager.connector_service_name", "p0-connector-looker-acme"),
					resource.TestCheckResourceAttr(lookerStaged, "secret_manager.connector_service_account", "p0-connector-looker-acme@"+lookerProject+".iam.gserviceaccount.com"),
					func(*terraform.State) error {
						item := f.credential()
						secretManager, _ := item["secretManager"].(map[string]any)
						if item["instanceUrl"] != lookerUrl || item["adminKeySecret"] != lookerAdminSecret ||
							secretManager["type"] != "gcp-sm" || secretManager["install"] != lookerProject {
							return fmt.Errorf("P0 item does not hold the staged install: %v", item)
						}
						return nil
					},
				),
			},
			// The framework re-plans after the step and fails on a non-empty plan, so this
			// also checks that the fields P0 assigns leave no diff.
			{
				Config: lookerConfig(f, lookerUrl, "42"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(lookerFinal, "state", "installed"),
					resource.TestCheckResourceAttr(lookerFinal, "instance_url", lookerUrl),
					resource.TestCheckResourceAttr(lookerFinal, "admin_key_secret", lookerAdminSecret),
					resource.TestCheckResourceAttr(lookerFinal, "content_folder_id", "42"),
					resource.TestCheckResourceAttr(lookerFinal, "secret_manager.connector_service_uri", "https://p0-connector-looker-acme-abc123-uw.a.run.app"),
					resource.TestCheckResourceAttr(lookerFinal, "secret_manager.connector_service_account", "p0-connector-looker-acme@"+lookerProject+".iam.gserviceaccount.com"),
					func(*terraform.State) error {
						if item := f.credential(); item["state"] != "installed" || item["contentFolderId"] != "42" {
							return fmt.Errorf("P0 item is not installed with the folder: %v", item)
						}
						return nil
					},
				),
			},
			{
				ResourceName:      lookerStaged,
				ImportState:       true,
				ImportStateId:     lookerId,
				ImportStateVerify: true,
				// The staged resource's state is from before the install was completed.
				ImportStateVerifyIgnore: []string{"state"},
			},
			{
				ResourceName:      lookerFinal,
				ImportState:       true,
				ImportStateId:     lookerId,
				ImportStateVerify: true,
			},
			// The folder has no install step, so a new one updates the install in place.
			{
				Config: lookerConfig(f, lookerUrl, "43"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(lookerStaged, plancheck.ResourceActionNoop),
						plancheck.ExpectResourceAction(lookerFinal, plancheck.ResourceActionUpdate),
					},
				},
				Check: func(*terraform.State) error {
					if item := f.credential(); item["state"] != "installed" || item["contentFolderId"] != "43" {
						return fmt.Errorf("P0 item does not hold the new folder: %v", item)
					}
					return nil
				},
			},
			// P0 fixes the instance URL when it stages the install, so a new one replaces it.
			{
				Config: lookerConfig(f, otherUrl, "43"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(lookerStaged, plancheck.ResourceActionReplace),
						plancheck.ExpectResourceAction(lookerFinal, plancheck.ResourceActionReplace),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(lookerFinal, "instance_url", otherUrl),
					resource.TestCheckResourceAttr(lookerFinal, "state", "installed"),
					func(*terraform.State) error {
						if item := f.credential(); item["instanceUrl"] != otherUrl || item["state"] != "installed" {
							return fmt.Errorf("P0 item does not hold the new instance: %v", item)
						}
						return nil
					},
				),
			},
			// The connector is granted read on the Admin key's secret alone, so P0 fixes it
			// too, and a new one replaces the install.
			{
				Config: lookerConfigWith(f, otherUrl, otherSecret, "43"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(lookerStaged, plancheck.ResourceActionReplace),
						plancheck.ExpectResourceAction(lookerFinal, plancheck.ResourceActionReplace),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(lookerFinal, "admin_key_secret", otherSecret),
					func(*terraform.State) error {
						if item := f.credential(); item["adminKeySecret"] != otherSecret || item["state"] != "installed" {
							return fmt.Errorf("P0 item does not hold the new secret: %v", item)
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
func TestLookerAiRollback(t *testing.T) {
	f := newLookerFake(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkLookerDestroyed(f),
		Steps: []resource.TestStep{
			{Config: lookerConfig(f, lookerUrl, "")},
			{
				Config: lookerStagedConfig(f),
				Check: func(*terraform.State) error {
					if item := f.credential(); item == nil || item["state"] != "stage" {
						return fmt.Errorf("P0 item was not returned to stage: %v", item)
					}
					return nil
				},
			},
		},
	})
}

// An install check that fails, as when the Admin key's secret has no version yet,
// fails the apply with P0's message. The next apply checks again.
func TestLookerAiFailedCheck(t *testing.T) {
	f := newLookerFake(t)
	f.failedCheck = "The connector couldn't find the Looker Admin API key secret " + lookerAdminSecret + "."

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkLookerDestroyed(f),
		Steps: []resource.TestStep{
			{
				Config:      lookerConfig(f, lookerUrl, ""),
				ExpectError: harborError("Looker install check failed", `Looker instance "acme"`, f.failedCheck),
			},
			{
				PreConfig: func() { f.failedCheck = "" },
				Config:    lookerConfig(f, lookerUrl, ""),
				Check:     resource.TestCheckResourceAttr(lookerFinal, "state", "installed"),
			},
		},
	})
}

// P0 keeps a content folder once it is set, so the plan refuses to remove one.
func TestLookerAiContentFolderRemoval(t *testing.T) {
	f := newLookerFake(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkLookerDestroyed(f),
		Steps: []resource.TestStep{
			{Config: lookerConfig(f, lookerUrl, "42")},
			{
				Config:      lookerConfig(f, lookerUrl, ""),
				PlanOnly:    true,
				ExpectError: harborError("Cannot remove content_folder_id", "Set it to another folder"),
			},
		},
	})
}

func TestLookerAiValidation(t *testing.T) {
	staged := func(id, url, secret string) string {
		return fmt.Sprintf(`
resource "p0_looker_ai_staged" "example" {
  id               = %q
  instance_url     = %q
  admin_key_secret = %q

  secret_manager = {
    type       = "gcp-sm"
    project_id = %q
  }
}
`, id, url, secret, lookerProject)
	}
	for _, tc := range []struct {
		name, config, error string
	}{
		{"id with an underscore", staged("acme_prod", lookerUrl, lookerAdminSecret), "Use only letters, digits and hyphens"},
		{"url with a path", staged(lookerId, lookerUrl+"/browse", lookerAdminSecret), "base URL in lowercase, with no path"},
		{"custom domain", staged(lookerId, "https://looker.acme.com", lookerAdminSecret), "base URL in lowercase, with no path"},
		{"http url", staged(lookerId, "http://acme.looker.app", lookerAdminSecret), "base URL in lowercase, with no path"},
		{"secret resource path", staged(lookerId, lookerUrl, "projects/p/secrets/"+lookerAdminSecret), "not its resource path"},
		{"aws secret manager", `
resource "p0_looker_ai_staged" "example" {
  id               = "acme"
  instance_url     = "https://acme.looker.app"
  admin_key_secret = "key"

  secret_manager = {
    type       = "aws-sm"
    project_id = "agents-project"
  }
}
`, "gcp-sm"},
		{"folder url", `
resource "p0_looker_ai" "example" {
  id                = "acme"
  instance_url      = "https://acme.looker.app"
  admin_key_secret  = "key"
  content_folder_id = "/folders/42"

  secret_manager = {
    type       = "gcp-sm"
    project_id = "agents-project"
  }
}
`, "last part of its URL"},
		{"staged without a secret manager", `
resource "p0_looker_ai_staged" "example" {
  id               = "acme"
  instance_url     = "https://acme.looker.app"
  admin_key_secret = "key"
}
`, `The argument "secret_manager" is required`},
		{"staged without an instance url", `
resource "p0_looker_ai_staged" "example" {
  id               = "acme"
  admin_key_secret = "key"

  secret_manager = {
    type       = "gcp-sm"
    project_id = "agents-project"
  }
}
`, `The argument "instance_url" is required`},
		{"staged without an admin key secret", `
resource "p0_looker_ai_staged" "example" {
  id           = "acme"
  instance_url = "https://acme.looker.app"

  secret_manager = {
    type       = "gcp-sm"
    project_id = "agents-project"
  }
}
`, `The argument "admin_key_secret" is required`},
		{"final without a secret manager", `
resource "p0_looker_ai" "example" {
  id               = "acme"
  instance_url     = "https://acme.looker.app"
  admin_key_secret = "key"
}
`, `The argument "secret_manager" is required`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newLookerFake(t)
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{
						Config:      providerConfig(f.fakeP0) + tc.config,
						PlanOnly:    true,
						ExpectError: harborError(tc.error),
					},
				},
			})
		})
	}
}

// An item that P0 returns without its secret manager fails the refresh of both
// resources with a diagnostic, not a panic or an inconsistent result.
func TestLookerAiMissingSecretManager(t *testing.T) {
	f := newLookerFake(t)
	var secretManager any
	// The final resource takes literal values and so doesn't depend on the staged
	// one, so that Terraform refreshes it even when the staged one fails.
	independent := lookerStagedConfig(f) + fmt.Sprintf(`
resource "p0_looker_ai" "example" {
  id               = %q
  instance_url     = %q
  admin_key_secret = %q

  secret_manager = {
    type       = "gcp-sm"
    project_id = %q
  }
}
`, lookerId, lookerUrl, lookerAdminSecret, lookerProject)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkLookerDestroyed(f),
		Steps: []resource.TestStep{
			{Config: lookerConfig(f, lookerUrl, "")},
			{
				PreConfig: func() {
					f.update("looker-ai", "credential", lookerId, func(item map[string]any) {
						secretManager = item["secretManager"]
						delete(item, "secretManager")
					})
				},
				Config:   independent,
				PlanOnly: true,
				// Once for each resource.
				ExpectError: harborError(
					"Bad API response", "The Looker install acme carries no secret manager configuration",
					"Bad API response", "The Looker install acme carries no secret manager configuration",
				),
			},
			{
				PreConfig: func() {
					f.update("looker-ai", "credential", lookerId, func(item map[string]any) {
						item["secretManager"] = secretManager
					})
				},
				Config: lookerConfig(f, lookerUrl, ""),
			},
		},
	})
}

// Completing the install sends P0 the final resource's whole configuration on every
// step, so P0 checks the install against what Terraform holds, not only against what
// was staged.
func TestLookerAiSendsConfiguration(t *testing.T) {
	f := newLookerFake(t)
	var staged int

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkLookerDestroyed(f),
		Steps: []resource.TestStep{
			{
				Config: lookerStagedConfig(f),
				Check:  func(*terraform.State) error { staged = f.stepCount(); return nil },
			},
			{
				Config: lookerConfig(f, lookerUrl, "42"),
				Check: func(*terraform.State) error {
					f.mu.Lock()
					defer f.mu.Unlock()

					steps := f.steps[staged:]
					if len(steps) != 3 {
						return fmt.Errorf("the install took %d steps, want stage, verify and configure", len(steps))
					}
					for _, step := range steps {
						secretManager, _ := step.body["secretManager"].(map[string]any)
						if step.body["instanceUrl"] != lookerUrl || step.body["adminKeySecret"] != lookerAdminSecret ||
							step.body["contentFolderId"] != "42" ||
							secretManager["type"] != "gcp-sm" || secretManager["install"] != lookerProject {
							return fmt.Errorf("the %s step did not send the configuration: %v", step.nextState, step.body)
						}
					}
					return nil
				},
			},
		},
	})
}

// An update runs P0's install check again, so a folder change that fails the check
// fails the apply and leaves P0's folder as it was.
func TestLookerAiUpdateChecksAgain(t *testing.T) {
	f := newLookerFake(t)
	var installed int

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkLookerDestroyed(f),
		Steps: []resource.TestStep{
			{
				Config: lookerConfig(f, lookerUrl, "42"),
				Check:  func(*terraform.State) error { installed = f.stepCount(); return nil },
			},
			{
				PreConfig:   func() { f.failedCheck = "Looker has no folder 43." },
				Config:      lookerConfig(f, lookerUrl, "43"),
				ExpectError: harborError("Looker install check failed", `Looker instance "acme"`, "Looker has no folder 43."),
			},
			{
				PreConfig: func() { f.failedCheck = "" },
				Config:    lookerConfig(f, lookerUrl, "43"),
				Check: func(*terraform.State) error {
					// The refused update, then the update that P0 accepted.
					want := []string{"configure", "configure", "installed"}
					if got := f.stepsSince(installed); !slices.Equal(got, want) {
						return fmt.Errorf("the updates took the steps %v, want %v", got, want)
					}
					if item := f.credential(); item["contentFolderId"] != "43" || item["state"] != "installed" {
						return fmt.Errorf("P0 item does not hold the new folder: %v", item)
					}
					return nil
				},
			},
		},
	})
}
