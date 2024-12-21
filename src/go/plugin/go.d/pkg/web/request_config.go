// SPDX-License-Identifier: GPL-3.0-or-later

package web

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/buildinfo"
	"github.com/netdata/netdata/go/plugins/pkg/executable"
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
	headers := make(map[string]string, len(r.Headers))
	for k, v := range r.Headers {
		headers[k] = v
	}
	r.Headers = headers
	return r
}

var userAgent = fmt.Sprintf("Netdata %s.plugin/%s", executable.Name, buildinfo.Version)

// NewHTTPRequest returns a new *http.Requests given a RequestConfig configuration and an error if any.
func NewHTTPRequest(cfg RequestConfig) (*http.Request, error) {
	var body io.Reader
	if cfg.Body != "" {
		body = strings.NewReader(cfg.Body)
	}

	req, err := http.NewRequest(cfg.Method, cfg.URL, body)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", userAgent)

	if cfg.Username != "" || cfg.Password != "" {
		req.SetBasicAuth(cfg.Username, cfg.Password)
	}

	if cfg.ProxyUsername != "" && cfg.ProxyPassword != "" {
		basicAuth := base64.StdEncoding.EncodeToString([]byte(cfg.ProxyUsername + ":" + cfg.ProxyPassword))
		req.Header.Set("Proxy-Authorization", "Basic "+basicAuth)
	}

	for k, v := range cfg.Headers {
		switch k {
		case "host", "Host":
			req.Host = v
		default:
			req.Header.Set(k, v)
		}
	}

	return req, nil
}

func NewHTTPRequestWithPath(cfg RequestConfig, urlPath string) (*http.Request, error) {
	cfg = cfg.Copy()

	v, err := url.JoinPath(cfg.URL, urlPath)
	if err != nil {
		return nil, fmt.Errorf("failed to join URL path: %v", err)
	}
	cfg.URL = v

	return NewHTTPRequest(cfg)
}

func URLQuery(key, value string) string {
	return url.Values{key: []string{value}}.Encode()
}
