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
//   - handles authentication
//   - converts to/from JSON
//   - treats response codes 400 and higher as errors
type P0ProviderClient struct {
	BaseUrl        string
	Authentication string
	Client         *http.Client
}

func (c *P0ProviderClient) Do(req *http.Request, responseJson any) (*http.Response, error) {
	req.Header.Add("Accept", "application/json")

	resp, errDo := c.Client.Do(req)
	if errDo != nil {
		return resp, errDo
	}
	defer resp.Body.Close()

	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return resp, readErr
	}

	parseErr := json.Unmarshal(body, &responseJson)
	if parseErr != nil {
		return resp, parseErr
	}

	// If the response contains "error", throw that here
	var generic map[string]any
	genericErr := json.Unmarshal(body, &generic)
	if genericErr != nil {
		return resp, genericErr
	}
	if generic["error"] != nil || resp.StatusCode >= 400 {
		errorText, ok := generic["error"].(string)
		if !ok {
			errorErr := fmt.Errorf(
				"got error response from P0, but error was not a string. Please contact support@p0.dev. HTTP status code: %s",
				resp.Status,
			)
			return resp, errorErr
		}
		return resp, fmt.Errorf("%s: %s", resp.Status, errorText)
	}

	return resp, nil
}

func (c *P0ProviderClient) Get(path string, responseJson any) (*http.Response, error) {
	req, errNew := http.NewRequest("GET", fmt.Sprintf("%s/%s", c.BaseUrl, path), nil)
	req.Header.Add("Accept", "application/json")
	req.Header.Add("Authorization", c.Authentication)
	if errNew != nil {
		return nil, errNew
	}
	return c.Do(req, responseJson)
}

func (c *P0ProviderClient) Delete(path string) (*http.Response, error) {
	req, errNew := http.NewRequest("DELETE", fmt.Sprintf("%s/%s", c.BaseUrl, path), nil)
	req.Header.Add("Authorization", c.Authentication)
	if errNew != nil {
		return nil, errNew
	}

	resp, errDo := c.Client.Do(req)
	if errDo != nil {
		return resp, errDo
	}
	defer resp.Body.Close()

	// Endpoints should not return content
	// TODO: Render actual error
	if resp.StatusCode != 204 {
		return resp, fmt.Errorf("unexpected return code during delete: %d", resp.StatusCode)
	}
	return resp, nil
}

func (c *P0ProviderClient) doBody(method string, path string, requestJson any, responseJson any) (*http.Response, error) {
	buf, marshalErr := json.Marshal(&requestJson)
	if marshalErr != nil {
		return nil, marshalErr
	}

	reader := bytes.NewReader(buf)

	req, errNew := http.NewRequest(method, fmt.Sprintf("%s/%s", c.BaseUrl, path), reader)
	req.Header.Add("Accept", "application/json")
	req.Header.Add("Authorization", c.Authentication)
	req.Header.Add("Content-Type", "application/json")
	if errNew != nil {
		return nil, errNew
	}
	return c.Do(req, responseJson)
}

func (c *P0ProviderClient) Post(path string, requestJson any, responseJson any) (*http.Response, error) {
	return c.doBody("POST", path, requestJson, responseJson)
}

func (c *P0ProviderClient) Put(path string, requestJson any, responseJson any) (*http.Response, error) {
	return c.doBody("PUT", path, requestJson, responseJson)
}
