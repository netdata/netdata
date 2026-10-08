// SPDX-License-Identifier: GPL-3.0-or-later

package app

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

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunSlackAndWebhook(t *testing.T) {
	tests := map[string]struct {
		slackStatus   int
		webhookStatus int
		secretSource  string
		oversized     bool
		skipSlack     bool
		wantCode      int
		wantError     string
	}{
		"both accepted":          {slackStatus: 200},
		"Slack 201 rejected":     {slackStatus: 201, wantError: "slack returned HTTP 201"},
		"Slack 204 rejected":     {slackStatus: 204, wantError: "slack returned HTTP 204"},
		"Slack payload rejected": {slackStatus: 400, wantError: "slack returned HTTP 400"},
		"Slack rate limited":     {slackStatus: 429, wantError: "slack returned HTTP 429"},
		"Slack unavailable":      {slackStatus: 503, wantError: "slack returned HTTP 503"},
		"Slack redirect refused": {slackStatus: 302, wantError: "slack returned HTTP 302"},
		"all providers fail": {
			slackStatus:   403,
			webhookStatus: 503,
			wantCode:      1,
			wantError:     "all attempted destinations failed",
		},
		"environment URL": {slackStatus: 200, secretSource: "env"},
		"file URL":        {slackStatus: 200, secretSource: "file"},
		"missing selected Slack secret": {
			secretSource: "missing",
			skipSlack:    true,
			wantError:    "secret environment variable is not set",
		},
		"Slack limit does not block generic webhook": {
			oversized: true,
			skipSlack: true,
			wantError: "3000-character Block Kit limit",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			type received struct {
				provider, method, contentType, authorization, payload string
				err                                                   error
			}
			requests := make(chan received, 4)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				provider := "webhook"
				status := test.webhookStatus
				if status == 0 {
					status = http.StatusNoContent
				}
				if strings.HasPrefix(r.URL.Path, "/slack/") {
					provider, status = "slack", test.slackStatus
				}
				body, err := io.ReadAll(r.Body)
				requests <- received{provider, r.Method, r.Header.Get("Content-Type"), r.Header.Get("Authorization"), string(body), err}
				w.Header().Set("Location", "/unexpected-redirect")
				w.WriteHeader(status)
				_, _ = io.WriteString(w, "synthetic-private-value response body")
			}))
			defer server.Close()
			endpoint := server.URL + "/slack/synthetic-private-value"
			switch test.secretSource {
			case "env":
				t.Setenv("NOTIFIER_SLACK_URL", endpoint)
				endpoint = "${env:NOTIFIER_SLACK_URL}"
			case "file":
				path := filepath.Join(t.TempDir(), "slack-url")
				require.NoError(t, os.WriteFile(path, []byte(endpoint+"\n"), 0600))
				endpoint = "${file:" + path + "}"
			case "missing":
				t.Setenv("NOTIFIER_SLACK_MISSING", "")
				require.NoError(t, os.Unsetenv("NOTIFIER_SLACK_MISSING"))
				endpoint = "${env:NOTIFIER_SLACK_MISSING}"
			}
			config := fmt.Sprintf(`version: 1
destinations:
  chat: {type: slack, url: %q}
  archive: {type: webhook, url: %q, bearer_token: synthetic-private-value}
  unused: {type: slack, url: '${env:NOTIFIER_UNSELECTED_SLACK}'}
routing:
  roles:
    sysadmin: [chat, archive]
    dba: [chat]
`, endpoint, server.URL+"/archive")
			event := testutil.ExpectedEvent()
			event.URL = "https://example.com/alert?id=1&view=chart#details"
			if test.oversized {
				event.Info = strings.Repeat("x", 3001)
			}
			input, err := json.Marshal(event)
			require.NoError(t, err)
			var stdout, stderr bytes.Buffer
			code := Run(
				context.Background(),
				[]string{"send", "--config", writeConfig(t, config), "--role", "sysadmin", "--role", "dba"},
				bytes.NewReader(input),
				&stdout,
				&stderr,
			)
			assert.Equal(t, test.wantCode, code)
			assert.Empty(t, stdout.String())
			assert.NotContains(t, stderr.String(), "synthetic-private-value")
			assert.NotContains(t, stderr.String(), server.URL)
			if test.wantError != "" {
				assert.Contains(t, stderr.String(), test.wantError)
			} else {
				assert.Contains(t, stderr.String(), `destination "chat" sent`)
			}
			wantCount := 2
			if test.skipSlack {
				wantCount = 1
			}
			require.Len(t, requests, wantCount, "no duplicate sends, redirects, retries or unselected requests")
			if !test.skipSlack {
				request := <-requests
				wantPayload, err := os.ReadFile(fixturePath("slack-full.json"))
				require.NoError(t, err)
				assert.JSONEq(t, string(wantPayload), request.payload)
				request.payload = "" // Payload has its own complete JSON comparison above.
				assert.Equal(t, received{provider: "slack", method: "POST", contentType: "application/json"}, request)
			}
			request := <-requests
			assert.JSONEq(t, string(input), request.payload)
			request.payload = ""
			assert.Equal(
				t,
				received{
					provider:      "webhook",
					method:        "POST",
					contentType:   "application/json",
					authorization: "Bearer synthetic-private-value",
				},
				request,
			)
		})
	}
}
