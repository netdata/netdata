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
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunDiscord(t *testing.T) {
	tests := map[string]struct {
		status                                   int
		otherStatus                              int
		secret                                   string
		explicit, second, oversized, skipDiscord bool
		code                                     int
		err                                      string
	}{
		"mixed providers":           {status: 200},
		"separate Discord channels": {status: 200, second: true},
		"explicit destination":      {status: 200, explicit: true},
		"environment URL":           {status: 200, secret: "env"},
		"file URL":                  {status: 200, secret: "file"},
		"missing secret": {
			secret:      "missing",
			skipDiscord: true,
			err:         "secret resolved to an empty value",
		},
		"invalid resolved query": {
			secret:      "bad-query",
			skipDiscord: true,
			err:         "invalid discord webhook URL query",
		},
		"invalid resolved URL":       {secret: "bad-url", skipDiscord: true, err: "absolute HTTP(S)"},
		"201 refused":                {status: 201, err: "discord returned HTTP 201"},
		"unconfirmed 204 refused":    {status: 204, err: "discord returned HTTP 204"},
		"invalid payload":            {status: 400, err: "discord returned HTTP 400"},
		"invalid token":              {status: 401, err: "discord returned HTTP 401"},
		"rate limited without retry": {status: 429, err: "discord returned HTTP 429"},
		"service unavailable":        {status: 503, err: "discord returned HTTP 503"},
		"redirect refused":           {status: 302, err: "discord returned HTTP 302"},
		"all destinations fail": {
			status:      403,
			otherStatus: 503,
			code:        1,
			err:         "all attempted destinations failed",
		},
		"explicit failure": {
			status:   403,
			explicit: true,
			code:     1,
			err:      "all attempted destinations failed",
		},
		"provider limit does not block others": {oversized: true, skipDiscord: true, err: "1024-character embed limit"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			type request struct {
				path, query, method, contentType, authorization, userAgent string
				payload                                                    string
				err                                                        error
			}
			requests := make(chan request, 8)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				requests <- request{
					path: r.URL.Path, query: r.URL.RawQuery, method: r.Method, contentType: r.Header.Get("Content-Type"),
					authorization: r.Header.Get("Authorization"), userAgent: r.UserAgent(), payload: string(body), err: err,
				}
				status := test.otherStatus
				if strings.HasPrefix(r.URL.Path, "/discord") {
					status = test.status
				}
				if status == 0 {
					status = http.StatusOK
					if r.URL.Path == "/archive" {
						status = http.StatusNoContent
					}
				}
				w.Header().Set("Location", "/unexpected-redirect")
				w.Header().Set("Retry-After", "0")
				w.WriteHeader(status)
				_, _ = io.WriteString(w, "synthetic-private-value response")
			}))
			defer server.Close()
			endpoint := server.URL + "/discord/synthetic-private-value?wait=false&thread_id=123&note=a%26b&note=second"
			switch test.secret {
			case "env", "bad-query", "bad-url", "missing":
				if test.secret == "bad-query" {
					endpoint = server.URL + "/discord/synthetic-private-value?private=%zz"
				}
				if test.secret == "bad-url" {
					endpoint = "https://user:synthetic-private-value@example.com/hook"
				}
				if test.secret == "missing" {
					endpoint = ""
				}
				t.Setenv("NOTIFIER_DISCORD_URL", " "+endpoint+"\n")
				endpoint = "${env:NOTIFIER_DISCORD_URL}"
			case "file":
				path := filepath.Join(t.TempDir(), "url")
				require.NoError(t, os.WriteFile(path, []byte(" "+endpoint+"\n"), 0600))
				endpoint = "${file:" + path + "}"
			}
			extraRole := ""
			if test.second {
				extraRole = ", other"
			}
			config := fmt.Sprintf(`version: 1
destinations:
  chat: {type: discord, url: %q}
  other: {type: discord, url: %q}
  slack: {type: slack, url: %q}
  archive: {type: webhook, url: %q, bearer_token: synthetic-private-value}
  unused: {type: discord, url: '${env:NOTIFIER_DISCORD_UNUSED}'}
routing:
  roles:
    ops: [chat, slack, archive]
    dba: [chat%s]
`, endpoint, server.URL+"/discord-other", server.URL+"/slack?keep=value", server.URL+"/archive?keep=value", extraRole)
			event := expectedEvent()
			event.URL = "https://example.com/alert?id=1&view=chart#details"
			if test.oversized {
				event.Alert = strings.Repeat("a", 1025)
			}
			input, err := json.Marshal(event)
			require.NoError(t, err)
			args := []string{"send", "--config", writeConfig(t, config)}
			if test.explicit {
				args = append(args, "--destination", "chat")
			} else {
				args = append(args, "--role", "ops", "--role", "dba")
			}
			var stdout, stderr bytes.Buffer
			code := Run(context.Background(), args, bytes.NewReader(input), &stdout, &stderr)
			assert.Equal(t, test.code, code)
			assert.Empty(t, stdout.String())
			assert.NotContains(t, stderr.String(), "synthetic-private-value")
			assert.NotContains(t, stderr.String(), server.URL)
			if test.err != "" {
				assert.Contains(t, stderr.String(), test.err)
			}
			var want []request
			if !test.skipDiscord {
				want = append(
					want,
					request{
						path:  "/discord/synthetic-private-value",
						query: "note=a%26b&note=second&thread_id=123&wait=true",
					},
				)
			}
			if !test.explicit {
				want = append(
					want,
					request{path: "/slack", query: "keep=value"},
					request{path: "/archive", query: "keep=value", authorization: "Bearer synthetic-private-value"},
				)
			}
			if test.second {
				want = append(want, request{path: "/discord-other", query: "wait=true"})
			}
			require.Len(
				t,
				requests,
				len(want),
				"one request per selected name, without retries, redirects or channel loops",
			)
			for _, expected := range want {
				got := <-requests
				var payload []byte
				switch expected.path {
				case "/archive":
					payload = input
				case "/slack":
					payload, err = os.ReadFile(filepath.Join("testdata", "slack-full.json"))
					require.NoError(t, err)
					payload = []byte(strings.ReplaceAll(string(payload), "test_alert", event.Alert))
				default:
					payload, err = os.ReadFile(filepath.Join("testdata", "discord-full.json"))
					require.NoError(t, err)
				}
				assert.JSONEq(t, string(payload), got.payload)
				got.payload = ""
				expected.method, expected.contentType, expected.userAgent = "POST", "application/json", "netdata-alarm-notify"
				assert.Equal(t, expected, got)
			}
		})
	}
}

func TestDiscordLiteralQueryValidation(t *testing.T) {
	for name, query := range map[string]string{"bad escape": "?private=%zz", "invalid separator": "?private=a;b"} {
		t.Run(name, func(t *testing.T) {
			config := strings.Replace(
				configForURL("https://example.com/hook"+query),
				"type: webhook",
				"type: discord",
				1,
			)
			got, err := readConfig(strings.NewReader(config))
			require.ErrorContains(t, err, "invalid discord webhook URL query")
			assert.Equal(t, Config{}, got)
			assert.NotContains(t, err.Error(), "private")
		})
	}
}
