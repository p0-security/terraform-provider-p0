package provider

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

const (
	driveProject = "agents-project"
	driveId      = "0AbCdEfGhIjKlMnOpQr"
	driveName    = "engineering"
)

// The GCP Service Account Creator and the Google Drive connector in one project, and
// a shared drive the connector manages, as a customer's Terraform installs them.
func googleDriveConfig(f *fakeP0, driveUrl string) string {
	return providerConfig(f) + fmt.Sprintf(`
resource "p0_gcp_service_account_creator_staged" "example" {
  project_id = %q
}

resource "p0_gcp_service_account_creator" "example" {
  project_id = p0_gcp_service_account_creator_staged.example.project_id
}

resource "p0_google_drive_connector_staged" "example" {
  project_id = p0_gcp_service_account_creator.example.project_id
}

resource "p0_google_drive_connector" "example" {
  project_id = p0_google_drive_connector_staged.example.project_id
}

resource "p0_google_drive_shared_drive" "example" {
  id         = %q
  drive_url  = %q
  project_id = p0_google_drive_connector.example.project_id
}
`, driveProject, driveName, driveUrl)
}

// Adds the fields P0's installers do: a connector's identifiers when it is staged, its
// invocation URL once it is verified, and a shared drive's ID read from its URL.
func newGoogleDriveFake(t *testing.T) *fakeP0 {
	f := newFakeP0(t)
	f.normalize = func(key fakeItemKey, item map[string]any) {
		switch {
		case key.integration == "gcp-service-account" && key.component == "iam-write",
			key.integration == "google-drive-ai" && key.component == "connector":
			service := "p0-connector-" + key.integration
			item["connectorRegion"] = "us-west1"
			item["connectorServiceName"] = service
			item["connectorServiceAccount"] = fmt.Sprintf("%s@%s.iam.gserviceaccount.com", service, key.id)
			if item["state"] != "stage" {
				item["connectorServiceUri"] = "https://" + service + ".a.run.app"
			}
		case key.integration == "google-drive-ai" && key.component == "shared-drive":
			url, _ := item["googleDriveUrl"].(string)
			if match := regexp.MustCompile(`folders/([\w-]+)`).FindStringSubmatch(url); match != nil {
				item["googleDriveId"] = match[1]
			}
		}
	}
	return f
}

func TestGoogleDrive(t *testing.T) {
	f := newGoogleDriveFake(t)
	driveUrl := "https://drive.google.com/drive/folders/" + driveId

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: func(*terraform.State) error {
			for _, key := range [][3]string{
				{"gcp-service-account", "iam-write", driveProject},
				{"google-drive-ai", "connector", driveProject},
				{"google-drive-ai", "shared-drive", driveName},
			} {
				if item := f.item(key[0], key[1], key[2]); item != nil {
					return fmt.Errorf("%s was not removed from P0: %v", key, item)
				}
			}
			return nil
		},
		Steps: []resource.TestStep{
			// The framework re-plans after the step and fails on a non-empty plan, so this
			// also checks that the fields P0 assigns leave no diff.
			{
				Config: googleDriveConfig(f, driveUrl),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("p0_gcp_service_account_creator_staged.example", "state", "stage"),
					resource.TestCheckResourceAttr("p0_gcp_service_account_creator_staged.example", "region", "us-west1"),
					resource.TestCheckResourceAttr("p0_gcp_service_account_creator_staged.example", "connector_service_name", "p0-connector-gcp-service-account"),
					resource.TestCheckResourceAttr(
						"p0_gcp_service_account_creator_staged.example", "connector_service_account",
						"p0-connector-gcp-service-account@"+driveProject+".iam.gserviceaccount.com",
					),
					resource.TestCheckResourceAttr("p0_gcp_service_account_creator.example", "state", "installed"),
					resource.TestCheckResourceAttr("p0_gcp_service_account_creator.example", "connector_service_uri", "https://p0-connector-gcp-service-account.a.run.app"),
					resource.TestCheckResourceAttr("p0_google_drive_connector_staged.example", "connector_service_name", "p0-connector-google-drive-ai"),
					resource.TestCheckResourceAttr("p0_google_drive_connector.example", "state", "installed"),
					resource.TestCheckResourceAttr("p0_google_drive_connector.example", "connector_service_uri", "https://p0-connector-google-drive-ai.a.run.app"),
					resource.TestCheckResourceAttr("p0_google_drive_shared_drive.example", "state", "installed"),
					resource.TestCheckResourceAttr("p0_google_drive_shared_drive.example", "google_drive_id", driveId),
					func(*terraform.State) error {
						item := f.item("google-drive-ai", "shared-drive", driveName)
						if item["googleDriveUrl"] != driveUrl || item["projectId"] != driveProject {
							return fmt.Errorf("P0 item does not hold the configured drive: %v", item)
						}
						return nil
					},
				),
			},
			{
				ResourceName:      "p0_gcp_service_account_creator.example",
				ImportState:       true,
				ImportStateId:     driveProject,
				ImportStateVerify: true,
				// The import's identifier attribute, not "id", which these resources lack.
				ImportStateVerifyIdentifierAttribute: "project_id",
			},
			{
				ResourceName:                         "p0_google_drive_connector.example",
				ImportState:                          true,
				ImportStateId:                        driveProject,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "project_id",
			},
			{
				ResourceName:      "p0_google_drive_shared_drive.example",
				ImportState:       true,
				ImportStateId:     driveName,
				ImportStateVerify: true,
			},
			// A drive's URL is only read when the item is created, so a new one replaces it.
			{
				Config: googleDriveConfig(f, "https://drive.google.com/drive/u/1/folders/0AnotherDrive?usp=sharing"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("p0_google_drive_shared_drive.example", "google_drive_id", "0AnotherDrive"),
					resource.TestCheckResourceAttr("p0_google_drive_shared_drive.example", "state", "installed"),
				),
			},
		},
	})
}

// Destroying only a final connector resource returns its item to "stage", so the
// staged resource still owns it.
func TestGoogleDriveConnectorRollback(t *testing.T) {
	f := newGoogleDriveFake(t)
	staged := providerConfig(f) + fmt.Sprintf(`
resource "p0_google_drive_connector_staged" "example" {
  project_id = %q
}
`, driveProject)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: staged + `
resource "p0_google_drive_connector" "example" {
  project_id = p0_google_drive_connector_staged.example.project_id
}
`,
			},
			{
				Config: staged,
				Check: func(*terraform.State) error {
					item := f.item("google-drive-ai", "connector", driveProject)
					if item == nil || item["state"] != "stage" {
						return fmt.Errorf("P0 item was not returned to stage: %v", item)
					}
					return nil
				},
			},
		},
	})
}

func TestGoogleDriveSharedDriveValidation(t *testing.T) {
	for _, tc := range []struct {
		name, id, url, error string
	}{
		{"uppercase name", "Engineering", "https://drive.google.com/drive/folders/" + driveId, "lowercase letters"},
		{"trailing hyphen", "engineering-", "https://drive.google.com/drive/folders/" + driveId, "lowercase letters"},
		{"file url", driveName, "https://drive.google.com/file/d/" + driveId + "/view", "shared drive's URL"},
		{"docs url", driveName, "https://docs.google.com/drive/folders/" + driveId, "shared drive's URL"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newGoogleDriveFake(t)
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{
						Config: providerConfig(f) + fmt.Sprintf(`
resource "p0_google_drive_shared_drive" "example" {
  id         = %q
  drive_url  = %q
  project_id = %q
}
`, tc.id, tc.url, driveProject),
						PlanOnly:    true,
						ExpectError: regexp.MustCompile(tc.error),
					},
				},
			})
		})
	}
}
