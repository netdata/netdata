// SPDX-License-Identifier: GPL-3.0-or-later

package web

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/buildinfo"
	"github.com/netdata/netdata/go/plugins/pkg/executable"
	"github.com/netdata/netdata/go/plugins/pkg/hostinfo"
)

// RequestConfig is the configuration of the HTTP request.
// This structure is not intended to be used directly as part of a module's configuration.
// Supported configuration file formats: YAML.
type RequestConfig struct {
	// URL specifies the URL to access.
	URL string `yaml:"url" json:"url"`

	// Username specifies the username for basic HTTPConfig authentication.
	Username string `yaml:"username,omitempty" json:"username"`

	// Password specifies the password for basic HTTPConfig authentication.
	Password string `yaml:"password,omitempty" json:"password"`

	// BearerTokenFile specifies the path to a file containing a bearer token
	// to be used for HTTP authentication.
	// The token is read from the file and included in the Authorization header as "Bearer <token>".
	BearerTokenFile string `yaml:"bearer_token_file,omitempty" json:"bearer_token_file"`

	// ProxyUsername specifies the username for basic HTTPConfig authentication.
	// It is used to authenticate a user agent to a proxy server.
	ProxyUsername string `yaml:"proxy_username,omitempty" json:"proxy_username"`

	// ProxyPassword specifies the password for basic HTTPConfig authentication.
	// It is used to authenticate a user agent to a proxy server.
	ProxyPassword string `yaml:"proxy_password,omitempty" json:"proxy_password"`

	// Method specifies the HTTPConfig method (GET, POST, PUT, etc.). An empty string means GET.
	Method string `yaml:"method,omitempty" json:"method"`

	// Headers specifies the HTTP request header fields to be sent by the client.
	Headers map[string]string `yaml:"headers,omitempty" json:"headers"`

	// Body specifies the HTTP request body to be sent by the client.
	Body string `yaml:"body,omitempty" json:"body"`
}

// Copy makes a full copy of the RequestConfig.
func (r RequestConfig) Copy() RequestConfig {
	if r.Headers == nil {
		return r
	}

	headers := make(map[string]string, len(r.Headers))
	for k, v := range r.Headers {
		headers[k] = v
	}
	r.Headers = headers
	return r
}

var userAgent = fmt.Sprintf("Netdata %s.plugin/%s", executable.Name, buildinfo.Version)

// NewHTTPRequest returns a new *http.Request given a RequestConfig configuration and an error if any.
func NewHTTPRequest(cfg RequestConfig) (*http.Request, error) {
	var body io.Reader
	if cfg.Body != "" {
		body = strings.NewReader(cfg.Body)
	}

	method := cfg.Method
	if method == "" {
		method = http.MethodGet
	}

	req, err := http.NewRequest(method, cfg.URL, body)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", userAgent)

	if err := setAuthentication(req, cfg); err != nil {
		return nil, err
	}

	if cfg.ProxyUsername != "" && cfg.ProxyPassword != "" {
		basicAuth := base64.StdEncoding.EncodeToString([]byte(cfg.ProxyUsername + ":" + cfg.ProxyPassword))
		req.Header.Set("Proxy-Authorization", "Basic "+basicAuth)
	}

	for k, v := range cfg.Headers {
		switch strings.ToLower(k) {
		case "host":
			req.Host = v
		default:
			req.Header.Set(k, v)
		}
	}

	return req, nil
}

func setAuthentication(req *http.Request, cfg RequestConfig) error {
	// Priority: Bearer Token > Basic Auth
	switch {
	case cfg.BearerTokenFile != "":
		return setBearerTokenAuth(req, cfg.BearerTokenFile)
	case cfg.Username != "" || cfg.Password != "":
		req.SetBasicAuth(cfg.Username, cfg.Password)
	}
	return nil
}

func setBearerTokenAuth(req *http.Request, tokenFile string) error {
	tokenBs, err := os.ReadFile(tokenFile)
	if err != nil {
		// Ignore K8s service account token errors when running outside the cluster
		if strings.HasPrefix(tokenFile, "/var/run/secrets/") && !hostinfo.IsInsideK8sCluster() {
			return nil
		}
		return fmt.Errorf("bearer token file: %w", err)
	}

	token := strings.TrimSpace(string(tokenBs))
	if token == "" {
		return fmt.Errorf("bearer token file is empty")
	}

	req.Header.Set("Authorization", "Bearer "+token)
	return nil
}

// NewHTTPRequestWithPath creates a new HTTP request with the given path appended to the base URL.
func NewHTTPRequestWithPath(cfg RequestConfig, urlPath string) (*http.Request, error) {
	// Make a copy to avoid modifying the original config
	cfg = cfg.Copy()

	// Join the paths properly
	v, err := url.JoinPath(cfg.URL, urlPath)
	if err != nil {
		return nil, fmt.Errorf("failed to join URL path: %w", err)
	}
	cfg.URL = v

	return NewHTTPRequest(cfg)
}

// URLQuery creates a URL-encoded query string from a single key-value pair.
func URLQuery(key, value string) string {
	return url.Values{key: []string{value}}.Encode()
}

// URLQueryMulti creates a URL-encoded query string from multiple key-value pairs.
func URLQueryMulti(params map[string]string) string {
	if len(params) == 0 {
		return ""
	}

	values := url.Values{}
	for k, v := range params {
		values.Set(k, v)
	}
	return values.Encode()
}
