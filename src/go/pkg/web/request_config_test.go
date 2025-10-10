// SPDX-License-Identifier: GPL-3.0-or-later

package web

import (
	"encoding/base64"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequest_Copy(t *testing.T) {
	tests := map[string]struct {
		orig   RequestConfig
		change func(req *RequestConfig)
		verify func(t *testing.T, orig, copy RequestConfig)
	}{
		"change headers": {
			orig: RequestConfig{
				URL:    "http://127.0.0.1:19999/api/v1/info",
				Method: "POST",
				Headers: map[string]string{
					"X-Api-Key": "secret",
				},
				Username:      "username",
				Password:      "password",
				ProxyUsername: "proxy_username",
				ProxyPassword: "proxy_password",
			},
			change: func(req *RequestConfig) {
				req.Headers["header_key"] = "header_value"
			},
			verify: func(t *testing.T, orig, copy RequestConfig) {
				assert.Equal(t, 1, len(orig.Headers))
				assert.Equal(t, 2, len(copy.Headers))
			},
		},
		"nil headers": {
			orig: RequestConfig{
				URL: "http://127.0.0.1:19999/api/v1/info",
			},
			change: func(req *RequestConfig) {
				req.Headers = map[string]string{"new": "header"}
			},
			verify: func(t *testing.T, orig, copy RequestConfig) {
				assert.Nil(t, orig.Headers)
				assert.NotNil(t, copy.Headers)
			},
		},
		"change URL": {
			orig: RequestConfig{
				URL: "http://example.com",
			},
			change: func(req *RequestConfig) {
				req.URL = "http://changed.com"
			},
			verify: func(t *testing.T, orig, copy RequestConfig) {
				assert.Equal(t, "http://example.com", orig.URL)
				assert.Equal(t, "http://changed.com", copy.URL)
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			reqCopy := test.orig.Copy()

			// Initial state should be equal
			assert.Equal(t, test.orig, reqCopy)

			// Apply changes
			test.change(&reqCopy)

			// Verify changes don't affect original
			assert.NotEqual(t, test.orig, reqCopy)

			// Run custom verification if provided
			if test.verify != nil {
				test.verify(t, test.orig, reqCopy)
			}
		})
	}
}

func TestNewHTTPRequest(t *testing.T) {
	// Create a temporary file for bearer token test
	tmpDir := t.TempDir()
	bearerTokenFile := filepath.Join(tmpDir, "token")
	err := os.WriteFile(bearerTokenFile, []byte("test-bearer-token"), 0644)
	require.NoError(t, err)

	tests := map[string]struct {
		req      RequestConfig
		validate func(t *testing.T, req *http.Request, cfg RequestConfig)
		wantErr  bool
		errMsg   string
	}{
		"empty config": {
			req: RequestConfig{},
			validate: func(t *testing.T, req *http.Request, cfg RequestConfig) {
				assert.Equal(t, "GET", req.Method)
				assert.Equal(t, "", req.URL.String())
				assert.NotEmpty(t, req.Header.Get("User-Agent"))
			},
		},
		"full config": {
			req: RequestConfig{
				URL:           "http://127.0.0.1:19999/api/v1/info",
				Method:        "POST",
				Body:          "test body content",
				Username:      "user",
				Password:      "pass",
				ProxyUsername: "proxy_user",
				ProxyPassword: "proxy_pass",
				Headers: map[string]string{
					"X-Custom-Header": "custom-value",
					"Content-Type":    "application/json",
				},
			},
			validate: func(t *testing.T, req *http.Request, cfg RequestConfig) {
				assert.Equal(t, cfg.URL, req.URL.String())
				assert.Equal(t, cfg.Method, req.Method)

				// Check body
				body, err := io.ReadAll(req.Body)
				require.NoError(t, err)
				assert.Equal(t, cfg.Body, string(body))

				// Check basic auth
				user, pass, ok := req.BasicAuth()
				assert.True(t, ok)
				assert.Equal(t, cfg.Username, user)
				assert.Equal(t, cfg.Password, pass)

				// Check proxy auth
				proxyAuth := req.Header.Get("Proxy-Authorization")
				assert.NotEmpty(t, proxyAuth)
				proxyUser, proxyPass, ok := parseBasicAuth(proxyAuth)
				assert.True(t, ok)
				assert.Equal(t, cfg.ProxyUsername, proxyUser)
				assert.Equal(t, cfg.ProxyPassword, proxyPass)

				// Check headers
				for k, v := range cfg.Headers {
					assert.Equal(t, v, req.Header.Get(k))
				}
			},
		},
		"bearer token authentication": {
			req: RequestConfig{
				URL:             "http://example.com",
				BearerTokenFile: bearerTokenFile,
			},
			validate: func(t *testing.T, req *http.Request, cfg RequestConfig) {
				auth := req.Header.Get("Authorization")
				assert.Equal(t, "Bearer test-bearer-token", auth)
			},
		},
		"bearer token file not found": {
			req: RequestConfig{
				URL:             "http://example.com",
				BearerTokenFile: "/non/existent/file",
			},
			wantErr: true,
			errMsg:  "bearer token file",
		},
		"bearer token takes precedence over basic auth": {
			req: RequestConfig{
				URL:             "http://example.com",
				Username:        "user",
				Password:        "pass",
				BearerTokenFile: bearerTokenFile,
			},
			validate: func(t *testing.T, req *http.Request, cfg RequestConfig) {
				// Should have bearer token, not basic auth
				auth := req.Header.Get("Authorization")
				assert.Equal(t, "Bearer test-bearer-token", auth)

				// Basic auth should not be set
				_, _, ok := req.BasicAuth()
				assert.False(t, ok)
			},
		},
		"special headers - host lowercase": {
			req: RequestConfig{
				URL: "http://example.com",
				Headers: map[string]string{
					"host": "custom-host.com",
				},
			},
			validate: func(t *testing.T, req *http.Request, cfg RequestConfig) {
				assert.Equal(t, "custom-host.com", req.Host)
				// host header should not be in req.Header
				assert.Empty(t, req.Header.Get("host"))
			},
		},
		"special headers - Host uppercase": {
			req: RequestConfig{
				URL: "http://example.com",
				Headers: map[string]string{
					"Host": "custom-host.com",
				},
			},
			validate: func(t *testing.T, req *http.Request, cfg RequestConfig) {
				assert.Equal(t, "custom-host.com", req.Host)
				assert.Empty(t, req.Header.Get("Host"))
			},
		},
		"proxy auth without username": {
			req: RequestConfig{
				URL:           "http://example.com",
				ProxyPassword: "proxy_pass",
			},
			validate: func(t *testing.T, req *http.Request, cfg RequestConfig) {
				// Proxy auth should not be set if username is missing
				assert.Empty(t, req.Header.Get("Proxy-Authorization"))
			},
		},
		"invalid URL": {
			req: RequestConfig{
				URL: "://invalid-url",
			},
			wantErr: true,
		},
		"empty body": {
			req: RequestConfig{
				URL:  "http://example.com",
				Body: "",
			},
			validate: func(t *testing.T, req *http.Request, cfg RequestConfig) {
				assert.Nil(t, req.Body)
			},
		},
		"default GET method": {
			req: RequestConfig{
				URL: "http://example.com",
			},
			validate: func(t *testing.T, req *http.Request, cfg RequestConfig) {
				assert.Equal(t, "GET", req.Method)
			},
		},
		"custom method": {
			req: RequestConfig{
				URL:    "http://example.com",
				Method: "DELETE",
			},
			validate: func(t *testing.T, req *http.Request, cfg RequestConfig) {
				assert.Equal(t, "DELETE", req.Method)
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			httpReq, err := NewHTTPRequest(test.req)

			if test.wantErr {
				assert.Error(t, err)
				if test.errMsg != "" {
					assert.Contains(t, err.Error(), test.errMsg)
				}
				assert.Nil(t, httpReq)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, httpReq)

			if test.validate != nil {
				test.validate(t, httpReq, test.req)
			}
		})
	}
}

func TestNewHTTPRequestWithPath(t *testing.T) {
	tests := map[string]struct {
		config  RequestConfig
		path    string
		wantURL string
		wantErr bool
		errMsg  string
	}{
		"base url with path": {
			config:  RequestConfig{URL: "http://127.0.0.1:65535"},
			path:    "/bar",
			wantURL: "http://127.0.0.1:65535/bar",
		},
		"url with trailing slash": {
			config:  RequestConfig{URL: "http://127.0.0.1:65535/"},
			path:    "bar",
			wantURL: "http://127.0.0.1:65535/bar",
		},
		"url with path and trailing slash": {
			config:  RequestConfig{URL: "http://127.0.0.1:65535/foo/"},
			path:    "/bar",
			wantURL: "http://127.0.0.1:65535/foo/bar",
		},
		"url with path no trailing slash": {
			config:  RequestConfig{URL: "http://127.0.0.1:65535/foo"},
			path:    "bar",
			wantURL: "http://127.0.0.1:65535/foo/bar",
		},
		"empty path": {
			config:  RequestConfig{URL: "http://example.com"},
			path:    "",
			wantURL: "http://example.com",
		},
		"path with query params": {
			config:  RequestConfig{URL: "http://example.com"},
			path:    "/path?key=value",
			wantURL: "http://example.com/path%3Fkey=value", // url.JoinPath correctly escapes special chars
		},
		"complex path": {
			config:  RequestConfig{URL: "http://example.com/api/v1"},
			path:    "../v2/endpoint",
			wantURL: "http://example.com/api/v2/endpoint",
		},
		"preserve headers": {
			config: RequestConfig{
				URL: "http://example.com",
				Headers: map[string]string{
					"X-Custom": "value",
				},
			},
			path:    "/test",
			wantURL: "http://example.com/test",
		},
		"invalid base URL": {
			config:  RequestConfig{URL: "://invalid"},
			path:    "/path",
			wantErr: true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			// Store original headers count
			originalHeadersCount := len(test.config.Headers)

			req, err := NewHTTPRequestWithPath(test.config, test.path)

			if test.wantErr {
				assert.Error(t, err)
				if test.errMsg != "" {
					assert.Contains(t, err.Error(), test.errMsg)
				}
				assert.Nil(t, req)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, req)
			assert.Equal(t, test.wantURL, req.URL.String())

			// Verify original config wasn't modified
			assert.Equal(t, originalHeadersCount, len(test.config.Headers))
		})
	}
}

func TestURLQuery(t *testing.T) {
	tests := map[string]struct {
		key   string
		value string
		want  string
	}{
		"simple query": {
			key:   "foo",
			value: "bar",
			want:  "foo=bar",
		},
		"empty value": {
			key:   "key",
			value: "",
			want:  "key=",
		},
		"special characters": {
			key:   "key",
			value: "value with spaces & special=chars",
			want:  "key=value+with+spaces+%26+special%3Dchars",
		},
		"unicode": {
			key:   "name",
			value: "测试",
			want:  "name=%E6%B5%8B%E8%AF%95",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got := URLQuery(test.key, test.value)
			assert.Equal(t, test.want, got)
		})
	}
}

func parseBasicAuth(auth string) (username, password string, ok bool) {
	const prefix = "Basic "
	if len(auth) < len(prefix) || !strings.EqualFold(auth[:len(prefix)], prefix) {
		return "", "", false
	}

	decoded, err := base64.StdEncoding.DecodeString(auth[len(prefix):])
	if err != nil {
		return "", "", false
	}

	decodedStr := string(decoded)
	idx := strings.IndexByte(decodedStr, ':')
	if idx < 0 {
		return "", "", false
	}

	return decodedStr[:idx], decodedStr[idx+1:], true
}
