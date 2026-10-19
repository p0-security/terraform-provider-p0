package provider

import (
	"context"
	"fmt"
	"maps"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"testing"

	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	installlaunchdarkly "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/launchdarkly"
)

const (
	ldConnector       = "prod"
	ldConnectorName   = "p0-launchdarkly-prod"
	ldProject         = "acme-prod"
	ldServiceAccount  = ldConnectorName + "@" + ldProject + ".iam.gserviceaccount.com"
	ldServiceUri      = "https://" + ldConnectorName + "-abc123-uw.a.run.app"
	ldAccount         = "acme-prod"
	ldSecret          = "launchdarkly-acme-prod-token"
	ldSecretsProject  = "acme-secrets"
	ldSecretsAccount  = "210987654321"
	ldDomains         = `example[.]com`
	ldAccountAddress  = "p0_launchdarkly_account.example"
	ldConnectorFinal  = "p0_launchdarkly_connector.example"
	ldConnectorStaged = "p0_launchdarkly_connector_staged.example"
)

// Where a LaunchDarkly connector runs, as a customer's Terraform sets it, with one
// field set to value.
func ldGcpHosting(field, value string) string {
	fields := map[string]string{"project_id": ldProject, "connector_name": ldConnectorName, "connector_region": "us-west1"}
	fields[field] = value
	return fmt.Sprintf(`{
    type             = "gcp"
    project_id       = %q
    connector_name   = %q
    connector_region = %q
  }`, fields["project_id"], fields["connector_name"], fields["connector_region"])
}

func ldAwsHosting(field, value string) string {
	fields := map[string]string{"account_id": "123456789012", "connector_name": ldConnectorName, "connector_region": "us-west-2"}
	fields[field] = value
	return fmt.Sprintf(`{
    type             = "aws"
    account_id       = %q
    connector_name   = %q
    connector_region = %q
  }`, fields["account_id"], fields["connector_name"], fields["connector_region"])
}

var (
	ldGcp = ldGcpHosting("project_id", ldProject)
	ldAws = ldAwsHosting("connector_name", ldConnectorName)
)

func ldStaged(id, hosting string) string {
	return fmt.Sprintf(`
resource "p0_launchdarkly_connector_staged" "example" {
  id      = %q
  hosting = %s
}
`, id, hosting)
}

func ldStagedConfig(f *ldFake, hosting string) string {
	return providerConfig(f.fakeP0) + ldStaged(ldConnector, hosting)
}

// A connector staged at one hosting, and installed at another. A customer's Terraform
// gives both the same one.
func ldMovedConfig(f *ldFake, staged, final string) string {
	return ldStagedConfig(f, staged) + fmt.Sprintf(`
resource "p0_launchdarkly_connector" "example" {
  id      = p0_launchdarkly_connector_staged.example.id
  hosting = %s
}
`, final)
}

func ldConnectorConfig(f *ldFake, hosting string) string {
	return ldMovedConfig(f, hosting, hosting)
}

// A LaunchDarkly account as a customer's Terraform sets it. connector is an HCL
// expression, such as a reference to the connector's resource, and vault an HCL
// object.
type ldAccountArgs struct {
	id, connector, instance, plan, vault, secretName string
}

func ldGcpVault(project string) string {
	return fmt.Sprintf(`{
    type       = "gcp-sm"
    project_id = %q
  }`, project)
}

func ldAwsVault(account, region string) string {
	return fmt.Sprintf(`{
    type           = "aws-sm"
    account_id     = %q
    secrets_region = %q
  }`, account, region)
}

// An account on Google Cloud, reached through the connector of ldConnectorConfig.
func ldGcpAccount() ldAccountArgs {
	return ldAccountArgs{
		id:         ldAccount,
		connector:  "p0_launchdarkly_connector.example.id",
		instance:   "commercial",
		plan:       "enterprise",
		vault:      ldGcpVault(ldSecretsProject),
		secretName: ldSecret,
	}
}

func ldAwsAccount() ldAccountArgs {
	args := ldGcpAccount()
	args.vault = ldAwsVault(ldSecretsAccount, "us-west-2")
	args.secretName = "launchdarkly/acme-prod/token"
	return args
}

// The account with one change.
func (a ldAccountArgs) with(change func(*ldAccountArgs)) ldAccountArgs {
	change(&a)
	return a
}

func (a ldAccountArgs) hcl() string {
	return a.hclNamed("example")
}

func (a ldAccountArgs) hclNamed(name string) string {
	return fmt.Sprintf(`
resource "p0_launchdarkly_account" %q {
  id                = %q
  connector         = %s
  instance          = %q
  plan              = %q
  vault             = %s
  token_secret_name = %q
}
`, name, a.id, a.connector, a.instance, a.plan, a.vault, a.secretName)
}

// The connector installed at hosting, and the account it reaches.
func ldAccountConfig(f *ldFake, hosting string, account ldAccountArgs) string {
	return ldConnectorConfig(f, hosting) + account.hcl()
}

// What P0 reads from the connector when it verifies an account.
type ldReport struct {
	accountId, tokenKind, tokenOwnerMemberId, allowedDomains string
}

// Behaves as P0's LaunchDarkly installers do (packages/integrations/launchdarkly/ in
// the app), in their words. Update it when those installers change. The fields make
// it fail, for the test of each failure.
type ldFake struct {
	*fakeP0
	// P0's message for a connector that fails its install check, on verify.
	failedCheck string
	// P0's message for an account that fails its install check, on verify.
	failedAccountCheck string
	// What the connector reports for an account, by its stored item. By default, a
	// service token whose LaunchDarkly account follows from the secret's name, so
	// two accounts with one secret are one LaunchDarkly account.
	report func(item map[string]any) ldReport
	// The state each write to an account would move it to, in order, refused or not.
	accountSteps []string
	// How many writes to LaunchDarkly items P0 got, refused or not.
	writes int
}

// The IDs of the LaunchDarkly accounts that name the connector id, sorted.
func ldAccountsUsing(items map[fakeItemKey]map[string]any, id string) []string {
	users := []string{}
	for key, item := range items {
		if key.integration == "launchdarkly" && key.component == "account" && item["connector"] == id {
			users = append(users, key.id)
		}
	}
	slices.Sort(users)
	return users
}

func ldConnectorInUse(id string, action string, users []string) string {
	return fmt.Sprintf("The LaunchDarkly connector %s can't be %s while a LaunchDarkly account uses it (%s). Remove each account that uses it from P0 first.",
		id, action, strings.Join(users, ", "))
}

// The fields of a connector's hosting that P0 fixes when it creates the item.
var ldAddressFields = []string{"type", "accountId", "projectId", "connectorName", "connectorRegion"}

// The fields of an account that P0 reads from the connector. A PUT sets each to "",
// a verify to what the connector reports, and a configure keeps it.
var ldReportedFields = []string{"accountId", "tokenKind", "tokenOwnerMemberId", "allowedDomains"}

func newLdFake(t *testing.T) *ldFake {
	f := &ldFake{fakeP0: newFakeP0(t)}
	f.report = func(item map[string]any) ldReport {
		return ldReport{accountId: fmt.Sprintf("ld-%v", item["tokenSecretName"]), tokenKind: "service", allowedDomains: ldDomains}
	}

	f.refuse = func(step fakeStep, items map[fakeItemKey]map[string]any) (int, string) {
		if step.key.integration != "launchdarkly" {
			return 0, ""
		}
		f.writes++
		if step.key.component == "account" {
			return f.refuseAccount(step, items)
		}

		hosting, _ := step.body["hosting"].(map[string]any)
		if step.nextState == "stage" {
			// P0 takes a PUT over a connector it holds only when the hosting matches, and
			// no account uses the connector.
			if stored, ok := step.previous["hosting"].(map[string]any); ok {
				for _, field := range ldAddressFields {
					if hosting[field] != stored[field] {
						return http.StatusConflict, fmt.Sprintf("A LaunchDarkly connector with the ID %s already exists, and a connector can't be changed once it's added. "+
							"To run a connector somewhere else, add one under another ID.", step.key.id)
					}
				}
				if users := ldAccountsUsing(items, step.key.id); len(users) > 0 {
					return http.StatusConflict, ldConnectorInUse(step.key.id, "set up again", users)
				}
			}
			return 0, ""
		}

		if users := ldAccountsUsing(items, step.key.id); step.nextState == "configure" && step.previous["state"] == "installed" && len(users) > 0 {
			return http.StatusConflict, ldConnectorInUse(step.key.id, "verified again", users)
		}

		staged, _ := step.previous["hosting"].(map[string]any)
		for _, field := range ldAddressFields {
			if sent, ok := hosting[field]; ok && sent != staged[field] {
				return http.StatusUnprocessableEntity, fmt.Sprintf(
					"'hosting/%s/%s' can only be altered on initial installation. Create a new installation to change this field.",
					staged["type"], field,
				)
			}
		}
		if step.nextState == "configure" && f.failedCheck != "" {
			return http.StatusUnprocessableEntity, f.failedCheck
		}
		return 0, ""
	}

	f.normalize = func(key fakeItemKey, item map[string]any) {
		if key.integration != "launchdarkly" {
			return
		}
		if key.component == "account" {
			f.normalizeAccount(item)
			return
		}
		hosting, ok := item["hosting"].(map[string]any)
		if !ok || hosting["type"] != "gcp" {
			return
		}
		hosting["connectorServiceAccount"] = fmt.Sprintf("%s@%s.iam.gserviceaccount.com", hosting["connectorName"], hosting["projectId"])
		if item["state"] != "stage" {
			hosting["connectorServiceUri"] = fmt.Sprintf("https://%s-abc123-uw.a.run.app", hosting["connectorName"])
		}
	}

	return f
}

// Sets the fields P0 reads from the connector, as the account's state calls for.
func (f *ldFake) normalizeAccount(item map[string]any) {
	switch item["state"] {
	case "stage":
		for _, field := range ldReportedFields {
			item[field] = ""
		}
	case "configure":
		report := f.report(item)
		item["accountId"], item["tokenKind"] = report.accountId, report.tokenKind
		item["tokenOwnerMemberId"], item["allowedDomains"] = report.tokenOwnerMemberId, report.allowedDomains
	}
}

// The cloud each vault type is in, as a connector's hosting names it.
var ldVaultCloud = map[string]string{"aws-sm": "aws", "gcp-sm": "gcp"}

// Answers as P0's installer for an account does, in its words. The provider checks
// the ID, vault and secret name at plan time, so the fake doesn't repeat those checks
// for a new account:
//   - a PUT over an account that exists is refused if it changes or leaves out a field,
//     or if the account is installed. An unchanged one at stage or configure is staged
//     again.
//   - a new account needs an installed connector in its vault's cloud, and on AWS a
//     secret in another account than the connector's can't end like an ARN's suffix
//   - verify and configure refuse a change to any field, in the install framework's
//     words, and an account whose connector is gone or isn't installed
//   - only verify runs the install check, and it refuses a second account for one
//     LaunchDarkly account
func (f *ldFake) refuseAccount(step fakeStep, items map[fakeItemKey]map[string]any) (int, string) {
	f.accountSteps = append(f.accountSteps, step.nextState)
	id := step.key.id

	if step.nextState == "stage" {
		if step.previous != nil {
			if changedLdAccountField(step.previous, step.body, true) != "" {
				return http.StatusConflict, fmt.Sprintf("A LaunchDarkly account named %s already exists, and an account can't be changed once it's added. "+
					"To change its plan or anything else, remove it from P0 and add it again. To add another account, use another name.", id)
			}
			if step.previous["state"] == "installed" {
				return http.StatusConflict, fmt.Sprintf("The LaunchDarkly account %s is already installed, and there's nothing to change.", id)
			}
			return 0, ""
		}
		connectorId, _ := step.body["connector"].(string)
		connector := items[fakeItemKey{"launchdarkly", "connector", connectorId}]
		if connector == nil || connector["state"] != "installed" {
			return http.StatusBadRequest, "That LaunchDarkly connector isn't installed in P0. Install it, and finish its setup, before you add an account that uses it."
		}
		hosting, _ := connector["hosting"].(map[string]any)
		vault, _ := step.body["vault"].(map[string]any)
		vaultType, _ := vault["type"].(string)
		if ldVaultCloud[vaultType] != hosting["type"] {
			return http.StatusBadRequest, map[any]string{
				"aws": "The account's connector runs on AWS, so the access token's secret has to be in AWS Secrets Manager.",
				"gcp": "The account's connector runs on Google Cloud, so the access token's secret has to be in Google Secret Manager.",
			}[hosting["type"]]
		}
		name, _ := step.body["tokenSecretName"].(string)
		if vaultType == "aws-sm" && vault["install"] != hosting["accountId"] && regexp.MustCompile(`-[A-Za-z0-9]{6}$`).MatchString(name) {
			return http.StatusBadRequest, "Secrets Manager can't find a secret in another account by its name when the name ends in a hyphen and six characters, like -AbCdEf. " +
				"Use a secret whose name doesn't end that way, or one in the connector's own account."
		}
		return 0, ""
	}

	if field := changedLdAccountField(step.previous, step.body, false); field != "" {
		return http.StatusUnprocessableEntity, fmt.Sprintf(
			"'%s' can only be altered on initial installation. Create a new installation to change this field.", field,
		)
	}
	connectorId, _ := step.previous["connector"].(string)
	connector := items[fakeItemKey{"launchdarkly", "connector", connectorId}]
	switch {
	case connector == nil:
		return http.StatusUnprocessableEntity, fmt.Sprintf("The LaunchDarkly connector %s, which the account %s uses, isn't in P0 any more. "+
			"Add a connector with that ID, in the cloud that holds the account's secret, then retry.", connectorId, id)
	case connector["state"] != "installed":
		return http.StatusUnprocessableEntity, fmt.Sprintf("The LaunchDarkly connector %s, which the account %s uses, isn't installed. Finish installing it, then retry.", connectorId, id)
	case step.nextState != "configure":
		return 0, ""
	case f.failedAccountCheck != "":
		return http.StatusUnprocessableEntity, f.failedAccountCheck
	}
	accountId := f.report(step.previous).accountId
	others := []string{}
	for key, item := range items {
		if key.integration == "launchdarkly" && key.component == "account" && key.id != id && item["accountId"] == accountId {
			others = append(others, key.id)
		}
	}
	if len(others) > 0 {
		slices.Sort(others)
		return http.StatusConflict, fmt.Sprintf("The access token of the LaunchDarkly account %s is for the same LaunchDarkly account as %s, which P0 already has. "+
			"P0 can hold each LaunchDarkly account only once. Use the one P0 has, or remove it from P0 before you add this one.", id, strings.Join(others, ", "))
	}
	return 0, ""
}

// The first field of a stored account that body changes, by its path as P0 names it,
// or "" if body changes none. A PUT has to send the whole account, so whole counts a
// field it leaves out as a change. Verify and configure are merged over the stored
// account first.
func changedLdAccountField(item, body map[string]any, whole bool) string {
	for _, field := range []string{"connector", "tokenSecretName"} {
		if sent, ok := body[field]; (ok || whole) && sent != item[field] {
			return field
		}
	}
	for _, field := range []string{"instance", "plan", "vault"} {
		sent, ok := body[field].(map[string]any)
		if !ok {
			if whole {
				return field
			}
			continue
		}
		stored, _ := item[field].(map[string]any)
		if sent["type"] != stored["type"] {
			return field
		}
		for _, key := range slices.Sorted(maps.Keys(sent)) {
			if sent[key] != stored[key] {
				return fmt.Sprintf("%s/%s/%s", field, sent["type"], key)
			}
		}
	}
	return ""
}

func (f *ldFake) connector() map[string]any {
	return f.item("launchdarkly", "connector", ldConnector)
}

func (f *ldFake) account(id string) map[string]any {
	return f.item("launchdarkly", "account", id)
}

func (f *ldFake) steps() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.accountSteps)
}

// The connector's item in P0, if it is at state and holds exactly the hosting want.
func (f *ldFake) checkConnector(state string, want map[string]any) error {
	item := f.connector()
	hosting, _ := item["hosting"].(map[string]any)
	if item["state"] != state || len(hosting) != len(want) {
		return fmt.Errorf("P0 item = %v; want it at %s with the hosting %v", item, state, want)
	}
	for field, value := range want {
		if hosting[field] != value {
			return fmt.Errorf("P0 item = %v; want it at %s with the hosting %v", item, state, want)
		}
	}
	return nil
}

// The account's item in P0, if it is at state and holds exactly the fields want, and
// the fields P0 reads from the connector.
func (f *ldFake) checkAccount(state string, want map[string]any) error {
	item := f.account(ldAccount)
	fields := map[string]any{}
	for field, value := range item {
		if field != "label" && field != "state" && !slices.Contains(ldReportedFields, field) {
			fields[field] = value
		}
	}
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

func checkLdDestroyed(f *ldFake, accounts ...string) func(*terraform.State) error {
	return func(*terraform.State) error {
		for _, id := range accounts {
			if item := f.account(id); item != nil {
				return fmt.Errorf("the account %s was not removed from P0: %v", id, item)
			}
		}
		if item := f.connector(); item != nil {
			return fmt.Errorf("the connector was not removed from P0: %v", item)
		}
		return nil
	}
}

// Terraform wraps a diagnostic's text, so the words of each part of an error may be on
// different lines. The parts match in order, with anything between them.
func ldError(parts ...string) *regexp.Regexp {
	patterns := []string{}
	for _, part := range parts {
		patterns = append(patterns, strings.Join(strings.Fields(regexp.QuoteMeta(part)), `\s+`))
	}
	return regexp.MustCompile(`(?s)` + strings.Join(patterns, `.*`))
}

// The connector on Google Cloud: staging it reads the service account that P0
// generates, and installing it reads the Cloud Run URL that P0 resolves.
func TestLaunchDarklyConnectorOnGoogleCloud(t *testing.T) {
	f := newLdFake(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkLdDestroyed(f),
		Steps: []resource.TestStep{
			// The framework re-plans after the step and fails on a non-empty plan, so this
			// also checks that the fields P0 assigns leave no diff.
			{
				Config: ldConnectorConfig(f, ldGcp),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(ldConnectorStaged, "state", "stage"),
					resource.TestCheckResourceAttr(ldConnectorStaged, "hosting.connector_service_account", ldServiceAccount),
					resource.TestCheckNoResourceAttr(ldConnectorStaged, "hosting.connector_service_uri"),
					resource.TestCheckResourceAttr(ldConnectorFinal, "state", "installed"),
					resource.TestCheckResourceAttr(ldConnectorFinal, "hosting.connector_service_account", ldServiceAccount),
					resource.TestCheckResourceAttr(ldConnectorFinal, "hosting.connector_service_uri", ldServiceUri),
					func(*terraform.State) error {
						return f.checkConnector("installed", map[string]any{
							"type":                    "gcp",
							"projectId":               ldProject,
							"connectorName":           ldConnectorName,
							"connectorRegion":         "us-west1",
							"connectorServiceAccount": ldServiceAccount,
							"connectorServiceUri":     ldServiceUri,
						})
					},
				),
			},
			{
				ResourceName:      ldConnectorStaged,
				ImportState:       true,
				ImportStateId:     ldConnector,
				ImportStateVerify: true,
				// The staged resource's state is from before the connector was installed.
				ImportStateVerifyIgnore: []string{"state", "hosting.connector_service_uri"},
			},
			{
				ResourceName:      ldConnectorFinal,
				ImportState:       true,
				ImportStateId:     ldConnector,
				ImportStateVerify: true,
			},
			// The ID goes into a URL path as it is, so one that P0 would refuse isn't read.
			{
				ResourceName:  ldConnectorFinal,
				ImportState:   true,
				ImportStateId: "prod/other",
				ExpectError:   ldError("Invalid LaunchDarkly connector ID", "Use lowercase letters, digits and hyphens"),
			},
		},
	})
}

// The connector on AWS, where P0 generates nothing: it derives the function's address
// from the hosting.
func TestLaunchDarklyConnectorOnAws(t *testing.T) {
	f := newLdFake(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkLdDestroyed(f),
		Steps: []resource.TestStep{
			{
				Config: ldConnectorConfig(f, ldAws),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr(ldConnectorStaged, "hosting.connector_service_account"),
					resource.TestCheckResourceAttr(ldConnectorFinal, "state", "installed"),
					resource.TestCheckResourceAttr(ldConnectorFinal, "hosting.account_id", "123456789012"),
					func(*terraform.State) error {
						return f.checkConnector("installed", map[string]any{
							"type":            "aws",
							"accountId":       "123456789012",
							"connectorName":   ldConnectorName,
							"connectorRegion": "us-west-2",
						})
					},
				),
			},
			{
				ResourceName:      ldConnectorFinal,
				ImportState:       true,
				ImportStateId:     ldConnector,
				ImportStateVerify: true,
			},
		},
	})
}

// Where a connector runs is fixed when it is staged, so a new hosting replaces the
// staged resource, and the final resource with it, in one apply.
func TestLaunchDarklyConnectorHostingReplacesBoth(t *testing.T) {
	f := newLdFake(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkLdDestroyed(f),
		Steps: []resource.TestStep{
			{Config: ldConnectorConfig(f, ldGcp)},
			{
				Config: ldConnectorConfig(f, ldGcpHosting("connector_region", "europe-west1")),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(ldConnectorStaged, plancheck.ResourceActionReplace),
						plancheck.ExpectResourceAction(ldConnectorFinal, plancheck.ResourceActionReplace),
					},
				},
				Check: func(*terraform.State) error {
					if hosting, _ := f.connector()["hosting"].(map[string]any); hosting["connectorRegion"] != "europe-west1" {
						return fmt.Errorf("P0 item = %v; want the connector installed where it moved to", f.connector())
					}
					return nil
				},
			},
		},
	})
}

// Destroying only the final resource returns its item to "stage", so the staged
// resource still owns it, and a later apply installs it again.
func TestLaunchDarklyConnectorRollback(t *testing.T) {
	f := newLdFake(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkLdDestroyed(f),
		Steps: []resource.TestStep{
			{Config: ldConnectorConfig(f, ldAws)},
			{
				Config: ldStagedConfig(f, ldAws),
				Check: func(*terraform.State) error {
					return f.checkConnector("stage", map[string]any{
						"type":            "aws",
						"accountId":       "123456789012",
						"connectorName":   ldConnectorName,
						"connectorRegion": "us-west-2",
					})
				},
			},
			{
				Config: ldConnectorConfig(f, ldAws),
				Check:  resource.TestCheckResourceAttr(ldConnectorFinal, "state", "installed"),
			},
		},
	})
}

// The final resource only verifies the item, so P0 refuses a hosting that isn't the
// staged one, and a connector that fails P0's check fails the apply with P0's
// message. Either way the connector stays staged, and the next apply checks it again.
func TestLaunchDarklyConnectorFailedCheck(t *testing.T) {
	t.Run("moved", func(t *testing.T) {
		f := newLdFake(t)
		resource.UnitTest(t, resource.TestCase{
			ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
			CheckDestroy:             checkLdDestroyed(f),
			Steps: []resource.TestStep{
				{
					Config: ldMovedConfig(f, ldGcp, ldGcpHosting("connector_region", "europe-west1")),
					ExpectError: ldError(
						"LaunchDarkly connector install check failed",
						"'hosting/gcp/connectorRegion' can only be altered on initial installation",
					),
				},
				{
					PreConfig: func() {
						if item := f.connector(); item["state"] != "stage" {
							t.Errorf("P0 item after the refused install = %v; want it staged", item)
						}
					},
					Config: ldConnectorConfig(f, ldGcp),
					Check:  resource.TestCheckResourceAttr(ldConnectorFinal, "state", "installed"),
				},
			},
		})
	})

	t.Run("failed check", func(t *testing.T) {
		f := newLdFake(t)
		f.failedCheck = "P0 couldn't find the Cloud Run service p0-launchdarkly-prod."
		resource.UnitTest(t, resource.TestCase{
			ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
			CheckDestroy:             checkLdDestroyed(f),
			Steps: []resource.TestStep{
				{
					Config:      ldConnectorConfig(f, ldGcp),
					ExpectError: ldError("LaunchDarkly connector install check failed", `LaunchDarkly connector "prod"`, f.failedCheck),
				},
				{
					PreConfig: func() { f.failedCheck = "" },
					Config:    ldConnectorConfig(f, ldGcp),
					ConfigPlanChecks: resource.ConfigPlanChecks{
						PreApply: []plancheck.PlanCheck{
							plancheck.ExpectResourceAction(ldConnectorStaged, plancheck.ResourceActionNoop),
							plancheck.ExpectResourceAction(ldConnectorFinal, plancheck.ResourceActionCreate),
						},
					},
					Check: resource.TestCheckResourceAttr(ldConnectorFinal, "state", "installed"),
				},
			},
		})
	})
}

// A connector that P0 already holds, such as one added in the P0 app, isn't staged
// again. The apply fails, writes nothing, and says to import the connector.
func TestLaunchDarklyConnectorStagedOverExisting(t *testing.T) {
	f := newLdFake(t)
	f.integrations["launchdarkly"] = true
	f.items[fakeItemKey{"launchdarkly", "connector", ldConnector}] = map[string]any{
		"hosting": map[string]any{
			"type": "gcp", "projectId": ldProject, "connectorName": ldConnectorName, "connectorRegion": "us-west1",
			"connectorServiceAccount": ldServiceAccount,
		},
		"label": ldConnector, "state": "installed",
	}
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: ldStagedConfig(f, ldGcp),
				ExpectError: ldError(
					"LaunchDarkly connector already exists in P0", `P0 already has a LaunchDarkly connector "prod"`,
					"terraform import <address> prod",
				),
			},
		},
	})
	if f.writes != 0 {
		t.Errorf("P0 got %d writes; want none", f.writes)
	}
}

// While an account uses the connector, P0 won't return the connector to "stage". The
// examples' account refers to the connector, so Terraform removes the account first
// and the rollback goes through. An account that names the connector by a literal ID
// doesn't order them, and destroying the connector alone fails and says why.
func TestLaunchDarklyConnectorWithAccount(t *testing.T) {
	t.Run("removed together", func(t *testing.T) {
		f := newLdFake(t)
		resource.UnitTest(t, resource.TestCase{
			ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
			CheckDestroy:             checkLdDestroyed(f, ldAccount),
			Steps: []resource.TestStep{
				{Config: ldAccountConfig(f, ldGcp, ldGcpAccount())},
				{
					Config: ldStagedConfig(f, ldGcp),
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
		f := newLdFake(t)
		literal := ldGcpAccount().with(func(a *ldAccountArgs) { a.connector = fmt.Sprintf("%q", ldConnector) })
		resource.UnitTest(t, resource.TestCase{
			ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
			CheckDestroy:             checkLdDestroyed(f, ldAccount),
			Steps: []resource.TestStep{
				{Config: ldConnectorConfig(f, ldGcp)},
				{Config: ldConnectorConfig(f, ldGcp) + literal.hcl()},
				{
					Config: ldStagedConfig(f, ldGcp) + literal.hcl(),
					ExpectError: ldError(
						"LaunchDarkly connector is in use", "Remove each p0_launchdarkly_account that uses it first",
						"409 Conflict: The LaunchDarkly connector prod can't be set up again while a LaunchDarkly account uses it (acme-prod).",
					),
				},
				{
					PreConfig: func() {
						if item := f.connector(); item["state"] != "installed" {
							t.Errorf("P0 item = %v; want the connector still installed", item)
						}
					},
					Config: ldConnectorConfig(f, ldGcp),
				},
			},
		})
	})
}

// Terraform never plans an update of the staged resource, since each attribute that
// can be set requires replacement. One that reaches the provider anyway fails.
func TestLaunchDarklyConnectorStagedUpdateFails(t *testing.T) {
	var resp fwresource.UpdateResponse
	installlaunchdarkly.NewLaunchDarklyConnectorStaged().Update(context.Background(), fwresource.UpdateRequest{}, &resp)

	errs := resp.Diagnostics.Errors()
	if len(errs) != 1 || !strings.Contains(errs[0].Detail(), "Please report this as a provider bug") {
		t.Errorf("diagnostics = %v; want one error asking for a bug report", resp.Diagnostics)
	}
}

func TestLaunchDarklyConnectorValidation(t *testing.T) {
	const invalidId = "Use lowercase letters, digits and hyphens, starting and ending with a letter or digit, up to 63 characters"
	for _, tc := range []struct {
		name, config string
		errors       []string
	}{
		{"id with capitals", ldStaged("Prod", ldGcp), []string{invalidId}},
		{"id with a path", ldStaged("prod/other", ldGcp), []string{invalidId}},
		{"AWS name ECR wouldn't take", ldStaged("prod", ldAwsHosting("connector_name", "p0--launchdarkly")), []string{"Invalid LaunchDarkly connector name", "Use 2 to 64 characters", `Got: "p0--launchdarkly"`}},
		{"Google Cloud name too long for a service account", ldStaged("prod", ldGcpHosting("connector_name", "p0-launchdarkly-connector-production")), []string{"Use 6 to 30 characters"}},
		{"account that isn't one", ldStaged("prod", ldAwsHosting("account_id", "1234")), []string{"12 numeric digits"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newLdFake(t)
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{
						Config:      providerConfig(f.fakeP0) + tc.config,
						PlanOnly:    true,
						ExpectError: ldError(tc.errors...),
					},
				},
			})
		})
	}
}

var ldGcpAccountItem = map[string]any{
	"connector":       ldConnector,
	"instance":        map[string]any{"type": "commercial"},
	"plan":            map[string]any{"type": "enterprise"},
	"vault":           map[string]any{"type": "gcp-sm", "install": ldSecretsProject},
	"tokenSecretName": ldSecret,
}

// The account behind a connector on Google Cloud, with its secret in Google Secret
// Manager. Creating it stages, verifies and configures the item, and reads what P0
// learned from the connector.
func TestLaunchDarklyAccountOnGoogleCloud(t *testing.T) {
	f := newLdFake(t)
	config := ldAccountConfig(f, ldGcp, ldGcpAccount())

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkLdDestroyed(f, ldAccount),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(ldAccountAddress, "state", "installed"),
					resource.TestCheckResourceAttr(ldAccountAddress, "connector", ldConnector),
					resource.TestCheckResourceAttr(ldAccountAddress, "instance", "commercial"),
					resource.TestCheckResourceAttr(ldAccountAddress, "plan", "enterprise"),
					resource.TestCheckResourceAttr(ldAccountAddress, "vault.type", "gcp-sm"),
					resource.TestCheckResourceAttr(ldAccountAddress, "vault.project_id", ldSecretsProject),
					resource.TestCheckNoResourceAttr(ldAccountAddress, "vault.account_id"),
					resource.TestCheckResourceAttr(ldAccountAddress, "token_secret_name", ldSecret),
					resource.TestCheckResourceAttr(ldAccountAddress, "account_id", "ld-"+ldSecret),
					resource.TestCheckResourceAttr(ldAccountAddress, "token_kind", "service"),
					resource.TestCheckResourceAttr(ldAccountAddress, "token_owner_member_id", ""),
					resource.TestCheckResourceAttr(ldAccountAddress, "allowed_domains", ldDomains),
					func(*terraform.State) error {
						if steps := f.steps(); !slices.Equal(steps, []string{"stage", "configure", "installed"}) {
							return fmt.Errorf("P0 got the account's steps %v; want stage, verify, configure", steps)
						}
						return f.checkAccount("installed", ldGcpAccountItem)
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
				ResourceName:      ldAccountAddress,
				ImportState:       true,
				ImportStateId:     ldAccount,
				ImportStateVerify: true,
			},
			{
				ResourceName:  ldAccountAddress,
				ImportState:   true,
				ImportStateId: "acme/prod",
				ExpectError:   ldError("Invalid LaunchDarkly account ID", "Use lowercase letters, digits and hyphens", `Got: "acme/prod"`),
			},
		},
	})
}

// The account behind a connector on AWS, with its secret in AWS Secrets Manager, on
// another plan and instance, with an admin's personal token.
func TestLaunchDarklyAccountOnAws(t *testing.T) {
	f := newLdFake(t)
	f.report = func(map[string]any) ldReport {
		return ldReport{accountId: "ld-eu", tokenKind: "personal", tokenOwnerMemberId: "5f0c1a2b3c4d5e6f7a8b9c0d", allowedDomains: ldDomains}
	}
	account := ldAwsAccount().with(func(a *ldAccountArgs) { a.instance, a.plan = "eu", "non-enterprise" })
	config := ldAccountConfig(f, ldAws, account)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkLdDestroyed(f, ldAccount),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(ldAccountAddress, "state", "installed"),
					resource.TestCheckResourceAttr(ldAccountAddress, "vault.account_id", ldSecretsAccount),
					resource.TestCheckResourceAttr(ldAccountAddress, "vault.secrets_region", "us-west-2"),
					resource.TestCheckResourceAttr(ldAccountAddress, "token_kind", "personal"),
					resource.TestCheckResourceAttr(ldAccountAddress, "token_owner_member_id", "5f0c1a2b3c4d5e6f7a8b9c0d"),
					func(*terraform.State) error {
						return f.checkAccount("installed", map[string]any{
							"connector":       ldConnector,
							"instance":        map[string]any{"type": "eu"},
							"plan":            map[string]any{"type": "non-enterprise"},
							"vault":           map[string]any{"type": "aws-sm", "install": ldSecretsAccount, "secretsRegion": "us-west-2"},
							"tokenSecretName": "launchdarkly/acme-prod/token",
						})
					},
				),
			},
			{
				ResourceName:      ldAccountAddress,
				ImportState:       true,
				ImportStateId:     ldAccount,
				ImportStateVerify: true,
			},
		},
	})
}

// A second connector, for an account to move to.
const ldSecondConnector = `
resource "p0_launchdarkly_connector_staged" "second" {
  id = "prod-2"
  hosting = {
    type             = "gcp"
    project_id       = "acme-prod"
    connector_name   = "p0-launchdarkly-prod-2"
    connector_region = "us-west1"
  }
}

resource "p0_launchdarkly_connector" "second" {
  id = p0_launchdarkly_connector_staged.second.id
  hosting = {
    type             = "gcp"
    project_id       = "acme-prod"
    connector_name   = "p0-launchdarkly-prod-2"
    connector_region = "us-west1"
  }
}
`

// P0 fixes every field of an account when it adds it, the plan among them. A change
// to any replaces the account, and leaves its connector alone.
func TestLaunchDarklyAccountChangeReplaces(t *testing.T) {
	gcp, aws := ldGcpAccount(), ldAwsAccount()

	for _, tc := range []struct {
		name          string
		hosting       string
		before, after ldAccountArgs
		// The changed field, as P0 stores it, under the account's new ID.
		id, field string
		value     any
	}{
		{"id", ldGcp, gcp, gcp.with(func(a *ldAccountArgs) { a.id = "acme-staging" }), "acme-staging", "tokenSecretName", ldSecret},
		{"connector", ldGcp, gcp, gcp.with(func(a *ldAccountArgs) { a.connector = "p0_launchdarkly_connector.second.id" }), ldAccount, "connector", "prod-2"},
		{"instance", ldGcp, gcp, gcp.with(func(a *ldAccountArgs) { a.instance = "federal" }), ldAccount, "instance", map[string]any{"type": "federal"}},
		{"plan", ldGcp, gcp, gcp.with(func(a *ldAccountArgs) { a.plan = "non-enterprise" }), ldAccount, "plan", map[string]any{"type": "non-enterprise"}},
		{"token secret name", ldGcp, gcp, gcp.with(func(a *ldAccountArgs) { a.secretName = ldSecret + "-2" }), ldAccount, "tokenSecretName", ldSecret + "-2"},
		{
			"vault project", ldGcp, gcp, gcp.with(func(a *ldAccountArgs) { a.vault = ldGcpVault("acme-secrets-2") }),
			ldAccount, "vault", map[string]any{"type": "gcp-sm", "install": "acme-secrets-2"},
		},
		{
			"vault account", ldAws, aws, aws.with(func(a *ldAccountArgs) { a.vault = ldAwsVault("123456789012", "us-west-2") }),
			ldAccount, "vault", map[string]any{"type": "aws-sm", "install": "123456789012", "secretsRegion": "us-west-2"},
		},
		{
			"vault region", ldAws, aws, aws.with(func(a *ldAccountArgs) { a.vault = ldAwsVault(ldSecretsAccount, "eu-west-1") }),
			ldAccount, "vault", map[string]any{"type": "aws-sm", "install": ldSecretsAccount, "secretsRegion": "eu-west-1"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newLdFake(t)
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				CheckDestroy:             checkLdDestroyed(f, ldAccount, tc.id),
				Steps: []resource.TestStep{
					{Config: ldAccountConfig(f, tc.hosting, tc.before) + ldSecondConnector},
					{
						Config: ldAccountConfig(f, tc.hosting, tc.after) + ldSecondConnector,
						ConfigPlanChecks: resource.ConfigPlanChecks{
							PreApply: []plancheck.PlanCheck{
								plancheck.ExpectResourceAction(ldAccountAddress, plancheck.ResourceActionReplace),
								plancheck.ExpectResourceAction(ldConnectorFinal, plancheck.ResourceActionNoop),
								plancheck.ExpectResourceAction(ldConnectorStaged, plancheck.ResourceActionNoop),
							},
						},
						Check: resource.ComposeAggregateTestCheckFunc(
							resource.TestCheckResourceAttr(ldAccountAddress, "state", "installed"),
							func(*terraform.State) error {
								item := f.account(tc.id)
								if fmt.Sprint(item[tc.field]) != fmt.Sprint(tc.value) || item["state"] != "installed" {
									return fmt.Errorf("P0 item = %v; want it installed with %s %v", item, tc.field, tc.value)
								}
								if tc.id != ldAccount && f.account(ldAccount) != nil {
									return fmt.Errorf("the old account is still in P0: %v", f.account(ldAccount))
								}
								return nil
							},
						),
					},
				},
			})
		})
	}

	// A vault on another cloud needs a connector on that cloud, so the plan alone is
	// checked here.
	t.Run("vault type", func(t *testing.T) {
		f := newLdFake(t)
		resource.UnitTest(t, resource.TestCase{
			ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
			CheckDestroy:             checkLdDestroyed(f, ldAccount),
			Steps: []resource.TestStep{
				{Config: ldAccountConfig(f, ldGcp, gcp)},
				{
					Config:             ldAccountConfig(f, ldGcp, gcp.with(func(a *ldAccountArgs) { a.vault = ldAwsVault(ldSecretsAccount, "us-west-2") })),
					PlanOnly:           true,
					ExpectNonEmptyPlan: true,
					ConfigPlanChecks: resource.ConfigPlanChecks{
						PostApplyPostRefresh: []plancheck.PlanCheck{
							plancheck.ExpectResourceAction(ldAccountAddress, plancheck.ResourceActionReplace),
						},
					},
				},
			},
		})
	})
}

// An account that fails P0's install check, on verify, fails the apply with P0's
// message and the way out, and stays staged. Its resource is tainted, so the next
// apply replaces it.
func TestLaunchDarklyAccountFailedCheck(t *testing.T) {
	f := newLdFake(t)
	const message = "The LaunchDarkly account acme-prod failed its install check while checking its plan: the token can't read custom roles."
	f.failedAccountCheck = message

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkLdDestroyed(f, ldAccount),
		Steps: []resource.TestStep{
			{
				Config: ldAccountConfig(f, ldGcp, ldGcpAccount()),
				ExpectError: ldError(
					"P0 didn't finish installing the LaunchDarkly account", `LaunchDarkly account "acme-prod"`, "422 Unprocessable Entity: "+message,
					"terraform apply -replace=<address>", "terraform state rm <address>",
				),
			},
			{
				PreConfig: func() {
					if err := f.checkAccount("stage", ldGcpAccountItem); err != nil {
						t.Errorf("after the failed step: %v", err)
					}
					f.failedAccountCheck = ""
				},
				Config: ldAccountConfig(f, ldGcp, ldGcpAccount()),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(ldAccountAddress, plancheck.ResourceActionReplace),
						plancheck.ExpectResourceAction(ldConnectorFinal, plancheck.ResourceActionNoop),
					},
				},
				Check: resource.TestCheckResourceAttr(ldAccountAddress, "state", "installed"),
			},
		},
	})
}

// P0 can hold each LaunchDarkly account only once, and refuses, on verify, a second item
// whose token is for the same LaunchDarkly account. The apply fails with P0's message.
func TestLaunchDarklyAccountDuplicate(t *testing.T) {
	f := newLdFake(t)
	copied := ldGcpAccount().with(func(a *ldAccountArgs) { a.id = "acme-prod-copy" })

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkLdDestroyed(f, ldAccount, "acme-prod-copy"),
		Steps: []resource.TestStep{
			{Config: ldAccountConfig(f, ldGcp, ldGcpAccount())},
			{
				Config: ldAccountConfig(f, ldGcp, ldGcpAccount()) + copied.hclNamed("copy"),
				ExpectError: ldError(
					"409 Conflict: The access token of the LaunchDarkly account acme-prod-copy is for the same LaunchDarkly account as acme-prod, which P0 already has.",
				),
			},
		},
	})
}

// P0 has the last word on what the plan can't check, and its refusal fails the apply
// in its own words, and stages nothing.
func TestLaunchDarklyAccountRefusedByP0(t *testing.T) {
	// The configuration names the connector by its ID alone, so the plan can't check
	// that the vault is in the connector's cloud.
	t.Run("vault in another cloud", func(t *testing.T) {
		f := newLdFake(t)
		resource.UnitTest(t, resource.TestCase{
			ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
			CheckDestroy:             checkLdDestroyed(f, ldAccount),
			Steps: []resource.TestStep{
				{
					Config: ldAccountConfig(f, ldGcp, ldAwsAccount()),
					ExpectError: ldError(
						"Could not stage account component",
						"400 Bad Request: The account's connector runs on Google Cloud, so the access token's secret has to be in Google Secret Manager.",
					),
				},
				{
					PreConfig: func() {
						if item := f.account(ldAccount); item != nil {
							t.Errorf("P0 stored the refused account: %v", item)
						}
					},
					Config: ldConnectorConfig(f, ldGcp),
				},
			},
		})
	})

	// P0 checks the account through its connector, so the connector must be installed
	// first. Referring to the connector's final resource orders them.
	t.Run("connector not installed", func(t *testing.T) {
		f := newLdFake(t)
		account := ldGcpAccount().with(func(a *ldAccountArgs) { a.connector = "p0_launchdarkly_connector_staged.example.id" })
		resource.UnitTest(t, resource.TestCase{
			ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
			CheckDestroy:             checkLdDestroyed(f, ldAccount),
			Steps: []resource.TestStep{
				{
					Config:      ldStagedConfig(f, ldGcp) + account.hcl(),
					ExpectError: ldError("Could not stage account component", "400 Bad Request: That LaunchDarkly connector isn't installed in P0."),
				},
			},
		})
	})
}

// An account that P0 already holds, such as one added in the P0 app, isn't created
// again, even when it matches the configuration. The apply fails, writes nothing, and
// says to import the account.
func TestLaunchDarklyAccountCreateOverExisting(t *testing.T) {
	f := newLdFake(t)
	key := fakeItemKey{"launchdarkly", "account", ldAccount}

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: ldConnectorConfig(f, ldGcp)},
			{
				PreConfig: func() {
					f.mu.Lock()
					defer f.mu.Unlock()
					existing := deepMerge(nil, ldGcpAccountItem)
					existing["label"], existing["state"] = ldAccount, "configure"
					f.items[key] = existing
				},
				Config: ldAccountConfig(f, ldGcp, ldGcpAccount()),
				ExpectError: ldError(
					"LaunchDarkly account already exists in P0", `P0 already has a LaunchDarkly account "acme-prod"`,
					"terraform import <address> acme-prod",
				),
			},
			{
				PreConfig: func() {
					if steps := f.steps(); len(steps) != 0 {
						t.Errorf("P0 got the account's steps %v; want no write", steps)
					}
					if item := f.account(ldAccount); item["state"] != "configure" {
						t.Errorf("P0 item = %v; want the existing account as it was", item)
					}
					f.mu.Lock()
					defer f.mu.Unlock()
					delete(f.items, key)
				},
				Config: ldConnectorConfig(f, ldGcp),
			},
		},
	})
}

// Puts an installed connector and an account at state in P0, as the P0 app would,
// with the account's fields as given.
func (f *ldFake) seedAccount(state string, fields map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.integrations["launchdarkly"] = true
	f.items[fakeItemKey{"launchdarkly", "connector", ldConnector}] = map[string]any{
		"hosting": map[string]any{"type": "gcp", "projectId": ldProject, "connectorName": ldConnectorName, "connectorRegion": "us-west1"},
		"label":   ldConnector,
		"state":   "installed",
	}
	item := deepMerge(nil, fields)
	item["label"], item["state"] = ldAccount, state
	f.normalizeAccount(item)
	f.items[fakeItemKey{"launchdarkly", "account", ldAccount}] = item
}

// An account that P0 holds but hasn't finished installing, as when it was staged in
// the P0 app, plans an update once imported, and applying it finishes the install:
// from "stage" with verify and then configure, from "configure" with configure alone.
// An imported account that differs from the configuration is refused by P0, in words
// that say so.
func TestLaunchDarklyAccountImportFinishesInstall(t *testing.T) {
	account := ldGcpAccount().with(func(a *ldAccountArgs) { a.connector = fmt.Sprintf("%q", ldConnector) })
	destroyed := func(f *ldFake) func(*terraform.State) error {
		return func(*terraform.State) error {
			if item := f.account(ldAccount); item != nil {
				return fmt.Errorf("the account was not removed from P0: %v", item)
			}
			return nil
		}
	}
	importStep := func(f *ldFake, config, state string, fields map[string]any) resource.TestStep {
		return resource.TestStep{
			PreConfig:          func() { f.seedAccount(state, fields) },
			Config:             config,
			ResourceName:       ldAccountAddress,
			ImportState:        true,
			ImportStateId:      ldAccount,
			ImportStatePersist: true,
		}
	}

	for _, state := range []string{"stage", "configure"} {
		t.Run(state, func(t *testing.T) {
			f := newLdFake(t)
			config := providerConfig(f.fakeP0) + account.hcl()
			want := map[string][]string{"stage": {"configure", "installed"}, "configure": {"installed"}}[state]
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				CheckDestroy:             destroyed(f),
				Steps: []resource.TestStep{
					importStep(f, config, state, ldGcpAccountItem),
					{
						Config: config,
						ConfigPlanChecks: resource.ConfigPlanChecks{
							PreApply: []plancheck.PlanCheck{
								plancheck.ExpectResourceAction(ldAccountAddress, plancheck.ResourceActionUpdate),
							},
						},
						Check: resource.ComposeAggregateTestCheckFunc(
							resource.TestCheckResourceAttr(ldAccountAddress, "state", "installed"),
							resource.TestCheckResourceAttr(ldAccountAddress, "allowed_domains", ldDomains),
							func(*terraform.State) error {
								if steps := f.steps(); !slices.Equal(steps, want) {
									return fmt.Errorf("P0 got the account's steps %v; want %v", steps, want)
								}
								return f.checkAccount("installed", ldGcpAccountItem)
							},
						),
					},
				},
			})
		})
	}

	// Every field replaces, so an imported account that differs from the configuration
	// is replaced: P0 is never asked to change a field it fixed.
	t.Run("differs from the configuration", func(t *testing.T) {
		f := newLdFake(t)
		config := providerConfig(f.fakeP0) + account.hcl()
		other := deepMerge(ldGcpAccountItem, map[string]any{"plan": map[string]any{"type": "non-enterprise"}})
		resource.UnitTest(t, resource.TestCase{
			ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
			CheckDestroy:             destroyed(f),
			Steps: []resource.TestStep{
				importStep(f, config, "stage", other),
				{
					Config: config,
					ConfigPlanChecks: resource.ConfigPlanChecks{
						PreApply: []plancheck.PlanCheck{
							plancheck.ExpectResourceAction(ldAccountAddress, plancheck.ResourceActionReplace),
						},
					},
					Check: func(*terraform.State) error {
						return f.checkAccount("installed", ldGcpAccountItem)
					},
				},
			},
		})
	})
}

// On AWS, P0 reads a secret in another account than the connector's by its partial
// ARN, so a name ending like an ARN's suffix is refused there, at plan time once the
// connector is in P0. In the connector's own account the name is taken.
func TestLaunchDarklyAccountSecretNameAcrossAccounts(t *testing.T) {
	const suffixed = "launchdarkly-token-AbC123"
	other := ldAwsAccount().with(func(a *ldAccountArgs) { a.secretName = suffixed })
	own := other.with(func(a *ldAccountArgs) { a.vault = ldAwsVault("123456789012", "us-west-2") })
	f := newLdFake(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkLdDestroyed(f, ldAccount),
		Steps: []resource.TestStep{
			{Config: ldConnectorConfig(f, ldAws)},
			{
				Config:   ldAccountConfig(f, ldAws, other),
				PlanOnly: true,
				ExpectError: ldError(
					"Invalid access token secret name",
					"Secrets Manager can't find a secret in another account by its name when the name ends in a hyphen and six characters",
				),
			},
			{
				Config: ldAccountConfig(f, ldAws, own),
				Check:  resource.TestCheckResourceAttr(ldAccountAddress, "state", "installed"),
			},
		},
	})
}

func TestLaunchDarklyAccountValidation(t *testing.T) {
	const invalidId = "Use lowercase letters, digits and hyphens, starting and ending with a letter or digit, up to 63 characters"
	const invalidAwsName = "That doesn't look like a secret name. Enter the name of the secret that holds the LaunchDarkly access token"
	const invalidGcpName = "That isn't a secret's name in Google Secret Manager."
	base := ldGcpAccount()
	base.connector = fmt.Sprintf("%q", ldConnector)
	aws := base
	aws.vault = ldAwsVault(ldSecretsAccount, "us-west-2")
	vault := func(fields string) func(*ldAccountArgs) {
		return func(a *ldAccountArgs) { a.vault = "{\n" + fields + "\n  }" }
	}

	for _, tc := range []struct {
		name   string
		args   ldAccountArgs
		errors []string
	}{
		{"id with capitals", base.with(func(a *ldAccountArgs) { a.id = "Acme-Prod" }), []string{invalidId}},
		{"id with a slash", base.with(func(a *ldAccountArgs) { a.id = "acme/prod" }), []string{invalidId}},
		{"connector with a path", base.with(func(a *ldAccountArgs) { a.connector = `"prod/other"` }), []string{"connector", invalidId}},
		{"instance that isn't one", base.with(func(a *ldAccountArgs) { a.instance = "us" }), []string{`value must be one of: ["commercial" "eu" "federal"]`}},
		{"plan that isn't one", base.with(func(a *ldAccountArgs) { a.plan = "starter" }), []string{`value must be one of: ["enterprise" "non-enterprise"]`}},
		{"secret ARN in AWS", aws.with(func(a *ldAccountArgs) {
			a.secretName = "arn:aws:secretsmanager:us-west-2:210987654321:secret:launchdarkly-token"
		}), []string{"Invalid access token secret name", invalidAwsName}},
		{"secret resource name in Google Cloud", base.with(func(a *ldAccountArgs) { a.secretName = "projects/acme-secrets/secrets/launchdarkly-token" }), []string{invalidGcpName}},
		{"secret name with whitespace around it", base.with(func(a *ldAccountArgs) { a.secretName = " " + ldSecret }), []string{"P0 removes the whitespace around 'token_secret_name'"}},
		{"AWS vault with no region", base.with(vault(`    type = "aws-sm"
    account_id = "210987654321"`)), []string{`'vault.secrets_region' is required when 'vault.type' is "aws-sm"`}},
		{"Google Cloud vault with an account", base.with(vault(`    type = "gcp-sm"
    project_id = "acme-secrets"
    account_id = "210987654321"`)), []string{`'vault.account_id' may only be set when 'vault.type' is "aws-sm"`}},
		{"vault of another kind", base.with(vault(`    type = "azure-kv"`)), []string{`value must be one of: ["aws-sm" "gcp-sm"]`}},
		{"account that isn't one", aws.with(func(a *ldAccountArgs) { a.vault = ldAwsVault("1234", "us-west-2") }), []string{"that holds the LaunchDarkly access token"}},
		{"region in GovCloud", aws.with(func(a *ldAccountArgs) { a.vault = ldAwsVault(ldSecretsAccount, "us-gov-west-1") }), []string{"Unsupported AWS region for the access token's secret", "P0's LaunchDarkly connector supports only AWS commercial regions for now."}},
		{"project too short", base.with(func(a *ldAccountArgs) { a.vault = ldGcpVault("acme") }), []string{"Choose the Google Secret Manager installation"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newLdFake(t)
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{
						Config:      providerConfig(f.fakeP0) + tc.args.hcl(),
						PlanOnly:    true,
						ExpectError: ldError(tc.errors...),
					},
				},
			})
		})
	}
}
