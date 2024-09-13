// SPDX-License-Identifier: GPL-3.0-or-later

package web

import (
	"encoding/base64"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequest_Copy(t *testing.T) {
	tests := map[string]struct {
		orig   RequestConfig
		change func(req *RequestConfig)
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
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			reqCopy := test.orig.Copy()

			assert.Equal(t, test.orig, reqCopy)
			test.change(&reqCopy)
			assert.NotEqual(t, test.orig, reqCopy)
		})
	}
}

func TestNewHTTPRequest(t *testing.T) {
	tests := map[string]struct {
		req     RequestConfig
		wantErr bool
	}{
		"test url": {
			req: RequestConfig{
				URL: "http://127.0.0.1:19999/api/v1/info",
			},
			wantErr: false,
		},
		"test body": {
			req: RequestConfig{
				Body: "content",
			},
			wantErr: false,
		},
		"test method": {
			req: RequestConfig{
				Method: "POST",
			},
			wantErr: false,
		},
		"test headers": {
			req: RequestConfig{
				Headers: map[string]string{
					"X-Api-Key": "secret",
				},
			},
			wantErr: false,
		},
		"test special headers (host)": {
			req: RequestConfig{
				Headers: map[string]string{
					"host": "Host",
				},
			},
			wantErr: false,
		},
		"test special headers (Host)": {
			req: RequestConfig{
				Headers: map[string]string{
					"Host": "Host",
				},
			},
			wantErr: false,
		},
		"test username and password": {
			req: RequestConfig{
				Username: "username",
				Password: "password",
			},
			wantErr: false,
		},
		"test proxy username and proxy password": {
			req: RequestConfig{
				ProxyUsername: "proxy_username",
				ProxyPassword: "proxy_password",
			},
			wantErr: false,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			httpReq, err := NewHTTPRequest(test.req)

			if test.wantErr {
				assert.Error(t, err)
				assert.Nil(t, httpReq)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, httpReq)
			require.IsType(t, (*http.Request)(nil), httpReq)

			assert.Equal(t, test.req.URL, httpReq.URL.String())

			if test.req.Body != "" {
				assert.NotNil(t, httpReq.Body)
			}

			if test.req.Username != "" || test.req.Password != "" {
				user, pass, ok := httpReq.BasicAuth()
				assert.True(t, ok)
				assert.Equal(t, test.req.Username, user)
				assert.Equal(t, test.req.Password, pass)
			}

			if test.req.Method != "" {
				assert.Equal(t, test.req.Method, httpReq.Method)
			}

			if test.req.ProxyUsername != "" || test.req.ProxyPassword != "" {
				user, pass, ok := parseBasicAuth(httpReq.Header.Get("Proxy-Authorization"))
				assert.True(t, ok)
				assert.Equal(t, test.req.ProxyUsername, user)
				assert.Equal(t, test.req.ProxyPassword, pass)
			}

			for k, v := range test.req.Headers {
				switch k {
				case "host", "Host":
					assert.Equal(t, httpReq.Host, v)
				default:
					assert.Equal(t, v, httpReq.Header.Get(k))
				}
			}
		})
	}
}

func TestNewRequest(t *testing.T) {
	tests := map[string]struct {
		url     string
		path    string
		wantURL string
	}{
		"base url": {
			url:     "http://127.0.0.1:65535",
			path:    "/bar",
			wantURL: "http://127.0.0.1:65535/bar",
		},
		"with path": {
			url:     "http://127.0.0.1:65535/foo/",
			path:    "/bar",
			wantURL: "http://127.0.0.1:65535/foo/bar",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			req, err := NewHTTPRequestWithPath(RequestConfig{URL: test.url}.Copy(), test.path)
			require.NoError(t, err)

			assert.Equal(t, test.wantURL, req.URL.String())
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
