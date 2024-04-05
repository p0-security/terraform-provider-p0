// Copyright (c) 2024 P0 Security, Inc
// SPDX-License-Identifier: MPL-2.0

package internal

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Like http.Client, except:
// - handles authentication
// - converts to/from JSON
// - treats response codes 400 and higher as errors
type P0ProviderData struct {
	BaseUrl        string
	Authentication string
	Client         *http.Client
}

func (data *P0ProviderData) Do(req *http.Request, value any) error {
	req.Header.Add("Accept", "application/json")

	resp, errDo := data.Client.Do(req)
	if errDo != nil {
		return errDo
	}
	defer resp.Body.Close()

	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return readErr
	}

	parseErr := json.Unmarshal(body, &value)
	if parseErr != nil {
		return parseErr
	}

	// If the response contains "error", throw that here
	var generic map[string]any
	genericErr := json.Unmarshal(body, &generic)
	if genericErr != nil {
		return genericErr
	}
	if generic["error"] != nil || resp.StatusCode >= 400 {
		errorText, ok := generic["error"].(string)
		if !ok {
			return fmt.Errorf(
				"got error response from P0, but error was not a string. Please contact support@p0.dev. HTTP status code: %s",
				resp.Status,
			)
		}
		return fmt.Errorf("%s: %s", resp.Status, errorText)
	}

	return nil
}

func (data *P0ProviderData) Get(path string, value any) error {
	req, errNew := http.NewRequest("GET", fmt.Sprintf("%s/%s", data.BaseUrl, path), nil)
	req.Header.Add("Accept", "application/json")
	req.Header.Add("Authorization", data.Authentication)
	if errNew != nil {
		return errNew
	}
	return data.Do(req, value)
}

func (data *P0ProviderData) Delete(path string) error {
	req, errNew := http.NewRequest("DELETE", fmt.Sprintf("%s/%s", data.BaseUrl, path), nil)
	req.Header.Add("Authorization", data.Authentication)
	if errNew != nil {
		return errNew
	}

	resp, errDo := data.Client.Do(req)
	if errDo != nil {
		return errDo
	}
	defer resp.Body.Close()

	// Endpoints should not return content
	// TODO: Render actual error
	if resp.StatusCode != 204 {
		return fmt.Errorf("unexpected return code during delete: %d", resp.StatusCode)
	}
	return nil
}

func (data *P0ProviderData) doBody(method string, path string, body any, value any) error {
	buf, marshalErr := json.Marshal(&body)
	if marshalErr != nil {
		return marshalErr
	}

	reader := bytes.NewReader(buf)

	req, errNew := http.NewRequest(method, fmt.Sprintf("%s/%s", data.BaseUrl, path), reader)
	req.Header.Add("Accept", "application/json")
	req.Header.Add("Authorization", data.Authentication)
	req.Header.Add("Content-Type", "application/json")
	if errNew != nil {
		return errNew
	}
	return data.Do(req, value)
}

func (data *P0ProviderData) Post(path string, body any, value any) error {
	return data.doBody("POST", path, body, value)
}

func (data *P0ProviderData) Put(path string, body any, value any) error {
	return data.doBody("PUT", path, body, value)
}
