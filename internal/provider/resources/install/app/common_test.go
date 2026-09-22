package installapp

import (
	"strings"
	"testing"
)

func TestConnectorRegionRegex(t *testing.T) {
	cases := map[string]bool{
		"us-east-1":               true,
		"us-gov-west-1":           true,
		"ap-southeast-4":          true,
		"us-central1":             true,
		"northamerica-northeast1": true,
		// Google Cloud's numbering runs into two digits.
		"europe-west10": true,
		"europe-west12": true,
		"useast1":       false,
		"us-east":       false,
		"us-east-123":   false,
		"US-EAST-1":     false,
	}

	for region, want := range cases {
		if got := ConnectorRegionRegex.MatchString(region); got != want {
			t.Errorf("ConnectorRegionRegex.MatchString(%q) = %v, want %v", region, got, want)
		}
	}
}

func TestCloudRunServiceNameRegex(t *testing.T) {
	cases := map[string]bool{
		"p0-custom-app-connector": true,
		"connector1":              true,
		"c":                       true,
		strings.Repeat("a", 49):   true,
		strings.Repeat("a", 50):   false,
		"P0-Connector":            false,
		"p0_connector":            false,
		"p0-connector-":           false,
		"1connector":              false,
		"":                        false,
	}

	for name, want := range cases {
		if got := CloudRunServiceNameRegex.MatchString(name); got != want {
			t.Errorf("CloudRunServiceNameRegex.MatchString(%q) = %v, want %v", name, got, want)
		}
	}
}
