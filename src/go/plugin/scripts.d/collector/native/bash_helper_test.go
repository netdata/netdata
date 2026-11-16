// SPDX-License-Identifier: GPL-3.0-or-later

package native

import (
	"bytes"
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
	bash := "bash"
	if runtime.GOOS == "darwin" {
		bash = "/bin/bash" // Exercise the supported Bash 3.2 on macOS.
	}
	return exec.Command(bash, argv...).Output()
}

func TestBashHelper_EncodesStrings(t *testing.T) {
	value := "quotes \" ' slash \\ literal \\n unicode λ & $() `ticks`"
	for i := 1; i < 32; i++ {
		value += string(rune(i))
	}
	out, err := runBashHelper(
		t,
		`nd_begin; nd_metric depth; nd_sample "$ND_FAMILY" 1 text "$2";
nd_check backlog Backlog queue; nd_check_sample "$ND_FAMILY" unknown queue "$2"; nd_end`,
		value,
	)
	require.NoError(t, err)
	var result snapshot
	require.NoError(t, json.Unmarshal(out, &result))
	assert.Equal(t, value, result.Metrics[0].Samples[0].Labels["text"])
	assert.Equal(t, value, result.Checks[0].Samples[0].Labels["queue"])
}

func TestBashHelper_Snapshot(t *testing.T) {
	tests := map[string]struct {
		code    string
		wantErr bool
	}{
		"empty":            {code: "nd_begin; nd_end"},
		"empty family":     {code: "nd_begin; nd_metric depth; nd_end"},
		"unlabeled metric": {code: `nd_begin; nd_metric depth; nd_sample "$ND_FAMILY" 1; nd_end`},
		"NaN value":        {code: `nd_begin; nd_metric depth; nd_sample "$ND_FAMILY" NaN`, wantErr: true},
		"leading zero":     {code: `nd_begin; nd_metric depth; nd_sample "$ND_FAMILY" 01`, wantErr: true},
		"odd label pairs":  {code: `nd_begin; nd_metric depth; nd_sample "$ND_FAMILY" 1 odd`, wantErr: true},
		"invalid state": {
			code:    `nd_begin; nd_check backlog Backlog; nd_check_sample "$ND_FAMILY" INVALID`,
			wantErr: true,
		},
		"missing check arg":    {code: "nd_begin; nd_check backlog", wantErr: true},
		"wrong handle kind":    {code: `nd_begin; nd_check backlog Backlog; nd_sample "$ND_FAMILY" 1`, wantErr: true},
		"invalid handle":       {code: `nd_begin; nd_sample '1+0' 1`, wantErr: true},
		"overflow handle":      {code: `nd_begin; nd_metric depth; nd_sample 18446744073709551616 1`, wantErr: true},
		"invalid chart handle": {code: `nd_begin; nd_chart '0+0' Title Family 1`, wantErr: true},
		"invalid chart priority": {
			code:    `nd_begin; nd_metric depth; nd_chart "$ND_FAMILY" Title Family 0`,
			wantErr: true,
		},
		"missing active states": {
			code:    `nd_begin; nd_stateset worker enum running; nd_state_sample "$ND_FAMILY" 2 running`,
			wantErr: true,
		},
		"empty bitset": {
			code: `nd_begin; nd_stateset features bitset read write; nd_state_sample "$ND_FAMILY" 0; nd_end`,
		},
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
			_, err = decodeSnapshot(out)
			require.NoError(t, err)
		})
	}
}

func TestBashHelper_InterleavedFamilies(t *testing.T) {
	out, err := runBashHelper(t, `nd_begin
nd_metric depth gauge jobs; depth=$ND_FAMILY
nd_chart "$depth" 'Queue depth' Queues 42
nd_stateset worker enum 'in progress' stopped; worker=$ND_FAMILY
nd_stateset features bitset read write; features=$ND_FAMILY
nd_check backlog Backlog queue; backlog=$ND_FAMILY
nd_sample "$depth" 1.5 queue mail
nd_state_sample "$worker" 1 'in progress' worker_id alpha
nd_check_sample "$backlog" critical queue mail
nd_sample "$depth" 2e1 queue batch
nd_state_sample "$features" 2 read write
nd_check_sample "$backlog" ok queue batch
nd_end`)
	require.NoError(t, err)
	_, err = decodeSnapshot(out)
	require.NoError(t, err)
	assert.JSONEq(t, `{
 "version":"v1",
 "metrics":[
  {"name":"depth","type":"gauge","unit":"jobs","chart_meta":{"title":"Queue depth","family":"Queues","priority":42},
   "samples":[{"value":1.5,"labels":{"queue":"mail"}},{"value":20,"labels":{"queue":"batch"}}]},
  {"name":"worker","type":"stateset","mode":"enum","states":["in progress","stopped"],
   "samples":[{"active":["in progress"],"labels":{"worker_id":"alpha"}}]},
  {"name":"features","type":"stateset","mode":"bitset","states":["read","write"],
   "samples":[{"active":["read","write"],"labels":{}}]}
 ],
 "checks":[{"id":"backlog","title":"Backlog","by_labels":["queue"],
  "samples":[{"state":"critical","labels":{"queue":"mail"}},{"state":"ok","labels":{"queue":"batch"}}]}]
}`, string(out))
}

func TestBashHelper_RejectsActiveCountWithoutErrexit(t *testing.T) {
	for _, count := range []string{"1", "01", "-1", "1+0", "9223372036854775808", "18446744073709551616"} {
		t.Run(count, func(t *testing.T) {
			out, err := runBashHelper(t, `nd_begin
nd_stateset flags bitset read write
set +e
nd_state_sample "$ND_FAMILY" "$2"
status=$?
set -e
[[ $status != 0 ]] || exit 1
nd_state_sample "$ND_FAMILY" 1 read
nd_end`, count)
			require.NoError(t, err, "invalid counts must return failure without aborting the caller")
			snap, err := decodeSnapshot(out)
			require.NoError(t, err)
			require.Len(t, snap.Metrics[0].Samples, 1, "failed calls must not append observations")
			assert.Equal(t, []string{"read"}, snap.Metrics[0].Samples[0].Active)
		})
	}
}

func TestBashHelper_ResetsInterleavedSnapshot(t *testing.T) {
	out, err := runBashHelper(t, `nd_begin
for ((i=0;i<64;i++)); do nd_metric "metric_$i"; done
for ((i=0;i<64;i++)); do handle=$(((i*37)%64)); nd_sample "$handle" "$handle"; done
nd_end
nd_begin
nd_metric metric_0
nd_chart "$ND_FAMILY" '' '' ''
nd_sample "$ND_FAMILY" 99
nd_end`)
	require.NoError(t, err)
	frames := bytes.Split(bytes.TrimSpace(out), []byte{'\n'})
	require.Len(t, frames, 2)
	first, err := decodeSnapshot(frames[0])
	require.NoError(t, err)
	require.Len(t, first.Metrics, 64)
	for i, family := range first.Metrics {
		require.Len(t, family.Samples, 1)
		assert.Equal(t, float64(i), *family.Samples[0].Value)
	}
	_, err = decodeSnapshot(frames[1])
	require.NoError(t, err)
	assert.JSONEq(t, `{"version":"v1","metrics":[{"name":"metric_0","type":"gauge","unit":"value",
 "chart_meta":{},"samples":[{"value":99,"labels":{}}]}],"checks":[]}`, string(frames[1]))
}

func TestBashHelper_SnapshotShapesBothModes(t *testing.T) {
	setupRunner(t)
	tests := map[string]struct {
		body       string
		wantSeries int
	}{
		"empty":        {body: "", wantSeries: 0},
		"metrics only": {body: `nd_metric depth; nd_sample "$ND_FAMILY" 1`, wantSeries: 1},
		"checks only": {
			body:       `nd_check backlog Backlog queue; nd_check_sample "$ND_FAMILY" critical queue mail`,
			wantSeries: 4,
		},
		"stateset": {
			body:       `nd_stateset worker enum 'in progress' stopped; nd_state_sample "$ND_FAMILY" 1 'in progress'`,
			wantSeries: 2,
		},
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
