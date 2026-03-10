// SPDX-License-Identifier: GPL-3.0-or-later

package secretresolver

import (
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
	"regexp"
	"strings"
)

// reGCPSafeProjectID validates GCP project IDs, including domain-scoped IDs (e.g., "domain.com:project-id").
var reGCPSafeProjectID = regexp.MustCompile(`^[a-zA-Z0-9._:-]+$`)

// reGCPSafeName validates GCP secret names and versions.
var reGCPSafeName = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

func (r *Resolver) resolveGCPSM(ref, original string) (string, error) {
	project, rest, ok := strings.Cut(ref, "/")
	if !ok || project == "" || rest == "" {
		return "", fmt.Errorf("resolving secret '%s': reference must be in format 'project/secret' or 'project/secret/version'", original)
	}

	secret, version, hasVersion := strings.Cut(rest, "/")
	if !hasVersion || version == "" {
		version = "latest"
	}

	if !reGCPSafeProjectID.MatchString(project) {
		return "", fmt.Errorf("resolving secret '%s': invalid project ID '%s'", original, project)
	}
	if !reGCPSafeName.MatchString(secret) {
		return "", fmt.Errorf("resolving secret '%s': invalid secret name '%s'", original, secret)
	}
	if !reGCPSafeName.MatchString(version) {
		return "", fmt.Errorf("resolving secret '%s': invalid version '%s'", original, version)
	}

	token, err := r.gcpGetAccessToken(original)
	if err != nil {
		return "", err
	}

	baseURL := r.gcpSecretManagerEndpoint
	if baseURL == "" {
		baseURL = "https://secretmanager.googleapis.com"
	}
	secretURL := fmt.Sprintf(
		"%s/v1/projects/%s/secrets/%s/versions/%s:access",
		baseURL, project, secret, version,
	)

	req, err := http.NewRequest(http.MethodGet, secretURL, nil)
	if err != nil {
		return "", fmt.Errorf("resolving secret '%s': creating request: %w", original, err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := r.gcpHTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("resolving secret '%s': request failed: %w", original, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("resolving secret '%s': reading response: %w", original, err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("resolving secret '%s': GCP Secret Manager returned HTTP %d: %s", original, resp.StatusCode, truncateBody(body))
	}

	var result struct {
		Payload struct {
			Data string `json:"data"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("resolving secret '%s': parsing response: %w", original, err)
	}

	decoded, err := base64.StdEncoding.DecodeString(result.Payload.Data)
	if err != nil {
		return "", fmt.Errorf("resolving secret '%s': decoding secret data: %w", original, err)
	}

	return string(decoded), nil
}

func (r *Resolver) gcpGetAccessToken(original string) (string, error) {
	// Try service account JSON first (avoids 2s metadata timeout on non-GCE hosts).
	if os.Getenv("GOOGLE_APPLICATION_CREDENTIALS") != "" {
		token, err := r.gcpGetTokenServiceAccount()
		if err != nil {
			return "", fmt.Errorf("resolving secret '%s': %w", original, err)
		}
		return token, nil
	}

	// Fall back to metadata server (GCE/GKE).
	token, err := r.gcpGetTokenMetadata()
	if err != nil {
		return "", fmt.Errorf("resolving secret '%s': cannot obtain GCP access token (set GOOGLE_APPLICATION_CREDENTIALS or run on GCE/GKE): %w", original, err)
	}

	return token, nil
}

func (r *Resolver) gcpGetTokenMetadata() (string, error) {
	req, err := http.NewRequest(http.MethodGet,
		"http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/token", nil)
	if err != nil {
		return "", fmt.Errorf("creating metadata token request: %w", err)
	}
	req.Header.Set("Metadata-Flavor", "Google")

	resp, err := r.gcpMetadataHTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("metadata token request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("reading metadata token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("metadata token request returned HTTP %d: %s", resp.StatusCode, truncateBody(body))
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

func (r *Resolver) gcpGetTokenServiceAccount() (string, error) {
	credFile := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
	if credFile == "" {
		return "", fmt.Errorf("GOOGLE_APPLICATION_CREDENTIALS is not set")
	}

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

	now := r.now().Unix()

	signedJWT, err := gcpCreateSignedJWT(sa.ClientEmail, sa.TokenURI, sa.PrivateKey, now)
	if err != nil {
		return "", err
	}

	form := url.Values{
		"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"},
		"assertion":  {signedJWT},
	}

	resp, err := r.gcpHTTPClient.PostForm(sa.TokenURI, form)
	if err != nil {
		return "", fmt.Errorf("token exchange request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("reading token exchange response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token exchange returned HTTP %d: %s", resp.StatusCode, truncateBody(body))
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

func gcpCreateSignedJWT(clientEmail, tokenURI, privateKeyPEM string, nowUnix int64) (string, error) {
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
	claims := string(claimsJSON)

	headerB64 := base64.RawURLEncoding.EncodeToString([]byte(header))
	claimsB64 := base64.RawURLEncoding.EncodeToString([]byte(claims))
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

	sigB64 := base64.RawURLEncoding.EncodeToString(sig)

	return unsigned + "." + sigB64, nil
}
