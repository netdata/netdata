// SPDX-License-Identifier: GPL-3.0-or-later

package native

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAgent_FunctionRouting(t *testing.T) {
	setupRunner(t)
	for _, mode := range []string{modeOneshot, modePersistent} {
		t.Run(mode, func(t *testing.T) {
			registry, dir := functionFixture(t, mode, true)
			a := startTestAgent(t, registry, dir)
			a.enable(t, "alpha", 23)
			a.enable(t, "beta", 41)
			result := a.call(t, "bootstrap", "native-fixture:items info", "", 200)
			validateFunctionUI(t, result)
			data := functionJSON(t, result)
			assert.Contains(t, fmt.Sprint(data["required_params"]), "alpha")
			assert.Contains(t, fmt.Sprint(data["required_params"]), "beta")
			requireNoFile(t, filepath.Join(dir, "requests")) // bootstrap info must not execute the script
			a.call(t, "unknown", "native-fixture:items __job:missing", "", 404)
			result = a.call(t, "info", "native-fixture:items info __job:alpha", "", 200)
			validateFunctionUI(t, result)
			assert.Contains(t, result, `"queue"`)
			result = a.call(
				t,
				"data",
				"native-fixture:items __job:beta queue:undeclared",
				`{"filter":"synthetic"}`,
				200,
			)
			validateFunctionUI(t, result)
			rows := functionJSON(t, result)["data"].([]any)
			assert.Equal(t, float64(41), rows[0].([]any)[0])
			raw := a.call(t, "raw", "native-fixture:items __job:alpha raw", "", 500)
			assert.Contains(t, raw, `9007199254740993`)
			result = a.call(t, "error", "native-fixture:items __job:alpha error", "", 503)
			validateFunctionUI(t, result)
			for line := range strings.SplitSeq(a.out.String(), "\n") {
				if strings.HasPrefix(line, "CHART ") {
					assert.NotContains(t, line, "native-fixture", "function-only packages create no charts")
				}
			}
			requests, err := os.ReadFile(filepath.Join(dir, "requests"))
			require.NoError(t, err)
			assert.NotContains(t, string(requests), `"method": "collect"`)
			a.call(t, "disable-alpha", "config scripts.d:collector:native-fixture:alpha disable", "", 200)
			a.call(t, "detached-alpha", "native-fixture:items __job:alpha", "", 404)
			a.call(t, "beta-survives", "native-fixture:items __job:beta", "", 200)
			a.call(t, "disable-beta", "config scripts.d:collector:native-fixture:beta disable", "", 200)
			a.waitOutput(t, `FUNCTION_DEL GLOBAL "native-fixture:items"`)
		})
	}
}

func TestAgent_FunctionReplacement(t *testing.T) {
	setupRunner(t)
	registry, dir := functionFixture(t, modePersistent, true)
	a := startTestAgent(t, registry, dir)
	a.enable(t, "alpha", 23)
	a.send(t, "active", "native-fixture:items __job:alpha wait", "")
	waitFile(t, filepath.Join(dir, "23.active"))
	a.send(t, "queued", "native-fixture:items __job:alpha queued-predecessor", "")
	a.call(
		t,
		"replace",
		"config scripts.d:collector:native-fixture:alpha update",
		`{"update_every":1,"config":{"text":"replacement","count":41}}`,
		202,
	)
	assert.NotContains(t, a.result(t, "active", 0), "active 200 ")
	assert.NotContains(t, a.result(t, "queued", 0), "queued 200 ")
	// Readiness is observable through the current job's Function output.
	var current string
	require.Eventually(t, func() bool {
		id := fmt.Sprintf("current-%d", time.Now().UnixNano())
		current = a.call(t, id, "native-fixture:items __job:alpha", "", 0)
		return strings.Contains(current, `[[41,`)
	}, 5*time.Second, 20*time.Millisecond)
	validateFunctionUI(t, current)
	requests, err := os.ReadFile(filepath.Join(dir, "requests"))
	require.NoError(t, err)
	assert.NotContains(t, string(requests), "queued-predecessor")
	a.send(t, "disable-active", "native-fixture:items __job:alpha wait", "")
	waitFile(t, filepath.Join(dir, "41.active"))
	a.call(t, "disable", "config scripts.d:collector:native-fixture:alpha disable", "", 200)
	assert.NotContains(t, a.result(t, "disable-active", 0), "disable-active 200 ")
	assert.Contains(t, a.out.String(), `FUNCTION_DEL GLOBAL "native-fixture:items"`)
}

func TestAgent_FunctionActiveCancellation(t *testing.T) {
	setupRunner(t)
	registry, dir := functionFixture(t, modePersistent, false)
	a := startTestAgent(t, registry, dir)
	a.enable(t, "alpha", 23)
	a.send(t, "active", "native-fixture:items __job:alpha wait", "")
	waitFile(t, filepath.Join(dir, "23.active"))
	a.cancel(t, "active")
	a.result(t, "active", 499)
	before := strings.Count(a.out.String(), " = 23")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "release"), nil, 0644))
	a.call(t, "after", "native-fixture:items __job:alpha", "", 200)
	require.Eventually(t, func() bool {
		return strings.Count(a.out.String(), " = 23") > before
	}, 3*time.Second, 10*time.Millisecond, "collection must resume after caller cancellation")
	assert.NotContains(t, a.out.String(), "status failed")
	assert.NotContains(t, a.out.String(), `FUNCTION_DEL GLOBAL "native-fixture:items"`)
}

func TestAgent_FunctionQueuedCancellation(t *testing.T) {
	setupRunner(t)
	registry, dir := functionFixture(t, modePersistent, true)
	a := startTestAgent(t, registry, dir)
	a.enable(t, "alpha", 23)
	a.send(t, "active", "native-fixture:items __job:alpha wait", "")
	waitFile(t, filepath.Join(dir, "23.active"))
	a.send(t, "queued", "native-fixture:items __job:alpha canceled-queued", "")
	a.cancel(t, "queued")
	a.result(t, "queued", 499)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "release"), nil, 0644))
	a.result(t, "active", 200)
	a.call(t, "after", "native-fixture:items __job:alpha", "", 200)
	requests, err := os.ReadFile(filepath.Join(dir, "requests"))
	require.NoError(t, err)
	assert.NotContains(t, string(requests), "canceled-queued")
}

func TestAgent_PackageDynCfgReplacement(t *testing.T) {
	setupRunner(t)
	registry, dir := configuredFixture(t, configPeer(t), modePersistent)
	a := startTestAgent(t, registry, dir)
	call := func(id, command, payload string, code int) string {
		t.Helper()
		return a.call(t, id, "config scripts.d:collector:native-fixture"+command, payload, code)
	}
	schema := call("schema", " schema", "", 200)
	assert.Contains(t, schema, `#/properties/config/definitions/text`)
	call("add", " add job", `{"update_every":1,"config":{"text":"synthetic","count":23}}`, 202)
	call("enable", ":job enable", "", 202)
	a.waitOutput(t, " = 23")
	call("invalid", ":job update", `{"update_every":1,"config":{"text":"synthetic","count":-5}}`, 422)
	assert.Contains(t, call("get-original", ":job get", "", 200), `"count":23`)
	call("update", ":job update", `{"update_every":1,"config":{"text":"replacement","count":41}}`, 202)
	a.waitOutput(t, " = 41")
	call("disable", ":job disable", "", 200)
}
