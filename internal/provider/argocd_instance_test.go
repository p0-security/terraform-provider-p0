package provider

import (
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// copy of internal/provider/harbor_instance_test.go, with the Argo CD instance's
// managing_app and namespace (INT-576).

const (
	argocdInstance        = "prod"
	argocdUrl             = "https://argocd.example.com"
	argocdManagingApp     = "infra/argocd"
	argocdSecret          = "argocd-prod-token"
	argocdSecretsProject  = "acme-secrets"
	argocdSecretsAccount  = "210987654321"
	argocdInstanceAddress = "p0_argocd_instance.example"
)

// An Argo CD instance as a customer's Terraform sets it. connector is an HCL expression,
// such as a reference to the connector's resource, and vault an HCL object.
// namespace is the attribute's HCL value: null to leave it out, or a quoted string.
type argocdInstanceArgs struct {
	id, connector, url, managingApp, namespace, vault, secretName string
}

func argocdGcpVault(project string) string {
	return fmt.Sprintf(`{
    type       = "gcp-sm"
    project_id = %q
  }`, project)
}

func argocdAwsVault(account, region string) string {
	return fmt.Sprintf(`{
    type           = "aws-sm"
    account_id     = %q
    secrets_region = %q
  }`, account, region)
}

// An instance on Google Cloud, reached through the connector of argocdConnectorConfig.
func argocdGcpInstance() argocdInstanceArgs {
	return argocdInstanceArgs{
		id:          argocdInstance,
		connector:   "p0_argocd_connector.example.id",
		url:         argocdUrl,
		managingApp: argocdManagingApp,
		namespace:   "null",
		vault:       argocdGcpVault(argocdSecretsProject),
		secretName:  argocdSecret,
	}
}

func argocdAwsInstance() argocdInstanceArgs {
	args := argocdGcpInstance()
	args.vault = argocdAwsVault(argocdSecretsAccount, "us-west-2")
	args.secretName = "argocd/prod/token"
	return args
}

// The instance with one change.
func (a argocdInstanceArgs) with(change func(*argocdInstanceArgs)) argocdInstanceArgs {
	change(&a)
	return a
}

func (a argocdInstanceArgs) hcl() string {
	return fmt.Sprintf(`
resource "p0_argocd_instance" "example" {
  id           = %q
  connector    = %s
  url          = %q
  managing_app = %q
  namespace    = %s
  vault        = %s
  secret_name  = %q
}
`, a.id, a.connector, a.url, a.managingApp, a.namespace, a.vault, a.secretName)
}

// The connector installed at hosting, and the instance it reaches.
func argocdInstanceConfig(f *argocdFake, hosting string, instance argocdInstanceArgs) string {
	return argocdConnectorConfig(f, hosting) + instance.hcl()
}

func (f *argocdFake) instance(id string) map[string]any {
	return f.item("argocd", "argocd-instance", id)
}

// The instance's item in P0, if it is at state and holds exactly the fields want.
func (f *argocdFake) checkInstance(state string, want map[string]any) error {
	item := f.instance(argocdInstance)
	fields := argocdItemFields(item)
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
func argocdItemFields(item map[string]any) map[string]any {
	fields := map[string]any{}
	for field, value := range item {
		if field != "label" && field != "state" {
			fields[field] = value
		}
	}
	return fields
}

func (f *argocdFake) steps() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.instanceSteps)
}

func checkArgocdInstanceDestroyed(f *argocdFake, ids ...string) func(*terraform.State) error {
	return func(state *terraform.State) error {
		for _, id := range ids {
			if item := f.instance(id); item != nil {
				return fmt.Errorf("the instance %s was not removed from P0: %v", id, item)
			}
		}
		return checkArgocdDestroyed(f)(state)
	}
}

var argocdGcpInstanceItem = map[string]any{
	"connector":   argocdConnector,
	"url":         argocdUrl,
	"managingApp": argocdManagingApp,
	"vault":       map[string]any{"type": "gcp-sm", "install": argocdSecretsProject},
	"secretName":  argocdSecret,
}

// The instance behind a connector on Google Cloud, with its secret in Google Secret
// Manager. Creating it stages, verifies and configures the item.
func TestArgocdInstanceOnGoogleCloud(t *testing.T) {
	f := newArgocdFake(t)
	config := argocdInstanceConfig(f, argocdGcp, argocdGcpInstance())

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkArgocdInstanceDestroyed(f, argocdInstance),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(argocdInstanceAddress, "state", "installed"),
					resource.TestCheckResourceAttr(argocdInstanceAddress, "connector", argocdConnector),
					resource.TestCheckResourceAttr(argocdInstanceAddress, "vault.type", "gcp-sm"),
					resource.TestCheckResourceAttr(argocdInstanceAddress, "vault.project_id", argocdSecretsProject),
					resource.TestCheckNoResourceAttr(argocdInstanceAddress, "vault.account_id"),
					resource.TestCheckNoResourceAttr(argocdInstanceAddress, "vault.secrets_region"),
					func(*terraform.State) error {
						if steps := f.steps(); !slices.Equal(steps, []string{"stage", "configure", "installed"}) {
							return fmt.Errorf("P0 got the instance's steps %v; want stage, verify, configure", steps)
						}
						return f.checkInstance("installed", argocdGcpInstanceItem)
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
				ResourceName:      argocdInstanceAddress,
				ImportState:       true,
				ImportStateId:     argocdInstance,
				ImportStateVerify: true,
			},
			// The ID goes into a URL path as it is, so one that P0 would refuse isn't read.
			{
				ResourceName:  argocdInstanceAddress,
				ImportState:   true,
				ImportStateId: "prod/other",
				ExpectError:   argocdError("Invalid Argo CD instance ID", "Use lowercase letters, digits and hyphens", `Got: "prod/other"`),
			},
		},
	})
}

// The instance behind a connector on AWS, with its secret in AWS Secrets Manager.
func TestArgocdInstanceOnAws(t *testing.T) {
	f := newArgocdFake(t)
	config := argocdInstanceConfig(f, argocdAws, argocdAwsInstance())

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkArgocdInstanceDestroyed(f, argocdInstance),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(argocdInstanceAddress, "state", "installed"),
					resource.TestCheckResourceAttr(argocdInstanceAddress, "vault.account_id", argocdSecretsAccount),
					resource.TestCheckResourceAttr(argocdInstanceAddress, "vault.secrets_region", "us-west-2"),
					resource.TestCheckNoResourceAttr(argocdInstanceAddress, "vault.project_id"),
					func(*terraform.State) error {
						return f.checkInstance("installed", map[string]any{
							"connector":   argocdConnector,
							"url":         argocdUrl,
							"managingApp": argocdManagingApp,
							"vault":       map[string]any{"type": "aws-sm", "install": argocdSecretsAccount, "secretsRegion": "us-west-2"},
							"secretName":  "argocd/prod/token",
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
				ResourceName:      argocdInstanceAddress,
				ImportState:       true,
				ImportStateId:     argocdInstance,
				ImportStateVerify: true,
			},
		},
	})
}

// A second connector, for an instance to move to.
const argocdSecondConnector = `
resource "p0_argocd_connector_staged" "second" {
  id = "prod-2"
  hosting = {
    type             = "gcp"
    project_id       = "acme-prod"
    connector_name   = "p0-argocd-prod-2"
    connector_region = "us-west1"
  }
}

resource "p0_argocd_connector" "second" {
  id = p0_argocd_connector_staged.second.id
  hosting = {
    type             = "gcp"
    project_id       = "acme-prod"
    connector_name   = "p0-argocd-prod-2"
    connector_region = "us-west1"
  }
}
`

// P0 fixes every field of an instance when it creates it. A new connector, vault or
// secret replaces the instance, and leaves its connector alone. A new URL is refused,
// unless the ID is new too.
func TestArgocdInstanceChangeReplaces(t *testing.T) {
	gcp, aws := argocdGcpInstance(), argocdAwsInstance()

	for _, tc := range []struct {
		name          string
		hosting       string
		before, after argocdInstanceArgs
		// The changed field, as P0 stores it.
		field string
		value any
	}{
		{"secret name", argocdGcp, gcp, gcp.with(func(a *argocdInstanceArgs) { a.secretName = "argocd-prod-token-2" }), "secretName", "argocd-prod-token-2"},
		{"connector", argocdGcp, gcp, gcp.with(func(a *argocdInstanceArgs) { a.connector = "p0_argocd_connector.second.id" }), "connector", "prod-2"},
		{
			"vault project", argocdGcp, gcp,
			gcp.with(func(a *argocdInstanceArgs) { a.vault = argocdGcpVault("acme-secrets-2") }),
			"vault", map[string]any{"type": "gcp-sm", "install": "acme-secrets-2"},
		},
		{
			"vault account", argocdAws, aws,
			aws.with(func(a *argocdInstanceArgs) { a.vault = argocdAwsVault("123456789012", "us-west-2") }),
			"vault", map[string]any{"type": "aws-sm", "install": "123456789012", "secretsRegion": "us-west-2"},
		},
		{
			"vault region", argocdAws, aws,
			aws.with(func(a *argocdInstanceArgs) { a.vault = argocdAwsVault(argocdSecretsAccount, "eu-west-1") }),
			"vault", map[string]any{"type": "aws-sm", "install": argocdSecretsAccount, "secretsRegion": "eu-west-1"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newArgocdFake(t)
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				CheckDestroy:             checkArgocdInstanceDestroyed(f, argocdInstance),
				Steps: []resource.TestStep{
					{Config: argocdInstanceConfig(f, tc.hosting, tc.before) + argocdSecondConnector},
					{
						Config: argocdInstanceConfig(f, tc.hosting, tc.after) + argocdSecondConnector,
						ConfigPlanChecks: resource.ConfigPlanChecks{
							PreApply: []plancheck.PlanCheck{
								plancheck.ExpectResourceAction(argocdInstanceAddress, plancheck.ResourceActionReplace),
								plancheck.ExpectResourceAction("p0_argocd_connector.example", plancheck.ResourceActionNoop),
								plancheck.ExpectResourceAction("p0_argocd_connector_staged.example", plancheck.ResourceActionNoop),
							},
						},
						Check: resource.ComposeAggregateTestCheckFunc(
							resource.TestCheckResourceAttr(argocdInstanceAddress, "state", "installed"),
							func(*terraform.State) error {
								item := f.instance(argocdInstance)
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

	// Replacing an installed instance under its ID would send its open access's
	// revokes to another argocd-rbac-cm, so the plan refuses a new URL, managing
	// Application or namespace, and nothing changes. A namespace set where it was
	// left out is a new one, even if it is Argo CD's default.
	for _, tc := range []struct {
		name, summary string
		before, after argocdInstanceArgs
		// The refusals the plan gives after the first, if any.
		also []string
	}{
		{"url", "An Argo CD instance's address can't change", gcp, gcp.with(func(a *argocdInstanceArgs) { a.url = "https://registry.example.com:8443" }), nil},
		{"managing app", "An Argo CD instance's managing application can't change", gcp, gcp.with(func(a *argocdInstanceArgs) { a.managingApp = "default/argocd" }), nil},
		{"namespace", "An Argo CD instance's namespace can't change", gcp, gcp.with(func(a *argocdInstanceArgs) { a.namespace = `"argocd"` }), nil},
		{"namespace left out", "An Argo CD instance's namespace can't change",
			gcp.with(func(a *argocdInstanceArgs) { a.namespace = `"gitops"` }), gcp, nil},
		// Each changed field is refused, not only the first.
		{"url and namespace", "An Argo CD instance's address can't change", gcp,
			gcp.with(func(a *argocdInstanceArgs) { a.url, a.namespace = "https://registry.example.com", `"gitops"` }),
			[]string{"An Argo CD instance's namespace can't change"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newArgocdFake(t)
			var installed map[string]any
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				CheckDestroy:             checkArgocdInstanceDestroyed(f, argocdInstance),
				Steps: []resource.TestStep{
					{
						Config: argocdInstanceConfig(f, argocdGcp, tc.before),
						Check: func(*terraform.State) error {
							installed = f.instance(argocdInstance)
							return nil
						},
					},
					{
						Config:   argocdInstanceConfig(f, argocdGcp, tc.after),
						PlanOnly: true,
						ExpectError: argocdError(append([]string{
							tc.summary,
							"Add a new p0_argocd_instance with its own id, and remove this one once its open access has ended.",
						}, tc.also...)...),
					},
					{
						Config:      argocdInstanceConfig(f, argocdGcp, tc.after),
						ExpectError: argocdError(tc.summary),
					},
					{
						PreConfig: func() {
							if item := f.instance(argocdInstance); fmt.Sprint(item) != fmt.Sprint(installed) {
								t.Errorf("P0 item after the refused change = %v; want it as it was, %v", item, installed)
							}
						},
						Config: argocdInstanceConfig(f, argocdGcp, tc.before),
						ConfigPlanChecks: resource.ConfigPlanChecks{
							PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
						},
					},
				},
			})
		})
	}

	// A new ID is a new item, and the old one goes. It can have a new URL: nothing
	// was granted under the new ID.
	t.Run("id", func(t *testing.T) {
		f := newArgocdFake(t)
		resource.UnitTest(t, resource.TestCase{
			ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
			CheckDestroy:             checkArgocdInstanceDestroyed(f, argocdInstance, "staging"),
			Steps: []resource.TestStep{
				{Config: argocdInstanceConfig(f, argocdGcp, gcp)},
				{
					Config: argocdInstanceConfig(f, argocdGcp, gcp.with(func(a *argocdInstanceArgs) {
						a.id, a.url = "staging", "https://staging.example.com"
					})),
					ConfigPlanChecks: resource.ConfigPlanChecks{
						PreApply: []plancheck.PlanCheck{
							plancheck.ExpectResourceAction(argocdInstanceAddress, plancheck.ResourceActionReplace),
						},
					},
					Check: func(*terraform.State) error {
						if item := f.instance(argocdInstance); item != nil {
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
		f := newArgocdFake(t)
		resource.UnitTest(t, resource.TestCase{
			ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
			CheckDestroy:             checkArgocdInstanceDestroyed(f, argocdInstance),
			Steps: []resource.TestStep{
				{Config: argocdInstanceConfig(f, argocdGcp, gcp)},
				{
					Config:             argocdInstanceConfig(f, argocdGcp, gcp.with(func(a *argocdInstanceArgs) { a.vault = argocdAwsVault(argocdSecretsAccount, "us-west-2") })),
					PlanOnly:           true,
					ExpectNonEmptyPlan: true,
					ConfigPlanChecks: resource.ConfigPlanChecks{
						PostApplyPostRefresh: []plancheck.PlanCheck{
							plancheck.ExpectResourceAction(argocdInstanceAddress, plancheck.ResourceActionReplace),
						},
					},
				},
			},
		})
	})
}

// The URL, the managing Application, the namespace, the vault and the secret's name
// can come from variables, which are unknown while Terraform validates the
// configuration, so the checks skip them then, and known when it plans.
func TestArgocdInstanceFromVariables(t *testing.T) {
	f := newArgocdFake(t)
	config := argocdConnectorConfig(f, argocdGcp) + fmt.Sprintf(`
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

variable "managing_app" {
  type    = string
  default = %q
}

variable "namespace" {
  type    = string
  default = %q
}

resource "p0_argocd_instance" "example" {
  id           = %q
  connector    = p0_argocd_connector.example.id
  url          = var.url
  managing_app = var.managing_app
  namespace    = var.namespace
  vault        = var.vault
  secret_name  = var.secret_name
}
`, argocdUrl, argocdGcpVault(argocdSecretsProject), argocdSecret, argocdManagingApp, "gitops", argocdInstance)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkArgocdInstanceDestroyed(f, argocdInstance),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: func(*terraform.State) error {
					want := maps.Clone(argocdGcpInstanceItem)
					want["namespace"] = "gitops"
					return f.checkInstance("installed", want)
				},
			},
			// P0 returns the namespace, so an import reads it back.
			{
				ResourceName:      argocdInstanceAddress,
				ImportState:       true,
				ImportStateId:     argocdInstance,
				ImportStateVerify: true,
			},
		},
	})
}

// An instance that fails P0's install check, on verify, fails the apply with P0's
// message and the way out, and stays staged: 422 for a failed check, 502 when the
// check couldn't reach Argo CD. Its resource is tainted, so the next apply replaces it,
// and can give it another URL: Terraform plans a tainted resource as a new one.
func TestArgocdInstanceFailedCheck(t *testing.T) {
	for _, tc := range []struct {
		name, status string
		unreachable  bool
	}{
		{"failed check", "422 Unprocessable Entity", false},
		{"Argo CD unreachable", "502 Bad Gateway", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newArgocdFake(t)
			const message = "The connector couldn't find its API token's secret in Google Secret Manager."
			f.failedInstanceCheck, f.instanceCheckUnreachable = message, tc.unreachable
			fixed := argocdGcpInstance().with(func(a *argocdInstanceArgs) { a.url = "https://registry.example.com" })

			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				CheckDestroy:             checkArgocdInstanceDestroyed(f, argocdInstance),
				Steps: []resource.TestStep{
					{
						Config: argocdInstanceConfig(f, argocdGcp, argocdGcpInstance()),
						ExpectError: argocdError(
							"P0 didn't finish installing the Argo CD instance", `Argo CD instance "prod"`, tc.status+": "+message,
							"terraform apply -replace=<address>", "terraform state rm <address>",
						),
					},
					{
						PreConfig: func() {
							if err := f.checkInstance("stage", argocdGcpInstanceItem); err != nil {
								t.Errorf("after the failed step: %v", err)
							}
							f.failedInstanceCheck = ""
						},
						Config: argocdInstanceConfig(f, argocdGcp, fixed),
						ConfigPlanChecks: resource.ConfigPlanChecks{
							PreApply: []plancheck.PlanCheck{
								plancheck.ExpectResourceAction(argocdInstanceAddress, plancheck.ResourceActionReplace),
								plancheck.ExpectResourceAction("p0_argocd_connector.example", plancheck.ResourceActionNoop),
							},
						},
						Check: func(*terraform.State) error {
							if item := f.instance(argocdInstance); item["url"] != "https://registry.example.com" || item["state"] != "installed" {
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
func TestArgocdInstanceRefusedByP0(t *testing.T) {
	// The configuration names the connector by its ID alone, so the plan can't check
	// that the vault is in the connector's cloud. P0 refuses to stage the instance.
	t.Run("vault in another cloud", func(t *testing.T) {
		f := newArgocdFake(t)
		resource.UnitTest(t, resource.TestCase{
			ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
			CheckDestroy:             checkArgocdInstanceDestroyed(f, argocdInstance),
			Steps: []resource.TestStep{
				{
					Config: argocdInstanceConfig(f, argocdGcp, argocdAwsInstance()),
					ExpectError: argocdError(
						"Could not stage argocd-instance component",
						"400 Bad Request: The instance's connector runs on Google Cloud, so the API token's secret has to be in Google Secret Manager.",
					),
				},
				{
					PreConfig: func() {
						if item := f.instance(argocdInstance); item != nil {
							t.Errorf("P0 stored the refused instance: %v", item)
						}
					},
					Config: argocdConnectorConfig(f, argocdGcp),
				},
			},
		})
	})

	t.Run("connector P0 doesn't have", func(t *testing.T) {
		f := newArgocdFake(t)
		instance := argocdGcpInstance().with(func(a *argocdInstanceArgs) { a.connector = `"missing"` })
		resource.UnitTest(t, resource.TestCase{
			ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
			CheckDestroy:             checkArgocdInstanceDestroyed(f, argocdInstance),
			Steps: []resource.TestStep{
				{
					Config:      argocdConnectorConfig(f, argocdGcp) + instance.hcl(),
					ExpectError: argocdError("Could not stage argocd-instance component", "400 Bad Request: That Argo CD connector isn't installed in P0."),
				},
			},
		})
	})

	// P0 checks the instance through its connector, so the connector must be installed
	// first. Referring to the connector's final resource orders them.
	t.Run("connector not installed", func(t *testing.T) {
		f := newArgocdFake(t)
		instance := argocdGcpInstance().with(func(a *argocdInstanceArgs) { a.connector = "p0_argocd_connector_staged.example.id" })
		resource.UnitTest(t, resource.TestCase{
			ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
			CheckDestroy:             checkArgocdInstanceDestroyed(f, argocdInstance),
			Steps: []resource.TestStep{
				{
					Config:      argocdStagedConfig(f, argocdGcp) + instance.hcl(),
					ExpectError: argocdError("Could not stage argocd-instance component", "400 Bad Request: That Argo CD connector isn't installed in P0.", "Install it, and finish its setup"),
				},
			},
		})
	})
}

// An instance that P0 already holds, such as one added in the P0 app, isn't created
// again, even when it matches the configuration: P0 would accept the PUT, which
// returns an installed instance to "stage". The apply fails, writes nothing, and says
// to import the instance.
func TestArgocdInstanceCreateOverExisting(t *testing.T) {
	for name, url := range map[string]string{"identical": argocdUrl, "different": "https://other.example.com"} {
		t.Run(name, func(t *testing.T) {
			f := newArgocdFake(t)
			key := fakeItemKey{"argocd", "argocd-instance", argocdInstance}
			existing := argocdItemFields(argocdGcpInstanceItem)
			existing["url"], existing["label"], existing["state"] = url, argocdInstance, "installed"

			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{Config: argocdConnectorConfig(f, argocdGcp)},
					{
						PreConfig: func() {
							f.mu.Lock()
							defer f.mu.Unlock()
							f.items[key] = existing
						},
						Config: argocdInstanceConfig(f, argocdGcp, argocdGcpInstance()),
						ExpectError: argocdError(
							"Argo CD instance already exists in P0", `P0 already has an Argo CD instance "prod"`,
							"terraform import <address> prod",
						),
					},
					{
						PreConfig: func() {
							if steps := f.steps(); len(steps) != 0 {
								t.Errorf("P0 got the instance's steps %v; want no write", steps)
							}
							if item := f.instance(argocdInstance); item["url"] != url || item["state"] != "installed" {
								t.Errorf("P0 item = %v; want the existing instance as it was", item)
							}
							f.mu.Lock()
							defer f.mu.Unlock()
							delete(f.items, key)
						},
						Config: argocdConnectorConfig(f, argocdGcp),
					},
				},
			})
		})
	}
}

// If P0 can't say whether the instance exists, as when the read fails with a 500, the
// create fails with P0's error and writes nothing: it never stages on a guess.
func TestArgocdInstanceCreateReadFails(t *testing.T) {
	f := newArgocdFake(t)
	fake := f.server.Config.Handler
	path := "/o/" + fakeOrg + "/integrations/argocd/config/argocd-instance/" + argocdInstance

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkArgocdInstanceDestroyed(f, argocdInstance),
		Steps: []resource.TestStep{
			{Config: argocdConnectorConfig(f, argocdGcp)},
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
				Config: argocdInstanceConfig(f, argocdGcp, argocdGcpInstance()),
				ExpectError: argocdError(
					"Error communicating with P0", `Could not check whether the Argo CD instance "prod" exists`,
					"500 Internal Server Error: P0 is having a bad day",
				),
			},
			{
				PreConfig: func() {
					f.server.Config.Handler = fake
					if steps := f.steps(); len(steps) != 0 {
						t.Errorf("P0 got the instance's steps %v; want no write", steps)
					}
					if item := f.instance(argocdInstance); item != nil {
						t.Errorf("P0 item = %v; want none", item)
					}
				},
				Config: argocdConnectorConfig(f, argocdGcp),
			},
		},
	})
}

// On AWS, P0 reads a secret in another account than the connector's by its partial
// ARN, so a name ending like an ARN's suffix is refused there, at plan time once the
// connector is in P0. In the connector's own account the name is taken. In an apply
// that creates both, Terraform plans the instance again once the connector exists,
// and the check refuses it then, before anything is staged.
func TestArgocdInstanceSecretNameAcrossAccounts(t *testing.T) {
	const suffixed = "argocd-token-AbC123"
	const refusal = "Secrets Manager can't find a secret in another account by its name when the name ends in a hyphen and six characters"
	other := argocdAwsInstance().with(func(a *argocdInstanceArgs) { a.secretName = suffixed })
	own := other.with(func(a *argocdInstanceArgs) { a.vault = argocdAwsVault("123456789012", "us-west-2") })

	t.Run("plan", func(t *testing.T) {
		f := newArgocdFake(t)
		resource.UnitTest(t, resource.TestCase{
			ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
			CheckDestroy:             checkArgocdInstanceDestroyed(f, argocdInstance),
			Steps: []resource.TestStep{
				{Config: argocdConnectorConfig(f, argocdAws)},
				{
					Config:      argocdInstanceConfig(f, argocdAws, other),
					PlanOnly:    true,
					ExpectError: argocdError("Invalid API token secret name", refusal),
				},
				{
					Config: argocdInstanceConfig(f, argocdAws, own),
					Check:  resource.TestCheckResourceAttr(argocdInstanceAddress, "state", "installed"),
				},
			},
		})
	})

	t.Run("apply", func(t *testing.T) {
		f := newArgocdFake(t)
		resource.UnitTest(t, resource.TestCase{
			ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
			CheckDestroy:             checkArgocdInstanceDestroyed(f, argocdInstance),
			Steps: []resource.TestStep{
				{
					Config:      argocdInstanceConfig(f, argocdAws, other),
					ExpectError: argocdError("Invalid API token secret name", refusal),
				},
				{
					PreConfig: func() {
						if steps := f.steps(); len(steps) != 0 {
							t.Errorf("P0 got the instance's steps %v; want none", steps)
						}
					},
					Config: argocdConnectorConfig(f, argocdAws),
				},
			},
		})
	})
}

// While an instance uses the connector, P0 won't return the connector to "stage". The
// examples' instance refers to the connector, so Terraform removes the instance first
// and the rollback goes through. An instance that names the connector by a literal ID
// doesn't order them, and destroying the connector alone fails and says why.
func TestArgocdConnectorWithInstance(t *testing.T) {
	t.Run("removed together", func(t *testing.T) {
		f := newArgocdFake(t)
		resource.UnitTest(t, resource.TestCase{
			ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
			CheckDestroy:             checkArgocdInstanceDestroyed(f, argocdInstance),
			Steps: []resource.TestStep{
				{Config: argocdInstanceConfig(f, argocdGcp, argocdGcpInstance())},
				{
					Config: argocdStagedConfig(f, argocdGcp),
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
		f := newArgocdFake(t)
		literal := argocdGcpInstance().with(func(a *argocdInstanceArgs) { a.connector = fmt.Sprintf("%q", argocdConnector) })
		resource.UnitTest(t, resource.TestCase{
			ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
			CheckDestroy:             checkArgocdInstanceDestroyed(f, argocdInstance),
			Steps: []resource.TestStep{
				{Config: argocdConnectorConfig(f, argocdGcp)},
				{Config: argocdConnectorConfig(f, argocdGcp) + literal.hcl()},
				{
					Config: argocdStagedConfig(f, argocdGcp) + literal.hcl(),
					ExpectError: argocdError(
						"Argo CD connector is in use", "Remove each p0_argocd_instance that uses it first",
						"409 Conflict: The Argo CD connector prod can't be staged again while an Argo CD instance uses it (prod).",
					),
				},
				{
					PreConfig: func() {
						if item := f.connector(); item["state"] != "installed" {
							t.Errorf("P0 item = %v; want the connector still installed", item)
						}
					},
					Config: argocdConnectorConfig(f, argocdGcp),
				},
			},
		})
	})
}

// The provider never sends these, so the fake's answers to them are exercised
// directly: a PUT over an instance P0 holds, changed or unchanged, before and after
// it is installed, and a verify that changes a field.
func TestArgocdFakeInstanceRefusals(t *testing.T) {
	f := newArgocdFake(t)
	f.integrations["argocd"] = true
	f.items[fakeItemKey{"argocd", "connector", argocdConnector}] = map[string]any{
		"hosting": map[string]any{"type": "gcp"}, "state": "installed",
	}
	item := "/o/" + fakeOrg + "/integrations/argocd/config/argocd-instance/" + argocdInstance
	send := func(method, path, url string) int {
		t.Helper()
		body := fmt.Sprintf(`{"connector":%q,"url":%q,"secretName":%q,"vault":{"type":"gcp-sm","install":%q}}`, argocdConnector, url, argocdSecret, argocdSecretsProject)
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
		{"the first PUT", http.MethodPut, item, argocdUrl, http.StatusOK},
		{"an identical PUT", http.MethodPut, item, argocdUrl, http.StatusOK},
		{"a PUT that changes the URL", http.MethodPut, item, "https://other.example.com", http.StatusConflict},
		{"a verify that changes the URL", http.MethodPost, item + "/verify", "https://other.example.com", http.StatusUnprocessableEntity},
		{"a verify that changes nothing", http.MethodPost, item + "/verify", argocdUrl, http.StatusOK},
		{"an identical PUT at configure", http.MethodPut, item, argocdUrl, http.StatusOK},
		{"a verify after it", http.MethodPost, item + "/verify", argocdUrl, http.StatusOK},
		{"a configure", http.MethodPost, item + "/configure", argocdUrl, http.StatusOK},
		{"an identical PUT once installed", http.MethodPut, item, argocdUrl, http.StatusConflict},
	} {
		if got := send(tc.method, tc.path, tc.url); got != tc.want {
			t.Errorf("%s: status %d; want %d", tc.name, got, tc.want)
		}
	}
	if stored := f.instance(argocdInstance); stored["url"] != argocdUrl {
		t.Errorf("P0 item = %v; want the first URL kept", stored)
	}
}

// An instance that P0 no longer has is dropped from state, as other resources are, and
// the next apply creates it again.
func TestArgocdInstanceGoneFromP0(t *testing.T) {
	key := fakeItemKey{"argocd", "argocd-instance", argocdInstance}

	t.Run("deleted", func(t *testing.T) {
		f := newArgocdFake(t)
		config := argocdInstanceConfig(f, argocdGcp, argocdGcpInstance())
		resource.UnitTest(t, resource.TestCase{
			ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
			CheckDestroy:             checkArgocdInstanceDestroyed(f, argocdInstance),
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
							plancheck.ExpectResourceAction(argocdInstanceAddress, plancheck.ResourceActionCreate),
						},
					},
					Check: func(*terraform.State) error {
						return f.checkInstance("installed", argocdGcpInstanceItem)
					},
				},
			},
		})
	})
}

// An instance that P0 holds but hasn't finished installing, as when it was staged in
// the P0 app, plans an update once imported, and applying it finishes the install:
// from "stage" with verify and then configure, from "configure" with configure alone.
func TestArgocdInstanceImportFinishesInstall(t *testing.T) {
	for _, state := range []string{"stage", "configure"} {
		t.Run(state, func(t *testing.T) {
			f := newArgocdFake(t)
			instance := argocdGcpInstance()
			instance.connector = fmt.Sprintf("%q", argocdConnector)
			config := providerConfig(f.fakeP0) + instance.hcl()
			want := map[string][]string{"stage": {"configure", "installed"}, "configure": {"installed"}}[state]

			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				CheckDestroy: func(*terraform.State) error {
					if item := f.instance(argocdInstance); item != nil {
						return fmt.Errorf("the instance was not removed from P0: %v", item)
					}
					return nil
				},
				Steps: []resource.TestStep{
					{
						PreConfig:          func() { f.seedInstance(state, argocdUrl) },
						Config:             config,
						ResourceName:       argocdInstanceAddress,
						ImportState:        true,
						ImportStateId:      argocdInstance,
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
								plancheck.ExpectResourceAction(argocdInstanceAddress, plancheck.ResourceActionUpdate),
							},
						},
						Check: resource.ComposeAggregateTestCheckFunc(
							resource.TestCheckResourceAttr(argocdInstanceAddress, "state", "installed"),
							func(*terraform.State) error {
								if steps := f.steps(); !slices.Equal(steps, want) {
									return fmt.Errorf("P0 got the instance's steps %v; want %v", steps, want)
								}
								return f.checkInstance("installed", argocdGcpInstanceItem)
							},
						),
					},
				},
			})
		})
	}
}

// Puts an installed connector and an instance at state in P0, as the P0 app would.
func (f *argocdFake) seedInstance(state, url string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.integrations["argocd"] = true
	f.items[fakeItemKey{"argocd", "connector", argocdConnector}] = map[string]any{
		"hosting": map[string]any{"type": "gcp", "projectId": argocdProject, "connectorName": argocdConnectorName, "connectorRegion": "us-west1"},
		"label":   argocdConnector,
		"state":   "installed",
	}
	item := argocdItemFields(argocdGcpInstanceItem)
	item["url"], item["label"], item["state"] = url, argocdInstance, state
	f.items[fakeItemKey{"argocd", "argocd-instance", argocdInstance}] = item
}

// An imported instance whose install P0 won't finish isn't tainted, so every apply
// tries again. The diagnostic says how to start over, and once the cause is gone the
// next apply finishes from where P0 left the item.
func TestArgocdInstanceImportedInstallFails(t *testing.T) {
	instance := argocdGcpInstance().with(func(a *argocdInstanceArgs) { a.connector = fmt.Sprintf("%q", argocdConnector) })
	destroyed := func(f *argocdFake) func(*terraform.State) error {
		return func(*terraform.State) error {
			if item := f.instance(argocdInstance); item != nil {
				return fmt.Errorf("the instance was not removed from P0: %v", item)
			}
			return nil
		}
	}
	importStaged := func(f *argocdFake, config, url string) resource.TestStep {
		return resource.TestStep{
			PreConfig:          func() { f.seedInstance("stage", url) },
			Config:             config,
			ResourceName:       argocdInstanceAddress,
			ImportState:        true,
			ImportStateId:      argocdInstance,
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
		{"connector not installed", "The Argo CD connector prod, which the instance prod uses, isn't installed. Finish installing it, then retry.",
			func(item map[string]any) map[string]any { item["state"] = "configure"; return item }},
		{"connector gone", "The Argo CD connector prod, which the instance prod uses, isn't in P0 any more.",
			func(map[string]any) map[string]any { return nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newArgocdFake(t)
			config := providerConfig(f.fakeP0) + instance.hcl()
			key := fakeItemKey{"argocd", "connector", argocdConnector}
			var installed map[string]any
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				CheckDestroy:             destroyed(f),
				Steps: []resource.TestStep{
					{
						PreConfig: func() {
							f.seedInstance("configure", argocdUrl)
							f.mu.Lock()
							defer f.mu.Unlock()
							installed = argocdItemFields(f.items[key])
							installed["label"], installed["state"] = argocdConnector, "installed"
							if broken := tc.break_(f.items[key]); broken == nil {
								delete(f.items, key)
							}
						},
						Config:             config,
						ResourceName:       argocdInstanceAddress,
						ImportState:        true,
						ImportStateId:      argocdInstance,
						ImportStatePersist: true,
					},
					{
						Config: config,
						ExpectError: argocdError(
							"P0 didn't finish installing the Argo CD instance", "422 Unprocessable Entity: "+tc.message,
							"terraform apply -replace=<address>", "terraform state rm <address>",
						),
					},
					{
						PreConfig: func() {
							if err := f.checkInstance("configure", argocdGcpInstanceItem); err != nil {
								t.Errorf("after the refused configure: %v", err)
							}
							f.mu.Lock()
							defer f.mu.Unlock()
							f.items[key], f.instanceSteps = installed, nil
						},
						Config: config,
						ConfigPlanChecks: resource.ConfigPlanChecks{
							PreApply: []plancheck.PlanCheck{
								plancheck.ExpectResourceAction(argocdInstanceAddress, plancheck.ResourceActionUpdate),
							},
						},
						Check: func(*terraform.State) error {
							if steps := f.steps(); !slices.Equal(steps, []string{"installed"}) {
								return fmt.Errorf("P0 got the instance's steps %v; want configure alone", steps)
							}
							return f.checkInstance("installed", argocdGcpInstanceItem)
						},
					},
				},
			})
		})
	}

	// The instance in P0 has another URL than the configuration. It was never
	// installed, so the plan lets the URL through, and P0 refuses to change it.
	t.Run("differs from the configuration", func(t *testing.T) {
		f := newArgocdFake(t)
		config := providerConfig(f.fakeP0) + instance.hcl()
		resource.UnitTest(t, resource.TestCase{
			ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
			CheckDestroy:             destroyed(f),
			Steps: []resource.TestStep{
				importStaged(f, config, "https://other.example.com"),
				{
					Config: config,
					ExpectError: argocdError(
						"Argo CD instance differs from the one in P0", "'url' can only be altered on initial installation",
						"terraform apply -replace=<address>",
					),
				},
			},
		})
	})
}

// An instance that P0 returns with no vault, or one this provider doesn't support,
// such as one changed in the P0 app, fails the refresh and says how to go on.
func TestArgocdInstanceUnreadableVault(t *testing.T) {
	f := newArgocdFake(t)
	config := argocdInstanceConfig(f, argocdGcp, argocdGcpInstance())
	setVault := func(vault any) func() {
		return func() {
			f.update("argocd", "argocd-instance", argocdInstance, func(item map[string]any) {
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
		CheckDestroy:             checkArgocdInstanceDestroyed(f, argocdInstance),
		Steps: []resource.TestStep{
			{Config: config},
			{
				PreConfig:   setVault(nil),
				Config:      config,
				PlanOnly:    true,
				ExpectError: argocdError("Bad API response", `P0 returned the Argo CD instance "prod" without its vault`, "terraform state rm <address>"),
			},
			{
				PreConfig:   setVault(map[string]any{"type": "azure-kv", "install": "acme"}),
				Config:      config,
				PlanOnly:    true,
				ExpectError: argocdError("Unsupported Argo CD instance vault", `The Argo CD instance prod has a "azure-kv" vault`, "terraform state rm"),
			},
			{
				PreConfig: setVault(map[string]any{"type": "gcp-sm", "install": argocdSecretsProject}),
				Config:    config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})
}

func TestArgocdInstanceValidation(t *testing.T) {
	const invalidId = "Use lowercase letters, digits and hyphens, starting and ending with a letter or digit, up to 63 characters"
	const invalidUrl = "Enter the instance's address as https://<host> or https://<host>:<port>"
	const invalidAwsName = "That doesn't look like a secret name."
	const invalidGcpName = "That isn't a secret's name in Google Secret Manager."
	const invalidApp = "Enter the Argo CD Application that manages argocd-rbac-cm as <project>/<application>"
	const invalidNamespace = "Enter the Kubernetes namespace Argo CD runs in"
	base := argocdGcpInstance()
	base.connector = fmt.Sprintf("%q", argocdConnector)
	aws := base
	aws.vault = argocdAwsVault(argocdSecretsAccount, "us-west-2")
	vault := func(fields string) func(*argocdInstanceArgs) {
		return func(a *argocdInstanceArgs) { a.vault = "{\n" + fields + "\n  }" }
	}

	for _, tc := range []struct {
		name   string
		args   argocdInstanceArgs
		errors []string
	}{
		{"id with capitals", base.with(func(a *argocdInstanceArgs) { a.id = "Prod" }), []string{invalidId}},
		{"id over 63 characters", base.with(func(a *argocdInstanceArgs) { a.id = strings.Repeat("a", 64) }), []string{"string length must be at most 63"}},
		{"connector with a path", base.with(func(a *argocdInstanceArgs) { a.connector = `"prod/other"` }), []string{"connector", invalidId}},
		{"URL with a trailing slash", base.with(func(a *argocdInstanceArgs) { a.url = argocdUrl + "/" }), []string{"Invalid Argo CD instance URL", invalidUrl}},
		{"URL with plain http", base.with(func(a *argocdInstanceArgs) { a.url = "http://argocd.example.com" }), []string{invalidUrl}},
		{"URL with the default port", base.with(func(a *argocdInstanceArgs) { a.url = argocdUrl + ":443" }), []string{invalidUrl}},
		{"URL with an uppercase host", base.with(func(a *argocdInstanceArgs) { a.url = "https://Argo CD.example.com" }), []string{invalidUrl}},
		{"URL with credentials", base.with(func(a *argocdInstanceArgs) { a.url = "https://admin:not-a-real-secret@argocd.example.com" }), []string{invalidUrl}},
		{"URL with a root path", base.with(func(a *argocdInstanceArgs) { a.url = argocdUrl + "/argocd" }), []string{invalidUrl, "Argo CD served under a root path isn't supported"}},
		{"URL with an empty label", base.with(func(a *argocdInstanceArgs) { a.url = "https://argocd..example.com" }), []string{invalidUrl}},
		{"URL with port 0", base.with(func(a *argocdInstanceArgs) { a.url = argocdUrl + ":0" }), []string{invalidUrl}},
		{"managing app without a project", base.with(func(a *argocdInstanceArgs) { a.managingApp = "argocd" }), []string{"Invalid Argo CD managing application", invalidApp}},
		{"managing app in another namespace", base.with(func(a *argocdInstanceArgs) { a.managingApp = "infra/argocd/argocd" }), []string{invalidApp}},
		{"managing app with capitals", base.with(func(a *argocdInstanceArgs) { a.managingApp = "Infra/ArgoCD" }), []string{invalidApp}},
		{"managing app with a comma", base.with(func(a *argocdInstanceArgs) { a.managingApp = "infra/argocd,admin" }), []string{invalidApp}},
		{"managing app with whitespace around it", base.with(func(a *argocdInstanceArgs) { a.managingApp = argocdManagingApp + "\n" }), []string{"P0 removes the whitespace around 'managing_app'"}},
		{"namespace with capitals", base.with(func(a *argocdInstanceArgs) { a.namespace = `"ArgoCD"` }), []string{"Invalid Argo CD namespace", invalidNamespace}},
		{"namespace with a dot", base.with(func(a *argocdInstanceArgs) { a.namespace = `"argo.cd"` }), []string{invalidNamespace}},
		{"namespace over 63 characters", base.with(func(a *argocdInstanceArgs) { a.namespace = fmt.Sprintf("%q", strings.Repeat("n", 64)) }), []string{invalidNamespace}},
		{"empty namespace", base.with(func(a *argocdInstanceArgs) { a.namespace = `""` }), []string{"Leave namespace out, rather than empty, for argocd"}},
		{"namespace with whitespace around it", base.with(func(a *argocdInstanceArgs) { a.namespace = `" argocd"` }), []string{"P0 removes the whitespace around 'namespace'"}},
		{"URL with an IPv4 address in three parts", base.with(func(a *argocdInstanceArgs) { a.url = "https://1.2.3" }), []string{invalidUrl}},
		// P0 would trim the URL and store it, and the apply would then fail on a value
		// that reads back different.
		{"URL with whitespace around it", base.with(func(a *argocdInstanceArgs) { a.url = argocdUrl + " " }), []string{"Whitespace around a value", "P0 removes the whitespace around 'url'"}},
		{"secret ARN in AWS", aws.with(func(a *argocdInstanceArgs) {
			a.secretName = "arn:aws:secretsmanager:us-west-2:210987654321:secret:argocd-token"
		}), []string{"Invalid API token secret name", invalidAwsName}},
		{"API token's JSON as the name", aws.with(func(a *argocdInstanceArgs) {
			a.secretName = `{"token":"not-a-real-token"}`
		}), []string{invalidAwsName}},
		{"secret resource name in Google Cloud", base.with(func(a *argocdInstanceArgs) { a.secretName = "projects/acme-secrets/secrets/argocd-token" }), []string{invalidGcpName}},
		{"secret name over 255 characters in Google Cloud", base.with(func(a *argocdInstanceArgs) { a.secretName = strings.Repeat("s", 256) }), []string{invalidGcpName}},
		{"secret name with whitespace around it", base.with(func(a *argocdInstanceArgs) { a.secretName = " " + argocdSecret }), []string{"P0 removes the whitespace around 'secret_name'"}},
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
		{"account that isn't one", aws.with(func(a *argocdInstanceArgs) { a.vault = argocdAwsVault("1234", "us-west-2") }), []string{"Choose the AWS Secrets Manager installation, by its 12-digit AWS account ID"}},
		{"region that isn't one", aws.with(func(a *argocdInstanceArgs) { a.vault = argocdAwsVault(argocdSecretsAccount, "us west 2") }), []string{"Enter the AWS region the secret is in, such as us-west-2."}},
		{"region without its last hyphen", aws.with(func(a *argocdInstanceArgs) { a.vault = argocdAwsVault(argocdSecretsAccount, "us-west2") }), []string{"Enter the AWS region the secret is in, such as us-west-2."}},
		{"region in GovCloud", aws.with(func(a *argocdInstanceArgs) { a.vault = argocdAwsVault(argocdSecretsAccount, "us-gov-west-1") }), []string{"Unsupported AWS region for the API token's secret", "Argo CD supports only AWS commercial regions for now."}},
		{"region in China", aws.with(func(a *argocdInstanceArgs) { a.vault = argocdAwsVault(argocdSecretsAccount, "cn-north-1") }), []string{"Argo CD supports only AWS commercial regions for now."}},
		{"project with an underscore", base.with(func(a *argocdInstanceArgs) { a.vault = argocdGcpVault("acme_secrets") }), []string{"Choose the Google Secret Manager installation, by its Google Cloud project ID"}},
		{"project with capitals", base.with(func(a *argocdInstanceArgs) { a.vault = argocdGcpVault("Acme-Secrets") }), []string{"Choose the Google Secret Manager installation"}},
		{"project too short", base.with(func(a *argocdInstanceArgs) { a.vault = argocdGcpVault("acme") }), []string{"Choose the Google Secret Manager installation"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newArgocdFake(t)
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{
						Config:      providerConfig(f.fakeP0) + tc.args.hcl(),
						PlanOnly:    true,
						ExpectError: argocdError(tc.errors...),
					},
				},
			})
		})
	}
}
