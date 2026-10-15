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
	// Optional: left out of the configuration when empty.
	adminSecretName string
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
	admin := ""
	if a.adminSecretName != "" {
		admin = fmt.Sprintf("\n  admin_secret_name = %q", a.adminSecretName)
	}
	return fmt.Sprintf(`
resource "p0_harbor_instance" "example" {
  id          = %q
  connector   = %s
  url         = %q
  vault       = %s
  secret_name = %q%s
}
`, a.id, a.connector, a.url, a.vault, a.secretName, admin)
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

// An instance that fails P0's install check, on verify, fails the apply with P0's
// message and the way out, and stays staged: 422 for a failed check, 502 when the
// check couldn't reach Harbor. Its resource is tainted, so the next apply replaces it,
// and can give it another URL: Terraform plans a tainted resource as a new one.
func TestHarborInstanceFailedCheck(t *testing.T) {
	for _, tc := range []struct {
		name, status string
		unreachable  bool
	}{
		{"failed check", "422 Unprocessable Entity", false},
		{"Harbor unreachable", "502 Bad Gateway", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newHarborFake(t)
			const message = "The connector couldn't find its robot account's secret in Google Secret Manager."
			f.failedInstanceCheck, f.instanceCheckUnreachable = message, tc.unreachable
			fixed := harborGcpInstance().with(func(a *harborInstanceArgs) { a.url = "https://registry.example.com" })

			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				CheckDestroy:             checkHarborInstanceDestroyed(f, harborInstance),
				Steps: []resource.TestStep{
					{
						Config: harborInstanceConfig(f, harborGcp, harborGcpInstance()),
						ExpectError: harborError(
							"P0 didn't finish installing the Harbor instance", `Harbor instance "prod"`, tc.status+": "+message,
							"terraform apply -replace=<address>", "terraform state rm <address>",
						),
					},
					{
						PreConfig: func() {
							if err := f.checkInstance("stage", harborGcpInstanceItem); err != nil {
								t.Errorf("after the failed step: %v", err)
							}
							f.failedInstanceCheck = ""
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
						"400 Bad Request: The instance's connector runs on Google Cloud, so the robot account's secret has to be in Google Secret Manager.",
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
					ExpectError: harborError("Could not stage harbor-instance component", "400 Bad Request: That Harbor connector isn't installed in P0."),
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
					ExpectError: harborError("Could not stage harbor-instance component", "400 Bad Request: That Harbor connector isn't installed in P0.", "Install it, and finish its setup"),
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

// On AWS, P0 reads a secret in another account than the connector's by its partial
// ARN, so a name ending like an ARN's suffix is refused there, at plan time once the
// connector is in P0. In the connector's own account the name is taken. In an apply
// that creates both, Terraform plans the instance again once the connector exists,
// and the check refuses it then, before anything is staged.
func TestHarborInstanceSecretNameAcrossAccounts(t *testing.T) {
	const suffixed = "harbor-robot-AbC123"
	const refusal = "Secrets Manager can't find a secret in another account by its name when the name ends in a hyphen and six characters"
	other := harborAwsInstance().with(func(a *harborInstanceArgs) { a.secretName = suffixed })
	own := other.with(func(a *harborInstanceArgs) { a.vault = harborAwsVault("123456789012", "us-west-2") })

	t.Run("plan", func(t *testing.T) {
		f := newHarborFake(t)
		resource.UnitTest(t, resource.TestCase{
			ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
			CheckDestroy:             checkHarborInstanceDestroyed(f, harborInstance),
			Steps: []resource.TestStep{
				{Config: harborConnectorConfig(f, harborAws)},
				{
					Config:      harborInstanceConfig(f, harborAws, other),
					PlanOnly:    true,
					ExpectError: harborError("Invalid robot account secret name", refusal),
				},
				{
					Config: harborInstanceConfig(f, harborAws, own),
					Check:  resource.TestCheckResourceAttr(harborInstanceAddress, "state", "installed"),
				},
			},
		})
	})

	t.Run("apply", func(t *testing.T) {
		f := newHarborFake(t)
		resource.UnitTest(t, resource.TestCase{
			ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
			CheckDestroy:             checkHarborInstanceDestroyed(f, harborInstance),
			Steps: []resource.TestStep{
				{
					Config:      harborInstanceConfig(f, harborAws, other),
					ExpectError: harborError("Invalid robot account secret name", refusal),
				},
				{
					PreConfig: func() {
						if steps := f.steps(); len(steps) != 0 {
							t.Errorf("P0 got the instance's steps %v; want none", steps)
						}
					},
					Config: harborConnectorConfig(f, harborAws),
				},
			},
		})
	})
}

// While an instance uses the connector, P0 won't return the connector to "stage". The
// examples' instance refers to the connector, so Terraform removes the instance first
// and the rollback goes through. An instance that names the connector by a literal ID
// doesn't order them, and destroying the connector alone fails and says why.
func TestHarborConnectorWithInstance(t *testing.T) {
	t.Run("removed together", func(t *testing.T) {
		f := newHarborFake(t)
		resource.UnitTest(t, resource.TestCase{
			ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
			CheckDestroy:             checkHarborInstanceDestroyed(f, harborInstance),
			Steps: []resource.TestStep{
				{Config: harborInstanceConfig(f, harborGcp, harborGcpInstance())},
				{
					Config: harborStagedConfig(f, harborGcp),
					Check: func(*terraform.State) error {
						if item := f.connector(); item["state"] != "stage" {
							return fmt.Errorf("P0 item = %v; want the connector staged", item)
						}
						return nil
					},
				},
			},
		})
	})

	t.Run("connector removed alone", func(t *testing.T) {
		f := newHarborFake(t)
		literal := harborGcpInstance().with(func(a *harborInstanceArgs) { a.connector = fmt.Sprintf("%q", harborConnector) })
		resource.UnitTest(t, resource.TestCase{
			ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
			CheckDestroy:             checkHarborInstanceDestroyed(f, harborInstance),
			Steps: []resource.TestStep{
				{Config: harborConnectorConfig(f, harborGcp)},
				{Config: harborConnectorConfig(f, harborGcp) + literal.hcl()},
				{
					Config: harborStagedConfig(f, harborGcp) + literal.hcl(),
					ExpectError: harborError(
						"Harbor connector is in use", "Remove each p0_harbor_instance that uses it first",
						"409 Conflict: The Harbor connector prod can't be staged again while a Harbor instance uses it (prod).",
					),
				},
				{
					PreConfig: func() {
						if item := f.connector(); item["state"] != "installed" {
							t.Errorf("P0 item = %v; want the connector still installed", item)
						}
					},
					Config: harborConnectorConfig(f, harborGcp),
				},
			},
		})
	})
}

// An instance with an admin credential's secret, for the system administrator access.
// P0 stores the name beside the robot's secret, fixes it at creation, and holds it to
// the same rules, the cross-account suffix included.
func TestHarborInstanceAdminSecret(t *testing.T) {
	const admin = "harbor-prod-admin"
	withAdmin := harborGcpInstance().with(func(a *harborInstanceArgs) { a.adminSecretName = admin })
	stored := harborItemFields(harborGcpInstanceItem)
	stored["adminSecretName"] = admin

	t.Run("set, changed and removed", func(t *testing.T) {
		f := newHarborFake(t)
		resource.UnitTest(t, resource.TestCase{
			ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
			CheckDestroy:             checkHarborInstanceDestroyed(f, harborInstance),
			Steps: []resource.TestStep{
				{
					Config: harborInstanceConfig(f, harborGcp, withAdmin),
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttr(harborInstanceAddress, "admin_secret_name", admin),
						func(*terraform.State) error { return f.checkInstance("installed", stored) },
					),
				},
				{
					ResourceName:      harborInstanceAddress,
					ImportState:       true,
					ImportStateId:     harborInstance,
					ImportStateVerify: true,
				},
				{
					Config: harborInstanceConfig(f, harborGcp, withAdmin.with(func(a *harborInstanceArgs) { a.adminSecretName = "harbor-prod-admin-2" })),
					ConfigPlanChecks: resource.ConfigPlanChecks{
						PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(harborInstanceAddress, plancheck.ResourceActionReplace)},
					},
					Check: func(*terraform.State) error {
						if item := f.instance(harborInstance); item["adminSecretName"] != "harbor-prod-admin-2" {
							return fmt.Errorf("P0 item = %v; want the new admin secret", item)
						}
						return nil
					},
				},
				// Unset, the name is left out of every request, and P0 stores none.
				{
					Config: harborInstanceConfig(f, harborGcp, harborGcpInstance()),
					ConfigPlanChecks: resource.ConfigPlanChecks{
						PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(harborInstanceAddress, plancheck.ResourceActionReplace)},
					},
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckNoResourceAttr(harborInstanceAddress, "admin_secret_name"),
						func(*terraform.State) error { return f.checkInstance("installed", harborGcpInstanceItem) },
					),
				},
			},
		})
	})

	// A blank name in P0 is none, so it reads as unset and plans nothing.
	t.Run("blank in P0", func(t *testing.T) {
		f := newHarborFake(t)
		config := harborInstanceConfig(f, harborGcp, harborGcpInstance())
		resource.UnitTest(t, resource.TestCase{
			ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
			CheckDestroy:             checkHarborInstanceDestroyed(f, harborInstance),
			Steps: []resource.TestStep{
				{Config: config},
				{
					PreConfig: func() {
						f.update("harbor", "harbor-instance", harborInstance, func(item map[string]any) { item["adminSecretName"] = " " })
					},
					Config:           config,
					ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
				},
			},
		})
	})

	t.Run("cross-account suffix", func(t *testing.T) {
		f := newHarborFake(t)
		suffixed := harborAwsInstance().with(func(a *harborInstanceArgs) { a.adminSecretName = "harbor-admin-AbC123" })
		resource.UnitTest(t, resource.TestCase{
			ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
			CheckDestroy:             checkHarborInstanceDestroyed(f, harborInstance),
			Steps: []resource.TestStep{
				{Config: harborConnectorConfig(f, harborAws)},
				{
					Config:   harborInstanceConfig(f, harborAws, suffixed),
					PlanOnly: true,
					ExpectError: harborError("Invalid Harbor admin credential secret name",
						"Secrets Manager can't find a secret in another account by its name when the name ends in a hyphen and six characters"),
				},
				{
					Config: harborInstanceConfig(f, harborAws, suffixed.with(func(a *harborInstanceArgs) { a.vault = harborAwsVault("123456789012", "us-west-2") })),
					Check:  resource.TestCheckResourceAttr(harborInstanceAddress, "admin_secret_name", "harbor-admin-AbC123"),
				},
			},
		})
	})
}

// The provider never sends these, so the fake's answers to them are exercised
// directly: a PUT over an instance P0 holds, changed or unchanged, before and after
// it is installed, and a verify that changes a field.
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
		{"an identical PUT at configure", http.MethodPut, item, harborUrl, http.StatusOK},
		{"a verify after it", http.MethodPost, item + "/verify", harborUrl, http.StatusOK},
		{"a configure", http.MethodPost, item + "/configure", harborUrl, http.StatusOK},
		{"an identical PUT once installed", http.MethodPut, item, harborUrl, http.StatusConflict},
	} {
		if got := send(tc.method, tc.path, tc.url); got != tc.want {
			t.Errorf("%s: status %d; want %d", tc.name, got, tc.want)
		}
	}
	if stored := f.instance(harborInstance); stored["url"] != harborUrl {
		t.Errorf("P0 item = %v; want the first URL kept", stored)
	}
}

// The fake holds the admin credential's secret as app#7493 does: a PUT over an
// instance has to agree on it, leaving it out agrees only with an instance that has
// none, and a verify can't change it.
func TestHarborFakeInstanceAdminSecret(t *testing.T) {
	f := newHarborFake(t)
	f.integrations["harbor"] = true
	f.items[fakeItemKey{"harbor", "connector", harborConnector}] = map[string]any{
		"hosting": map[string]any{"type": "gcp"}, "state": "installed",
	}
	item := "/o/" + fakeOrg + "/integrations/harbor/config/harbor-instance/" + harborInstance
	send := func(method, path, admin string) int {
		t.Helper()
		adminField := ""
		if admin != "-" {
			adminField = fmt.Sprintf(`,"adminSecretName":%q`, admin)
		}
		body := fmt.Sprintf(`{"connector":%q,"url":%q,"secretName":%q,"vault":{"type":"gcp-sm","install":%q}%s}`,
			harborConnector, harborUrl, harborSecret, harborSecretsProject, adminField)
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
		name, method, path, admin string
		want                      int
	}{
		{"the first PUT", http.MethodPut, item, "harbor-admin", http.StatusOK},
		{"the same PUT", http.MethodPut, item, "harbor-admin", http.StatusOK},
		{"a PUT without it", http.MethodPut, item, "-", http.StatusConflict},
		{"a PUT with a blank one", http.MethodPut, item, " ", http.StatusConflict},
		{"a PUT with another", http.MethodPut, item, "harbor-admin-2", http.StatusConflict},
		{"a verify with another", http.MethodPost, item + "/verify", "harbor-admin-2", http.StatusUnprocessableEntity},
		{"a verify without it", http.MethodPost, item + "/verify", "-", http.StatusOK},
	} {
		if got := send(tc.method, tc.path, tc.admin); got != tc.want {
			t.Errorf("%s: status %d; want %d", tc.name, got, tc.want)
		}
	}
	if stored := f.instance(harborInstance); stored["adminSecretName"] != "harbor-admin" {
		t.Errorf("P0 item = %v; want the first admin secret kept", stored)
	}
}

// An instance that P0 no longer has is dropped from state, as other resources are, and
// the next apply creates it again.
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

	// P0 checks the instance's connector on configure too. The instance waits at
	// configure until the connector is back, or installed, and then finishes with
	// configure alone.
	for _, tc := range []struct {
		name, message string
		break_        func(item map[string]any) map[string]any
	}{
		{"connector not installed", "The Harbor connector prod, which the instance prod uses, isn't installed. Finish installing it, then retry.",
			func(item map[string]any) map[string]any { item["state"] = "configure"; return item }},
		{"connector gone", "The Harbor connector prod, which the instance prod uses, isn't in P0 any more.",
			func(map[string]any) map[string]any { return nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newHarborFake(t)
			config := providerConfig(f.fakeP0) + instance.hcl()
			key := fakeItemKey{"harbor", "connector", harborConnector}
			var installed map[string]any
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				CheckDestroy:             destroyed(f),
				Steps: []resource.TestStep{
					{
						PreConfig: func() {
							f.seedInstance("configure", harborUrl)
							f.mu.Lock()
							defer f.mu.Unlock()
							installed = harborItemFields(f.items[key])
							installed["label"], installed["state"] = harborConnector, "installed"
							if broken := tc.break_(f.items[key]); broken == nil {
								delete(f.items, key)
							}
						},
						Config:             config,
						ResourceName:       harborInstanceAddress,
						ImportState:        true,
						ImportStateId:      harborInstance,
						ImportStatePersist: true,
					},
					{
						Config: config,
						ExpectError: harborError(
							"P0 didn't finish installing the Harbor instance", "422 Unprocessable Entity: "+tc.message,
							"terraform apply -replace=<address>", "terraform state rm <address>",
						),
					},
					{
						PreConfig: func() {
							if err := f.checkInstance("configure", harborGcpInstanceItem); err != nil {
								t.Errorf("after the refused configure: %v", err)
							}
							f.mu.Lock()
							defer f.mu.Unlock()
							f.items[key], f.instanceSteps = installed, nil
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
	}

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
		{"admin secret ARN in AWS", aws.with(func(a *harborInstanceArgs) {
			a.adminSecretName = "arn:aws:secretsmanager:us-west-2:210987654321:secret:harbor-admin"
		}), []string{"Invalid Harbor admin credential secret name", "the secret that holds the Harbor admin credential"}},
		{"admin secret resource name in Google Cloud", base.with(func(a *harborInstanceArgs) { a.adminSecretName = "projects/acme-secrets/secrets/harbor-admin" }), []string{"Invalid Harbor admin credential secret name", invalidGcpName}},
		{"blank admin secret name", base.with(func(a *harborInstanceArgs) { a.adminSecretName = " " }), []string{"Invalid Harbor admin credential secret name"}},
		{"admin secret name with whitespace around it", base.with(func(a *harborInstanceArgs) { a.adminSecretName = "harbor-admin " }), []string{"P0 removes the whitespace around 'admin_secret_name'"}},
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
		{"account that isn't one", aws.with(func(a *harborInstanceArgs) { a.vault = harborAwsVault("1234", "us-west-2") }), []string{"Choose the AWS Secrets Manager installation, by its 12-digit AWS account ID"}},
		{"region that isn't one", aws.with(func(a *harborInstanceArgs) { a.vault = harborAwsVault(harborSecretsAccount, "us west 2") }), []string{"Enter the AWS region the secret is in, such as us-west-2."}},
		{"region without its last hyphen", aws.with(func(a *harborInstanceArgs) { a.vault = harborAwsVault(harborSecretsAccount, "us-west2") }), []string{"Enter the AWS region the secret is in, such as us-west-2."}},
		{"region in GovCloud", aws.with(func(a *harborInstanceArgs) { a.vault = harborAwsVault(harborSecretsAccount, "us-gov-west-1") }), []string{"Unsupported AWS region for the robot account's secret", "Harbor supports only AWS commercial regions for now."}},
		{"region in China", aws.with(func(a *harborInstanceArgs) { a.vault = harborAwsVault(harborSecretsAccount, "cn-north-1") }), []string{"Harbor supports only AWS commercial regions for now."}},
		{"project with an underscore", base.with(func(a *harborInstanceArgs) { a.vault = harborGcpVault("acme_secrets") }), []string{"Choose the Google Secret Manager installation, by its Google Cloud project ID"}},
		{"project with capitals", base.with(func(a *harborInstanceArgs) { a.vault = harborGcpVault("Acme-Secrets") }), []string{"Choose the Google Secret Manager installation"}},
		{"project too short", base.with(func(a *harborInstanceArgs) { a.vault = harborGcpVault("acme") }), []string{"Choose the Google Secret Manager installation"}},
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
