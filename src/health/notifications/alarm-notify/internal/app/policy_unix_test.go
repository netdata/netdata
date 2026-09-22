// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || darwin

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/testutil"

	notifyevent "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/event"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/notifier"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunCommandStatusPolicies(t *testing.T) {
	t.Setenv("NOTIFIER_POLICY_MISSING", "")
	require.NoError(t, os.Unsetenv("NOTIFIER_POLICY_MISSING"))
	for name, test := range map[string]struct {
		status  string
		missing bool
		code    int
		message string
		calls   bool
	}{
		"skip warning": {status: "WARNING", message: "0 succeeded, 0 failed, 1 skipped"},
		"skip clear":   {status: "CLEAR", message: "0 succeeded, 0 failed, 1 skipped"},
		"skip secret":  {status: "WARNING", missing: true, message: "0 succeeded, 0 failed, 1 skipped"},
		"critical":     {status: "CRITICAL", calls: true, message: "1 succeeded, 0 failed, 0 skipped"},
		"critical still needs secret": {status: "CRITICAL", missing: true, code: 1,
			message: "environment variable is not set"},
	} {
		t.Run(name, func(t *testing.T) {
			dst, capture := testCommandDestination(t, "record")
			if test.missing {
				dst["env"].(map[string]string)["TOKEN"] = "${env:NOTIFIER_POLICY_MISSING}"
			}
			cfg := testConfig{Version: 1, Destinations: map[string]map[string]any{"dev": dst},
				Routing: notifier.Routing{Policies: map[string]*notifier.DestinationPolicy{"dev": {NoWarn: true, NoClear: true}}}}
			event := testutil.ExpectedEvent()
			event.Status = test.status
			input, err := json.Marshal(event)
			require.NoError(t, err)
			var stdout, stderr bytes.Buffer
			assert.Equal(t, test.code, Run(context.Background(),
				[]string{"send", "--config", writeCommandConfig(t, cfg), "--destination", "dev"},
				bytes.NewReader(input), &stdout, &stderr), stderr.String())
			assert.Empty(t, stdout.String())
			assert.Contains(t, stderr.String(), test.message)
			if test.calls {
				captured := readCommandCaptures(t, capture)
				require.Len(t, captured, 1)
				var got notifyevent.Event
				require.NoError(t, json.NewDecoder(strings.NewReader(captured[0].Input)).Decode(&got))
				assert.Equal(t, event, got)
			} else {
				_, err := os.Stat(capture)
				assert.ErrorIs(t, err, os.ErrNotExist, "filtered commands must not start")
			}
		})
	}
}
