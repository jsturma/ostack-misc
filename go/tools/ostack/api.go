package ostack

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"time"
)

// NewClient returns an HTTP client with a 60s timeout for API calls.
func NewClient() *http.Client {
	return &http.Client{Timeout: 60 * time.Second}
}

func apiCall(client *http.Client, method, url, token string, body []byte) ([]byte, error) {
	var req *http.Request
	var err error
	if body != nil {
		req, err = http.NewRequest(method, url, bytes.NewReader(body))
	} else {
		req, err = http.NewRequest(method, url, nil)
	}
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Auth-Token", token)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("API %s %s: HTTP %d", method, url, resp.StatusCode)
	}
	return data, nil
}

func apiGet(client *http.Client, url, token string) ([]byte, error) {
	return apiCall(client, http.MethodGet, url, token, nil)
}
