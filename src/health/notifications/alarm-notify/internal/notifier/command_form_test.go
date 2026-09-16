// SPDX-License-Identifier: GPL-3.0-or-later

package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestRunFormProviders(t *testing.T) {
	for provider := range map[string]struct{}{"prowl": {}, "kavenegar": {}} {
		for name, test := range map[string]struct{ source, status string }{
			"literal warning": {"literal", "WARNING"}, "env critical": {"env", "CRITICAL"}, "file clear": {"file", "CLEAR"},
		} {
			t.Run(provider+"/"+name, func(t *testing.T) {
				dst := formTestDestination(provider)
				type request struct {
					Method, Path, Query, ContentType, Accept, UserAgent, Authorization string
					Form                                                               url.Values
				}
				requests := make(chan request, 2)
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if err := r.ParseForm(); err != nil {
						t.Error(err)
					}
					requests <- request{r.Method, r.URL.EscapedPath(), r.URL.RawQuery, r.Header.Get("Content-Type"), r.Header.Get("Accept"), r.Header.Get("User-Agent"), r.Header.Get("Authorization"), r.PostForm}
					if provider == "prowl" {
						_, _ = io.WriteString(w, prowlTestResponse)
					} else {
						_, _ = io.WriteString(w, kavenegarTestResponse)
					}
				}))
				defer server.Close()
				dst.APIURL = server.URL + "/proxy/"
				for _, field := range []struct {
					name  string
					value *string
				}{{"key", &dst.APIKey}, {"url", &dst.APIURL}} {
					if test.source == "env" {
						key := "FORM_TEST_" + strings.ToUpper(field.name)
						t.Setenv(key, " \n"+*field.value+"\n")
						*field.value = "${env:" + key + "}"
					}
					if test.source == "file" {
						path := filepath.Join(t.TempDir(), field.name)
						require.NoError(t, os.WriteFile(path, []byte(*field.value+"\n"), 0600))
						*field.value = "${file:" + path + "}"
					}
				}
				unused := formTestDestination(provider)
				unused.APIKey = "${env:UNSET_UNUSED_FORM_KEY}"
				unused.APIURL = "${file:" + filepath.Join(t.TempDir(), "absent") + "}"
				config := Config{
					Version:      1,
					Destinations: map[string]Destination{"target": dst, "unused": unused},
					Routing: Routing{
						Roles:   map[string][]string{"ops": {"target", "target"}},
						Default: []string{"target"},
					},
				}
				data, err := yaml.Marshal(config)
				require.NoError(t, err)
				event := expectedEvent()
				event.URL = "https://example.com/alert?id=1&view=chart#details"
				event.Status = test.status
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
				want := formTestFixture(t, provider, "full")
				for field, values := range want {
					for i, value := range values {
						if test.status == "CRITICAL" {
							values[i] = strings.ReplaceAll(value, "WARNING", "CRITICAL")
						}
						if test.status == "CLEAR" {
							values[i] = strings.NewReplacer("CLEAR → WARNING", "CRITICAL → CLEAR", "WARNING:", "CLEAR:").
								Replace(value)
						}
					}
					want[field] = values
				}
				path, accept := "/proxy/synthetic-key/sms/send.json", "application/json"
				if provider == "prowl" {
					path, accept = "/proxy/add", "application/xml"
					if test.status == "CRITICAL" {
						want.Set("priority", "2")
					}
					if test.status == "CLEAR" {
						want.Set("priority", "0")
					}
				}
				assert.Equal(
					t,
					request{
						"POST",
						path,
						"",
						"application/x-www-form-urlencoded",
						accept,
						"netdata-alarm-notify",
						"",
						want,
					},
					<-requests,
				)
				assert.Contains(t, stderr.String(), `destination "target" sent`)
				assert.NotContains(t, stderr.String(), server.URL)
				assert.NotContains(t, stderr.String(), formTestDestination(provider).APIKey)
			})
		}
	}
}

func TestRunFormFailures(t *testing.T) {
	for provider := range map[string]struct{}{"prowl": {}, "kavenegar": {}} {
		for name, test := range map[string]struct {
			status      int
			body, err   string
			withSuccess bool
		}{
			"rejection": {400, "synthetic-private-value", "HTTP 400", false}, "invalid ack": {200, "synthetic-private-value", "invalid", false},
			"redirect": {307, "", "HTTP 307", false}, "mixed fanout": {403, "synthetic-private-value", "HTTP 403", true},
		} {
			t.Run(provider+"/"+name, func(t *testing.T) {
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
				dst := formTestDestination(provider)
				dst.APIURL = server.URL
				cfg := Config{
					Version: 1,
					Destinations: map[string]Destination{
						"target": dst,
						"after":  {Type: "webhook", URL: server.URL + "/after"},
					},
				}
				cfg.Routing.Roles = map[string][]string{"ops": {"target", "after"}}
				data, err := yaml.Marshal(cfg)
				require.NoError(t, err)
				args := []string{"send", "--config", writeConfig(t, string(data)), "--destination", "target"}
				if test.withSuccess {
					args = []string{"send", "--config", writeConfig(t, string(data)), "--role", "ops"}
				}
				var stdout, stderr bytes.Buffer
				code := Run(context.Background(), args, strings.NewReader(validEvent), &stdout, &stderr)
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
				assert.NotContains(t, stderr.String(), "synthetic-private-value")
				assert.NotContains(t, stderr.String(), dst.APIKey)
				assert.NotContains(t, stderr.String(), server.URL)
			})
		}
	}
}

func TestRunFormSecretValidation(t *testing.T) {
	for provider, host := range map[string]string{"prowl": "api.prowlapp.com", "kavenegar": "api.kavenegar.com"} {
		for name, test := range map[string]struct{ field, value, err string }{
			"empty": {"api_key", " \n", "empty value"}, "invalid key": {"api_key", "synthetic-private-value\nother", "without whitespace"},
			"nested key":        {"api_key", "${env:SYNTHETIC_NESTED}", "without whitespace"},
			"insecure official": {"api_url", "http://" + strings.ToUpper(host) + "./", "HTTPS"},
			"query":             {"api_url", "https://example.com/?synthetic-private-value", "query"}, "fragment": {"api_url", "https://example.com/#", "fragment"},
		} {
			t.Run(provider+"/"+name, func(t *testing.T) {
				var calls atomic.Int32
				server := httptest.NewServer(
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }),
				)
				defer server.Close()
				dst := formTestDestination(provider)
				dst.APIURL = server.URL
				t.Setenv("FORM_INVALID_SECRET", test.value)
				if test.field == "api_key" {
					dst.APIKey = "${env:FORM_INVALID_SECRET}"
				} else {
					dst.APIURL = "${env:FORM_INVALID_SECRET}"
					dst.APIKey = "${file:" + filepath.Join(t.TempDir(), "unread-key") + "}"
				}
				data, err := yaml.Marshal(Config{Version: 1, Destinations: map[string]Destination{"target": dst}})
				require.NoError(t, err)
				var stdout, stderr bytes.Buffer
				assert.Equal(
					t,
					1,
					Run(
						context.Background(),
						[]string{"send", "--config", writeConfig(t, string(data)), "--destination", "target"},
						strings.NewReader(validEvent),
						&stdout,
						&stderr,
					),
				)
				assert.Zero(t, calls.Load())
				assert.Empty(t, stdout.String())
				assert.Contains(t, stderr.String(), test.err)
				assert.NotContains(t, stderr.String(), "synthetic-private-value")
				assert.NotContains(t, stderr.String(), "could not read secret file")
			})
		}
	}
}

func TestRunKavenegarEscapedKey(t *testing.T) {
	const key = "synthetic/key?%&#"
	paths := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths <- r.RequestURI
		_, _ = io.WriteString(w, kavenegarTestResponse)
	}))
	defer server.Close()
	cfg := fmt.Sprintf(
		"version: 1\ndestinations:\n  target: {type: kavenegar, api_key: %q, api_url: %q, sender: '12345', recipient: '+15005550009'}\n",
		key,
		server.URL+"/v1/",
	)
	var stdout, stderr bytes.Buffer
	require.Equal(
		t,
		0,
		Run(
			context.Background(),
			[]string{"send", "--config", writeConfig(t, cfg), "--destination", "target"},
			strings.NewReader(validEvent),
			&stdout,
			&stderr,
		),
		stderr.String(),
	)
	assert.Equal(t, "/v1/synthetic%2Fkey%3F%25&%23/sms/send.json", <-paths)
}

func TestRunProwlPayloadLimit(t *testing.T) {
	var prowl, webhook atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/add" {
			prowl.Add(1)
		} else {
			webhook.Add(1)
		}
		w.WriteHeader(204)
	}))
	defer server.Close()
	dst := formTestDestination("prowl")
	dst.APIURL = server.URL
	cfg := Config{
		Version:      1,
		Destinations: map[string]Destination{"prowl": dst, "webhook": {Type: "webhook", URL: server.URL + "/webhook"}},
		Routing:      Routing{Roles: map[string][]string{"ops": {"prowl", "webhook"}}},
	}
	data, err := yaml.Marshal(cfg)
	require.NoError(t, err)
	event := expectedEvent()
	event.Info = strings.Repeat("界", 4000)
	input, err := json.Marshal(event)
	require.NoError(t, err)
	var stdout, stderr bytes.Buffer
	assert.Equal(
		t,
		0,
		Run(
			context.Background(),
			[]string{"send", "--config", writeConfig(t, string(data)), "--role", "ops"},
			bytes.NewReader(input),
			&stdout,
			&stderr,
		),
	)
	assert.Zero(t, prowl.Load())
	assert.EqualValues(t, 1, webhook.Load())
	assert.Empty(t, stdout.String())
	assert.Contains(t, stderr.String(), "description exceeds the 10000-byte limit")
	assert.Contains(t, stderr.String(), "1 succeeded, 1 failed")
}
