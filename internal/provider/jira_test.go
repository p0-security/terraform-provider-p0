package provider

import (
	"fmt"
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/p0-security/terraform-provider-p0/internal"
)

const (
	jiraSite          = "acme.atlassian.net"
	jiraConnectorName = "p0-jira-acme"
	jiraProject       = "acme-prod"
	jiraClientId      = "AbCdEfGhIjKlMnOpQrStUvWxYz012345"
	jiraCloudId       = "0b5c5c5e-1b1e-4b1e-9b1e-0b5c5c5e1b1e"
	jiraDomains       = `example\.com`
	jiraAccount       = jiraConnectorName + "@" + jiraProject + ".iam.gserviceaccount.com"
	jiraServiceUri    = "https://" + jiraConnectorName + "-abc123-uw.a.run.app"
)

// Where a site's connector runs, its vault and its secret, as a customer's Terraform
// sets them.
type jiraCloud struct {
	hosting string
	vault   string
	secret  string
}

var jiraAws = jiraCloud{
	hosting: `{
    type             = "aws"
    account_id       = "123456789012"
    connector_name   = "p0-jira-acme"
    connector_region = "us-west-2"
  }`,
	vault: `{
    type           = "aws-sm"
    account_id     = "123456789012"
    secrets_region = "us-west-2"
  }`,
	secret: "p0/jira/acme",
}

var jiraGcp = jiraCloud{
	hosting: `{
    type             = "gcp"
    project_id       = "acme-prod"
    connector_name   = "p0-jira-acme"
    connector_region = "us-west1"
  }`,
	vault: `{
    type       = "gcp-sm"
    project_id = "acme-secrets"
  }`,
	secret: "jira-acme",
}

// The site's resource, with the attributes that the cloud doesn't set.
type jiraSiteArgs struct {
	site, domainPattern, clientId string
}

var jiraDefaults = jiraSiteArgs{site: jiraSite, domainPattern: jiraDomains, clientId: jiraClientId}

func jiraResource(cloud jiraCloud, args jiraSiteArgs) string {
	return fmt.Sprintf(`
resource "p0_jira" "example" {
  site               = %q
  hosting            = %s
  vault              = %s
  client_secret_name = %q
  domain_pattern     = %q
  client_id          = %q
}
`, args.site, cloud.hosting, cloud.vault, cloud.secret, args.domainPattern, args.clientId)
}

func jiraConfig(f *jiraFake, cloud jiraCloud) string {
	return providerConfig(f.fakeP0) + jiraResource(cloud, jiraDefaults)
}

// Behaves as P0's Jira installer does for a site (packages/integrations/jira/backend
// in the app):
//   - staging one on Google Cloud names the service account the connector runs as
//   - verifying one on Google Cloud stores its Cloud Run URL
//   - configuring one checks the site through the connector with the client ID, and
//     stores its cloud ID, which is empty until then
//   - a PUT over a site that is installed, or whose fixed fields differ, is refused
//   - a verify or configure that would change a fixed field is refused, in the install
//     framework's words
//
// The provider checks the host, hosting, vault, secret name, domain pattern and client
// ID at plan time, so the fake doesn't repeat those checks.
type jiraFake struct {
	*fakeP0
	// P0's message for a site that fails its install check, on configure.
	failedCheck string
	// Each write to the site, in order, as the state it moves the item to and the body.
	writes []fakeStep
}

// The fields of a site that P0 fixes when it creates the item, beside its hosting and
// vault.
var jiraFixedFields = []string{"clientSecretName", "domainPattern"}

// The hosting fields that P0 sets itself.
var jiraGeneratedHostingFields = []string{"connectorServiceAccount", "connectorServiceUri"}

func newJiraFake(t *testing.T) *jiraFake {
	f := &jiraFake{fakeP0: newFakeP0(t)}

	f.refuse = func(step fakeStep, _ map[fakeItemKey]map[string]any) (int, string) {
		if step.key.integration != "jira" {
			return 0, ""
		}
		f.writes = append(f.writes, step)
		id := step.key.id

		if step.nextState == "stage" {
			if step.previous == nil {
				return 0, ""
			}
			if changedJiraField(step.previous, step.body, true) != "" {
				return http.StatusConflict, fmt.Sprintf("The Jira site %s is already added, and a site's connector, secret and allowed email domains can't be changed once it's added. "+
					"To change them, remove the site and add it again.", id)
			}
			if step.previous["state"] == "installed" {
				return http.StatusConflict, fmt.Sprintf("The Jira site %s is already installed, and there's nothing to change.", id)
			}
			return 0, ""
		}

		if field := changedJiraField(step.previous, step.body, false); field != "" {
			return http.StatusBadRequest, fmt.Sprintf("'%s' can only be altered on initial installation. Create a new installation to change this field.", field)
		}
		clientId, _ := deepMerge(step.previous, step.body)["clientId"].(string)
		// A verify checks the site through the connector once it has a client ID, at
		// the address the stored site holds, which a Google Cloud site has only once a
		// verify has stored it.
		if step.nextState == "configure" && clientId != "" {
			if hosting, _ := step.previous["hosting"].(map[string]any); hosting["type"] == "gcp" && hosting["connectorServiceUri"] == nil {
				return http.StatusBadRequest, "The Jira site " + id + " failed its install check: P0 has no address for the connector yet."
			}
		}
		if step.nextState == "installed" {
			if clientId == "" {
				return http.StatusBadRequest, "Enter the client ID of the service account's OAuth credential."
			}
			if f.failedCheck != "" {
				return http.StatusBadRequest, f.failedCheck
			}
		}
		return 0, ""
	}

	f.normalize = func(key fakeItemKey, item map[string]any) {
		if key.integration != "jira" {
			return
		}
		// P0 stores an empty cloud ID until configure stores the one the connector
		// found.
		if item["state"] == "installed" {
			item["cloudId"] = jiraCloudId
		} else if _, ok := item["cloudId"]; !ok {
			item["cloudId"] = ""
		}
		hosting, ok := item["hosting"].(map[string]any)
		if !ok || hosting["type"] != "gcp" {
			return
		}
		if _, ok := hosting["connectorServiceAccount"]; !ok {
			hosting["connectorServiceAccount"] = fmt.Sprintf("%s@%s.iam.gserviceaccount.com", hosting["connectorName"], hosting["projectId"])
		}
		if item["state"] != "stage" {
			hosting["connectorServiceUri"] = fmt.Sprintf("https://%s-abc123-uw.a.run.app", hosting["connectorName"])
		}
	}

	return f
}

// The first fixed field of a stored site that body changes, by its path as P0 names
// it, or "" if body changes none. A PUT has to send every fixed field, so whole counts
// one it leaves out as a change. Verify and configure are merged over the stored site
// first.
func changedJiraField(item, body map[string]any, whole bool) string {
	for _, element := range []string{"hosting", "vault"} {
		sent, ok := body[element].(map[string]any)
		if !ok {
			if whole {
				return element
			}
			continue
		}
		stored, _ := item[element].(map[string]any)
		if sent["type"] != stored["type"] {
			return element
		}
		for field, value := range stored {
			if slices.Contains(jiraGeneratedHostingFields, field) {
				continue
			}
			if sent[field] != value {
				return fmt.Sprintf("%s/%s/%s", element, stored["type"], field)
			}
		}
	}
	for _, field := range jiraFixedFields {
		if sent, ok := body[field]; (ok || whole) && sent != item[field] {
			return field
		}
	}
	return ""
}

func (f *jiraFake) site() map[string]any {
	return f.item("jira", "site", jiraSite)
}

// The writes to the site, as "<state> <fields>", the fields of each body sorted.
func (f *jiraFake) writeLog() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	log := []string{}
	for _, step := range f.writes {
		fields := []string{}
		for field := range step.body {
			fields = append(fields, field)
		}
		slices.Sort(fields)
		log = append(log, step.nextState+" "+strings.Join(fields, ","))
	}
	return log
}

func checkJiraDestroyed(f *jiraFake) func(*terraform.State) error {
	return func(*terraform.State) error {
		if item := f.site(); item != nil {
			return fmt.Errorf("the site was not removed from P0: %v", item)
		}
		return nil
	}
}

// The PUT that stages a site and the verify leave out the client ID, as P0's installer
// does, and configure sends it.
var jiraInstallWrites = []string{
	"stage clientSecretName,domainPattern,hosting,vault",
	"configure clientSecretName,domainPattern,hosting,vault",
	"installed clientId,clientSecretName,domainPattern,hosting,vault",
}

// The site on AWS, where P0 sets nothing in the hosting: it derives the function's
// address from it.
func TestJiraOnAws(t *testing.T) {
	f := newJiraFake(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkJiraDestroyed(f),
		Steps: []resource.TestStep{
			// The framework re-plans after the step and fails on a non-empty plan, so this
			// also checks that the fields P0 assigns leave no diff.
			{
				Config: jiraConfig(f, jiraAws),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("p0_jira.example", "state", "installed"),
					resource.TestCheckResourceAttr("p0_jira.example", "cloud_id", jiraCloudId),
					resource.TestCheckResourceAttr("p0_jira.example", "hosting.account_id", "123456789012"),
					resource.TestCheckNoResourceAttr("p0_jira.example", "hosting.connector_service_account"),
					resource.TestCheckNoResourceAttr("p0_jira.example", "hosting.connector_service_uri"),
					resource.TestCheckResourceAttr("p0_jira.example", "vault.secrets_region", "us-west-2"),
					func(*terraform.State) error {
						if got := f.writeLog(); !slices.Equal(got, jiraInstallWrites) {
							return fmt.Errorf("writes = %v; want %v", got, jiraInstallWrites)
						}
						item := f.site()
						if item["clientId"] != jiraClientId || item["domainPattern"] != jiraDomains || item["clientSecretName"] != jiraAws.secret {
							return fmt.Errorf("P0 item doesn't hold the configured site: %v", item)
						}
						return nil
					},
				),
			},
			{
				ResourceName:                         "p0_jira.example",
				ImportState:                          true,
				ImportStateId:                        jiraSite,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "site",
			},
			// The host goes into a URL path as it is, so one that P0 would refuse isn't read.
			{
				ResourceName:  "p0_jira.example",
				ImportState:   true,
				ImportStateId: "acme.atlassian.net/other",
				ExpectError:   harborError("Invalid Jira site", "Enter the site's host in lowercase", `Got: "acme.atlassian.net/other"`),
			},
		},
	})
}

// The site on Google Cloud: staging it reads the service account that P0 names after
// the connector, and installing it reads the Cloud Run URL that P0 resolves.
func TestJiraOnGoogleCloud(t *testing.T) {
	f := newJiraFake(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkJiraDestroyed(f),
		Steps: []resource.TestStep{
			{
				Config: jiraConfig(f, jiraGcp),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("p0_jira.example", "state", "installed"),
					resource.TestCheckResourceAttr("p0_jira.example", "cloud_id", jiraCloudId),
					resource.TestCheckResourceAttr("p0_jira.example", "hosting.project_id", jiraProject),
					resource.TestCheckResourceAttr("p0_jira.example", "hosting.connector_service_account", jiraAccount),
					resource.TestCheckResourceAttr("p0_jira.example", "hosting.connector_service_uri", jiraServiceUri),
					resource.TestCheckResourceAttr("p0_jira.example", "vault.project_id", "acme-secrets"),
					resource.TestCheckNoResourceAttr("p0_jira.example", "vault.account_id"),
					func(*terraform.State) error {
						if got := f.writeLog(); !slices.Equal(got, jiraInstallWrites) {
							return fmt.Errorf("writes = %v; want %v", got, jiraInstallWrites)
						}
						return nil
					},
				),
			},
			{
				ResourceName:                         "p0_jira.example",
				ImportState:                          true,
				ImportStateId:                        jiraSite,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "site",
			},
		},
	})
}

// A new client ID updates the site in place: an installed site gets the configure step
// alone, with the new client ID, and keeps its cloud ID.
func TestJiraClientIdUpdate(t *testing.T) {
	f := newJiraFake(t)
	newClientId := jiraSiteArgs{site: jiraSite, domainPattern: jiraDomains, clientId: "ZyXwVuTsRqPoNmLkJiHgFeDcBa543210"}

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkJiraDestroyed(f),
		Steps: []resource.TestStep{
			{Config: jiraConfig(f, jiraGcp)},
			{
				Config: providerConfig(f.fakeP0) + jiraResource(jiraGcp, newClientId),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("p0_jira.example", plancheck.ResourceActionUpdate),
						plancheck.ExpectKnownValue("p0_jira.example", tfjsonpath.New("cloud_id"), knownvalue.StringExact(jiraCloudId)),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("p0_jira.example", "client_id", newClientId.clientId),
					resource.TestCheckResourceAttr("p0_jira.example", "state", "installed"),
					resource.TestCheckResourceAttr("p0_jira.example", "hosting.connector_service_uri", jiraServiceUri),
					func(*terraform.State) error {
						log := f.writeLog()
						if last := log[len(log)-1]; len(log) != 4 || last != jiraInstallWrites[2] {
							return fmt.Errorf("writes = %v; want the install, then one configure", log)
						}
						if item := f.site(); item["clientId"] != newClientId.clientId {
							return fmt.Errorf("P0 item = %v; want the new client ID", item)
						}
						return nil
					},
				),
			},
		},
	})
}

// A site that fails P0's install check fails the apply with P0's message, and the
// next apply replaces it.
func TestJiraFailedCheck(t *testing.T) {
	f := newJiraFake(t)
	f.failedCheck = "The Jira site acme.atlassian.net failed its install check while checking the service account's Jira global permissions: " +
		"The connector's service account on acme.atlassian.net lacks the global permission ADMINISTER (Administer Jira)."

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkJiraDestroyed(f),
		Steps: []resource.TestStep{
			{
				Config:      jiraConfig(f, jiraAws),
				ExpectError: harborError("Jira install check failed", `Jira site "acme.atlassian.net"`, f.failedCheck),
			},
			{
				PreConfig: func() {
					if item := f.site(); item == nil || item["state"] != "configure" {
						t.Errorf("P0 item after the failed check = %v; want it at configure", item)
					}
					f.failedCheck = ""
				},
				Config: jiraConfig(f, jiraAws),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("p0_jira.example", plancheck.ResourceActionReplace),
					},
				},
				Check: resource.TestCheckResourceAttr("p0_jira.example", "state", "installed"),
			},
		},
	})
}

// A site that P0 hasn't finished installing, such as one added in the P0 app and
// imported before its checks passed, plans an update, which finishes the install
// and reads what P0 sets on the way.
func TestJiraImportedUnfinishedSite(t *testing.T) {
	f := newJiraFake(t)
	f.integrations["jira"] = true
	f.items[fakeItemKey{"jira", "site", jiraSite}] = map[string]any{
		"hosting": map[string]any{
			"type": "gcp", "projectId": jiraProject, "connectorName": jiraConnectorName, "connectorRegion": "us-west1",
			"connectorServiceAccount": jiraAccount,
		},
		"vault":            map[string]any{"type": "gcp-sm", "install": "acme-secrets"},
		"clientSecretName": jiraGcp.secret,
		"domainPattern":    jiraDomains,
		"cloudId":          "",
		"label":            jiraSite,
		"state":            "stage",
	}

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkJiraDestroyed(f),
		Steps: []resource.TestStep{
			{
				Config:             jiraConfig(f, jiraGcp),
				ResourceName:       "p0_jira.example",
				ImportState:        true,
				ImportStateId:      jiraSite,
				ImportStatePersist: true,
			},
			{
				Config: jiraConfig(f, jiraGcp),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("p0_jira.example", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("p0_jira.example", "state", "installed"),
					resource.TestCheckResourceAttr("p0_jira.example", "cloud_id", jiraCloudId),
					resource.TestCheckResourceAttr("p0_jira.example", "hosting.connector_service_uri", jiraServiceUri),
				),
			},
		},
	})
}

// A site that P0 has installed, such as one added in the P0 app, isn't staged again:
// P0 refuses the PUT, and the apply fails with its message.
func TestJiraSiteThatP0Installed(t *testing.T) {
	f := newJiraFake(t)
	f.integrations["jira"] = true
	f.items[fakeItemKey{"jira", "site", jiraSite}] = map[string]any{
		"hosting":          map[string]any{"type": "aws", "accountId": "123456789012", "connectorName": jiraConnectorName, "connectorRegion": "us-west-2"},
		"vault":            map[string]any{"type": "aws-sm", "install": "123456789012", "secretsRegion": "us-west-2"},
		"clientSecretName": jiraAws.secret,
		"domainPattern":    jiraDomains,
		"clientId":         jiraClientId,
		"cloudId":          jiraCloudId,
		"label":            jiraSite,
		"state":            "installed",
	}

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      jiraConfig(f, jiraAws),
				ExpectError: harborError("Could not stage site component", "409 Conflict: The Jira site acme.atlassian.net is already installed"),
			},
		},
	})
	if item := f.site(); item["state"] != "installed" {
		t.Errorf("P0 item = %v; want it still installed", item)
	}
}

// Every attribute but the client ID is fixed when P0 creates the site, so a change to
// one replaces it.
func TestJiraReplacement(t *testing.T) {
	f := newJiraFake(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkJiraDestroyed(f),
		Steps: []resource.TestStep{
			{Config: jiraConfig(f, jiraAws)},
			{
				Config: providerConfig(f.fakeP0) + jiraResource(jiraAws, jiraSiteArgs{site: jiraSite, domainPattern: `(.+\.)?example\.com`, clientId: jiraClientId}),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("p0_jira.example", plancheck.ResourceActionReplace),
					},
				},
				Check: func(*terraform.State) error {
					if item := f.site(); item["domainPattern"] != `(.+\.)?example\.com` || item["state"] != "installed" {
						return fmt.Errorf("P0 item = %v; want the installed site with the new pattern", item)
					}
					return nil
				},
			},
			{
				Config: jiraConfig(f, jiraGcp),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("p0_jira.example", plancheck.ResourceActionReplace),
					},
				},
				Check: resource.TestCheckResourceAttr("p0_jira.example", "hosting.connector_service_uri", jiraServiceUri),
			},
		},
	})
}

// A plan refuses what P0 would refuse, in P0's words, before anything is sent.
func TestJiraValidation(t *testing.T) {
	with := func(cloud jiraCloud, change func(*jiraCloud)) jiraCloud {
		change(&cloud)
		return cloud
	}
	args := func(change func(*jiraSiteArgs)) jiraSiteArgs {
		a := jiraDefaults
		change(&a)
		return a
	}
	awsVaultWithProject := with(jiraAws, func(c *jiraCloud) {
		c.vault = `{
    type           = "aws-sm"
    account_id     = "123456789012"
    secrets_region = "us-west-2"
    project_id     = "acme-secrets"
  }`
	})
	crossAccountAws := with(jiraAws, func(c *jiraCloud) {
		c.vault = `{
    type           = "aws-sm"
    account_id     = "210987654321"
    secrets_region = "us-west-2"
  }`
	})

	for _, tc := range []struct {
		name   string
		cloud  jiraCloud
		args   jiraSiteArgs
		errors []string
	}{
		{"site with a scheme", jiraAws, args(func(a *jiraSiteArgs) { a.site = "https://acme.atlassian.net" }), []string{"Invalid Jira site", "Enter the site's host in lowercase"}},
		{"site in capitals", jiraAws, args(func(a *jiraSiteArgs) { a.site = "Acme.atlassian.net" }), []string{"Invalid Jira site"}},
		{"site on another domain", jiraAws, args(func(a *jiraSiteArgs) { a.site = "acme.jira.com" }), []string{"Invalid Jira site"}},
		{
			"secret in Google Secret Manager for a connector on AWS",
			with(jiraAws, func(c *jiraCloud) { c.vault = jiraGcp.vault }),
			jiraDefaults,
			[]string{"Vault in another cloud than the connector", "The connector runs on AWS, so the client secret has to be in AWS Secrets Manager."},
		},
		{
			"secret in AWS Secrets Manager for a connector on Google Cloud",
			with(jiraGcp, func(c *jiraCloud) { c.vault = jiraAws.vault; c.secret = jiraAws.secret }),
			jiraDefaults,
			[]string{"Vault in another cloud than the connector", "The connector runs on Google Cloud, so the client secret has to be in Google Secret Manager."},
		},
		{"AWS connector name ECR wouldn't take", with(jiraAws, func(c *jiraCloud) { c.hosting = strings.Replace(jiraAws.hosting, "p0-jira-acme", "p0--jira", 1) }), jiraDefaults, []string{"Invalid connector name", "Use 2 to 64 characters"}},
		{"Google Cloud connector name too short", with(jiraGcp, func(c *jiraCloud) { c.hosting = strings.Replace(jiraGcp.hosting, "p0-jira-acme", "p0-jr", 1) }), jiraDefaults, []string{"Invalid connector name", "Use 6 to 30 characters"}},
		{"GovCloud connector", with(jiraAws, func(c *jiraCloud) { c.hosting = strings.Replace(jiraAws.hosting, "us-west-2", "us-gov-west-1", 1) }), jiraDefaults, []string{"Unsupported AWS region for the connector", "Jira supports only AWS commercial regions."}},
		{"Google Cloud zone for a region", with(jiraGcp, func(c *jiraCloud) { c.hosting = strings.Replace(jiraGcp.hosting, "us-west1", "us-west1-a", 1) }), jiraDefaults, []string{"Enter the Google Cloud region the connector runs in"}},
		{"Google Cloud project with an underscore", with(jiraGcp, func(c *jiraCloud) { c.hosting = strings.Replace(jiraGcp.hosting, "acme-prod", "acme_prod", 1) }), jiraDefaults, []string{"Enter the ID of the Google Cloud project the connector runs in: 6 to 30 lowercase letters"}},
		{"Google Cloud project number", with(jiraGcp, func(c *jiraCloud) { c.hosting = strings.Replace(jiraGcp.hosting, "acme-prod", "123456789012", 1) }), jiraDefaults, []string{"Invalid Google Cloud project for the connector", "Not the project's number or name."}},
		{"secret's project that isn't a project ID", with(jiraGcp, func(c *jiraCloud) { c.vault = strings.Replace(jiraGcp.vault, "acme-secrets", "Acme", 1) }), jiraDefaults, []string{"Choose the Google Secret Manager installation"}},
		{"secret in a China region", with(jiraAws, func(c *jiraCloud) { c.vault = strings.Replace(jiraAws.vault, "us-west-2", "cn-north-1", 1) }), jiraDefaults, []string{"Unsupported AWS region for the client secret"}},
		{"secret's account that isn't one", with(jiraAws, func(c *jiraCloud) { c.vault = strings.Replace(jiraAws.vault, "123456789012", "1234", 1) }), jiraDefaults, []string{"Choose the AWS Secrets Manager installation, by its 12-digit AWS account ID"}},
		{"project on an AWS secret", awsVaultWithProject, jiraDefaults, []string{"Unexpected vault field"}},
		{"secret ARN", with(jiraAws, func(c *jiraCloud) { c.secret = "arn:aws:secretsmanager:us-west-2:123456789012:secret:jira" }), jiraDefaults, []string{"Invalid client secret name", "Enter the name of the secret that holds the Jira client secret"}},
		{"Google secret's resource name", with(jiraGcp, func(c *jiraCloud) { c.secret = "projects/acme-secrets/secrets/jira" }), jiraDefaults, []string{"Invalid client secret name", "That isn't a secret's name in Google Secret Manager"}},
		{"secret in another account named like an ARN suffix", with(crossAccountAws, func(c *jiraCloud) { c.secret = "jira/acme-AbCdEf" }), jiraDefaults, []string{"Client secret name ends like an ARN suffix"}},
		{"blank domain pattern", jiraAws, args(func(a *jiraSiteArgs) { a.domainPattern = "" }), []string{"Missing domain pattern", "Allowed email domains are required"}},
		{"domain pattern over 1000 characters", jiraAws, args(func(a *jiraSiteArgs) { a.domainPattern = strings.Repeat("a", 1001) }), []string{"Allowed email domains must be at most 1000 characters"}},
		{"domain pattern with a trailing newline", jiraAws, args(func(a *jiraSiteArgs) { a.domainPattern = jiraDomains + "\n" }), []string{"Whitespace around a value"}},
		{"client ID with a space in it", jiraAws, args(func(a *jiraSiteArgs) { a.clientId = "abc def" }), []string{"Invalid client ID", "not its secret"}},
		{"blank client ID", jiraAws, args(func(a *jiraSiteArgs) { a.clientId = "" }), []string{"Missing client ID"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newJiraFake(t)
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{
						Config:      providerConfig(f.fakeP0) + jiraResource(tc.cloud, tc.args),
						PlanOnly:    true,
						ExpectError: harborError(tc.errors...),
					},
				},
			})
			if len(f.writes) != 0 {
				t.Errorf("P0 got %d writes; want none", len(f.writes))
			}
		})
	}

	// A secret in the connector's own account is read by its name, which may end like
	// an ARN's suffix.
	t.Run("secret in the connector's account named like an ARN suffix", func(t *testing.T) {
		f := newJiraFake(t)
		resource.UnitTest(t, resource.TestCase{
			ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
			CheckDestroy:             checkJiraDestroyed(f),
			Steps: []resource.TestStep{
				{
					Config: providerConfig(f.fakeP0) + jiraResource(with(jiraAws, func(c *jiraCloud) { c.secret = "jira/acme-AbCdEf" }), jiraDefaults),
					Check:  resource.TestCheckResourceAttr("p0_jira.example", "state", "installed"),
				},
			},
		})
	})
}

// The variables that name what TestAccJira installs: a Jira connector already deployed
// on AWS Lambda where they say, running as the role P0 names after it, with its
// DOMAIN_PATTERN set to P0_JIRA_DOMAIN_PATTERN, and a site whose service account's
// OAuth client secret is in a secret that the connector's role can read, in the
// connector's account. P0 checks all of it through the connector, so it must be real.
func jiraAccEnv(install *jiraAccInstall) map[string]*string {
	return map[string]*string{
		"P0_JIRA_SITE":               &install.site,
		"P0_JIRA_AWS_ACCOUNT_ID":     &install.accountId,
		"P0_JIRA_CONNECTOR_NAME":     &install.connectorName,
		"P0_JIRA_CONNECTOR_REGION":   &install.connectorRegion,
		"P0_JIRA_SECRETS_REGION":     &install.secretsRegion,
		"P0_JIRA_CLIENT_SECRET_NAME": &install.clientSecretName,
		"P0_JIRA_DOMAIN_PATTERN":     &install.domainPattern,
		"P0_JIRA_CLIENT_ID":          &install.clientId,
	}
}

type jiraAccInstall struct {
	site, accountId, connectorName, connectorRegion string
	secretsRegion, clientSecretName, domainPattern  string
	clientId                                        string
}

// TestAccJira installs into the P0 organization P0_ORG, served at P0_HOST (the
// provider's default host unless set), with the API token P0_API_TOKEN. Use an
// organization set aside for tests: it creates and deletes a real install.
//
// A copy of TestAcc1Password's, so that the two tests don't depend on each other.
const (
	jiraAccOrgEnv      = "P0_ORG"
	jiraAccHostEnv     = "P0_HOST"
	jiraAccTokenEnv    = "P0_API_TOKEN"
	jiraAccDefaultHost = "https://api.p0.app"
)

// Skips TestAccJira unless it has a P0 organization to run against and every
// P0_JIRA_* variable is set. It skips rather than fails, because CI sets TF_ACC
// without an organization.
func testAccJiraPreCheck(t *testing.T) {
	if os.Getenv(jiraAccOrgEnv) == "" || os.Getenv(jiraAccTokenEnv) == "" {
		t.Skipf("Set %s and %s to run the Jira acceptance tests against a P0 organization", jiraAccOrgEnv, jiraAccTokenEnv)
	}
	var missing []string
	for name := range jiraAccEnv(&jiraAccInstall{}) {
		if os.Getenv(name) == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		slices.Sort(missing)
		t.Skipf("Set %s to run the Jira acceptance tests", strings.Join(missing, ", "))
	}
}

func jiraAccHost() string {
	if host := os.Getenv(jiraAccHostEnv); host != "" {
		return host
	}
	return jiraAccDefaultHost
}

// Calls the acceptance-test organization's API directly, to check that a destroyed
// site is gone.
func jiraAccClient() *internal.P0ProviderData {
	return &internal.P0ProviderData{
		BaseUrl:        fmt.Sprintf("%s/o/%s", jiraAccHost(), os.Getenv(jiraAccOrgEnv)),
		Authentication: "Bearer " + os.Getenv(jiraAccTokenEnv),
		Client:         http.DefaultClient,
	}
}

// The site the examples install, on AWS, for what the P0_JIRA_* variables name. The
// provider reads the API token from P0_API_TOKEN itself.
func jiraAccConfig() string {
	var install jiraAccInstall
	for name, value := range jiraAccEnv(&install) {
		*value = os.Getenv(name)
	}
	return fmt.Sprintf(`
provider "p0" {
  org  = %q
  host = %q
}

resource "p0_jira" "test" {
  site = %q

  hosting = {
    type             = "aws"
    account_id       = %q
    connector_name   = %q
    connector_region = %q
  }

  vault = {
    type           = "aws-sm"
    account_id     = %q
    secrets_region = %q
  }

  client_secret_name = %q
  domain_pattern     = %q
  client_id          = %q
}
`, os.Getenv(jiraAccOrgEnv), jiraAccHost(),
		install.site, install.accountId, install.connectorName, install.connectorRegion,
		install.accountId, install.secretsRegion, install.clientSecretName, install.domainPattern, install.clientId)
}

// Checks that P0 no longer has the site behind any destroyed p0_jira resource.
func checkJiraAccDestroyed(client *internal.P0ProviderData) resource.TestCheckFunc {
	return func(state *terraform.State) error {
		for _, rs := range state.RootModule().Resources {
			if rs.Type != "p0_jira" {
				continue
			}
			site := rs.Primary.Attributes["site"]
			resp, err := client.Get("integrations/jira/config/site/"+site, &map[string]any{})
			if resp != nil && resp.StatusCode == http.StatusNotFound {
				continue
			}
			if err != nil {
				return fmt.Errorf("could not check that the Jira site %s was deleted: %w", site, err)
			}
			return fmt.Errorf("the Jira site %s is still installed in P0", site)
		}
		return nil
	}
}

// TestAccJira installs a site in a P0 organization (see testAccJiraPreCheck), checks
// that a second plan is empty, imports it, and destroys it. The organization must run
// a P0 version with the Jira integration.
func TestAccJira(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccJiraPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkJiraAccDestroyed(jiraAccClient()),
		Steps: []resource.TestStep{
			{
				Config: jiraAccConfig(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("p0_jira.test", "state", "installed"),
					resource.TestCheckResourceAttrSet("p0_jira.test", "cloud_id"),
				),
			},
			{
				ResourceName:                         "p0_jira.test",
				ImportState:                          true,
				ImportStateIdFunc:                    jiraAccImportId,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "site",
			},
		},
	})
}

func jiraAccImportId(state *terraform.State) (string, error) {
	rs, ok := state.RootModule().Resources["p0_jira.test"]
	if !ok {
		return "", fmt.Errorf("p0_jira.test isn't in the state")
	}
	return rs.Primary.Attributes["site"], nil
}
