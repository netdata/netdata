// SPDX-License-Identifier: GPL-3.0-or-later

package secretresolver

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
)

var awsHTTPClient = &http.Client{Timeout: 10 * time.Second}
var awsIMDSHTTPClient = &http.Client{
	Timeout:   2 * time.Second,
	Transport: &http.Transport{Proxy: nil}, // IMDS must never be proxied
}

type awsCredentials struct {
	accessKeyID     string
	secretAccessKey string
	sessionToken    string
}

func resolveAWSSM(ref, original string) (string, error) {
	secretName, jsonKey, _ := strings.Cut(ref, "#")
	if secretName == "" {
		return "", fmt.Errorf("resolving secret '%s': secret name is empty", original)
	}

	creds, err := awsGetCredentials()
	if err != nil {
		return "", fmt.Errorf("resolving secret '%s': %w", original, err)
	}

	region := os.Getenv("AWS_DEFAULT_REGION")
	if region == "" {
		region = os.Getenv("AWS_REGION")
	}
	if region == "" {
		return "", fmt.Errorf("resolving secret '%s': AWS region not set (need AWS_DEFAULT_REGION or AWS_REGION)", original)
	}

	secretString, err := awsGetSecretValue(creds, region, secretName, original)
	if err != nil {
		return "", err
	}

	if jsonKey == "" {
		return secretString, nil
	}

	var parsed map[string]any
	if err := json.Unmarshal([]byte(secretString), &parsed); err != nil {
		return "", fmt.Errorf("resolving secret '%s': parsing SecretString as JSON: %w", original, err)
	}

	val, ok := parsed[jsonKey]
	if !ok {
		return "", fmt.Errorf("resolving secret '%s': key '%s' not found in SecretString JSON", original, jsonKey)
	}

	if s, ok := val.(string); ok {
		return s, nil
	}
	b, err := json.Marshal(val)
	if err != nil {
		return "", fmt.Errorf("resolving secret '%s': encoding value for key '%s': %w", original, jsonKey, err)
	}
	return string(b), nil
}

func awsGetCredentials() (*awsCredentials, error) {
	// Try environment variables first.
	if ak := os.Getenv("AWS_ACCESS_KEY_ID"); ak != "" {
		sk := os.Getenv("AWS_SECRET_ACCESS_KEY")
		if sk == "" {
			return nil, fmt.Errorf("AWS_ACCESS_KEY_ID is set but AWS_SECRET_ACCESS_KEY is not")
		}
		return &awsCredentials{
			accessKeyID:     ak,
			secretAccessKey: sk,
			sessionToken:    os.Getenv("AWS_SESSION_TOKEN"),
		}, nil
	}

	// Try ECS container credentials.
	if uri := os.Getenv("AWS_CONTAINER_CREDENTIALS_RELATIVE_URI"); uri != "" {
		creds, err := awsGetECSCredentials(uri)
		if err == nil {
			return creds, nil
		}
	}

	// Try EC2 IMDS v2.
	return awsGetIMDSCredentials()
}

func awsGetECSCredentials(relativeURI string) (*awsCredentials, error) {
	url := "http://169.254.170.2" + relativeURI

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating ECS credentials request: %w", err)
	}

	resp, err := awsIMDSHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ECS credentials request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("reading ECS credentials response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ECS credentials returned HTTP %d: %s", resp.StatusCode, truncateBody(body))
	}

	var result struct {
		AccessKeyId     string `json:"AccessKeyId"`
		SecretAccessKey string `json:"SecretAccessKey"`
		Token           string `json:"Token"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parsing ECS credentials response: %w", err)
	}

	if result.AccessKeyId == "" || result.SecretAccessKey == "" {
		return nil, fmt.Errorf("ECS credentials response missing required fields")
	}

	return &awsCredentials{
		accessKeyID:     result.AccessKeyId,
		secretAccessKey: result.SecretAccessKey,
		sessionToken:    result.Token,
	}, nil
}

func awsGetIMDSCredentials() (*awsCredentials, error) {
	// Step 1: get IMDS v2 token.
	tokenReq, err := http.NewRequest(http.MethodPut, "http://169.254.169.254/latest/api/token", nil)
	if err != nil {
		return nil, fmt.Errorf("creating IMDS token request: %w", err)
	}
	tokenReq.Header.Set("X-aws-ec2-metadata-token-ttl-seconds", "21600")

	tokenResp, err := awsIMDSHTTPClient.Do(tokenReq)
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

	// Step 2: get IAM role name.
	roleReq, err := http.NewRequest(http.MethodGet, "http://169.254.169.254/latest/meta-data/iam/security-credentials/", nil)
	if err != nil {
		return nil, fmt.Errorf("creating IMDS role request: %w", err)
	}
	roleReq.Header.Set("X-aws-ec2-metadata-token", imdsToken)

	roleResp, err := awsIMDSHTTPClient.Do(roleReq)
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

	// Step 3: get credentials for the role.
	credReq, err := http.NewRequest(http.MethodGet, "http://169.254.169.254/latest/meta-data/iam/security-credentials/"+role, nil)
	if err != nil {
		return nil, fmt.Errorf("creating IMDS credentials request: %w", err)
	}
	credReq.Header.Set("X-aws-ec2-metadata-token", imdsToken)

	credResp, err := awsIMDSHTTPClient.Do(credReq)
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
		AccessKeyId     string `json:"AccessKeyId"`
		SecretAccessKey string `json:"SecretAccessKey"`
		Token           string `json:"Token"`
	}
	if err := json.Unmarshal(credBody, &result); err != nil {
		return nil, fmt.Errorf("parsing IMDS credentials response: %w", err)
	}

	if result.AccessKeyId == "" || result.SecretAccessKey == "" {
		return nil, fmt.Errorf("IMDS credentials response missing required fields")
	}

	return &awsCredentials{
		accessKeyID:     result.AccessKeyId,
		secretAccessKey: result.SecretAccessKey,
		sessionToken:    result.Token,
	}, nil
}

func awsGetSecretValue(creds *awsCredentials, region, secretName, original string) (string, error) {
	endpoint := fmt.Sprintf("https://secretsmanager.%s.amazonaws.com/", region)

	// Use json.Marshal to safely escape the secret name.
	secretIDJSON, err := json.Marshal(secretName)
	if err != nil {
		return "", fmt.Errorf("resolving secret '%s': encoding secret name: %w", original, err)
	}
	payload := `{"SecretId":` + string(secretIDJSON) + `}`

	now := time.Now().UTC()
	timestamp := now.Format("20060102T150405Z")
	datestamp := now.Format("20060102")

	headers := map[string]string{
		"host":         fmt.Sprintf("secretsmanager.%s.amazonaws.com", region),
		"x-amz-date":   timestamp,
		"x-amz-target": "secretsmanager.GetSecretValue",
		"content-type": "application/x-amz-json-1.1",
	}
	if creds.sessionToken != "" {
		headers["x-amz-security-token"] = creds.sessionToken
	}

	authHeader := awsSigV4Sign(
		"POST",
		"/",
		"",
		headers,
		payload,
		creds,
		region,
		datestamp,
		timestamp,
	)

	req, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("resolving secret '%s': creating request: %w", original, err)
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}
	req.Header.Set("Authorization", authHeader)

	resp, err := awsHTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("resolving secret '%s': request failed: %w", original, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("resolving secret '%s': reading response: %w", original, err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("resolving secret '%s': AWS Secrets Manager returned HTTP %d: %s", original, resp.StatusCode, truncateBody(body))
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

// awsSigV4Sign computes an AWS Signature Version 4 Authorization header value.
func awsSigV4Sign(method, uri, query string, headers map[string]string, payload string, creds *awsCredentials, region, datestamp, timestamp string) string {
	canonicalHeaders, signedHeaders := awsCanonicalHeaders(headers)

	payloadHash := awsSHA256Hex([]byte(payload))

	canonicalRequest := strings.Join([]string{
		method,
		uri,
		query,
		canonicalHeaders,
		signedHeaders,
		payloadHash,
	}, "\n")

	scope := datestamp + "/" + region + "/secretsmanager/aws4_request"

	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		timestamp,
		scope,
		awsSHA256Hex([]byte(canonicalRequest)),
	}, "\n")

	signingKey := awsDeriveSigningKey(creds.secretAccessKey, datestamp, region)
	signature := hex.EncodeToString(awsHMACSHA256(signingKey, []byte(stringToSign)))

	return fmt.Sprintf(
		"AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		creds.accessKeyID, scope, signedHeaders, signature,
	)
}

// awsCanonicalHeaders builds the canonical headers string and signed headers list.
func awsCanonicalHeaders(headers map[string]string) (string, string) {
	// Build a normalized map with lowercase keys for safe lookup.
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

// awsDeriveSigningKey derives the SigV4 signing key.
func awsDeriveSigningKey(secretKey, datestamp, region string) []byte {
	kDate := awsHMACSHA256([]byte("AWS4"+secretKey), []byte(datestamp))
	kRegion := awsHMACSHA256(kDate, []byte(region))
	kService := awsHMACSHA256(kRegion, []byte("secretsmanager"))
	return awsHMACSHA256(kService, []byte("aws4_request"))
}

// awsHMACSHA256 computes HMAC-SHA256.
func awsHMACSHA256(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}

// awsSHA256Hex computes the lowercase hex-encoded SHA-256 hash.
func awsSHA256Hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}
