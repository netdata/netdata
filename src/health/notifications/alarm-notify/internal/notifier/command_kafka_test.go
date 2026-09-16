// SPDX-License-Identifier: GPL-3.0-or-later

package notifier

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestRunKafka(t *testing.T) {
	for name, test := range map[string]struct {
		status                       int
		secret, err                  string
		explicit, skip, drop, second bool
	}{
		"mixed fanout": {}, "explicit": {explicit: true}, "second destination": {second: true},
		"env URL": {secret: "env"}, "file URL": {secret: "file"},
		"empty secret":         {secret: "empty", skip: true, err: "empty value"},
		"bad resolved URL":     {secret: "relative", skip: true, err: "absolute HTTP(S)"},
		"resolved credentials": {secret: "userinfo", skip: true, err: "user information"},
		"resolved fragment":    {secret: "fragment", skip: true, err: "fragment"},
		"nested reference":     {secret: "nested", skip: true, err: "absolute HTTP(S)"},
		"missing file":         {secret: "missing-file", skip: true, err: "could not read secret file"},
		"200 rejected":         {status: 200, err: "HTTP 200"}, "201 rejected": {status: 201, err: "HTTP 201"},
		"202 rejected": {status: 202, err: "HTTP 202"}, "205 rejected": {status: 205, err: "HTTP 205"},
		"unauthorized": {status: 401, err: "HTTP 401"}, "redirect refused": {status: 307, err: "HTTP 307"},
		"no rate limit retry": {status: 429, err: "HTTP 429"}, "no server retry": {status: 503, err: "HTTP 503"},
		"all failed":        {explicit: true, status: 500, err: "all attempted destinations failed"},
		"transport failure": {drop: true, err: "transport failed"},
	} {
		t.Run(name, func(t *testing.T) {
			type received struct {
				path, query, method, contentType, authorization, agent, body string
				err                                                          error
			}
			requests := make(chan received, 8)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				requests <- received{r.URL.Path, r.URL.RawQuery, r.Method, r.Header.Get("Content-Type"), r.Header.Get("Authorization"), r.UserAgent(), string(body), err}
				status := http.StatusNoContent
				if r.URL.Path == "/kafka/synthetic-private-value" {
					if test.drop {
						conn, _, err := w.(http.Hijacker).Hijack()
						if err == nil {
							_ = conn.Close()
						}
						return
					}
					if test.status != 0 {
						status = test.status
					}
				}
				w.Header().Set("Location", "/unexpected")
				w.Header().Set("Retry-After", "0")
				w.WriteHeader(status)
				_, _ = io.WriteString(w, "synthetic-private-value response")
			}))
			defer server.Close()
			dst := Destination{Type: "kafka", URL: server.URL + "/kafka/synthetic-private-value?key=a%26b&key=second", SenderIP: "192.0.2.1"}
			endpoint := dst.URL
			switch test.secret {
			case "empty":
				endpoint = ""
			case "relative":
				endpoint = "/synthetic-private-value"
			case "userinfo":
				endpoint = "https://user:synthetic-private-value@example.com/kafka"
			case "fragment":
				endpoint += "#synthetic-private-value"
			case "nested":
				endpoint = "${env:NESTED_KAFKA_URL}"
			}
			if test.secret == "file" || test.secret == "missing-file" {
				path := filepath.Join(t.TempDir(), "endpoint")
				if test.secret == "file" {
					require.NoError(t, os.WriteFile(path, []byte(" "+endpoint+"\n"), 0600))
				}
				dst.URL = "${file:" + path + "}"
			} else if test.secret != "" {
				t.Setenv("NOTIFIER_TEST_KAFKA_URL", " "+endpoint+"\n")
				dst.URL = "${env:NOTIFIER_TEST_KAFKA_URL}"
			}
			cfg := Config{Version: 1, Destinations: map[string]Destination{
				"bridge":  dst,
				"archive": {Type: "webhook", URL: server.URL + "/archive"},
				"unused":  {Type: "kafka", URL: "${env:UNUSED_KAFKA_URL}", SenderIP: "192.0.2.2"},
			}, Routing: Routing{Roles: map[string][]string{"ops": {"bridge", "archive"}, "dba": {"bridge"}}}}
			if test.second {
				cfg.Destinations["other"] = Destination{Type: "kafka", URL: server.URL + "/other", SenderIP: "2001:db8::1"}
				cfg.Routing.Roles["dba"] = append(cfg.Routing.Roles["dba"], "other")
			}
			config, err := yaml.Marshal(cfg)
			require.NoError(t, err)
			input := strings.Replace(validEvent, `"version": 1`, `"version": 1, "duration": 0, "non_clear_duration": 123`, 1)
			args := []string{"send", "--config", writeConfig(t, string(config))}
			if test.explicit {
				args = append(args, "--destination", "bridge")
			} else {
				args = append(args, "--role", "ops", "--role", "dba")
			}
			var stdout, stderr bytes.Buffer
			code := Run(context.Background(), args, strings.NewReader(input), &stdout, &stderr)
			wantCode := 0
			if test.explicit && test.err != "" {
				wantCode = 1
			}
			assert.Equal(t, wantCode, code)
			assert.Empty(t, stdout.String())
			assert.NotContains(t, stderr.String(), "synthetic-private-value")
			assert.NotContains(t, stderr.String(), server.URL)
			if test.err != "" {
				assert.Contains(t, stderr.String(), test.err)
			} else {
				assert.Contains(t, stderr.String(), `destination "bridge" sent`)
			}
			var want []received
			if !test.skip {
				want = append(want, received{path: "/kafka/synthetic-private-value", query: "key=a%26b&key=second", body: kafkaFullJSON})
			}
			if !test.explicit {
				want = append(want, received{path: "/archive", body: input})
			}
			if test.second {
				want = append(want, received{path: "/other", body: strings.Replace(kafkaFullJSON, "192.0.2.1", "2001:db8::1", 1)})
			}
			require.Len(t, requests, len(want), "one request per selected name; no retry or redirect")
			for _, expected := range want {
				got := <-requests
				assert.JSONEq(t, expected.body, got.body)
				expected.body, got.body = "", ""
				expected.method, expected.contentType, expected.agent = "POST", "application/json", "netdata-alarm-notify"
				assert.Equal(t, expected, got)
			}
		})
	}
}

func TestRunKafkaValidation(t *testing.T) {
	for name, endpoint := range map[string]string{
		"env":  "${env:UNREAD_KAFKA_URL}",
		"file": "${file:" + filepath.Join(t.TempDir(), "unread") + "}",
	} {
		t.Run(name, func(t *testing.T) {
			cfg := "version: 1\ndestinations:\n  bridge:\n    type: kafka\n    sender_ip: 192.0.2.1\n    url: '" + endpoint + "'\n"
			var stdout, stderr bytes.Buffer
			assert.Zero(t, Run(context.Background(), []string{"validate", "--config", writeConfig(t, cfg)}, strings.NewReader(""), &stdout, &stderr))
			assert.Equal(t, "configuration is valid\n", stdout.String())
			assert.Empty(t, stderr.String())
		})
	}
}

func TestRunKafkaCancellation(t *testing.T) {
	for name, test := range map[string]struct {
		timeout bool
		want    string
	}{
		"cancel": {want: "notification canceled"}, "timeout": {timeout: true, want: "notification timed out"},
	} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			stopped, cleanup := make(chan struct{}), make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				if !test.timeout {
					cancel()
				}
				select {
				case <-r.Context().Done():
					close(stopped)
				case <-cleanup:
				}
			}))
			defer server.Close()
			defer close(cleanup)
			cfg := "version: 1\ndestinations:\n  bridge:\n    type: kafka\n    sender_ip: 192.0.2.1\n    url: '" + server.URL + "'\n"
			args := []string{"send", "--config", writeConfig(t, cfg), "--destination", "bridge", "--timeout", "2s"}
			if test.timeout {
				args[len(args)-1] = "200ms"
			}
			var stdout, stderr bytes.Buffer
			assert.Equal(t, 1, Run(ctx, args, strings.NewReader(validEvent), &stdout, &stderr))
			assert.Contains(t, stderr.String(), test.want)
			assert.Empty(t, stdout.String())
			select {
			case <-stopped:
			case <-time.After(time.Second):
				t.Fatal("HTTP request was not canceled")
			}
		})
	}
}
