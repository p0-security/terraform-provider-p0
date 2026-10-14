package provider

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

const (
	harborInstance        = "prod"
	harborUrl             = "https://harbor.example.com"
	harborSecret          = "harbor-prod-robot"
	harborSecretsProject  = "acme-secrets"
	harborSecretsAccount  = "210987654321"
	harborInstanceAddress = "p0_harbor_instance.example"
)

// A Harbor instance as a customer's Terraform sets it. connector is an HCL expression,
// such as a reference to the connector's resource, and vault an HCL object.
type harborInstanceArgs struct {
	id, connector, url, vault, secretName string
}

func harborGcpVault(project string) string {
	return fmt.Sprintf(`{
    type       = "gcp-sm"
    project_id = %q
  }`, project)
}

func harborAwsVault(account, region string) string {
	return fmt.Sprintf(`{
    type           = "aws-sm"
    account_id     = %q
    secrets_region = %q
  }`, account, region)
}

// An instance on Google Cloud, reached through the connector of harborConnectorConfig.
func harborGcpInstance() harborInstanceArgs {
	return harborInstanceArgs{
		id:         harborInstance,
		connector:  "p0_harbor_connector.example.id",
		url:        harborUrl,
		vault:      harborGcpVault(harborSecretsProject),
		secretName: harborSecret,
	}
}

func harborAwsInstance() harborInstanceArgs {
	args := harborGcpInstance()
	args.vault = harborAwsVault(harborSecretsAccount, "us-west-2")
	args.secretName = "harbor/prod/robot"
	return args
}

// The instance with one change.
func (a harborInstanceArgs) with(change func(*harborInstanceArgs)) harborInstanceArgs {
	change(&a)
	return a
}

func (a harborInstanceArgs) hcl() string {
	return fmt.Sprintf(`
resource "p0_harbor_instance" "example" {
  id          = %q
  connector   = %s
  url         = %q
  vault       = %s
  secret_name = %q
}
`, a.id, a.connector, a.url, a.vault, a.secretName)
}

// The connector installed at hosting, and the instance it reaches.
func harborInstanceConfig(f *harborFake, hosting string, instance harborInstanceArgs) string {
	return harborConnectorConfig(f, hosting) + instance.hcl()
}

func (f *harborFake) instance(id string) map[string]any {
	return f.item("harbor", "harbor-instance", id)
}

// The instance's item in P0, if it is at state and holds exactly the fields want.
func (f *harborFake) checkInstance(state string, want map[string]any) error {
	item := f.instance(harborInstance)
	fields := harborItemFields(item)
	if item["state"] != state || len(fields) != len(want) {
		return fmt.Errorf("P0 item = %v; want it at %s with %v", item, state, want)
	}
	for field, value := range want {
		if fmt.Sprint(fields[field]) != fmt.Sprint(value) {
			return fmt.Errorf("P0 item = %v; want it at %s with %v", item, state, want)
		}
	}
	return nil
}

// The fields of an item that a request sets, without the label and state P0 adds.
func harborItemFields(item map[string]any) map[string]any {
	fields := map[string]any{}
	for field, value := range item {
		if field != "label" && field != "state" {
			fields[field] = value
		}
	}
	return fields
}

func (f *harborFake) steps() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.instanceSteps)
}

func checkHarborInstanceDestroyed(f *harborFake, ids ...string) func(*terraform.State) error {
	return func(state *terraform.State) error {
		for _, id := range ids {
			if item := f.instance(id); item != nil {
				return fmt.Errorf("the instance %s was not removed from P0: %v", id, item)
			}
		}
		return checkHarborDestroyed(f)(state)
	}
}

var harborGcpInstanceItem = map[string]any{
	"connector":  harborConnector,
	"url":        harborUrl,
	"vault":      map[string]any{"type": "gcp-sm", "install": harborSecretsProject},
	"secretName": harborSecret,
}

// The instance behind a connector on Google Cloud, with its secret in Google Secret
// Manager. Creating it stages, verifies and configures the item.
func TestHarborInstanceOnGoogleCloud(t *testing.T) {
	f := newHarborFake(t)
	config := harborInstanceConfig(f, harborGcp, harborGcpInstance())

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkHarborInstanceDestroyed(f, harborInstance),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(harborInstanceAddress, "state", "installed"),
					resource.TestCheckResourceAttr(harborInstanceAddress, "connector", harborConnector),
					resource.TestCheckResourceAttr(harborInstanceAddress, "vault.type", "gcp-sm"),
					resource.TestCheckResourceAttr(harborInstanceAddress, "vault.project_id", harborSecretsProject),
					resource.TestCheckNoResourceAttr(harborInstanceAddress, "vault.account_id"),
					resource.TestCheckNoResourceAttr(harborInstanceAddress, "vault.secrets_region"),
					func(*terraform.State) error {
						if steps := f.steps(); !slices.Equal(steps, []string{"stage", "configure", "installed"}) {
							return fmt.Errorf("P0 got the instance's steps %v; want stage, verify, configure", steps)
						}
						return f.checkInstance("installed", harborGcpInstanceItem)
					},
				),
			},
			// Applying the same configuration again changes nothing.
			{
				Config: config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				ResourceName:      harborInstanceAddress,
				ImportState:       true,
				ImportStateId:     harborInstance,
				ImportStateVerify: true,
			},
			// The ID goes into a URL path as it is, so one that P0 would refuse isn't read.
			{
				ResourceName:  harborInstanceAddress,
				ImportState:   true,
				ImportStateId: "prod/other",
				ExpectError:   harborError("Invalid Harbor instance ID", "Use lowercase letters, digits and hyphens", `Got: "prod/other"`),
			},
		},
	})
}

// The instance behind a connector on AWS, with its secret in AWS Secrets Manager.
func TestHarborInstanceOnAws(t *testing.T) {
	f := newHarborFake(t)
	config := harborInstanceConfig(f, harborAws, harborAwsInstance())

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkHarborInstanceDestroyed(f, harborInstance),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(harborInstanceAddress, "state", "installed"),
					resource.TestCheckResourceAttr(harborInstanceAddress, "vault.account_id", harborSecretsAccount),
					resource.TestCheckResourceAttr(harborInstanceAddress, "vault.secrets_region", "us-west-2"),
					resource.TestCheckNoResourceAttr(harborInstanceAddress, "vault.project_id"),
					func(*terraform.State) error {
						return f.checkInstance("installed", map[string]any{
							"connector":  harborConnector,
							"url":        harborUrl,
							"vault":      map[string]any{"type": "aws-sm", "install": harborSecretsAccount, "secretsRegion": "us-west-2"},
							"secretName": "harbor/prod/robot",
						})
					},
				),
			},
			{
				Config: config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				ResourceName:      harborInstanceAddress,
				ImportState:       true,
				ImportStateId:     harborInstance,
				ImportStateVerify: true,
			},
		},
	})
}

// A second connector, for an instance to move to.
const harborSecondConnector = `
resource "p0_harbor_connector_staged" "second" {
  id = "prod-2"
  hosting = {
    type             = "gcp"
    project_id       = "acme-prod"
    connector_name   = "p0-harbor-prod-2"
    connector_region = "us-west1"
  }
}

resource "p0_harbor_connector" "second" {
  id = p0_harbor_connector_staged.second.id
  hosting = {
    type             = "gcp"
    project_id       = "acme-prod"
    connector_name   = "p0-harbor-prod-2"
    connector_region = "us-west1"
  }
}
`

// P0 fixes every field of an instance when it creates it. A new connector, vault or
// secret replaces the instance, and leaves its connector alone. A new URL is refused,
// unless the ID is new too.
func TestHarborInstanceChangeReplaces(t *testing.T) {
	gcp, aws := harborGcpInstance(), harborAwsInstance()

	for _, tc := range []struct {
		name          string
		hosting       string
		before, after harborInstanceArgs
		// The changed field, as P0 stores it.
		field string
		value any
	}{
		{"secret name", harborGcp, gcp, gcp.with(func(a *harborInstanceArgs) { a.secretName = "harbor-prod-robot-2" }), "secretName", "harbor-prod-robot-2"},
		{"connector", harborGcp, gcp, gcp.with(func(a *harborInstanceArgs) { a.connector = "p0_harbor_connector.second.id" }), "connector", "prod-2"},
		{
			"vault project", harborGcp, gcp,
			gcp.with(func(a *harborInstanceArgs) { a.vault = harborGcpVault("acme-secrets-2") }),
			"vault", map[string]any{"type": "gcp-sm", "install": "acme-secrets-2"},
		},
		{
			"vault account", harborAws, aws,
			aws.with(func(a *harborInstanceArgs) { a.vault = harborAwsVault("123456789012", "us-west-2") }),
			"vault", map[string]any{"type": "aws-sm", "install": "123456789012", "secretsRegion": "us-west-2"},
		},
		{
			"vault region", harborAws, aws,
			aws.with(func(a *harborInstanceArgs) { a.vault = harborAwsVault(harborSecretsAccount, "eu-west-1") }),
			"vault", map[string]any{"type": "aws-sm", "install": harborSecretsAccount, "secretsRegion": "eu-west-1"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newHarborFake(t)
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				CheckDestroy:             checkHarborInstanceDestroyed(f, harborInstance),
				Steps: []resource.TestStep{
					{Config: harborInstanceConfig(f, tc.hosting, tc.before) + harborSecondConnector},
					{
						Config: harborInstanceConfig(f, tc.hosting, tc.after) + harborSecondConnector,
						ConfigPlanChecks: resource.ConfigPlanChecks{
							PreApply: []plancheck.PlanCheck{
								plancheck.ExpectResourceAction(harborInstanceAddress, plancheck.ResourceActionReplace),
								plancheck.ExpectResourceAction("p0_harbor_connector.example", plancheck.ResourceActionNoop),
								plancheck.ExpectResourceAction("p0_harbor_connector_staged.example", plancheck.ResourceActionNoop),
							},
						},
						Check: resource.ComposeAggregateTestCheckFunc(
							resource.TestCheckResourceAttr(harborInstanceAddress, "state", "installed"),
							func(*terraform.State) error {
								item := f.instance(harborInstance)
								if fmt.Sprint(item[tc.field]) != fmt.Sprint(tc.value) || item["state"] != "installed" {
									return fmt.Errorf("P0 item = %v; want it installed with %s %v", item, tc.field, tc.value)
								}
								return nil
							},
						),
					},
				},
			})
		})
	}

	// Replacing an installed instance under its ID would point its open access at
	// another Harbor's users, so the plan refuses a new URL, and nothing changes.
	t.Run("url", func(t *testing.T) {
		f := newHarborFake(t)
		moved := gcp.with(func(a *harborInstanceArgs) { a.url = "https://registry.example.com:8443" })
		resource.UnitTest(t, resource.TestCase{
			ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
			CheckDestroy:             checkHarborInstanceDestroyed(f, harborInstance),
			Steps: []resource.TestStep{
				{Config: harborInstanceConfig(f, harborGcp, gcp)},
				{
					Config:   harborInstanceConfig(f, harborGcp, moved),
					PlanOnly: true,
					ExpectError: harborError(
						"A Harbor instance's address can't change",
						"Add a new p0_harbor_instance with its own id, and remove this one once its open access has ended.",
					),
				},
				{
					Config:      harborInstanceConfig(f, harborGcp, moved),
					ExpectError: harborError("A Harbor instance's address can't change"),
				},
				{
					PreConfig: func() {
						if err := f.checkInstance("installed", harborGcpInstanceItem); err != nil {
							t.Errorf("after the refused change: %v", err)
						}
					},
					Config: harborInstanceConfig(f, harborGcp, gcp),
					ConfigPlanChecks: resource.ConfigPlanChecks{
						PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
					},
				},
			},
		})
	})

	// A new ID is a new item, and the old one goes. It can have a new URL: nothing
	// was granted under the new ID.
	t.Run("id", func(t *testing.T) {
		f := newHarborFake(t)
		resource.UnitTest(t, resource.TestCase{
			ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
			CheckDestroy:             checkHarborInstanceDestroyed(f, harborInstance, "staging"),
			Steps: []resource.TestStep{
				{Config: harborInstanceConfig(f, harborGcp, gcp)},
				{
					Config: harborInstanceConfig(f, harborGcp, gcp.with(func(a *harborInstanceArgs) {
						a.id, a.url = "staging", "https://staging.example.com"
					})),
					ConfigPlanChecks: resource.ConfigPlanChecks{
						PreApply: []plancheck.PlanCheck{
							plancheck.ExpectResourceAction(harborInstanceAddress, plancheck.ResourceActionReplace),
						},
					},
					Check: func(*terraform.State) error {
						if item := f.instance(harborInstance); item != nil {
							return fmt.Errorf("the old instance is still in P0: %v", item)
						}
						if item := f.instance("staging"); item["url"] != "https://staging.example.com" || item["state"] != "installed" {
							return fmt.Errorf("P0 item = %v; want the new instance installed at the new URL", item)
						}
						return nil
					},
				},
			},
		})
	})

	// A vault on another cloud needs a connector on that cloud, so the plan alone is
	// checked here.
	t.Run("vault type", func(t *testing.T) {
		f := newHarborFake(t)
		resource.UnitTest(t, resource.TestCase{
			ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
			CheckDestroy:             checkHarborInstanceDestroyed(f, harborInstance),
			Steps: []resource.TestStep{
				{Config: harborInstanceConfig(f, harborGcp, gcp)},
				{
					Config:             harborInstanceConfig(f, harborGcp, gcp.with(func(a *harborInstanceArgs) { a.vault = harborAwsVault(harborSecretsAccount, "us-west-2") })),
					PlanOnly:           true,
					ExpectNonEmptyPlan: true,
					ConfigPlanChecks: resource.ConfigPlanChecks{
						PostApplyPostRefresh: []plancheck.PlanCheck{
							plancheck.ExpectResourceAction(harborInstanceAddress, plancheck.ResourceActionReplace),
						},
					},
				},
			},
		})
	})
}

// The URL, the vault and the secret's name can come from variables, which are unknown
// while Terraform validates the configuration, so the checks skip them then, and
// known when it plans.
func TestHarborInstanceFromVariables(t *testing.T) {
	f := newHarborFake(t)
	config := harborConnectorConfig(f, harborGcp) + fmt.Sprintf(`
variable "url" {
  type    = string
  default = %q
}

variable "vault" {
  type = object({
    type       = string
    project_id = string
  })
  default = %s
}

variable "secret_name" {
  type    = string
  default = %q
}

resource "p0_harbor_instance" "example" {
  id          = %q
  connector   = p0_harbor_connector.example.id
  url         = var.url
  vault       = var.vault
  secret_name = var.secret_name
}
`, harborUrl, harborGcpVault(harborSecretsProject), harborSecret, harborInstance)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkHarborInstanceDestroyed(f, harborInstance),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: func(*terraform.State) error {
					return f.checkInstance("installed", harborGcpInstanceItem)
				},
			},
		},
	})
}

// An instance that fails P0's install check fails the apply with P0's message and the
// way out, and stays where the failed step left it: staged if verify failed, at
// configure if configure did. Its resource is tainted, so the next apply replaces it,
// and can give it another URL: Terraform plans a tainted resource as a new one.
func TestHarborInstanceFailedCheck(t *testing.T) {
	for _, tc := range []struct {
		name, leftAt string
		fail         func(f *harborFake, message string)
	}{
		{"verify", "stage", func(f *harborFake, message string) { f.failedInstanceCheck = message }},
		{"configure", "configure", func(f *harborFake, message string) { f.failedInstanceConfigure = message }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newHarborFake(t)
			const message = "The connector couldn't find its robot account's secret in Google Secret Manager."
			tc.fail(f, message)
			fixed := harborGcpInstance().with(func(a *harborInstanceArgs) { a.url = "https://registry.example.com" })

			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				CheckDestroy:             checkHarborInstanceDestroyed(f, harborInstance),
				Steps: []resource.TestStep{
					{
						Config: harborInstanceConfig(f, harborGcp, harborGcpInstance()),
						ExpectError: harborError(
							"P0 didn't finish installing the Harbor instance", `Harbor instance "prod"`, message,
							"terraform apply -replace=<address>", "terraform state rm <address>",
						),
					},
					{
						PreConfig: func() {
							if err := f.checkInstance(tc.leftAt, harborGcpInstanceItem); err != nil {
								t.Errorf("after the failed step: %v", err)
							}
							tc.fail(f, "")
						},
						Config: harborInstanceConfig(f, harborGcp, fixed),
						ConfigPlanChecks: resource.ConfigPlanChecks{
							PreApply: []plancheck.PlanCheck{
								plancheck.ExpectResourceAction(harborInstanceAddress, plancheck.ResourceActionReplace),
								plancheck.ExpectResourceAction("p0_harbor_connector.example", plancheck.ResourceActionNoop),
							},
						},
						Check: func(*terraform.State) error {
							if item := f.instance(harborInstance); item["url"] != "https://registry.example.com" || item["state"] != "installed" {
								return fmt.Errorf("P0 item = %v; want it installed at the corrected URL", item)
							}
							return nil
						},
					},
				},
			})
		})
	}
}

// P0 has the last word on what the plan can't check, and its refusal fails the apply
// in its own words.
func TestHarborInstanceRefusedByP0(t *testing.T) {
	// The configuration names the connector by its ID alone, so the plan can't check
	// that the vault is in the connector's cloud. P0 refuses to stage the instance.
	t.Run("vault in another cloud", func(t *testing.T) {
		f := newHarborFake(t)
		resource.UnitTest(t, resource.TestCase{
			ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
			CheckDestroy:             checkHarborInstanceDestroyed(f, harborInstance),
			Steps: []resource.TestStep{
				{
					Config: harborInstanceConfig(f, harborGcp, harborAwsInstance()),
					ExpectError: harborError(
						"Could not stage harbor-instance component",
						"400 Bad Request: The instance's secret manager must be in the connector's cloud.",
					),
				},
				{
					PreConfig: func() {
						if item := f.instance(harborInstance); item != nil {
							t.Errorf("P0 stored the refused instance: %v", item)
						}
					},
					Config: harborConnectorConfig(f, harborGcp),
				},
			},
		})
	})

	t.Run("connector P0 doesn't have", func(t *testing.T) {
		f := newHarborFake(t)
		instance := harborGcpInstance().with(func(a *harborInstanceArgs) { a.connector = `"missing"` })
		resource.UnitTest(t, resource.TestCase{
			ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
			CheckDestroy:             checkHarborInstanceDestroyed(f, harborInstance),
			Steps: []resource.TestStep{
				{
					Config:      harborConnectorConfig(f, harborGcp) + instance.hcl(),
					ExpectError: harborError("Could not stage harbor-instance component", `400 Bad Request: P0 has no Harbor connector "missing".`),
				},
			},
		})
	})

	// P0 checks the instance through its connector, so the connector must be installed
	// first. Referring to the connector's final resource orders them.
	t.Run("connector not installed", func(t *testing.T) {
		f := newHarborFake(t)
		instance := harborGcpInstance().with(func(a *harborInstanceArgs) { a.connector = "p0_harbor_connector_staged.example.id" })
		resource.UnitTest(t, resource.TestCase{
			ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
			CheckDestroy:             checkHarborInstanceDestroyed(f, harborInstance),
			Steps: []resource.TestStep{
				{
					Config:      harborStagedConfig(f, harborGcp) + instance.hcl(),
					ExpectError: harborError("P0 didn't finish installing the Harbor instance", `The Harbor connector "prod" isn't installed.`),
				},
			},
		})
	})
}

// An instance that P0 already holds, such as one added in the P0 app, isn't created
// again, even when it matches the configuration: P0 would accept the PUT, which
// returns an installed instance to "stage". The apply fails, writes nothing, and says
// to import the instance.
func TestHarborInstanceCreateOverExisting(t *testing.T) {
	for name, url := range map[string]string{"identical": harborUrl, "different": "https://other.example.com"} {
		t.Run(name, func(t *testing.T) {
			f := newHarborFake(t)
			key := fakeItemKey{"harbor", "harbor-instance", harborInstance}
			existing := harborItemFields(harborGcpInstanceItem)
			existing["url"], existing["label"], existing["state"] = url, harborInstance, "installed"

			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{Config: harborConnectorConfig(f, harborGcp)},
					{
						PreConfig: func() {
							f.mu.Lock()
							defer f.mu.Unlock()
							f.items[key] = existing
						},
						Config: harborInstanceConfig(f, harborGcp, harborGcpInstance()),
						ExpectError: harborError(
							"Harbor instance already exists in P0", `P0 already has a Harbor instance "prod"`,
							"terraform import <address> prod",
						),
					},
					{
						PreConfig: func() {
							if steps := f.steps(); len(steps) != 0 {
								t.Errorf("P0 got the instance's steps %v; want no write", steps)
							}
							if item := f.instance(harborInstance); item["url"] != url || item["state"] != "installed" {
								t.Errorf("P0 item = %v; want the existing instance as it was", item)
							}
							f.mu.Lock()
							defer f.mu.Unlock()
							delete(f.items, key)
						},
						Config: harborConnectorConfig(f, harborGcp),
					},
				},
			})
		})
	}
}

// If P0 can't say whether the instance exists, as when the read fails with a 500, the
// create fails with P0's error and writes nothing: it never stages on a guess.
func TestHarborInstanceCreateReadFails(t *testing.T) {
	f := newHarborFake(t)
	fake := f.server.Config.Handler
	path := "/o/" + fakeOrg + "/integrations/harbor/config/harbor-instance/" + harborInstance

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkHarborInstanceDestroyed(f, harborInstance),
		Steps: []resource.TestStep{
			{Config: harborConnectorConfig(f, harborGcp)},
			{
				PreConfig: func() {
					f.server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if r.Method == http.MethodGet && r.URL.Path == path {
							http.Error(w, `{"error":"P0 is having a bad day"}`, http.StatusInternalServerError)
							return
						}
						fake.ServeHTTP(w, r)
					})
				},
				Config: harborInstanceConfig(f, harborGcp, harborGcpInstance()),
				ExpectError: harborError(
					"Error communicating with P0", `Could not check whether the Harbor instance "prod" exists`,
					"500 Internal Server Error: P0 is having a bad day",
				),
			},
			{
				PreConfig: func() {
					f.server.Config.Handler = fake
					if steps := f.steps(); len(steps) != 0 {
						t.Errorf("P0 got the instance's steps %v; want no write", steps)
					}
					if item := f.instance(harborInstance); item != nil {
						t.Errorf("P0 item = %v; want none", item)
					}
				},
				Config: harborConnectorConfig(f, harborGcp),
			},
		},
	})
}

// The provider never sends these, so the fake's refusals for them are exercised
// directly: a PUT that changes an instance P0 holds, the same PUT unchanged, and a
// verify that changes a field.
func TestHarborFakeInstanceRefusals(t *testing.T) {
	f := newHarborFake(t)
	f.integrations["harbor"] = true
	f.items[fakeItemKey{"harbor", "connector", harborConnector}] = map[string]any{
		"hosting": map[string]any{"type": "gcp"}, "state": "installed",
	}
	item := "/o/" + fakeOrg + "/integrations/harbor/config/harbor-instance/" + harborInstance
	send := func(method, path, url string) int {
		t.Helper()
		body := fmt.Sprintf(`{"connector":%q,"url":%q,"secretName":%q,"vault":{"type":"gcp-sm","install":%q}}`, harborConnector, url, harborSecret, harborSecretsProject)
		req, err := http.NewRequest(method, f.server.URL+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		return resp.StatusCode
	}

	for _, tc := range []struct {
		name, method, path, url string
		want                    int
	}{
		{"the first PUT", http.MethodPut, item, harborUrl, http.StatusOK},
		{"an identical PUT", http.MethodPut, item, harborUrl, http.StatusOK},
		{"a PUT that changes the URL", http.MethodPut, item, "https://other.example.com", http.StatusConflict},
		{"a verify that changes the URL", http.MethodPost, item + "/verify", "https://other.example.com", http.StatusUnprocessableEntity},
		{"a verify that changes nothing", http.MethodPost, item + "/verify", harborUrl, http.StatusOK},
	} {
		if got := send(tc.method, tc.path, tc.url); got != tc.want {
			t.Errorf("%s: status %d; want %d", tc.name, got, tc.want)
		}
	}
	if stored := f.instance(harborInstance); stored["url"] != harborUrl {
		t.Errorf("P0 item = %v; want the first URL kept", stored)
	}
}

// With Harbor's feature flag off, P0 answers the request that stages an instance with
// a bare 404. The apply fails with what that means, and what to do about it.
func TestHarborInstanceUnavailable(t *testing.T) {
	f := newHarborFake(t)
	f.disabled = true
	instance := harborGcpInstance()
	instance.connector = fmt.Sprintf("%q", harborConnector)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkHarborInstanceDestroyed(f, harborInstance),
		Steps: []resource.TestStep{
			{
				Config: providerConfig(f.fakeP0) + instance.hcl(),
				ExpectError: harborError(
					"Harbor isn't enabled for this P0 organization",
					`P0 couldn't stage or install the Harbor instance "prod"`,
					"contact support@p0.dev to have it enabled",
					"P0's response: 404 Not Found: Not found",
				),
			},
		},
	})
}

// An instance that P0 no longer returns is dropped from state, as most resources are.
// One deleted in the P0 app is created again by the next apply. One that P0 only
// hides, as while Harbor's feature flag is off, is never written over: once it is
// back, the apply says to import it.
func TestHarborInstanceGoneFromP0(t *testing.T) {
	key := fakeItemKey{"harbor", "harbor-instance", harborInstance}

	t.Run("deleted", func(t *testing.T) {
		f := newHarborFake(t)
		config := harborInstanceConfig(f, harborGcp, harborGcpInstance())
		resource.UnitTest(t, resource.TestCase{
			ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
			CheckDestroy:             checkHarborInstanceDestroyed(f, harborInstance),
			Steps: []resource.TestStep{
				{Config: config},
				{
					PreConfig: func() {
						f.mu.Lock()
						defer f.mu.Unlock()
						delete(f.items, key)
					},
					Config: config,
					ConfigPlanChecks: resource.ConfigPlanChecks{
						PreApply: []plancheck.PlanCheck{
							plancheck.ExpectResourceAction(harborInstanceAddress, plancheck.ResourceActionCreate),
						},
					},
					Check: func(*terraform.State) error {
						return f.checkInstance("installed", harborGcpInstanceItem)
					},
				},
			},
		})
	})

	t.Run("hidden", func(t *testing.T) {
		f := newHarborFake(t)
		config := harborInstanceConfig(f, harborGcp, harborGcpInstance())
		var hidden map[string]any
		resource.UnitTest(t, resource.TestCase{
			ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
			Steps: []resource.TestStep{
				{Config: config},
				// The refresh drops the instance, and the flag is off for the apply.
				{
					PreConfig: func() {
						f.mu.Lock()
						defer f.mu.Unlock()
						hidden = f.items[key]
						delete(f.items, key)
						f.disabled = true
					},
					Config:      config,
					ExpectError: harborError("Harbor isn't enabled for this P0 organization", `the Harbor instance "prod"`),
				},
				{
					PreConfig: func() {
						f.mu.Lock()
						defer f.mu.Unlock()
						f.items[key] = hidden
						f.disabled = false
						f.instanceSteps = nil
					},
					Config:      config,
					ExpectError: harborError("Harbor instance already exists in P0", "terraform import <address> prod"),
				},
				{
					PreConfig: func() {
						if steps := f.steps(); len(steps) != 0 {
							t.Errorf("P0 got the instance's steps %v; want no write", steps)
						}
						if err := f.checkInstance("installed", harborGcpInstanceItem); err != nil {
							t.Errorf("after the refused create: %v", err)
						}
						f.mu.Lock()
						defer f.mu.Unlock()
						delete(f.items, key)
					},
					Config: harborConnectorConfig(f, harborGcp),
				},
			},
		})
	})
}

// An instance that P0 holds but hasn't finished installing, as when it was staged in
// the P0 app, plans an update once imported, and applying it finishes the install:
// from "stage" with verify and then configure, from "configure" with configure alone.
func TestHarborInstanceImportFinishesInstall(t *testing.T) {
	for _, state := range []string{"stage", "configure"} {
		t.Run(state, func(t *testing.T) {
			f := newHarborFake(t)
			instance := harborGcpInstance()
			instance.connector = fmt.Sprintf("%q", harborConnector)
			config := providerConfig(f.fakeP0) + instance.hcl()
			want := map[string][]string{"stage": {"configure", "installed"}, "configure": {"installed"}}[state]

			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				CheckDestroy: func(*terraform.State) error {
					if item := f.instance(harborInstance); item != nil {
						return fmt.Errorf("the instance was not removed from P0: %v", item)
					}
					return nil
				},
				Steps: []resource.TestStep{
					{
						PreConfig:          func() { f.seedInstance(state, harborUrl) },
						Config:             config,
						ResourceName:       harborInstanceAddress,
						ImportState:        true,
						ImportStateId:      harborInstance,
						ImportStatePersist: true,
						ImportStateCheck: func(states []*terraform.InstanceState) error {
							if len(states) != 1 || states[0].Attributes["state"] != state {
								return fmt.Errorf("imported %v; want one instance at %s", states, state)
							}
							return nil
						},
					},
					{
						Config: config,
						ConfigPlanChecks: resource.ConfigPlanChecks{
							PreApply: []plancheck.PlanCheck{
								plancheck.ExpectResourceAction(harborInstanceAddress, plancheck.ResourceActionUpdate),
							},
						},
						Check: resource.ComposeAggregateTestCheckFunc(
							resource.TestCheckResourceAttr(harborInstanceAddress, "state", "installed"),
							func(*terraform.State) error {
								if steps := f.steps(); !slices.Equal(steps, want) {
									return fmt.Errorf("P0 got the instance's steps %v; want %v", steps, want)
								}
								return f.checkInstance("installed", harborGcpInstanceItem)
							},
						),
					},
				},
			})
		})
	}
}

// Puts an installed connector and an instance at state in P0, as the P0 app would.
func (f *harborFake) seedInstance(state, url string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.integrations["harbor"] = true
	f.items[fakeItemKey{"harbor", "connector", harborConnector}] = map[string]any{
		"hosting": map[string]any{"type": "gcp", "projectId": harborProject, "connectorName": harborConnectorName, "connectorRegion": "us-west1"},
		"label":   harborConnector,
		"state":   "installed",
	}
	item := harborItemFields(harborGcpInstanceItem)
	item["url"], item["label"], item["state"] = url, harborInstance, state
	f.items[fakeItemKey{"harbor", "harbor-instance", harborInstance}] = item
}

// An imported instance whose install P0 won't finish isn't tainted, so every apply
// tries again. The diagnostic says how to start over, and once the cause is gone the
// next apply finishes from where P0 left the item.
func TestHarborInstanceImportedInstallFails(t *testing.T) {
	instance := harborGcpInstance().with(func(a *harborInstanceArgs) { a.connector = fmt.Sprintf("%q", harborConnector) })
	destroyed := func(f *harborFake) func(*terraform.State) error {
		return func(*terraform.State) error {
			if item := f.instance(harborInstance); item != nil {
				return fmt.Errorf("the instance was not removed from P0: %v", item)
			}
			return nil
		}
	}
	importStaged := func(f *harborFake, config, url string) resource.TestStep {
		return resource.TestStep{
			PreConfig:          func() { f.seedInstance("stage", url) },
			Config:             config,
			ResourceName:       harborInstanceAddress,
			ImportState:        true,
			ImportStateId:      harborInstance,
			ImportStatePersist: true,
		}
	}

	t.Run("configure fails", func(t *testing.T) {
		f := newHarborFake(t)
		f.failedInstanceConfigure = "The robot account can read every user's CLI secret."
		config := providerConfig(f.fakeP0) + instance.hcl()
		resource.UnitTest(t, resource.TestCase{
			ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
			CheckDestroy:             destroyed(f),
			Steps: []resource.TestStep{
				importStaged(f, config, harborUrl),
				{
					Config: config,
					ExpectError: harborError(
						"P0 didn't finish installing the Harbor instance", f.failedInstanceConfigure,
						"terraform apply -replace=<address>", "terraform state rm <address>",
					),
				},
				{
					PreConfig: func() {
						if err := f.checkInstance("configure", harborGcpInstanceItem); err != nil {
							t.Errorf("after the failed configure: %v", err)
						}
						f.mu.Lock()
						defer f.mu.Unlock()
						f.failedInstanceConfigure, f.instanceSteps = "", nil
					},
					Config: config,
					ConfigPlanChecks: resource.ConfigPlanChecks{
						PreApply: []plancheck.PlanCheck{
							plancheck.ExpectResourceAction(harborInstanceAddress, plancheck.ResourceActionUpdate),
						},
					},
					Check: func(*terraform.State) error {
						if steps := f.steps(); !slices.Equal(steps, []string{"installed"}) {
							return fmt.Errorf("P0 got the instance's steps %v; want configure alone", steps)
						}
						return f.checkInstance("installed", harborGcpInstanceItem)
					},
				},
			},
		})
	})

	// The instance in P0 has another URL than the configuration. It was never
	// installed, so the plan lets the URL through, and P0 refuses to change it.
	t.Run("differs from the configuration", func(t *testing.T) {
		f := newHarborFake(t)
		config := providerConfig(f.fakeP0) + instance.hcl()
		resource.UnitTest(t, resource.TestCase{
			ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
			CheckDestroy:             destroyed(f),
			Steps: []resource.TestStep{
				importStaged(f, config, "https://other.example.com"),
				{
					Config: config,
					ExpectError: harborError(
						"Harbor instance differs from the one in P0", "'url' can only be altered on initial installation",
						"terraform apply -replace=<address>",
					),
				},
			},
		})
	})
}

// An instance that P0 returns with no vault, or one this provider doesn't support,
// such as one changed in the P0 app, fails the refresh and says how to go on.
func TestHarborInstanceUnreadableVault(t *testing.T) {
	f := newHarborFake(t)
	config := harborInstanceConfig(f, harborGcp, harborGcpInstance())
	setVault := func(vault any) func() {
		return func() {
			f.update("harbor", "harbor-instance", harborInstance, func(item map[string]any) {
				if vault == nil {
					delete(item, "vault")
				} else {
					item["vault"] = vault
				}
			})
		}
	}

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkHarborInstanceDestroyed(f, harborInstance),
		Steps: []resource.TestStep{
			{Config: config},
			{
				PreConfig:   setVault(nil),
				Config:      config,
				PlanOnly:    true,
				ExpectError: harborError("Bad API response", `P0 returned the Harbor instance "prod" without its vault`, "terraform state rm <address>"),
			},
			{
				PreConfig:   setVault(map[string]any{"type": "azure-kv", "install": "acme"}),
				Config:      config,
				PlanOnly:    true,
				ExpectError: harborError("Unsupported Harbor instance vault", `The Harbor instance prod has a "azure-kv" vault`, "terraform state rm"),
			},
			{
				PreConfig: setVault(map[string]any{"type": "gcp-sm", "install": harborSecretsProject}),
				Config:    config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})
}

func TestHarborInstanceValidation(t *testing.T) {
	const invalidId = "Use lowercase letters, digits and hyphens, starting and ending with a letter or digit, up to 63 characters"
	const invalidUrl = "Enter the instance's address as https://<host> or https://<host>:<port>"
	const invalidAwsName = "That doesn't look like a secret name."
	const invalidGcpName = "That isn't a secret's name in Google Secret Manager."
	base := harborGcpInstance()
	base.connector = fmt.Sprintf("%q", harborConnector)
	aws := base
	aws.vault = harborAwsVault(harborSecretsAccount, "us-west-2")
	vault := func(fields string) func(*harborInstanceArgs) {
		return func(a *harborInstanceArgs) { a.vault = "{\n" + fields + "\n  }" }
	}

	for _, tc := range []struct {
		name   string
		args   harborInstanceArgs
		errors []string
	}{
		{"id with capitals", base.with(func(a *harborInstanceArgs) { a.id = "Prod" }), []string{invalidId}},
		{"id over 63 characters", base.with(func(a *harborInstanceArgs) { a.id = strings.Repeat("a", 64) }), []string{"string length must be at most 63"}},
		{"connector with a path", base.with(func(a *harborInstanceArgs) { a.connector = `"prod/other"` }), []string{"connector", invalidId}},
		{"URL with a trailing slash", base.with(func(a *harborInstanceArgs) { a.url = harborUrl + "/" }), []string{"Invalid Harbor instance URL", invalidUrl}},
		{"URL with plain http", base.with(func(a *harborInstanceArgs) { a.url = "http://harbor.example.com" }), []string{invalidUrl}},
		{"URL with the default port", base.with(func(a *harborInstanceArgs) { a.url = harborUrl + ":443" }), []string{invalidUrl}},
		{"URL with an uppercase host", base.with(func(a *harborInstanceArgs) { a.url = "https://Harbor.example.com" }), []string{invalidUrl}},
		{"URL with credentials", base.with(func(a *harborInstanceArgs) { a.url = "https://robot:not-a-real-secret@harbor.example.com" }), []string{invalidUrl}},
		{"URL with an IPv4 address in three parts", base.with(func(a *harborInstanceArgs) { a.url = "https://1.2.3" }), []string{invalidUrl}},
		// P0 would trim the URL and store it, and the apply would then fail on a value
		// that reads back different.
		{"URL with whitespace around it", base.with(func(a *harborInstanceArgs) { a.url = harborUrl + " " }), []string{"Whitespace around a value", "P0 removes the whitespace around 'url'"}},
		{"secret ARN in AWS", aws.with(func(a *harborInstanceArgs) {
			a.secretName = "arn:aws:secretsmanager:us-west-2:210987654321:secret:harbor-robot"
		}), []string{"Invalid robot account secret name", invalidAwsName}},
		{"robot account's file as the name", aws.with(func(a *harborInstanceArgs) {
			a.secretName = `{"name":"robot$p0-connector","secret":"not-a-real-secret"}`
		}), []string{invalidAwsName}},
		{"secret resource name in Google Cloud", base.with(func(a *harborInstanceArgs) { a.secretName = "projects/acme-secrets/secrets/harbor-robot" }), []string{invalidGcpName}},
		{"secret name over 255 characters in Google Cloud", base.with(func(a *harborInstanceArgs) { a.secretName = strings.Repeat("s", 256) }), []string{invalidGcpName}},
		{"secret name with whitespace around it", base.with(func(a *harborInstanceArgs) { a.secretName = " " + harborSecret }), []string{"P0 removes the whitespace around 'secret_name'"}},
		{"AWS vault with no account", base.with(vault(`    type = "aws-sm"
    secrets_region = "us-west-2"`)), []string{"Missing vault field", `'vault.account_id' is required when 'vault.type' is "aws-sm"`}},
		{"AWS vault with no region", base.with(vault(`    type = "aws-sm"
    account_id = "210987654321"`)), []string{`'vault.secrets_region' is required when 'vault.type' is "aws-sm"`}},
		{"AWS vault with a project", aws.with(vault(`    type = "aws-sm"
    account_id = "210987654321"
    secrets_region = "us-west-2"
    project_id = "acme-secrets"`)), []string{"Unexpected vault field", `'vault.project_id' may only be set when 'vault.type' is "gcp-sm"`}},
		{"Google Cloud vault with no project", base.with(vault(`    type = "gcp-sm"`)), []string{`'vault.project_id' is required when 'vault.type' is "gcp-sm"`}},
		{"Google Cloud vault with an account", base.with(vault(`    type = "gcp-sm"
    project_id = "acme-secrets"
    account_id = "210987654321"`)), []string{`'vault.account_id' may only be set when 'vault.type' is "aws-sm"`}},
		{"Google Cloud vault with a region", base.with(vault(`    type = "gcp-sm"
    project_id = "acme-secrets"
    secrets_region = "us-west-2"`)), []string{`'vault.secrets_region' may only be set when 'vault.type' is "aws-sm"`}},
		{"vault of another kind", base.with(vault(`    type = "azure-kv"`)), []string{`value must be one of: ["aws-sm" "gcp-sm"]`}},
		{"account that isn't one", aws.with(func(a *harborInstanceArgs) { a.vault = harborAwsVault("1234", "us-west-2") }), []string{"12 numeric digits"}},
		{"region that isn't one", aws.with(func(a *harborInstanceArgs) { a.vault = harborAwsVault(harborSecretsAccount, "us west 2") }), []string{"Must be a valid AWS region"}},
		{"region in GovCloud", aws.with(func(a *harborInstanceArgs) { a.vault = harborAwsVault(harborSecretsAccount, "us-gov-west-1") }), []string{"Unsupported AWS region for the robot account's secret", "Harbor doesn't support AWS GovCloud or China regions yet."}},
		{"region in China", aws.with(func(a *harborInstanceArgs) { a.vault = harborAwsVault(harborSecretsAccount, "cn-north-1") }), []string{"Harbor doesn't support AWS GovCloud or China regions yet."}},
		{"project with an underscore", base.with(func(a *harborInstanceArgs) { a.vault = harborGcpVault("acme_secrets") }), []string{"Google Cloud project identifiers must consist of alphanumeric characters and hyphens only"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newHarborFake(t)
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{
						Config:      providerConfig(f.fakeP0) + tc.args.hcl(),
						PlanOnly:    true,
						ExpectError: harborError(tc.errors...),
					},
				},
			})
		})
	}
}
