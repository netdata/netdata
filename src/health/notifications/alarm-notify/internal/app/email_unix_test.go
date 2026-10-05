// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || darwin

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net/mail"
	"strings"
	"testing"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/testutil"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/commandexec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunEmail(t *testing.T) {
	for name, test := range map[string]struct {
		mode, from string
		plain      bool
		code       int
	}{
		"default":       {mode: "record"},
		"quoted sender": {mode: "record", from: `Alert Bot <"alert bot"@example.com>`},
		"plain":         {mode: "record", plain: true},
		"failure":       {mode: "fail", code: 1},
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("NOTIFIER_TEST_AMBIENT_SECRET", "must-not-be-inherited")
			t.Setenv("NOTIFIER_TEST_EXPLICIT_SECRET", "synthetic-private-value")
			dst, capture := testCommandDestination(t, test.mode)
			dst["type"] = "email"
			dst["recipients"] = []string{"Ops <ops@example.com>", "other@example.com"}
			dst["from"], dst["plain_text_only"], dst["threading"] = test.from, new(test.plain), new(false)
			dst["env"].(map[string]string)["TOKEN"] = "${env:NOTIFIER_TEST_EXPLICIT_SECRET}"
			cfg := testConfig{Version: 1, Destinations: map[string]map[string]any{"target": dst}}
			event := testutil.ExpectedEvent()
			event.Info = "before\n.\nafter\x00end"
			input, err := json.Marshal(event)
			require.NoError(t, err)
			var stdout, stderr bytes.Buffer
			code := Run(context.Background(), []string{"send", "--config", writeCommandConfig(t, cfg), "--destination", "target"}, bytes.NewReader(input), &stdout, &stderr)
			require.Equal(t, test.code, code, stderr.String())
			assert.Empty(t, stdout.String())
			for _, private := range []string{"synthetic-private-value", dst["executable"].(string), "ops@example.com", "before"} {
				assert.NotContains(t, stderr.String(), private)
			}
			if test.code != 0 {
				assert.Contains(t, stderr.String(), "exited with status 7")
			}
			captures := readCommandCaptures(t, capture)
			require.Len(t, captures, 1)
			got := captures[0]
			args := []string{"-t", "-i"}
			if test.from != "" {
				args = append(args, "-f", `"alert bot"@example.com`)
			}
			assert.Equal(t, commandCapture{Args: args, Env: []string{
				"GORACE=atexit_sleep_ms=0", "NOTIFIER_TEST_CAPTURE=" + capture, "NOTIFIER_TEST_COMMAND_HELPER=" + test.mode, "PATH=" + commandexec.DefaultPath, "TOKEN=synthetic-private-value",
			}, Input: got.Input}, got)
			header, parts := testutil.ParseEmail(t, []byte(got.Input))
			to, err := header.AddressList("To")
			require.NoError(t, err)
			assert.Equal(t, []*mail.Address{{Name: "Ops", Address: "ops@example.com"}, {Address: "other@example.com"}}, to)
			if test.from != "" {
				from, err := header.AddressList("From")
				require.NoError(t, err)
				assert.Equal(t, []*mail.Address{{Name: "Alert Bot", Address: "alert bot@example.com"}}, from)
			} else {
				assert.Empty(t, header.Get("From"))
			}
			assert.Empty(t, header.Get("References"))
			assert.Empty(t, header.Get("In-Reply-To"))
			plain := "Temperature is high\r\nbefore\r\n.\r\nafter\x00end\r\nNode: test-node\r\nAlert: test_alert\r\nStatus: CLEAR → WARNING\r\nChart: test.chart\r\nContext: test.context\r\nValue: 42.5 C\r\nPrevious value: 0 C\r\nTime: 2026-09-14T12:00:00Z\r\nIncident ID: test-incident\r\n"
			want := []testutil.EmailPart{{Kind: "text/plain", Body: plain}}
			if !test.plain {
				want = append(want, testutil.EmailPart{Kind: "text/html", Body: "<!DOCTYPE html><html><body><pre>" + strings.TrimSuffix(plain, "\r\n") + "</pre></body></html>\r\n"})
			}
			assert.Equal(t, want, parts)
		})
	}
}
