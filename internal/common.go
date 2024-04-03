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

type P0ProviderData struct {
	BaseUrl        string
	Authentication string
	Client         *http.Client
}

func (data *P0ProviderData) Do(req *http.Request, value any) (err error) {
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
		errorText, errorErr := generic["error"].(string)
		if errorErr {
			return nil
		}
		return fmt.Errorf("%s: %s", resp.Status, errorText)
	}

	return nil
}

func (data *P0ProviderData) Get(path string, value any) (err error) {
	req, errNew := http.NewRequest("GET", fmt.Sprintf("%s/%s", data.BaseUrl, path), nil)
	req.Header.Add("Authorization", data.Authentication)
	if errNew != nil {
		return errNew
	}
	return data.Do(req, value)
}

func (data *P0ProviderData) Post(path string, body []byte, value any) (err error) {
	reader := bytes.NewReader(body)
	req, errNew := http.NewRequest("POST", fmt.Sprintf("%s/%s", data.BaseUrl, path), reader)
	req.Header.Add("Authorization", data.Authentication)
	req.Header.Add("Content-Type", "application/json")
	if errNew != nil {
		return errNew
	}
	return data.Do(req, value)
}
