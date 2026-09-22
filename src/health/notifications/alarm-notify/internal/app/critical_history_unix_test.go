// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || darwin

package app

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/commandexec"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/notifier"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunCommandCriticalHistoryPreflight(t *testing.T) {
	for name, test := range map[string]struct {
		history string
		targets []string
	}{
		"missing before ordinary": {"", []string{"critical", "ordinary"}},
		"missing after ordinary":  {"", []string{"ordinary", "critical"}},
		"null before ordinary":    {"null", []string{"critical", "ordinary"}},
		"null after ordinary":     {"null", []string{"ordinary", "critical"}},
	} {
		t.Run(name, func(t *testing.T) {
			ordinary, capture := testCommandDestination(t, "record")
			critical, criticalCapture := testCommandDestination(t, "record")
			cfg := testConfig{Version: 1, Destinations: map[string]map[string]any{"ordinary": ordinary, "critical": critical}, Routing: notifier.Routing{Default: test.targets, Policies: map[string]*notifier.DestinationPolicy{"critical": {Critical: true}}}}
			for _, status := range []string{"WARNING", "CLEAR"} {
				t.Run(status, func(t *testing.T) {
					var stdout, stderr bytes.Buffer
					assert.Equal(t, 1, Run(context.Background(), []string{"send", "--config", writeCommandConfig(t, cfg), "--role", "unknown"}, bytes.NewReader(criticalHistoryInput(t, status, test.history)), &stdout, &stderr))
					assert.Empty(t, stdout.String())
					assert.Contains(t, stderr.String(), "critical_seen_since_clear")
					assert.Contains(t, stderr.String(), "delivery summary: 0 succeeded, 0 failed, 0 skipped\n")
					assert.NotContains(t, stderr.String(), "destination ")
					for _, path := range []string{capture, criticalCapture} {
						_, err := os.Stat(path)
						assert.ErrorIs(t, err, os.ErrNotExist)
					}
				})
			}
		})
	}
}

func TestRunCommandInputOnlyFacts(t *testing.T) {
	t.Setenv("NOTIFIER_COMMAND_CRITICAL_MISSING", "")
	require.NoError(t, os.Unsetenv("NOTIFIER_COMMAND_CRITICAL_MISSING"))
	for name, test := range map[string]struct {
		status, history string
		skip            bool
	}{
		"warning history":        {"WARNING", "true", false},
		"clear history":          {"CLEAR", "true", false},
		"critical unknown":       {"CRITICAL", "", false},
		"warning skipped secret": {"WARNING", "false", true},
		"clear skipped secret":   {"CLEAR", "false", true},
	} {
		t.Run(name, func(t *testing.T) {
			dst, capture := testCommandDestination(t, "record")
			if test.skip {
				dst["env"].(map[string]string)["TOKEN"] = "${env:NOTIFIER_COMMAND_CRITICAL_MISSING}"
			}
			cfg := testConfig{Version: 1, Destinations: map[string]map[string]any{"target": dst}, Routing: notifier.Routing{Policies: map[string]*notifier.DestinationPolicy{"target": {Critical: true}}}}
			var stdout, stderr bytes.Buffer
			require.Zero(t, Run(context.Background(), []string{"send", "--config", writeCommandConfig(t, cfg), "--destination", "target"}, strings.NewReader(testutil.WithProducerContext(string(criticalHistoryInput(t, test.status, test.history)), testutil.ProducerContextJSON)), &stdout, &stderr), stderr.String())
			assert.Empty(t, stdout.String())
			if test.skip {
				assert.Equal(t, "alarm-notify: destination \"target\" skipped: critical\nalarm-notify: delivery summary: 0 succeeded, 0 failed, 1 skipped\n", stderr.String())
				_, err := os.Stat(capture)
				assert.ErrorIs(t, err, os.ErrNotExist)
			} else {
				assert.Equal(t, "alarm-notify: destination \"target\" sent\nalarm-notify: delivery summary: 1 succeeded, 0 failed, 0 skipped\n", stderr.String())
				assert.Equal(t, []commandCapture{{Args: []string{}, Env: []string{"GORACE=atexit_sleep_ms=0", "NOTIFIER_TEST_CAPTURE=" + capture, "NOTIFIER_TEST_COMMAND_HELPER=record", "PATH=" + commandexec.DefaultPath}, Input: string(criticalHistoryInput(t, test.status, "")) + "\n"}}, readCommandCaptures(t, capture))
			}
		})
	}
}
