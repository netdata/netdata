// SPDX-License-Identifier: GPL-3.0-or-later

package secretresolver

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

// reAzureSafeName validates Azure Key Vault and secret names (alphanumeric + hyphens).
var reAzureSafeName = regexp.MustCompile(`^[a-zA-Z0-9-]+$`)

var azureHTTPClient = &http.Client{Timeout: 10 * time.Second}
var azureIMDSHTTPClient = &http.Client{
	Timeout:   2 * time.Second,
	Transport: &http.Transport{Proxy: nil}, // IMDS must never be proxied
}

func resolveAzureKV(ref, original string) (string, error) {
	vaultName, secretName, ok := strings.Cut(ref, "/")
	if !ok || vaultName == "" || secretName == "" {
		return "", fmt.Errorf("resolving secret '%s': reference must be in format 'vault-name/secret-name'", original)
	}
	if !reAzureSafeName.MatchString(vaultName) {
		return "", fmt.Errorf("resolving secret '%s': invalid vault name '%s' (must contain only alphanumeric characters and hyphens)", original, vaultName)
	}
	if !reAzureSafeName.MatchString(secretName) {
		return "", fmt.Errorf("resolving secret '%s': invalid secret name '%s' (must contain only alphanumeric characters and hyphens)", original, secretName)
	}

	token, err := azureGetAccessToken()
	if err != nil {
		return "", fmt.Errorf("resolving secret '%s': %w", original, err)
	}

	secretURL := fmt.Sprintf("https://%s.vault.azure.net/secrets/%s?api-version=7.4", vaultName, secretName)

	req, err := http.NewRequest(http.MethodGet, secretURL, nil)
	if err != nil {
		return "", fmt.Errorf("resolving secret '%s': creating request: %w", original, err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := azureHTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("resolving secret '%s': request failed: %w", original, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("resolving secret '%s': reading response: %w", original, err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("resolving secret '%s': Azure Key Vault returned HTTP %d: %s", original, resp.StatusCode, truncateBody(body))
	}

	var result struct {
		Value string `json:"value"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("resolving secret '%s': parsing response: %w", original, err)
	}

	return result.Value, nil
}

func azureGetAccessToken() (string, error) {
	// Try client credentials first (if all env vars are present).
	tenantID := os.Getenv("AZURE_TENANT_ID")
	clientID := os.Getenv("AZURE_CLIENT_ID")
	clientSecret := os.Getenv("AZURE_CLIENT_SECRET")

	if tenantID != "" && clientID != "" && clientSecret != "" {
		// All three client credential vars are set — use them exclusively.
		return azureGetTokenClientCredentials(tenantID, clientID, clientSecret)
	}

	// Fall back to managed identity (IMDS).
	return azureGetTokenManagedIdentity()
}

func azureGetTokenClientCredentials(tenantID, clientID, clientSecret string) (string, error) {
	tokenURL := fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/token", tenantID)

	form := url.Values{
		"client_id":     {clientID},
		"client_secret": {clientSecret},
		"scope":         {"https://vault.azure.net/.default"},
		"grant_type":    {"client_credentials"},
	}

	resp, err := azureHTTPClient.PostForm(tokenURL, form)
	if err != nil {
		return "", fmt.Errorf("client credentials token request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("reading client credentials token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("client credentials token request returned HTTP %d: %s", resp.StatusCode, truncateBody(body))
	}

	var result struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("parsing client credentials token response: %w", err)
	}

	if result.AccessToken == "" {
		return "", fmt.Errorf("client credentials token response missing access_token")
	}

	return result.AccessToken, nil
}

func azureGetTokenManagedIdentity() (string, error) {
	reqURL := "http://169.254.169.254/metadata/identity/oauth2/token?api-version=2018-02-01&resource=https://vault.azure.net"

	if clientID := os.Getenv("AZURE_CLIENT_ID"); clientID != "" {
		reqURL += "&client_id=" + url.QueryEscape(clientID)
	}

	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return "", fmt.Errorf("creating managed identity token request: %w", err)
	}
	req.Header.Set("Metadata", "true")

	resp, err := azureIMDSHTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("managed identity token request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("reading managed identity token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("managed identity token request returned HTTP %d: %s", resp.StatusCode, truncateBody(body))
	}

	var result struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("parsing managed identity token response: %w", err)
	}

	if result.AccessToken == "" {
		return "", fmt.Errorf("managed identity token response missing access_token")
	}

	return result.AccessToken, nil
}
