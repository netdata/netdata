// SPDX-License-Identifier: GPL-3.0-or-later

package aws

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore/internal/envx"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore/internal/httpx"
)

func (s *publishedStore) Resolve(ctx context.Context, req secretstore.ResolveRequest) (string, error) {
	return s.resolve(ctx, req)
}

func (s *publishedStore) resolve(ctx context.Context, req secretstore.ResolveRequest) (string, error) {
	secretName, jsonKey, _ := strings.Cut(req.Operand, "#")
	if secretName == "" {
		return "", fmt.Errorf("resolving secret '%s': store '%s': secret name is empty", req.Original, req.StoreKey)
	}

	creds, err := s.credentials(ctx)
	if err != nil {
		return "", fmt.Errorf("resolving secret '%s': store '%s': %w", req.Original, req.StoreKey, err)
	}

	region, err := s.region()
	if err != nil {
		return "", fmt.Errorf("resolving secret '%s': store '%s': %w", req.Original, req.StoreKey, err)
	}

	secretString, err := s.secretValue(ctx, creds, region, secretName, req.Original)
	if err != nil {
		return "", fmt.Errorf("resolving secret '%s': store '%s': %w", req.Original, req.StoreKey, err)
	}

	if jsonKey == "" {
		logResolvedRequest(ctx, req, secretName, "")
		return secretString, nil
	}

	var parsed map[string]any
	if err := json.Unmarshal([]byte(secretString), &parsed); err != nil {
		return "", fmt.Errorf("resolving secret '%s': store '%s': parsing SecretString as JSON: %w", req.Original, req.StoreKey, err)
	}
	val, ok := parsed[jsonKey]
	if !ok {
		return "", fmt.Errorf("resolving secret '%s': store '%s': key '%s' not found in SecretString JSON", req.Original, req.StoreKey, jsonKey)
	}
	if value, ok := val.(string); ok {
		logResolvedRequest(ctx, req, secretName, jsonKey)
		return value, nil
	}
	b, err := json.Marshal(val)
	if err != nil {
		return "", fmt.Errorf("resolving secret '%s': store '%s': encoding value for key '%s': %w", req.Original, req.StoreKey, jsonKey, err)
	}
	logResolvedRequest(ctx, req, secretName, jsonKey)
	return string(b), nil
}

func logResolvedRequest(ctx context.Context, req secretstore.ResolveRequest, secretName, jsonKey string) {
	log, ok := logger.LoggerFromContext(ctx)
	if !ok {
		return
	}
	if jsonKey == "" {
		log.Infof("resolved secret via aws-sm secretstore '%s' secret '%s'", req.StoreKey, secretName)
		return
	}
	log.Infof("resolved secret via aws-sm secretstore '%s' secret '%s' key '%s'", req.StoreKey, secretName, jsonKey)
}

func (s *publishedStore) region() (string, error) {
	if s.regionValue == "" {
		return "", fmt.Errorf("region is required")
	}
	return s.regionValue, nil
}

func (s *publishedStore) credentials(ctx context.Context) (*credentials, error) {
	switch s.mode {
	case "env":
		return envCredentials()
	case "ecs":
		uri, ok := envx.Lookup("AWS_CONTAINER_CREDENTIALS_RELATIVE_URI")
		if !ok || uri == "" {
			return nil, fmt.Errorf("AWS_CONTAINER_CREDENTIALS_RELATIVE_URI is not set")
		}
		return s.ecsCredentials(ctx, uri)
	case "imds":
		return s.imdsCredentials(ctx)
	default:
		return nil, fmt.Errorf("auth_mode '%s' is invalid for aws-sm", s.mode)
	}
}

func envCredentials() (*credentials, error) {
	ak, ok := envx.Lookup("AWS_ACCESS_KEY_ID")
	if !ok || ak == "" {
		return nil, fmt.Errorf("AWS_ACCESS_KEY_ID is not set")
	}
	sk, ok := envx.Lookup("AWS_SECRET_ACCESS_KEY")
	if !ok || sk == "" {
		return nil, fmt.Errorf("AWS_SECRET_ACCESS_KEY is not set")
	}
	token, _ := envx.Lookup("AWS_SESSION_TOKEN")
	return &credentials{accessKeyID: ak, secretAccessKey: sk, sessionToken: token}, nil
}

func (s *publishedStore) ecsCredentials(ctx context.Context, relativeURI string) (*credentials, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://169.254.170.2"+relativeURI, nil)
	if err != nil {
		return nil, fmt.Errorf("creating ECS credentials request: %w", err)
	}
	resp, err := s.runtime.imdsClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ECS credentials request failed: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("reading ECS credentials response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ECS credentials returned HTTP %d: %s", resp.StatusCode, httpx.TruncateBody(body))
	}
	var result struct {
		AccessKeyID     string `json:"AccessKeyId"`
		SecretAccessKey string `json:"SecretAccessKey"`
		Token           string `json:"Token"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parsing ECS credentials response: %w", err)
	}
	if result.AccessKeyID == "" || result.SecretAccessKey == "" {
		return nil, fmt.Errorf("ECS credentials response missing required fields")
	}
	return &credentials{
		accessKeyID:     result.AccessKeyID,
		secretAccessKey: result.SecretAccessKey,
		sessionToken:    result.Token,
	}, nil
}

func (s *publishedStore) imdsCredentials(ctx context.Context) (*credentials, error) {
	tokenReq, err := http.NewRequestWithContext(ctx, http.MethodPut, "http://169.254.169.254/latest/api/token", nil)
	if err != nil {
		return nil, fmt.Errorf("creating IMDS token request: %w", err)
	}
	tokenReq.Header.Set("X-aws-ec2-metadata-token-ttl-seconds", "21600")
	tokenResp, err := s.runtime.imdsClient.Do(tokenReq)
	if err != nil {
		return nil, fmt.Errorf("IMDS token request failed: %w", err)
	}
	defer tokenResp.Body.Close()
	tokenBody, err := io.ReadAll(io.LimitReader(tokenResp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("reading IMDS token response: %w", err)
	}
	if tokenResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("IMDS token request returned HTTP %d", tokenResp.StatusCode)
	}
	imdsToken := strings.TrimSpace(string(tokenBody))

	roleReq, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://169.254.169.254/latest/meta-data/iam/security-credentials/", nil)
	if err != nil {
		return nil, fmt.Errorf("creating IMDS role request: %w", err)
	}
	roleReq.Header.Set("X-aws-ec2-metadata-token", imdsToken)
	roleResp, err := s.runtime.imdsClient.Do(roleReq)
	if err != nil {
		return nil, fmt.Errorf("IMDS role request failed: %w", err)
	}
	defer roleResp.Body.Close()
	roleBody, err := io.ReadAll(io.LimitReader(roleResp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("reading IMDS role response: %w", err)
	}
	if roleResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("IMDS role request returned HTTP %d", roleResp.StatusCode)
	}
	role := strings.TrimSpace(string(roleBody))
	if role == "" {
		return nil, fmt.Errorf("IMDS returned empty role name")
	}

	credReq, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://169.254.169.254/latest/meta-data/iam/security-credentials/"+role, nil)
	if err != nil {
		return nil, fmt.Errorf("creating IMDS credentials request: %w", err)
	}
	credReq.Header.Set("X-aws-ec2-metadata-token", imdsToken)
	credResp, err := s.runtime.imdsClient.Do(credReq)
	if err != nil {
		return nil, fmt.Errorf("IMDS credentials request failed: %w", err)
	}
	defer credResp.Body.Close()
	credBody, err := io.ReadAll(io.LimitReader(credResp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("reading IMDS credentials response: %w", err)
	}
	if credResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("IMDS credentials request returned HTTP %d", credResp.StatusCode)
	}
	var result struct {
		AccessKeyID     string `json:"AccessKeyId"`
		SecretAccessKey string `json:"SecretAccessKey"`
		Token           string `json:"Token"`
	}
	if err := json.Unmarshal(credBody, &result); err != nil {
		return nil, fmt.Errorf("parsing IMDS credentials response: %w", err)
	}
	if result.AccessKeyID == "" || result.SecretAccessKey == "" {
		return nil, fmt.Errorf("IMDS credentials response missing required fields")
	}
	return &credentials{accessKeyID: result.AccessKeyID, secretAccessKey: result.SecretAccessKey, sessionToken: result.Token}, nil
}

func (s *publishedStore) secretValue(ctx context.Context, creds *credentials, region, secretName, original string) (string, error) {
	host := secretsManagerHost(region)
	endpoint := (&url.URL{
		Scheme: "https",
		Host:   host,
		Path:   "/",
	}).String()
	secretIDJSON, err := json.Marshal(secretName)
	if err != nil {
		return "", fmt.Errorf("resolving secret '%s': encoding secret name: %w", original, err)
	}
	payload := `{"SecretId":` + string(secretIDJSON) + `}`
	now := time.Now().UTC()
	timestamp := now.Format("20060102T150405Z")
	datestamp := now.Format("20060102")
	headers := map[string]string{
		"host":         host,
		"x-amz-date":   timestamp,
		"x-amz-target": "secretsmanager.GetSecretValue",
		"content-type": "application/x-amz-json-1.1",
	}
	if creds.sessionToken != "" {
		headers["x-amz-security-token"] = creds.sessionToken
	}
	authHeader := sigV4Sign("POST", "/", "", headers, payload, creds, region, datestamp, timestamp)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("resolving secret '%s': creating request: %w", original, err)
	}
	for k, v := range headers {
		httpReq.Header.Set(k, v)
	}
	httpReq.Host = host
	httpReq.Header.Set("Authorization", authHeader)
	resp, err := s.runtime.apiClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("resolving secret '%s': request failed: %w", original, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("resolving secret '%s': reading response: %w", original, err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("resolving secret '%s': AWS Secrets Manager returned HTTP %d: %s", original, resp.StatusCode, httpx.TruncateBody(body))
	}
	var result struct {
		SecretString *string `json:"SecretString"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("resolving secret '%s': parsing response: %w", original, err)
	}
	if result.SecretString == nil {
		return "", fmt.Errorf("resolving secret '%s': SecretString is empty (binary secrets are not supported)", original)
	}
	return *result.SecretString, nil
}

func secretsManagerHost(region string) string {
	suffix := "amazonaws.com"
	if strings.HasPrefix(region, "cn-") {
		suffix = "amazonaws.com.cn"
	}
	return fmt.Sprintf("secretsmanager.%s.%s", region, suffix)
}

func sigV4Sign(method, uri, query string, headers map[string]string, payload string, creds *credentials, region, datestamp, timestamp string) string {
	canonicalHeaders, signedHeaders := canonicalHeaders(headers)
	payloadHash := sha256Hex([]byte(payload))
	canonicalRequest := strings.Join([]string{method, uri, query, canonicalHeaders, signedHeaders, payloadHash}, "\n")
	scope := datestamp + "/" + region + "/secretsmanager/aws4_request"
	stringToSign := strings.Join([]string{"AWS4-HMAC-SHA256", timestamp, scope, sha256Hex([]byte(canonicalRequest))}, "\n")
	signingKey := deriveSigningKey(creds.secretAccessKey, datestamp, region)
	signature := hex.EncodeToString(hmacSHA256(signingKey, []byte(stringToSign)))
	return fmt.Sprintf("AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s", creds.accessKeyID, scope, signedHeaders, signature)
}

func canonicalHeaders(headers map[string]string) (string, string) {
	norm := make(map[string]string, len(headers))
	keys := make([]string, 0, len(headers))
	for k, v := range headers {
		lk := strings.ToLower(k)
		norm[lk] = v
		keys = append(keys, lk)
	}
	sort.Strings(keys)
	var canonical strings.Builder
	for _, k := range keys {
		canonical.WriteString(k)
		canonical.WriteByte(':')
		canonical.WriteString(strings.TrimSpace(norm[k]))
		canonical.WriteByte('\n')
	}
	return canonical.String(), strings.Join(keys, ";")
}

func deriveSigningKey(secretKey, datestamp, region string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+secretKey), []byte(datestamp))
	kRegion := hmacSHA256(kDate, []byte(region))
	kService := hmacSHA256(kRegion, []byte("secretsmanager"))
	return hmacSHA256(kService, []byte("aws4_request"))
}

func hmacSHA256(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	_, _ = h.Write(data)
	return h.Sum(nil)
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
