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
	"sync/atomic"
	"testing"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunLegacyChatMapping(t *testing.T) {
	type target struct{ path, query, channel string }
	for name, tt := range map[string]struct {
		method, settings, overlay, status string
		targets                           []target
		icon, color                       *string
		stock                             bool
	}{
		"Slack recipients and default": {
			method: "slack", settings: `SLACK_WEBHOOK_URL='ENDPOINT/slack'; DEFAULT_RECIPIENT_SLACK='ops #infra @user # ops'; role_recipients_slack[ops]=''`,
			targets: []target{{path: "/slack", channel: "#ops"}, {path: "/slack", channel: "#infra"}, {path: "/slack", channel: "@user"}, {path: "/slack"}},
		},
		"Slack filtering and role union": {
			method: "slack", settings: `SLACK_WEBHOOK_URL='ENDPOINT/slack'; DEFAULT_RECIPIENT_SLACK=unused; role_recipients_slack[ops]='one|nowarn #two|critical @three'; role_recipients_slack[all]='#two'`,
			targets: []target{{path: "/slack", channel: "#two"}, {path: "/slack", channel: "@three"}},
		},
		"Slack empty image base": {
			method: "slack", settings: `SLACK_WEBHOOK_URL='ENDPOINT/slack'; DEFAULT_RECIPIENT_SLACK='#ops'; images_base_url=''`,
			targets: []target{{path: "/slack", channel: "#ops"}},
		},
		"Slack overlay clears image base": {
			method: "slack", stock: true,
			settings: `SLACK_WEBHOOK_URL='ENDPOINT/slack'; DEFAULT_RECIPIENT_SLACK='#ops'; images_base_url='https://example.test/assets'`,
			overlay:  `images_base_url=''`, targets: []target{{path: "/slack", channel: "#ops"}},
		},
		"Slack stock overlay and custom icon": {
			method: "slack", stock: true, status: "CRITICAL",
			settings: `SLACK_WEBHOOK_URL='ENDPOINT/slack'; DEFAULT_RECIPIENT_SLACK='#ops'; images_base_url='https://example.test/assets'`,
			targets:  []target{{path: "/slack", channel: "#ops"}}, icon: new("https://example.test/assets/images/banner-icon-144x144.png"),
		},
		"Slack literal URL and icon": {
			method: "slack", status: "CLEAR", settings: `SLACK_WEBHOOK_URL='ENDPOINT/${env:CHAT_MAPPING_URL}?sig=${file:/missing}&keep=%2f'; DEFAULT_RECIPIENT_SLACK='#'; images_base_url='https://example.test/${env:CHAT_MAPPING_URL}'`,
			targets: []target{{path: "/${env:CHAT_MAPPING_URL}", query: "sig=${file:/missing}&keep=%2f"}},
			icon:    new("https://example.test/${env:CHAT_MAPPING_URL}/images/banner-icon-144x144.png"),
		},
		"Teams fixed URL deduplicates after filtering": {
			method: "msteams", settings: `MSTEAMS_WEBHOOK_URL='ENDPOINT/teams?sig=a%2fb&x=2&x=1'; role_recipients_msteams[ops]='first|nowarn second third'`,
			targets: []target{{path: "/teams", query: "sig=a%2fb&x=2&x=1"}},
		},
		"Teams substitutions and exact URL identity": {
			method: "msteams", settings: `MSTEAMS_WEBHOOK_URL='ENDPOINT/CHANNEL/CHANNEL?keep=%2f'; DEFAULT_RECIPIENT_MSTEAMS='a%2fb a%2Fb a%2fb'`,
			targets: []target{{path: "/a/b/a/b", query: "keep=%2f"}, {path: "/a/b/a/b", query: "keep=%2f"}},
		},
		"Teams full URLs as recipients": {
			method: "msteams", settings: `MSTEAMS_WEBHOOK_URL=CHANNEL; DEFAULT_RECIPIENT_MSTEAMS='ENDPOINT/one?sig=a%2Fb ENDPOINT/two?sig=c%2Fd'`,
			targets: []target{{path: "/one", query: "sig=a%2Fb"}, {path: "/two", query: "sig=c%2Fd"}},
		},
		"Teams stock aliases and overlay styles": {
			method: "msteams", stock: true, status: "CRITICAL",
			settings: `SEND_MSTEAMS=NO; MSTEAMS_WEBHOOK_URL=invalid; MSTEAMS_ICON_CRITICAL=unused; MSTEAMS_COLOR_CRITICAL=invalid; role_recipients_msteams[ops]=unused`,
			overlay:  `SEND_MSTEAM=YES; MSTEAM_WEBHOOK_URL='ENDPOINT/CHANNEL'; MSTEAM_ICON_CRITICAL=Alarm; MSTEAM_COLOR_CRITICAL=abcdef; role_recipients_msteam[ops]=old`,
			targets:  []target{{path: "/old"}}, icon: new("Alarm"), color: new("abcdef"),
		},
		"Teams empty alias preserves plural": {
			method: "msteams", settings: `MSTEAMS_WEBHOOK_URL='ENDPOINT/teams'; MSTEAM_WEBHOOK_URL=''; DEFAULT_RECIPIENT_MSTEAMS=ops; DEFAULT_RECIPIENT_MSTEAM=''; MSTEAMS_ICON_WARNING=Notice; MSTEAM_ICON_WARNING=''`,
			targets: []target{{path: "/teams"}}, icon: new("Notice"),
		},
		"Teams empty styles and default aliases": {
			method: "msteams", status: "CLEAR", settings: `MSTEAM_WEBHOOK_URL='ENDPOINT/CHANNEL'; DEFAULT_RECIPIENT_MSTEAM=old; MSTEAMS_ICON_CLEAR=''; MSTEAMS_COLOR_CLEAR=''`,
			targets: []target{{path: "/old"}}, icon: new(""), color: new(""),
		},
		"Teams literal URL": {
			method: "msteams", settings: `MSTEAMS_WEBHOOK_URL='ENDPOINT/${env:CHAT_MAPPING_URL}?sig=${file:/missing}'; DEFAULT_RECIPIENT_MSTEAMS=ops`,
			targets: []target{{path: "/${env:CHAT_MAPPING_URL}", query: "sig=${file:/missing}"}},
		},
		"Teams same-recipient policy union": {
			method: "msteams", settings: `MSTEAMS_WEBHOOK_URL='ENDPOINT/CHANNEL'; role_recipients_msteams[ops]='one|critical'; role_recipients_msteams[all]=one`,
			targets: []target{{path: "/one"}},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("PATH", t.TempDir())
			t.Setenv("CHAT_MAPPING_URL", "must-not-be-resolved")
			status := tt.status
			if status == "" {
				status = "WARNING"
			}
			fixture := "msteams-" + strings.ToLower(status) + "-full.json"
			if tt.method == "slack" {
				fixture = "slack-full.json"
			}
			data, err := os.ReadFile(fixturePath(fixture))
			require.NoError(t, err)
			if tt.method == "slack" {
				switch status {
				case "CRITICAL":
					data = []byte(strings.NewReplacer("CLEAR → WARNING", "WARNING → CRITICAL", "WARNING", "CRITICAL", `"warning"`, `"danger"`).Replace(string(data)))
				case "CLEAR":
					data = []byte(strings.NewReplacer("CLEAR → WARNING", "CRITICAL → CLEAR", "WARNING", "CLEAR", `"warning"`, `"good"`).Replace(string(data)))
				}
			}
			type request struct {
				path, query, method, contentType string
				payload                          string
			}
			requests := make(chan request, len(tt.targets)+1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				select {
				case requests <- request{r.URL.Path, r.URL.RawQuery, r.Method, r.Header.Get("Content-Type"), string(body)}:
				default:
					t.Error("unexpected extra request")
				}
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()
			args := []string{"send-legacy", "--role", "ops", "--role", "all", "--method", tt.method}
			if tt.stock {
				args = append(args, "--config", filepath.Join("..", "..", "..", "health_alarm_notify.conf"))
			}
			for _, text := range []string{tt.settings, tt.overlay} {
				args = append(args, "--config", writeConfig(t, strings.ReplaceAll(text, "ENDPOINT", server.URL)))
			}
			event, err := json.Marshal(testutil.EventForStatus(status, "full"))
			require.NoError(t, err)
			var stdout, stderr bytes.Buffer
			require.Equal(t, 0, Run(context.Background(), args, bytes.NewReader(event), &stdout, &stderr), stderr.String())
			assert.Empty(t, stdout.String())
			assert.Contains(t, stderr.String(), fmt.Sprintf("%d succeeded, 0 failed", len(tt.targets)))
			require.Len(t, requests, len(tt.targets))
			for _, target := range tt.targets {
				got := <-requests
				var want map[string]any
				require.NoError(t, json.Unmarshal(data, &want))
				if tt.method == "slack" {
					want["username"] = "netdata on test-node"
					want["icon_url"] = "https://registry.my-netdata.io/images/banner-icon-144x144.png"
					if target.channel != "" {
						want["channel"] = target.channel
					}
					if tt.icon != nil {
						want["icon_url"] = *tt.icon
					}
				} else {
					if tt.icon != nil {
						want["title"] = strings.TrimSpace(*tt.icon + " Alert " + status + " from Netdata on test-node")
					}
					if tt.color != nil {
						if *tt.color == "" {
							delete(want, "themeColor")
						} else {
							want["themeColor"] = *tt.color
						}
					}
				}
				payload, err := json.Marshal(want)
				require.NoError(t, err)
				assert.JSONEq(t, string(payload), got.payload)
				got.payload = ""
				assert.Equal(t, request{path: target.path, query: target.query, method: "POST", contentType: "application/json"}, got)
			}
			assert.NotContains(t, stderr.String(), server.URL)
			assert.NotContains(t, stderr.String(), "must-not-be-resolved")
		})
	}
}

func TestRunLegacyChatPreflight(t *testing.T) {
	for name, tt := range map[string]struct {
		overlay, err string
		code, calls  int
	}{
		"both selected by default":                    {calls: 2},
		"invalid later Slack URL":                     {overlay: `SLACK_WEBHOOK_URL='/synthetic-private-value'`, code: 1, err: "absolute HTTP(S)"},
		"invalid later Teams target":                  {overlay: `MSTEAMS_WEBHOOK_URL=CHANNEL; DEFAULT_RECIPIENT_MSTEAMS='ENDPOINT/valid /synthetic-private-value'`, code: 1, err: "absolute HTTP(S)"},
		"invalid Teams style":                         {overlay: `MSTEAMS_COLOR_WARNING=synthetic-private-value`, code: 1, err: "six hexadecimal"},
		"invalid singular Teams style":                {overlay: `MSTEAM_COLOR_CLEAR=synthetic-private-value`, code: 1, err: "six hexadecimal"},
		"Slack empty user":                            {overlay: `DEFAULT_RECIPIENT_SLACK='@'`, code: 1, err: "legacy channel"},
		"mixed images customization remains rejected": {overlay: `images_base_url='https://example.test/synthetic-private-value'`, code: 1, err: "images_base_url is not supported"},
		"missing history":                             {overlay: `DEFAULT_RECIPIENT_SLACK='ops|critical'`, code: 1, err: "critical_seen_since_clear"},
		"filtered before validation":                  {overlay: `SLACK_WEBHOOK_URL='/synthetic-private-value'; MSTEAMS_WEBHOOK_URL='/synthetic-private-value'; DEFAULT_RECIPIENT_SLACK='ops|nowarn|critical'; DEFAULT_RECIPIENT_MSTEAMS='ops|nowarn|critical'; MSTEAMS_COLOR_CLEAR=invalid`},
		"filtered Teams style ignored":                {overlay: `DEFAULT_RECIPIENT_MSTEAMS='ops|nowarn'; MSTEAMS_COLOR_CLEAR=invalid`, calls: 1},
		"missing Slack prerequisite":                  {overlay: `SLACK_WEBHOOK_URL=''; DEFAULT_RECIPIENT_SLACK='ops|unknown'`, calls: 1},
		"missing Teams prerequisite":                  {overlay: `MSTEAMS_WEBHOOK_URL=''; DEFAULT_RECIPIENT_MSTEAMS='ops|unknown'`, calls: 1},
	} {
		t.Run(name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()
			settings := fmt.Sprintf("SLACK_WEBHOOK_URL='%s/slack/synthetic-private-value'; DEFAULT_RECIPIENT_SLACK=ops; MSTEAMS_WEBHOOK_URL='%s/teams/synthetic-private-value'; DEFAULT_RECIPIENT_MSTEAMS=ops", server.URL, server.URL)
			args := []string{"send-legacy", "--role", "ops", "--config", writeConfig(t, settings), "--config", writeConfig(t, strings.ReplaceAll(tt.overlay, "ENDPOINT", server.URL))}
			var stdout, stderr bytes.Buffer
			assert.Equal(t, tt.code, Run(context.Background(), args, strings.NewReader(testutil.ValidEvent), &stdout, &stderr), stderr.String())
			assert.Equal(t, int32(tt.calls), calls.Load())
			assert.Contains(t, stderr.String(), fmt.Sprintf("%d succeeded, 0 failed", tt.calls))
			assert.Contains(t, stderr.String(), tt.err)
			assert.NotContains(t, stderr.String(), "synthetic-private-value")
			assert.NotContains(t, stderr.String(), server.URL)
			assert.Empty(t, stdout.String())
		})
	}
}
