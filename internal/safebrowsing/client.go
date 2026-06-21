// Package safebrowsing checks candidate URLs against the Google Safe
// Browsing API at link-creation time. If no API key is configured the
// check is skipped (logged once at startup) rather than blocking link
// creation — see SPEC.md section 8.
package safebrowsing

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

const apiURL = "https://safebrowsing.googleapis.com/v4/threatMatches:find"

type Client struct {
	apiKey     string
	httpClient *http.Client
}

func New(apiKey string) *Client {
	return &Client{apiKey: apiKey, httpClient: &http.Client{}}
}

// Enabled reports whether an API key was configured.
func (c *Client) Enabled() bool {
	return c.apiKey != ""
}

type threatMatchesRequest struct {
	Client     clientInfo `json:"client"`
	ThreatInfo threatInfo `json:"threatInfo"`
}

type clientInfo struct {
	ClientID      string `json:"clientId"`
	ClientVersion string `json:"clientVersion"`
}

type threatInfo struct {
	ThreatTypes      []string      `json:"threatTypes"`
	PlatformTypes    []string      `json:"platformTypes"`
	ThreatEntryTypes []string      `json:"threatEntryTypes"`
	ThreatEntries    []threatEntry `json:"threatEntries"`
}

type threatEntry struct {
	URL string `json:"url"`
}

type threatMatchesResponse struct {
	Matches []json.RawMessage `json:"matches"`
}

// IsMalicious checks rawURL against Safe Browsing's malware/phishing
// lists. If the client is not Enabled, it always returns false, nil.
func (c *Client) IsMalicious(ctx context.Context, rawURL string) (bool, error) {
	if !c.Enabled() {
		return false, nil
	}

	body := threatMatchesRequest{
		Client: clientInfo{ClientID: "url-shortener", ClientVersion: "1.0.0"},
		ThreatInfo: threatInfo{
			ThreatTypes:      []string{"MALWARE", "SOCIAL_ENGINEERING", "UNWANTED_SOFTWARE"},
			PlatformTypes:    []string{"ANY_PLATFORM"},
			ThreatEntryTypes: []string{"URL"},
			ThreatEntries:    []threatEntry{{URL: rawURL}},
		},
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return false, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		apiURL+"?key="+c.apiKey, bytes.NewReader(payload))
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("safe browsing API returned status %d", resp.StatusCode)
	}

	var result threatMatchesResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, err
	}
	return len(result.Matches) > 0, nil
}
