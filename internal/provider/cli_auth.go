// Copyright (c) HashiCorp, Inc. and P0 Security, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// p0Identity mirrors the subset of ~/.p0/identity.json that the provider reads.
type p0Identity struct {
	Credential struct {
		AccessToken string  `json:"access_token"`
		IdToken     string  `json:"id_token"`
		ExpiresAt   float64 `json:"expires_at"`
	} `json:"credential"`
	Org struct {
		TenantId    string `json:"tenantId"`
		SsoProvider string `json:"ssoProvider"`
	} `json:"org"`
}

// p0CliConfig mirrors the subset of ~/.p0/config.json that the provider reads.
type p0CliConfig struct {
	Fs struct {
		ApiKey string `json:"apiKey"`
	} `json:"fs"`
}

// p0ConfigDir returns the directory the P0 CLI persists its state to: ~/.p0,
// suffixed with -$P0_ENV when that variable is set.
func p0ConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := ".p0"
	if env, ok := os.LookupEnv("P0_ENV"); ok {
		dir = dir + "-" + env
	}
	return filepath.Join(home, dir), nil
}

// loadJsonFile reads and unmarshals the JSON file at path into v.
func loadJsonFile(path string, v any) error {
	contents, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(contents, v)
}

// firebaseProviderId maps a P0 org's SSO provider to the Firebase Auth provider
// ID used to exchange the CLI credential, mirroring the CLI's findProviderId.
func firebaseProviderId(ssoProvider string) (string, error) {
	switch ssoProvider {
	case "google":
		return "google.com", nil
	case "google-oidc":
		return "oidc.google-oidc", nil
	case "":
		return "", fmt.Errorf("password-based P0 logins are not supported by the Terraform provider; set the `api_token` attribute or P0_API_TOKEN instead")
	default:
		return "", fmt.Errorf("P0 SSO provider %q is not supported for CLI authentication; set the `api_token` attribute or P0_API_TOKEN instead", ssoProvider)
	}
}

// exchangeForFirebaseToken trades the OIDC credential issued to the P0 CLI for a
// Firebase ID token scoped to the org's tenant, mirroring the CLI's
// signInWithCredential call against Identity Toolkit.
func exchangeForFirebaseToken(ctx context.Context, apiKey, tenantId, providerId, idToken, accessToken string) (string, error) {
	postBody := url.Values{}
	postBody.Set("id_token", idToken)
	postBody.Set("access_token", accessToken)
	postBody.Set("providerId", providerId)

	reqBody, err := json.Marshal(map[string]any{
		"postBody":            postBody.Encode(),
		"requestUri":          "http://localhost",
		"returnIdpCredential": true,
		"returnSecureToken":   true,
		"tenantId":            tenantId,
	})
	if err != nil {
		return "", err
	}

	endpoint := fmt.Sprintf("https://identitytoolkit.googleapis.com/v1/accounts:signInWithIdp?key=%s", url.QueryEscape(apiKey))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(reqBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Firebase token exchange failed (%s): %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var result struct {
		IdToken string `json:"idToken"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", err
	}
	if result.IdToken == "" {
		return "", fmt.Errorf("Firebase token exchange returned no ID token")
	}
	return result.IdToken, nil
}

// cliFirebaseToken exchanges the credential persisted by the P0 CLI for a
// Firebase ID token that the P0 API accepts as a bearer token. It returns
// ("", nil) when the CLI has not been used (so callers can fall through to other
// auth sources), and an error when a CLI session exists but cannot be exchanged.
func cliFirebaseToken(ctx context.Context) (string, error) {
	dir, err := p0ConfigDir()
	if err != nil {
		return "", nil
	}

	var identity p0Identity
	if err := loadJsonFile(filepath.Join(dir, "identity.json"), &identity); err != nil {
		// No readable identity file: the user has not logged in with the CLI.
		return "", nil
	}

	// expires_at is a Unix timestamp in (fractional) seconds.
	expiresAt := time.Unix(0, int64(identity.Credential.ExpiresAt*float64(time.Second)))
	if !time.Now().Before(expiresAt) {
		return "", fmt.Errorf("the P0 CLI session has expired; run `p0 login` to refresh it")
	}

	var config p0CliConfig
	if err := loadJsonFile(filepath.Join(dir, "config.json"), &config); err != nil {
		return "", fmt.Errorf("could not read the P0 CLI config: %w", err)
	}
	if config.Fs.ApiKey == "" {
		return "", fmt.Errorf("the P0 CLI config is missing a Firebase API key")
	}

	providerId, err := firebaseProviderId(identity.Org.SsoProvider)
	if err != nil {
		return "", err
	}

	return exchangeForFirebaseToken(ctx, config.Fs.ApiKey,
		identity.Org.TenantId,
		providerId,
		identity.Credential.IdToken,
		identity.Credential.AccessToken)
}
