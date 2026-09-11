// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	snmpdiag "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/diagnostics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalCommands(t *testing.T) {
	root := t.TempDir()
	current, previous := "11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222"
	now := time.Now().UTC()
	var currentPath string
	for _, runID := range []string{current, previous} {
		directory := filepath.Join(root, snmpdiag.NormalDirectory, runID)
		require.NoError(t, os.MkdirAll(directory, 0700))
		path := filepath.Join(directory, "device-00000000000000000007.zst")
		hostname := "current.example"
		if runID == previous {
			hostname = "previous.example"
		} else {
			currentPath = path
		}
		var data bytes.Buffer
		require.NoError(t, snmpdiag.Write(&data, snmpdiag.Document{Format: snmpdiag.Format, Version: snmpdiag.Version, Kind: snmpdiag.KindNormal,
			Producer: snmpdiag.Producer{RunID: runID}, Normal: &snmpdiag.NormalDevice{RegistrationID: 7, RuntimeID: 1, Hostname: hostname, CapturedAt: now,
				Latest: &snmpdiag.NormalAttempt{ID: 2, Phase: "collect", StartedAt: now, CompletedAt: now, Samples: map[string]int64{"sample": 42}},
			},
		}))
		require.NoError(t, os.WriteFile(path, data.Bytes(), 0600))
	}
	require.NoError(t, os.WriteFile(filepath.Join(root, snmpdiag.NormalDirectory, "runs.json"), []byte(fmt.Sprintf(`{"current":%q,"previous":%q}`, current, previous)), 0600))
	for name, tc := range map[string]struct {
		args []string
		want string
		code int
	}{
		"list rejects normal":             {[]string{"list", "--input", root, "--normal"}, "does not support", 1},
		"list rejects previous run":       {[]string{"list", "--input", root, "--previous-run"}, "does not support", 1},
		"list rejects registration":       {[]string{"list", "--input", root, "--registration-id", "7"}, "does not support", 1},
		"list rejects combined selectors": {[]string{"list", "--input", root, "--normal", "--previous-run", "--registration-id", "7"}, "does not support", 1},
		"list both runs":                  {[]string{"list", "--input", root}, `"previous": true`, 0},
		"direct summary":                  {[]string{"summary", "--input", currentPath}, "current.example", 0},
		"directory summary":               {[]string{"summary", "--input", root, "--normal", "--registration-id", "7"}, "current.example", 0},
		"previous run":                    {[]string{"summary", "--input", root, "--normal", "--previous-run", "--registration-id", "7"}, "previous.example", 0},
		"validate":                        {[]string{"validate", "--input", currentPath}, `"valid": true`, 0},
		"inspect":                         {[]string{"inspect-device", "--input", currentPath, "--registration-id", "7"}, `"sample": 42`, 0},
		"wrong device":                    {[]string{"inspect-device", "--input", currentPath, "--registration-id", "8"}, "contains device 7", 1},
		"unretained device":               {[]string{"summary", "--input", root, "--normal", "--registration-id", "8"}, "not retained", 1},
		"no normal replay":                {[]string{"replay", "--input", currentPath}, "requires topology evidence", 1},
		"missing selection":               {[]string{"summary", "--input", root, "--normal"}, "requires --registration-id", 1},
		"conflicting selection":           {[]string{"summary", "--input", root, "--normal", "--registration-id", "7", "--checkpoint", "1"}, "cannot select a topology checkpoint", 1},
	} {
		t.Run(name, func(t *testing.T) {
			var out, errors bytes.Buffer
			assert.Equal(t, tc.code, run(tc.args, &out, &errors), errors.String())
			if tc.code == 0 {
				assert.Contains(t, out.String(), tc.want)
				assert.Empty(t, errors.String())
			} else {
				assert.Contains(t, errors.String(), tc.want)
			}
		})
	}
}
