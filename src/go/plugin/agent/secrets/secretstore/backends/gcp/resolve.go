// SPDX-License-Identifier: GPL-3.0-or-later

package gcp

import (
	"context"
	"crypto"
	cryptorand "crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore/internal/httpx"
)

func (s *publishedStore) Resolve(ctx context.Context, req secretstore.ResolveRequest) (string, error) {
	return s.resolve(ctx, req)
}

func (s *publishedStore) resolve(ctx context.Context, req secretstore.ResolveRequest) (string, error) {
	project, secretName, version, ok := parseOperand(req.Operand)
	if !ok {
		return "", fmt.Errorf("resolving secret '%s': store '%s': operand must be in format 'project/secret' or 'project/secret/version'", req.Original, req.StoreKey)
	}
	if !reGCPSafeProjectID.MatchString(project) {
		return "", fmt.Errorf("resolving secret '%s': store '%s': invalid project ID '%s'", req.Original, req.StoreKey, project)
	}
	if !reGCPSafeName.MatchString(secretName) {
		return "", fmt.Errorf("resolving secret '%s': store '%s': invalid secret name '%s'", req.Original, req.StoreKey, secretName)
	}
	if !reGCPSafeName.MatchString(version) {
		return "", fmt.Errorf("resolving secret '%s': store '%s': invalid version '%s'", req.Original, req.StoreKey, version)
	}

	token, err := s.accessToken(ctx)
	if err != nil {
		return "", fmt.Errorf("resolving secret '%s': store '%s': %w", req.Original, req.StoreKey, err)
	}

	baseURL := s.provider.secretEndpoint
	if baseURL == "" {
		baseURL = "https://secretmanager.googleapis.com"
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/v1/projects/%s/secrets/%s/versions/%s:access", baseURL, project, secretName, version), nil)
	if err != nil {
		return "", fmt.Errorf("resolving secret '%s': store '%s': creating request: %w", req.Original, req.StoreKey, err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+token)
	resp, err := s.provider.apiClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("resolving secret '%s': store '%s': request failed: %w", req.Original, req.StoreKey, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("resolving secret '%s': store '%s': reading response: %w", req.Original, req.StoreKey, err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("resolving secret '%s': store '%s': GCP Secret Manager returned HTTP %d: %s", req.Original, req.StoreKey, resp.StatusCode, httpx.TruncateBody(body))
	}
	var result struct {
		Payload struct {
			Data string `json:"data"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("resolving secret '%s': store '%s': parsing response: %w", req.Original, req.StoreKey, err)
	}
	decoded, err := base64.StdEncoding.DecodeString(result.Payload.Data)
	if err != nil {
		return "", fmt.Errorf("resolving secret '%s': store '%s': decoding secret data: %w", req.Original, req.StoreKey, err)
	}
	logResolvedRequest(ctx, req, project, secretName, version)
	return string(decoded), nil
}

func logResolvedRequest(ctx context.Context, req secretstore.ResolveRequest, project, secretName, version string) {
	if log, ok := logger.LoggerFromContext(ctx); ok {
		log.Infof("resolved secret via gcp-sm secretstore '%s' project '%s' secret '%s' version '%s'", req.StoreKey, project, secretName, version)
	}
}

func parseOperand(operand string) (string, string, string, bool) {
	project, rest, ok := strings.Cut(operand, "/")
	if !ok || project == "" || rest == "" {
		return "", "", "", false
	}
	secretName, version, hasVersion := strings.Cut(rest, "/")
	if !hasVersion || version == "" {
		version = "latest"
	}
	return project, secretName, version, secretName != ""
}

func (s *publishedStore) accessToken(ctx context.Context) (string, error) {
	switch s.mode {
	case "metadata":
		return s.metadataToken(ctx)
	case "service_account_file":
		path := s.serviceAccountFilePath
		if path == "" {
			return "", fmt.Errorf("mode_service_account_file.path is required")
		}
		return s.serviceAccountToken(ctx, path)
	default:
		return "", fmt.Errorf("mode '%s' is invalid for gcp-sm", s.mode)
	}
}

func (s *publishedStore) metadataToken(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/token", nil)
	if err != nil {
		return "", fmt.Errorf("creating metadata token request: %w", err)
	}
	req.Header.Set("Metadata-Flavor", "Google")
	resp, err := s.provider.metadataClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("metadata token request failed: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("reading metadata token response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("metadata token request returned HTTP %d: %s", resp.StatusCode, httpx.TruncateBody(body))
	}
	var result struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("parsing metadata token response: %w", err)
	}
	if result.AccessToken == "" {
		return "", fmt.Errorf("metadata token response missing access_token")
	}
	return result.AccessToken, nil
}

func (s *publishedStore) serviceAccountToken(ctx context.Context, credFile string) (string, error) {
	data, err := os.ReadFile(credFile)
	if err != nil {
		return "", fmt.Errorf("reading service account file '%s': %w", credFile, err)
	}
	var sa struct {
		ClientEmail string `json:"client_email"`
		PrivateKey  string `json:"private_key"`
		TokenURI    string `json:"token_uri"`
	}
	if err := json.Unmarshal(data, &sa); err != nil {
		return "", fmt.Errorf("parsing service account JSON: %w", err)
	}
	if sa.ClientEmail == "" || sa.PrivateKey == "" || sa.TokenURI == "" {
		return "", fmt.Errorf("service account JSON missing required fields (client_email, private_key, token_uri)")
	}
	now := s.provider.now().Unix()
	signedJWT, err := createSignedJWT(sa.ClientEmail, sa.TokenURI, sa.PrivateKey, now)
	if err != nil {
		return "", err
	}
	form := url.Values{
		"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"},
		"assertion":  {signedJWT},
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, sa.TokenURI, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("creating token exchange request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := s.provider.apiClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("token exchange request failed: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("reading token exchange response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token exchange returned HTTP %d: %s", resp.StatusCode, httpx.TruncateBody(body))
	}
	var result struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("parsing token exchange response: %w", err)
	}
	if result.AccessToken == "" {
		return "", fmt.Errorf("token exchange response missing access_token")
	}
	return result.AccessToken, nil
}

func createSignedJWT(clientEmail, tokenURI, privateKeyPEM string, nowUnix int64) (string, error) {
	header := `{"alg":"RS256","typ":"JWT"}`
	claimsMap := map[string]any{
		"iss":   clientEmail,
		"scope": "https://www.googleapis.com/auth/cloud-platform",
		"aud":   tokenURI,
		"iat":   nowUnix,
		"exp":   nowUnix + 3600,
	}
	claimsJSON, err := json.Marshal(claimsMap)
	if err != nil {
		return "", fmt.Errorf("marshaling JWT claims: %w", err)
	}
	headerB64 := base64.RawURLEncoding.EncodeToString([]byte(header))
	claimsB64 := base64.RawURLEncoding.EncodeToString(claimsJSON)
	unsigned := headerB64 + "." + claimsB64

	block, _ := pem.Decode([]byte(privateKeyPEM))
	if block == nil {
		return "", fmt.Errorf("failed to decode PEM private key")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return "", fmt.Errorf("parsing private key: %w", err)
	}
	rsaKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return "", fmt.Errorf("private key is not RSA")
	}
	hashed := sha256.Sum256([]byte(unsigned))
	sig, err := rsa.SignPKCS1v15(cryptorand.Reader, rsaKey, crypto.SHA256, hashed[:])
	if err != nil {
		return "", fmt.Errorf("signing JWT: %w", err)
	}
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}
