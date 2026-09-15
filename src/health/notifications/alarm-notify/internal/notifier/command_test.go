// SPDX-License-Identifier: GPL-3.0-or-later

package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeConfig(t *testing.T, text string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "notify.yaml")
	require.NoError(t, os.WriteFile(path, []byte(text), 0600))
	return path
}

func TestRunDelivery(t *testing.T) {
	tests := map[string]struct {
		status     string
		httpStatus int
		wantCode   int
	}{
		"warning accepted": {status: "WARNING", httpStatus: 200},
		"critical created": {status: "CRITICAL", httpStatus: 201},
		"clear no content": {status: "CLEAR", httpStatus: 204},
		"unauthorized":     {status: "WARNING", httpStatus: 401, wantCode: 1},
		"rate limited":     {status: "WARNING", httpStatus: 429, wantCode: 1},
		"server failure":   {status: "WARNING", httpStatus: 503, wantCode: 1},
		"redirect":         {status: "WARNING", httpStatus: 302, wantCode: 1},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			type request struct {
				Method        string
				Path          string
				Authorization string
				ContentType   string
				UserAgent     string
				Event         Event
				Err           error
			}
			requests := make(chan request, 2)
			var calls atomic.Int32
			server := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					var event Event
					err := json.NewDecoder(r.Body).Decode(&event)
					requests <- request{r.Method, r.URL.RequestURI(), r.Header.Get("Authorization"), r.Header.Get("Content-Type"), r.UserAgent(), event, err}
					w.Header().Set("Location", "/unexpected-redirect-target")
					w.WriteHeader(test.httpStatus)
					_, _ = io.WriteString(w, "synthetic-private-value response body")
				}),
			)
			defer server.Close()
			t.Setenv("NOTIFIER_TEST_URL", server.URL+"/notify?key=synthetic-private-value")
			t.Setenv("NOTIFIER_TEST_TOKEN", "synthetic-private-value")
			cfg := configForURL(
				"${env:NOTIFIER_TEST_URL}",
			) + "    bearer_token: '${env:NOTIFIER_TEST_TOKEN}'\n"
			var stdout, stderr bytes.Buffer
			input := strings.Replace(validEvent, "WARNING", test.status, 1)
			code := Run(
				context.Background(),
				[]string{"send", "--config", writeConfig(t, cfg), "--destination", "dev"},
				strings.NewReader(input),
				&stdout,
				&stderr,
			)
			assert.Equal(t, test.wantCode, code)
			assert.Empty(t, stdout.String())
			assert.NotContains(t, stderr.String(), "synthetic-private-value")
			if test.wantCode != 0 {
				assert.Contains(t, stderr.String(), fmt.Sprintf("HTTP %d", test.httpStatus))
			}
			require.EqualValues(t, 1, calls.Load(), "one request, without retries or redirects")
			wantEvent := expectedEvent()
			wantEvent.Status = test.status
			assert.Equal(t, request{
				Method:        "POST",
				Path:          "/notify?key=synthetic-private-value",
				Authorization: "Bearer synthetic-private-value",
				ContentType:   "application/json",
				UserAgent:     "netdata-alarm-notify",
				Event:         wantEvent,
			}, <-requests)
		})
	}
}

func TestRunValidationAndErrors(t *testing.T) {
	tests := map[string]struct {
		args   []string
		config string
		input  string
		code   int
		stdout string
		err    string
	}{
		"validate without resolving": {
			args:   []string{"validate"},
			config: configForURL("${env:NOTIFIER_TEST_MISSING}"),
			stdout: "configuration is valid\n",
		},
		"invalid config": {args: []string{"validate"}, config: "[", code: 1, err: "invalid YAML"},
		"unknown provider": {
			args:   []string{"validate"},
			config: strings.Replace(validConfig, "webhook", "gotify", 1),
			code:   1,
			err:    "not implemented yet",
		},
		"unknown destination": {
			args:   []string{"send", "--destination", "absent"},
			config: validConfig,
			code:   1,
			err:    "not configured",
		},
		"missing destination": {
			args:   []string{"send"},
			config: validConfig,
			code:   1,
			err:    "provide --config",
		},
		"mixed selectors": {
			args:   []string{"send", "--destination", "dev", "--role", "sysadmin"},
			config: validConfig,
			code:   1,
			err:    "either --destination or --role",
		},
		"blank role": {
			args:   []string{"send", "--role", " "},
			config: validConfig,
			code:   1,
			err:    "invalid command options",
		},
		"empty explicit destination with role": {
			args:   []string{"send", "--destination", "", "--role", "sysadmin"},
			config: validConfig,
			code:   1,
			err:    "invalid command options",
		},
		"validate rejects role selector": {
			args:   []string{"validate", "--role", "sysadmin"},
			config: validConfig,
			code:   1,
			err:    "invalid command options",
		},
		"validate routing without resolving": {
			args: []string{"validate"},
			config: configForURL(
				"${env:NOTIFIER_TEST_MISSING}",
			) + "routing:\n  roles:\n    sysadmin: [dev]\n",
			stdout: "configuration is valid\n",
		},
		"invalid event with no targets": {
			args:   []string{"send", "--role", "silent"},
			config: validConfig,
			input:  "synthetic-private-value",
			code:   1,
			err:    "invalid JSON",
		},
		"invalid event": {
			args:   []string{"send", "--destination", "dev"},
			config: validConfig,
			input:  "synthetic-private-value",
			code:   1,
			err:    "invalid JSON",
		},
		"missing secret": {
			args:   []string{"send", "--destination", "dev"},
			config: configForURL("${env:NOTIFIER_TEST_MISSING}"),
			input:  validEvent,
			code:   1,
			err:    "environment variable is not set",
		},
		"zero timeout": {
			args:   []string{"validate", "--timeout", "0"},
			config: validConfig,
			code:   1,
			err:    "positive --timeout",
		},
		"negative timeout": {
			args:   []string{"validate", "--timeout", "-1s"},
			config: validConfig,
			code:   1,
			err:    "positive --timeout",
		},
		"bad flag value": {
			args:   []string{"validate", "--timeout", "synthetic-private-value"},
			config: validConfig,
			code:   1,
			err:    "invalid command options",
		},
		"unknown flag": {
			args:   []string{"validate", "--synthetic-private-value"},
			config: validConfig,
			code:   1,
			err:    "invalid command options",
		},
		"positional argument": {
			args:   []string{"validate", "synthetic-private-value"},
			config: validConfig,
			code:   1,
			err:    "positional arguments",
		},
	}
	t.Setenv("NOTIFIER_TEST_MISSING", "")
	require.NoError(t, os.Unsetenv("NOTIFIER_TEST_MISSING"))
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			args := append(
				append([]string(nil), test.args...),
				"--config",
				writeConfig(t, test.config),
			)
			var stdout, stderr bytes.Buffer
			got := Run(context.Background(), args, strings.NewReader(test.input), &stdout, &stderr)
			assert.Equal(t, test.code, got)
			assert.Equal(t, test.stdout, stdout.String())
			if test.err != "" {
				assert.Contains(t, stderr.String(), test.err)
			} else {
				assert.Empty(t, stderr.String())
			}
			assert.NotContains(t, stderr.String(), "synthetic-private-value")
		})
	}
}

func TestRunUsage(t *testing.T) {
	tests := map[string]struct {
		args []string
		code int
		help bool
	}{
		"help":            {args: []string{"--help"}, help: true},
		"send help":       {args: []string{"send", "-h"}, help: true},
		"validate help":   {args: []string{"validate", "--help"}, help: true},
		"missing command": {code: 1},
		"unknown command": {args: []string{"synthetic-private-value"}, code: 1},
		"missing config":  {args: []string{"validate"}, code: 1},
		"missing file": {
			args: []string{"validate", "--config", filepath.Join(t.TempDir(), "missing")},
			code: 1,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			assert.Equal(
				t,
				test.code,
				Run(context.Background(), test.args, strings.NewReader(""), &stdout, &stderr),
			)
			if test.help {
				assert.Equal(t, usage, stdout.String())
			} else {
				assert.Empty(t, stdout.String())
			}
			assert.NotContains(t, stderr.String(), "synthetic-private-value")
		})
	}
}

func TestRunCancellationDuringInput(t *testing.T) {
	for name, cancelImmediately := range map[string]bool{"deadline": false, "cancellation": true} {
		t.Run(name, func(t *testing.T) {
			reader, writer := io.Pipe()
			defer reader.Close()
			defer writer.Close()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if cancelImmediately {
				cancel()
			}
			var stdout, stderr bytes.Buffer
			code := Run(
				ctx,
				[]string{
					"send",
					"--config",
					writeConfig(t, validConfig),
					"--destination",
					"dev",
					"--timeout",
					"20ms",
				},
				reader,
				&stdout,
				&stderr,
			)
			assert.Equal(t, 1, code)
			assert.Empty(t, stdout.String())
			if cancelImmediately {
				assert.Contains(t, stderr.String(), "canceled")
			} else {
				assert.Contains(t, stderr.String(), "timed out")
			}
		})
	}
}

func TestRunNetworkFailures(t *testing.T) {
	tests := map[string]struct {
		tls    bool
		hang   bool
		closed bool
		want   string
	}{
		"TLS verification":   {tls: true, want: "transport failed"},
		"connection refused": {closed: true, want: "transport failed"},
		"deadline":           {hang: true, want: "timed out"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			stop := make(chan struct{})
			canceled := make(chan struct{})
			server := httptest.NewUnstartedServer(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if test.hang {
						_, _ = io.Copy(io.Discard, r.Body)
						select {
						case <-r.Context().Done():
							close(canceled)
						case <-stop:
						}
						return
					}
					w.WriteHeader(http.StatusNoContent)
				}),
			)
			server.Config.ErrorLog = log.New(io.Discard, "", 0)
			if test.tls {
				server.StartTLS()
			} else {
				server.Start()
			}
			defer server.Close()
			defer close(stop)
			if test.closed {
				server.Close()
			}
			path := writeConfig(t, configForURL(server.URL+"/synthetic-private-value"))
			var stdout, stderr bytes.Buffer
			assert.Equal(
				t,
				1,
				Run(
					context.Background(),
					[]string{
						"send",
						"--config",
						path,
						"--destination",
						"dev",
						"--timeout",
						"200ms",
					},
					strings.NewReader(validEvent),
					&stdout,
					&stderr,
				),
			)
			assert.Empty(t, stdout.String())
			assert.Contains(t, stderr.String(), test.want)
			assert.NotContains(t, stderr.String(), "synthetic-private-value")
			if test.hang {
				select {
				case <-canceled:
				case <-time.After(time.Second):
					t.Fatal("timed-out request was not canceled at the receiver")
				}
			}
		})
	}
}
