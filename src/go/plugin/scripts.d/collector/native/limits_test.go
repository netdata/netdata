// SPDX-License-Identifier: GPL-3.0-or-later

package native

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Real peers move a snapshot well above 1 MiB through both transports.
func TestLimits_LargeSnapshot(t *testing.T) {
	setupRunner(t)
	samples := make([]metricSample, 10000)
	for i := range samples {
		value := float64(i)
		samples[i] = metricSample{
			Value: &value,
			Labels: map[string]string{
				"queue":       fmt.Sprintf("queue-%05d", i),
				"region":      "eu-west-1",
				"service":     "payments-api",
				"environment": "production",
			},
		}
	}
	data, err := json.Marshal(snapshot{
		Version: "v1",
		Metrics: []metricFamily{{Name: "depth", Samples: samples}},
		Checks:  []checkFamily{},
	})
	require.NoError(t, err)
	require.Greater(t, len(data), 1<<20)
	const cat = `cat "$(dirname "$0")/response.json"` + "\n"
	for _, mode := range []string{modeOneshot, modePersistent} {
		t.Run(mode, func(t *testing.T) {
			var c *Collector
			var dir string
			frame := data
			if mode == modePersistent {
				c, dir = persistentCollector(t, readyLine+"read -r request\n"+cat+"read -r request\n")
				frame = append(append([]byte(`{"id":"1","result":`), data...), '}')
			} else {
				c, dir = fixtureCollector(t, cat)
			}
			c.Timeout = confopt.Duration(10 * time.Second)
			require.NoError(t, os.WriteFile(filepath.Join(dir, "response.json"), append(frame, '\n'), 0644))
			if mode == modePersistent {
				startRuntime(t, c).waitReady(t)
			}
			values, err := collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
			require.NoError(t, err)
			assert.Len(t, values, len(samples))
			total := float64(0)
			for _, value := range values {
				total += value
			}
			assert.Equal(t, float64(len(samples)*(len(samples)-1)/2), total)
		})
	}
}

// Multi-chunk configuration and Function payloads reach the script in both modes.
func TestLimits_LargeFunctionAndConfiguration(t *testing.T) {
	setupRunner(t)
	for _, mode := range []string{modeOneshot, modePersistent} {
		t.Run(mode, func(t *testing.T) {
			registry, dir := functionFixture(t, mode, true)
			peer := "import base64\n" + strings.Replace(
				functionPeer,
				`    args = req["args"]`,
				`    args = req["args"]
    if "large" in args:
        return {"version":"v1", "status":200, "columns":{}, "data":[[config["text"], len(base64.b64decode(req["payload_base64"]))]]}`,
				1,
			)
			require.NoError(t, os.WriteFile(filepath.Join(dir, "peer.py"), []byte(peer), 0644))
			c := registry["native-fixture"].CreateV2().(*Collector)
			text := strings.Repeat("x", testTransferBytes)
			c.ScriptConfig = Settings{
				"text": text,
			}
			c.Timeout = confopt.Duration(10 * time.Second)
			require.NoError(t, c.Init(context.Background()))
			require.NoError(t, c.Check(context.Background()))
			if mode == modePersistent {
				startRuntime(t, c).waitReady(t)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			result, err := c.executeFunction(ctx, funcapi.RawMethodRequest{
				Method:  "items",
				Args:    []string{"large"},
				Payload: bytes.Repeat([]byte{'x'}, testTransferBytes),
			})
			require.NoError(t, err)
			require.Equal(t, 200, result.Status)
			assert.Equal(t, [][]any{{text, json.Number(strconv.Itoa(testTransferBytes))}}, result.Data)
		})
	}
}

func TestLimits_ConfigurationBudget(t *testing.T) {
	registry, _ := configuredFixture(t, "exit 0\n", modeOneshot)
	c := registry["native-fixture"].CreateV2().(*Collector)
	c.messageLimit = testMessageLimit
	c.ScriptConfig = Settings{"text": strings.Repeat("x", testMessageLimit)}
	require.ErrorContains(t, c.Init(context.Background()), "config envelope exceeds 64 MiB")
	assert.Error(t, c.Check(context.Background()), "oversized configuration must not initialize the collector")
}

func TestLimits_OneshotBudget(t *testing.T) {
	setupRunner(t)
	c, _ := fixtureCollectorWithLimit(
		t, fmt.Sprintf("head -c %d /dev/zero\n", testMessageLimit+1), testMessageLimit,
	)
	require.ErrorIs(t, c.Collect(context.Background()), errResponseTooLarge)
	assert.Empty(t, rawSeries(c), "oversized output must not publish metrics")
}

func TestLimits_FunctionBeforeRuntime(t *testing.T) {
	registry, dir := functionFixture(t, modePersistent, true)
	c := initFunctionCollectorWithLimit(t, registry, testMessageLimit)
	result, err := c.executeFunction(context.Background(), funcapi.RawMethodRequest{
		Method:  "items",
		Payload: make([]byte, testMessageLimit*3/4),
	})
	require.ErrorContains(t, err, "exceeds 64 MiB", "size rejection precedes runtime admission")
	assert.Nil(t, result)
	requireNoFile(t, filepath.Join(dir, "starts"))
}
