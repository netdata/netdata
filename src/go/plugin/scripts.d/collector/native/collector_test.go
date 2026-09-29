// SPDX-License-Identifier: GPL-3.0-or-later

package native

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupRunner(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("Bash fixture requires Unix; protocol tests are portable")
	}
	// Exercise the production command/cancellation path. This shim substitutes only
	// privilege dropping, which cannot be exercised by an unprivileged unit test.
	path := filepath.Join(t.TempDir(), "nd-run")
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\nexec \"$@\"\n"), 0755))
	t.Cleanup(ndexec.SetRunnerPathsForTests(path, ""))
}

func fixtureCollector(t *testing.T, body string) (*Collector, string) {
	t.Helper()
	dir := t.TempDir()
	script := filepath.Join(dir, "collect.sh")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/bash\nset -eu\n"+body), 0755))
	charts := `version: v1
context_namespace: fixture
engine:
  autogen:
    enabled: false
groups:
  - family: Queue
    metrics: [depth, processed_total]
    charts:
      - id: depth
        title: Queue Depth
        context: depth
        units: jobs
        instances:
          by_labels: [queue]
        label_promotion: [region]
        dimensions:
          - selector: depth
            name: depth
      - id: processed
        title: Jobs Processed
        context: processed
        units: jobs/s
        instances:
          by_labels: [queue]
        dimensions:
          - selector: processed_total
            name: processed
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "charts.yaml"), []byte(charts), 0644))
	path := filepath.Join(dir, "manifest.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`version: v1
command: [./collect.sh]
charts: charts.yaml
metrics:
  - {name: depth, type: gauge, unit: jobs}
  - {name: processed_total, type: counter, unit: jobs}
checks:
  - id: backlog
    title: Queue Backlog
    by_labels: [queue]
`), 0644))
	c := New()
	c.Manifest = path
	// Permission policy has its own production tests in pathvalidate. Everything
	// after local path validation runs unchanged, including the real executable.
	c.validateExecutable = func(path string) (string, error) { _, err := os.Stat(path); return path, err }
	require.NoError(t, c.Init(context.Background()))
	require.NoError(t, c.Check(context.Background()))
	return c, dir
}

func validResponse(state string) string {
	return fmt.Sprintf(
		`{"version":"v1","metrics":[{"name":"depth","value":17,"labels":{"queue":"mail","region":"east"}},{"name":"processed_total","value":100,"labels":{"queue":"mail"}}],"checks":[{"id":"backlog","state":%q,"labels":{"queue":"mail"}}]}`,
		state,
	)
}

func TestCollectSnapshots(t *testing.T) {
	setupRunner(t)
	c, dir := fixtureCollector(t, "[[ $1 == collect ]]\ncat \"$(dirname \"$0\")/response.json\"\n")
	file := filepath.Join(dir, "response.json")
	for _, state := range []string{"critical", "warning", "unknown", "ok"} {
		require.NoError(t, os.WriteFile(file, []byte(validResponse(state)), 0644))
		values, err := collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
		require.NoError(t, err)
		expected := map[string]float64{
			`depth{queue="mail",region="east"}`: 17,
			`processed_total{queue="mail"}`:     100,
		}
		for _, candidate := range checkStates {
			v := float64(0)
			if state == candidate {
				v = 1
			}
			expected[`native.check.backlog{native.check.backlog="`+candidate+`",queue="mail"}`] = v
		}
		assert.Equal(t, expected, values)
		collecttest.AssertChartCoverage(
			t,
			c,
			collecttest.ChartCoverageExpectation{
				RequiredContexts: map[string][]string{
					"fixture.depth": {
						"depth",
					}, "fixture.processed": {"processed"}, "native_script.check_state": {"ok", "warning", "critical", "unknown"},
				},
			},
		)
	}
	// A valid empty snapshot means disappearance; it must not replay the last OK.
	require.NoError(t, os.WriteFile(file, []byte(`{"version":"v1","metrics":[],"checks":[]}`), 0644))
	values, err := collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
	require.NoError(t, err)
	assert.Empty(t, values)
}

func TestDecodeResponse(t *testing.T) {
	c, _ := fixtureCollector(t, "exit 0\n")
	tests := map[string]string{
		"truncated":               `{"version":"v1"`,
		"trailing":                validResponse("ok") + `{}`,
		"unsupported version":     `{"version":"v2"}`,
		"unknown field":           `{"version":"v1","surprise":1}`,
		"case folded field":       `{"version":"v1","Metrics":[]}`,
		"case folded check state": `{"version":"v1","checks":[{"id":"backlog","state":"critical","State":"ok","labels":{"queue":"mail"}}]}`,
		"null metric label":       `{"version":"v1","metrics":[{"name":"depth","value":1,"labels":{"queue":null}}]}`,
		"null check metadata":     `{"version":"v1","checks":[{"id":"backlog","state":"ok","labels":{"queue":"mail","region":null}}]}`,
		"null metrics":            `{"version":"v1","metrics":null}`,
		"null checks":             `{"version":"v1","checks":null}`,
		"null labels":             `{"version":"v1","metrics":[{"name":"depth","value":1,"labels":null}]}`,

		"missing value":              `{"version":"v1","metrics":[{"name":"depth"}]}`,
		"null value":                 `{"version":"v1","metrics":[{"name":"depth","value":null}]}`,
		"overflow":                   `{"version":"v1","metrics":[{"name":"depth","value":1e999}]}`,
		"negative counter":           `{"version":"v1","metrics":[{"name":"processed_total","value":-1}]}`,
		"undeclared metric":          `{"version":"v1","metrics":[{"name":"new","value":0}]}`,
		"duplicate empty labels":     `{"version":"v1","metrics":[{"name":"depth","value":0},{"name":"depth","value":1,"labels":{}}]}`,
		"duplicate reordered labels": `{"version":"v1","metrics":[{"name":"depth","value":0,"labels":{"a":"1","b":"2"}},{"name":"depth","value":1,"labels":{"b":"2","a":"1"}}]}`,
		"duplicate key":              `{"version":"v1","version":"v1"}`,
		"duplicate label key":        `{"version":"v1","metrics":[{"name":"depth","value":0,"labels":{"queue":"first","queue":"second"}}]}`,
		"missing identity":           `{"version":"v1","checks":[{"id":"backlog","state":"ok"}]}`,
		"invalid state":              strings.Replace(validResponse("ok"), `"state":"ok"`, `"state":"bad"`, 1),
		"duplicate check":            `{"version":"v1","checks":[{"id":"backlog","state":"ok","labels":{"queue":"a","region":"east"}},{"id":"backlog","state":"critical","labels":{"queue":"a","region":"west"}}]}`,
		"invalid utf8":               "{\"version\":\"v1\",\"x\":\"\xff\"}",
	}
	for name, data := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := c.definition.decodeResponse([]byte(data))
			require.Error(t, err)
			assert.NotContains(t, err.Error(), "1e999", "errors must not expose raw sample values")
		})
	}
}

func TestInvalidBatchWritesNothing(t *testing.T) {
	setupRunner(t)
	c, _ := fixtureCollector(
		t,
		`printf '%s\n' '{"version":"v1","metrics":[{"name":"depth","value":123},{"name":"missing","value":1}]}'`+"\n",
	)
	managed, ok := metrix.AsCycleManagedStore(c.store)
	require.True(t, ok)
	managed.CycleController().BeginCycle()
	require.Error(t, c.Collect(context.Background()))
	// Commit deliberately in this test to prove the parser staged nothing, even
	// before the production runtime's additional AbortCycle protection.
	managed.CycleController().CommitCycleSuccess()
	values := map[string]float64{}
	c.store.Read(metrix.ReadRaw()).
		ForEachSeries(func(name string, _ metrix.LabelView, value float64) { values[name] = value })
	assert.Empty(t, values)
}

func TestRunCommandFailureAndLimit(t *testing.T) {
	setupRunner(t)
	for _, tc := range []struct {
		name, body string
		timeout    time.Duration
		want       string
	}{
		{"nonzero", "printf '%s' 'SYNTHETIC_SECRET' >&2; exit 7", time.Second, "exit status 7"},
		{"timeout", "sleep 30", 50 * time.Millisecond, "deadline exceeded"},
		{"over limit", fmt.Sprintf("head -c %d /dev/zero; sleep 30", maxMessageBytes+1), 5 * time.Second, "exceeds 64 MiB"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := fixtureCollector(t, tc.body+"\n")
			start := time.Now()
			data, err := runCommand(context.Background(), tc.timeout, c.definition.Command, nil)
			require.ErrorContains(t, err, tc.want)
			assert.Nil(t, data)
			assert.NotContains(t, err.Error(), "SYNTHETIC_SECRET")
			assert.Less(t, time.Since(start), tc.timeout+time.Second)
		})
	}
	// Test the exact protocol boundary independently of executable chunking.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	b := responseBuffer{
		cancel: cancel,
	}
	n, err := b.Write(make([]byte, maxMessageBytes))
	require.NoError(t, err)
	assert.Equal(t, maxMessageBytes, n)
	_, err = b.Write([]byte{1})
	require.ErrorIs(t, err, errResponseTooLarge)
	assert.ErrorIs(t, ctx.Err(), context.Canceled)
}

func TestBashEncoder(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Bash helper")
	}
	helper, err := filepath.Abs("../../lib/native.sh")
	require.NoError(t, err)
	value := "quotes \" ' slash \\ literal \\n unicode λ & $() `ticks`"
	for i := 1; i < 32; i++ {
		value += string(rune(i))
	}
	cmd := exec.Command(
		"bash",
		"-c",
		`set -eu; source "$1"; nd_begin; nd_metric depth 1 text "$2"; nd_check backlog unknown queue "$2"; nd_end`,
		"test",
		helper,
		value,
	)
	out, err := cmd.Output()
	require.NoError(t, err)
	var result response
	require.NoError(t, json.Unmarshal(out, &result))
	assert.Equal(t, value, result.Metrics[0].Labels["text"])
	assert.Equal(t, value, result.Checks[0].Labels["queue"])
	for _, code := range []string{"nd_begin; nd_end", "nd_begin; nd_metric depth 1; nd_end"} {
		out, err = exec.Command("bash", "-c", `set -eu; source "$1"; `+code, "test", helper).Output()
		require.NoError(t, err)
		assert.True(t, json.Valid(out))
	}
	for _, code := range []string{"nd_metric depth NaN", "nd_metric depth 01", "nd_metric depth 1 odd", "nd_check backlog INVALID"} {
		require.Error(t, exec.Command("bash", "-c", `set -eu; source "$1"; nd_begin; `+code, "test", helper).Run())
	}
}

func TestCheckWithoutIdentityLabels(t *testing.T) {
	setupRunner(t)
	c, dir := fixtureCollector(t, `printf '%s' '{"version":"v1","checks":[{"id":"probe","state":"critical"}]}'`+"\n")
	require.NoError(
		t,
		os.WriteFile(
			filepath.Join(dir, "manifest.yaml"),
			[]byte("version: v1\ncommand: [./collect.sh]\nchecks:\n  - {id: probe, title: Probe}\n"),
			0644,
		),
	)
	require.NoError(t, c.Init(context.Background()))
	_, err := collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
	require.NoError(t, err)
	collecttest.AssertChartCoverage(
		t,
		c,
		collecttest.ChartCoverageExpectation{
			RequiredContexts: map[string][]string{
				"native_script.check_state": {"critical", "ok", "warning", "unknown"},
			},
		},
	)
}

func TestManifestValidation(t *testing.T) {
	c, dir := fixtureCollector(t, "printf ran > \"$(dirname \"$0\")/executed\"\nexit 99\n")
	valid, err := os.ReadFile(c.Manifest)
	require.NoError(t, err)
	cases := map[string]string{
		"mode":            string(valid) + "mode: push\n",
		"version":         strings.Replace(string(valid), "version: v1", "version: v2", 1),
		"unknown field":   string(valid) + "typo: true\n",
		"second document": string(valid) + "---\nversion: v1\n",
		"duplicate declaration": strings.Replace(
			string(valid),
			"  - {name: processed_total, type: counter, unit: jobs}",
			"  - {name: depth, type: gauge, unit: jobs}",
			1,
		),
		"reserved metric":    strings.Replace(string(valid), "name: depth", "name: native.check.test", 1),
		"invalid type":       strings.Replace(string(valid), "type: gauge", "type: typo", 1),
		"duplicate identity": strings.Replace(string(valid), "by_labels: [queue]", "by_labels: [queue, queue]", 1),
		"empty command":      strings.Replace(string(valid), "command: [./collect.sh]", "command: []", 1),
		"secret file":        "SYNTHETIC_SECRET\n",
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			require.NoError(t, os.WriteFile(c.Manifest, []byte(data), 0644))
			err := c.Init(context.Background())
			require.Error(t, err)
			assert.NotContains(t, err.Error(), "SYNTHETIC_SECRET")
		})
	}
	require.NoError(t, os.WriteFile(c.Manifest, valid, 0644))
	// Neither initialization nor Check runs the executable.
	require.NoError(t, c.Init(context.Background()))
	require.NoError(t, c.Check(context.Background()))
	_, err = os.Stat(filepath.Join(dir, "executed"))
	require.ErrorIs(t, err, os.ErrNotExist)
	c.Manifest = "relative.yaml"
	require.ErrorContains(t, c.Init(context.Background()), "absolute")
	untrusted := New()
	untrusted.Manifest = filepath.Join(dir, "manifest.yaml")
	if runtime.GOOS != "windows" {
		require.Error(
			t,
			untrusted.Init(context.Background()),
			"production path validation must reject the test-owned executable",
		)
	}
}

func TestConfigurationSerialize(t *testing.T) {
	collecttest.TestConfigurationSerialize(
		t,
		New(),
		[]byte(
			`{"manifest":"/opt/custom/manifest.yaml","update_every":15,"timeout":3.5,"autodetection_retry":60,"config":{"enabled":false,"count":0,"nested":{"optional":null}}}`,
		),
		[]byte(
			"manifest: /opt/custom/manifest.yaml\nupdate_every: 15\ntimeout: 3.5\nautodetection_retry: 60\nconfig:\n  enabled: false\n  count: 0\n  nested:\n    optional: null\n",
		),
	)
}

func TestConfigSchemaMatchesMetadata(t *testing.T) {
	collecttest.AssertConfigSchemaMatchesMetadataWith(
		t,
		"config_schema.json",
		"metadata.dev.yaml",
		collecttest.ConfigSchemaCheck{
			Defaults: true,
		},
	)
}

func TestLabelKeysAreData(t *testing.T) {
	c, _ := fixtureCollector(t, "exit 0\n")
	data := `{"version":"v1","metrics":[{"name":"depth","value":0,"labels":{"Version":"x","metrics":"y","State":"z"}}]}`
	result, err := c.definition.decodeResponse([]byte(data))
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"Version": "x", "metrics": "y", "State": "z"}, result.Metrics[0].Labels)
}
