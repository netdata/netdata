// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || darwin

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func customCaptureFunction(keys []string) string {
	return `custom_sender() {
 for key in ` + strings.Join(keys, " ") + `; do printf '%s\000%s\000' "$key" "${!key}"; done >"$capture"
}`
}

func readCustomCapture(t *testing.T, path string) map[string]string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	parts := strings.Split(string(data), "\x00")
	require.Equal(t, "", parts[len(parts)-1])
	parts = parts[:len(parts)-1]
	require.Zero(t, len(parts)%2)
	values := make(map[string]string)
	for i := 0; i < len(parts); i += 2 {
		values[parts[i]] = parts[i+1]
	}
	require.Len(t, values, len(parts)/2)
	return values
}

func TestRunLegacyCustomContext(t *testing.T) {
	bash := testutil.Bash(t)
	base := map[string]string{
		"roles": "ops", "host": "test-node", "args_host": "test-node", "when": "1789387200", "name": "test_alert", "chart": "test.chart", "context": "test.context",
		"status": "WARNING", "old_status": "CLEAR", "summary": "Temperature is high", "info": testutil.ExpectedEvent().Info,
		"date": "2026-09-14T12:00:00Z", "date_utc": "2026-09-14T12:00:00Z", "value": "42.5", "old_value": "0", "units": "C",
		"value_string": "42.5 C", "old_value_string": "0 C", "status_message": "needs attention", "duration": "", "non_clear_duration": "",
		"unique_id": "", "alarm_id": "", "event_id": "", "src": "", "calc_expression": "", "calc_param_values": "", "total_warnings": "", "total_critical": "",
		"total_warn_alarms": "", "total_crit_alarms": "", "classification": "", "edit_command_line": "", "child_machine_guid": "", "transition_id": "", "component": "", "type": "",
		"to_custom": "one two", "custom_fact": "Temperature is high / $(literal) ${env:CUSTOM_AMBIENT}",
	}
	for name, tt := range map[string]struct {
		producer string
		values   map[string]string
	}{
		"missing facts":           {},
		"explicit zero and empty": {producer: `{"unique_id":0,"alarm_id":0,"event_id":0,"total_warnings":0,"total_critical":0,"value_string":"","old_value_string":""}`, values: map[string]string{"unique_id": "0", "alarm_id": "0", "event_id": "0", "total_warnings": "0", "total_critical": "0", "value_string": "", "old_value_string": ""}},
		"all producer facts": {producer: testutil.ProducerContextJSON, values: map[string]string{
			"unique_id": "42", "alarm_id": "7", "event_id": "3", "src": "line=12,file=/etc/netdata/health.d/example.conf",
			"value_string": "42.50 °C", "old_value_string": "0.00 °C", "calc_expression": "$this > 40", "calc_param_values": "$this = 42.5",
			"total_warnings": "2", "total_critical": "0", "total_warn_alarms": "other_alert=123,another_alert=456", "total_crit_alarms": "",
			"classification": "System", "edit_command_line": "edit-config health.d/example.conf", "child_machine_guid": "00000000-0000-0000-0000-000000000001",
			"transition_id": "00000000-0000-0000-0000-000000000002", "component": "Sensors", "type": "Environment",
		}},
	} {
		t.Run(name, func(t *testing.T) {
			capture := filepath.Join(t.TempDir(), "capture")
			config := fmt.Sprintf("bash='%s'; capture='%s'; DEFAULT_RECIPIENT_CUSTOM='one two'; literal='$(literal) ${env:CUSTOM_AMBIENT}'; custom_fact=\"$summary / $literal\"\n", bash, capture)
			want := maps.Clone(base)
			maps.Copy(want, tt.values)
			config += customCaptureFunction(slices.Sorted(maps.Keys(want)))
			var stdout, stderr bytes.Buffer
			args := []string{"send-legacy", "--method", "custom", "--role", "ops", "--config", writeConfig(t, config)}
			require.Equal(t, 0, Run(t.Context(), args, strings.NewReader(testutil.WithProducerContext(testutil.ValidEvent, tt.producer)), &stdout, &stderr), stderr.String())
			assert.Empty(t, stdout.String())
			assert.Contains(t, stderr.String(), "1 succeeded, 0 failed")
			assert.Equal(t, want, readCustomCapture(t, capture))
		})
	}
}

func TestRunLegacyCustomPresentation(t *testing.T) {
	bash := testutil.Bash(t)
	for name, tt := range map[string]struct {
		status, old, alarm, severity, raised, image, color string
		duration, total                                    *uint32
		images                                             string
	}{
		"warning":                   {status: "WARNING", old: "CLEAR", alarm: "Disk full = 42.5 C", severity: "WARNING", image: "alert-128-orange.png", color: "#ffc107"},
		"escalation":                {status: "CRITICAL", old: "WARNING", duration: new(uint32(61)), total: new(uint32(120)), alarm: "Disk full = 42.5 C", severity: "Escalated to CRITICAL", raised: "(alarm is raised for 2 minutes)", image: "alert-128-red.png", color: "#ca414b"},
		"demotion":                  {status: "WARNING", old: "CRITICAL", duration: new(uint32(61)), total: new(uint32(61)), alarm: "Disk full = 42.5 C", severity: "Demoted to WARNING", raised: "(was critical for 1 minute and 1 second)", image: "alert-128-orange.png", color: "#ffc107"},
		"recovery":                  {status: "CLEAR", old: "CRITICAL", duration: new(uint32(61)), total: new(uint32(120)), alarm: "Disk full (alarm was raised for 2 minutes)", severity: "Recovered from CRITICAL", raised: "(alarm was raised for 2 minutes)", image: "check-mark-2-128-green.png", color: "#77ca6d", images: "https://example.invalid/custom"},
		"recovery missing duration": {status: "CLEAR", old: "WARNING", alarm: "Disk full", severity: "Recovered from WARNING", image: "check-mark-2-128-green.png", color: "#77ca6d"},
		"recovery missing previous": {status: "CLEAR", alarm: "Disk full", severity: "Recovered", image: "check-mark-2-128-green.png", color: "#77ca6d"},
		"zero duration":             {status: "CLEAR", old: "WARNING", duration: new(uint32(0)), total: new(uint32(0)), alarm: "Disk full (was warning for 0 second)", severity: "Recovered from WARNING", raised: "(was warning for 0 second)", image: "check-mark-2-128-green.png", color: "#77ca6d"},
	} {
		t.Run(name, func(t *testing.T) {
			capture := filepath.Join(t.TempDir(), "capture")
			e := testutil.ExpectedEvent()
			e.Node = "a b/温度"
			e.Alert = "a b/温度"
			e.Chart = "a b/温度"
			e.Summary = "Disk_full"
			e.URL = "https://example.invalid/alert?a=1&b=2"
			e.Status = tt.status
			e.PreviousStatus = tt.old
			e.Duration = tt.duration
			e.NonClearDuration = tt.total
			input, err := json.Marshal(e)
			require.NoError(t, err)
			imageBase := tt.images
			if imageBase == "" {
				imageBase = "https://registry.my-netdata.io"
			}
			want := map[string]string{"alarm": tt.alarm, "severity": tt.severity, "raised_for": tt.raised, "image": imageBase + "/images/" + tt.image, "color": tt.color,
				"duration_txt": "", "non_clear_duration_txt": "", "goto_url": e.URL, "url_host": "a%20b%2f%e6%b8%a9%e5%ba%a6", "url_chart": "a%20b%2f%e6%b8%a9%e5%ba%a6", "url_name": "a%20b%2f%e6%b8%a9%e5%ba%a6", "url_value_string": "42.5%20C"}
			if tt.duration != nil {
				if *tt.duration == 0 {
					want["duration_txt"] = "0 second"
				} else {
					want["duration_txt"] = "1 minute and 1 second"
				}
			}
			if tt.total != nil {
				want["non_clear_duration_txt"] = map[uint32]string{0: "0 second", 61: "1 minute and 1 second", 120: "2 minutes"}[*tt.total]
			}
			config := fmt.Sprintf("bash='%s'; capture='%s'; DEFAULT_RECIPIENT_CUSTOM=one; images_base_url='%s'\n", bash, capture, tt.images) + customCaptureFunction(slices.Sorted(maps.Keys(want)))
			var stdout, stderr bytes.Buffer
			require.Equal(t, 0, Run(t.Context(), []string{"send-legacy", "--method", "custom", "--role", "ops", "--config", writeConfig(t, config)}, bytes.NewReader(input), &stdout, &stderr), stderr.String())
			assert.Equal(t, want, readCustomCapture(t, capture))
		})
	}
}

func TestRunLegacyCustomRouting(t *testing.T) {
	bash := testutil.Bash(t)
	for name, tt := range map[string]struct {
		overlay, producer, input, want, err string
		stock, disabledBash, unselected     bool
		code                                int
	}{
		"default batch":            {want: "1\x00one two\x00one two\x00"},
		"role only":                {overlay: `DEFAULT_RECIPIENT_CUSTOM=''; role_recipients_custom[ops]='x x y'`, want: "1\x00x y\x00x y\x00"},
		"filtered union":           {overlay: `role_recipients_custom[ops]='x|critical x y|nowarn z'`, want: "1\x00x z\x00x z\x00"},
		"history seen":             {overlay: `DEFAULT_RECIPIENT_CUSTOM='one|critical'`, input: strings.Replace(testutil.ValidEvent, `"version": 1`, `"version":1,"critical_seen_since_clear":true`, 1), want: "1\x00one\x00one\x00"},
		"history missing":          {overlay: `DEFAULT_RECIPIENT_CUSTOM='one|critical'`, code: 1, err: "critical_seen_since_clear", disabledBash: true},
		"stateless before history": {overlay: `DEFAULT_RECIPIENT_CUSTOM='one|nowarn|critical'`, disabledBash: true},
		"disabled":                 {overlay: `SEND_CUSTOM=NO`, disabledBash: true},
		"suppressed":               {overlay: `role_recipients_custom[ops]=disabled`, disabledBash: true},
		"no recipients":            {overlay: `DEFAULT_RECIPIENT_CUSTOM=''`, disabledBash: true},
		"unselected":               {unselected: true, disabledBash: true},
		"stock overridden":         {stock: true, want: "1\x00one two\x00one two\x00"},
		"last function wins":       {overlay: `custom_sender() { printf last >"$capture"; }`, want: "last"},
		"nonzero safe":             {overlay: `custom_sender() { echo synthetic-private-value >&2; return 42; }`, code: 1, err: "status 42"},
	} {
		t.Run(name, func(t *testing.T) {
			capture := filepath.Join(t.TempDir(), "capture")
			executable := bash
			if tt.disabledBash {
				executable = "/missing/bash"
			}
			config := fmt.Sprintf("bash='%s'; capture='%s'; DEFAULT_RECIPIENT_CUSTOM='one two'\n", executable, capture) + `custom_sender() { printf '%s\000' "$#" "$1" "$to_custom" >>"$capture"; }` + "\n" + tt.overlay
			args := []string{"send-legacy", "--role", "ops", "--method", "custom"}
			if tt.unselected {
				args[len(args)-1] = "discord"
			}
			if tt.stock {
				args = append(args, "--config", "../../../health_alarm_notify.conf")
			}
			args = append(args, "--config", writeConfig(t, config))
			input := tt.input
			if input == "" {
				input = testutil.ValidEvent
			}
			var stdout, stderr bytes.Buffer
			require.Equal(t, tt.code, Run(t.Context(), args, strings.NewReader(input), &stdout, &stderr), stderr.String())
			assert.Empty(t, stdout.String())
			assert.NotContains(t, stderr.String(), "synthetic-private-value")
			if tt.err != "" {
				assert.Contains(t, stderr.String(), tt.err)
			}
			if tt.want == "" {
				assert.NoFileExists(t, capture)
			} else {
				data, err := os.ReadFile(capture)
				require.NoError(t, err)
				assert.Equal(t, tt.want, string(data))
			}
		})
	}
}

func TestRunLegacyCustomAtomicPreflight(t *testing.T) {
	bash := testutil.Bash(t)
	for name, tt := range map[string]struct {
		overlay string
		stock   bool
	}{
		"invalid second provider": {overlay: `DISCORD_WEBHOOK_URL=invalid`},
		"stock placeholder":       {stock: true},
		"missing custom":          {},
		"reserved shell scalar":   {overlay: `BASH_ENV=synthetic-private-value`},
	} {
		t.Run(name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(204) }))
			defer server.Close()
			capture := filepath.Join(t.TempDir(), "capture")
			config := fmt.Sprintf("bash='%s'; capture='%s'; DEFAULT_RECIPIENT_CUSTOM=one; DISCORD_WEBHOOK_URL='%s'; DEFAULT_RECIPIENT_DISCORD=one\n", bash, capture, server.URL)
			if name != "missing custom" && !tt.stock {
				config += `custom_sender() { printf sent >"$capture"; }` + "\n"
			}
			config += tt.overlay
			args := []string{"send-legacy", "--role", "ops", "--method", "custom", "--method", "discord"}
			if tt.stock {
				args = append(args, "--config", "../../../health_alarm_notify.conf")
			}
			args = append(args, "--config", writeConfig(t, config))
			var stdout, stderr bytes.Buffer
			require.Equal(t, 1, Run(context.Background(), args, strings.NewReader(testutil.ValidEvent), &stdout, &stderr))
			assert.Zero(t, calls.Load())
			assert.NoFileExists(t, capture)
			assert.NotContains(t, stderr.String(), "synthetic-private-value")
			assert.Contains(t, stderr.String(), "0 succeeded, 0 failed")
		})
	}
}
