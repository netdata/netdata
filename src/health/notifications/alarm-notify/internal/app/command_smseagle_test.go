// SPDX-License-Identifier: GPL-3.0-or-later

package app

import (
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/notifier"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/testutil"

	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestRunSMSEagle(t *testing.T) {
	for mode, method := range map[string]string{"sms": "messages/sms", "mms": "messages/mms", "ring": "calls/ring", "tts": "calls/tts", "tts_advanced": "calls/tts_advanced"} {
		for name, test := range map[string]struct{ source, status string }{"literal warning": {"literal", "WARNING"}, "env critical": {"env", "CRITICAL"}, "file clear": {"file", "CLEAR"}} {
			t.Run(mode+"/"+name, func(t *testing.T) {
				type request struct {
					Method, Path, Query, ContentType, Accept, UserAgent, Token, Authorization string
					Body                                                                      map[string]any
				}
				requests := make(chan request, 2)
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					var body map[string]any
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					requests <- request{r.Method, r.URL.EscapedPath(), r.URL.RawQuery, r.Header.Get("Content-Type"), r.Header.Get("Accept"), r.Header.Get("User-Agent"), r.Header.Get("Access-Token"), r.Header.Get("Authorization"), body}
					_, _ = io.WriteString(w, smseagleTestResponse)
				}))
				defer server.Close()
				dst := smseagleTestDestination()
				dst["api_url"], dst["message_type"] = server.URL+"/proxy/", mode
				for _, field := range []struct {
					name  string
					value string
				}{{"token", "access_token"}, {"url", "api_url"}} {
					if test.source == "env" {
						key := "SMSEAGLE_TEST_" + strings.ToUpper(field.name)
						t.Setenv(key, " \n"+dst[field.value].(string)+"\n")
						dst[field.value] = "${env:" + key + "}"
					}
					if test.source == "file" {
						path := filepath.Join(t.TempDir(), field.name)
						require.NoError(t, os.WriteFile(path, []byte(dst[field.value].(string)+"\n"), 0600))
						dst[field.value] = "${file:" + path + "}"
					}
				}
				unused := smseagleTestDestination()
				unused["api_url"] = "${file:" + filepath.Join(t.TempDir(), "absent") + "}"
				cfg := testConfig{
					Version:      1,
					Destinations: map[string]map[string]any{"target": dst, "unused": unused},
					Routing: notifier.Routing{
						Roles:   map[string][]string{"ops": {"target", "target"}},
						Default: []string{"target"},
					},
				}
				data, err := yaml.Marshal(cfg)
				require.NoError(t, err)
				event := testutil.ExpectedEvent()
				event.URL, event.Status = "https://example.com/alert?id=1&view=chart#details", test.status
				if test.status == "CLEAR" {
					event.PreviousStatus = "CRITICAL"
				}
				input, err := json.Marshal(event)
				require.NoError(t, err)
				var stdout, stderr bytes.Buffer
				code := Run(
					context.Background(),
					[]string{"send", "--config", writeConfig(t, string(data)), "--role", "ops", "--role", "unknown"},
					bytes.NewReader(input),
					&stdout,
					&stderr,
				)
				require.Equal(t, 0, code, stderr.String())
				assert.Empty(t, stdout.String())
				require.Len(t, requests, 1)
				assert.Equal(
					t,
					request{
						"POST",
						"/proxy/api/v2/" + method,
						"",
						"application/json",
						"application/json",
						"netdata-alarm-notify",
						"synthetic-token",
						"",
						smseagleTestFixture(t, "full", mode, test.status),
					},
					<-requests,
				)
				assert.Contains(t, stderr.String(), `destination "target" sent`)
				assert.NotContains(t, stderr.String(), "synthetic-token")
				assert.NotContains(t, stderr.String(), server.URL)
			})
		}
	}
}

func TestRunSMSEagleFailures(t *testing.T) {
	for name, test := range map[string]struct {
		status      int
		body, err   string
		withSuccess bool
	}{
		"HTTP":     {401, "synthetic-private-value", "HTTP 401", false},
		"redirect": {307, "", "HTTP 307", false}, "bad ack": {200, "synthetic-private-value", "invalid", false},
		"partial batch": {200, `[{"status":"queued","id":1},{"status":"error","message":"synthetic-private-value"}]`, "queued 1 of 2", false},
		"mixed fanout":  {200, `[{"status":"queued","id":1},{"status":"error"}]`, "queued 1 of 2", true},
	} {
		t.Run(name, func(t *testing.T) {
			var target, after, redirect atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				switch r.URL.Path {
				case "/after":
					after.Add(1)
					w.WriteHeader(204)
				case "/redirect":
					redirect.Add(1)
					w.WriteHeader(204)
				default:
					target.Add(1)
					w.Header().Set("Location", "/redirect")
					w.WriteHeader(test.status)
					_, _ = io.WriteString(w, test.body)
				}
			}))
			defer server.Close()
			dst := smseagleTestDestination()
			dst["api_url"] = server.URL
			cfg := testConfig{
				Version: 1,
				Destinations: map[string]map[string]any{
					"target": dst,
					"after":  {"type": "webhook", "url": server.URL + "/after"},
				},
				Routing: notifier.Routing{Roles: map[string][]string{"ops": {"target", "after"}}},
			}
			data, err := yaml.Marshal(cfg)
			require.NoError(t, err)
			args := []string{"send", "--config", writeConfig(t, string(data)), "--destination", "target"}
			if test.withSuccess {
				args[len(args)-2], args[len(args)-1] = "--role", "ops"
			}
			var stdout, stderr bytes.Buffer
			code := Run(context.Background(), args, strings.NewReader(testutil.ValidEvent), &stdout, &stderr)
			if test.withSuccess {
				assert.Equal(t, 0, code)
				assert.EqualValues(t, 1, after.Load())
			} else {
				assert.Equal(t, 1, code)
				assert.Zero(t, after.Load())
			}
			assert.EqualValues(t, 1, target.Load())
			assert.Zero(t, redirect.Load())
			assert.Empty(t, stdout.String())
			assert.Contains(t, stderr.String(), test.err)
			for _, secret := range []string{"synthetic-private-value", dst["access_token"].(string), server.URL, dst["recipients"].([]string)[0]} {
				assert.NotContains(t, stderr.String(), secret)
			}
		})
	}
}

func TestRunSMSEaglePreflightFailures(t *testing.T) {
	for name, test := range map[string]struct{ field, value, err string }{
		"empty token": {"access_token", " \n", "empty value"}, "token controls": {"access_token", "synthetic-private-value\nother", "without whitespace"},
		"nested token": {"access_token", "${env:SYNTHETIC_NESTED}", "without whitespace"},
		"empty URL":    {"api_url", "\n", "empty value"}, "URL query": {"api_url", "https://example.com/?synthetic-private-value", "query"},
		"URL userinfo": {"api_url", "https://user:synthetic-private-value@example.com", "user information"},
		"TTS length":   {"text", strings.Repeat("界", 961), "960-character"},
	} {
		t.Run(name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }),
			)
			defer server.Close()
			dst := smseagleTestDestination()
			dst["api_url"] = server.URL
			event := testutil.ExpectedEvent()
			t.Setenv("SMSEAGLE_INVALID_SECRET", test.value)
			switch test.field {
			case "access_token":
				dst["access_token"] = "${env:SMSEAGLE_INVALID_SECRET}"
			case "api_url":
				dst["api_url"] = "${env:SMSEAGLE_INVALID_SECRET}"
				dst["access_token"] = "${file:" + filepath.Join(t.TempDir(), "unread-token") + "}"
			case "text":
				dst["message_type"], event.Summary = "tts", test.value
			}
			data, err := yaml.Marshal(testConfig{Version: 1, Destinations: map[string]map[string]any{"target": dst}})
			require.NoError(t, err)
			input, err := json.Marshal(event)
			require.NoError(t, err)
			var stdout, stderr bytes.Buffer
			code := Run(
				context.Background(),
				[]string{"send", "--config", writeConfig(t, string(data)), "--destination", "target"},
				bytes.NewReader(input),
				&stdout,
				&stderr,
			)
			assert.Equal(t, 1, code)
			assert.Zero(t, calls.Load())
			assert.Empty(t, stdout.String())
			assert.Contains(t, stderr.String(), test.err)
			assert.NotContains(t, stderr.String(), "synthetic-private-value")
			assert.NotContains(t, stderr.String(), "could not read secret file")
		})
	}
}
