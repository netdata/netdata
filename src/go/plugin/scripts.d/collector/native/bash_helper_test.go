// SPDX-License-Identifier: GPL-3.0-or-later

package native

import (
	"encoding/json"
	"os/exec"
	"runtime"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Tests for the shipped Bash helper, lib/native.sh.

func runBashHelper(t *testing.T, code string, args ...string) ([]byte, error) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("Bash helper")
	}
	argv := append([]string{"-c", `set -eu; source "$1"; ` + code, "test", bashHelperPath(t)}, args...)
	return exec.Command("bash", argv...).Output()
}

func TestBashHelper_EncodesStrings(t *testing.T) {
	value := "quotes \" ' slash \\ literal \\n unicode λ & $() `ticks`"
	for i := 1; i < 32; i++ {
		value += string(rune(i))
	}
	out, err := runBashHelper(
		t,
		`nd_begin; nd_metric depth 1 text "$2"; nd_check backlog unknown queue "$2"; nd_end`,
		value,
	)
	require.NoError(t, err)
	var result snapshot
	require.NoError(t, json.Unmarshal(out, &result))
	assert.Equal(t, value, result.Metrics[0].Labels["text"])
	assert.Equal(t, value, result.Checks[0].Labels["queue"])
}

func TestBashHelper_Snapshot(t *testing.T) {
	tests := map[string]struct {
		code    string
		wantErr bool
	}{
		"empty":             {code: "nd_begin; nd_end"},
		"unlabeled metric":  {code: "nd_begin; nd_metric depth 1; nd_end"},
		"NaN value":         {code: "nd_begin; nd_metric depth NaN", wantErr: true},
		"leading zero":      {code: "nd_begin; nd_metric depth 01", wantErr: true},
		"odd label pairs":   {code: "nd_begin; nd_metric depth 1 odd", wantErr: true},
		"invalid state":     {code: "nd_begin; nd_check backlog INVALID", wantErr: true},
		"missing check arg": {code: "nd_begin; nd_check backlog", wantErr: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			out, err := runBashHelper(t, tc.code)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.True(t, json.Valid(out))
		})
	}
}

func TestBashHelper_SnapshotShapesBothModes(t *testing.T) {
	setupRunner(t)
	tests := map[string]struct {
		body       string
		wantSeries int
	}{
		"empty":        {body: "", wantSeries: 0},
		"metrics only": {body: "nd_metric depth 1", wantSeries: 1},
		"checks only":  {body: "nd_check backlog critical queue mail", wantSeries: 4}, // one series per check state
	}
	for name, tc := range tests {
		for _, mode := range []string{modeOneshot, modePersistent} {
			t.Run(name+"/"+mode, func(t *testing.T) {
				body := bashHelper(t) + "nd_begin\n" + tc.body + "\nnd_end\n"
				var c *Collector
				if mode == modePersistent {
					c, _ = persistentCollector(t, bashHelper(t)+"nd_ready\nwhile nd_next; do\n"+body+"done\n")
					startRuntime(t, c).waitReady(t)
				} else {
					c, _ = fixtureCollector(t, body)
				}
				values, err := collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
				require.NoError(t, err)
				assert.Len(t, values, tc.wantSeries)
			})
		}
	}
}
