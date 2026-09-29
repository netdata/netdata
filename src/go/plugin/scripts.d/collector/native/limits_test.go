// SPDX-License-Identifier: GPL-3.0-or-later

package native

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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

func TestLargeCollectionSnapshots(t *testing.T) {
	setupRunner(t)
	samples := make([]metricSample, 10000)
	for i := range samples {
		value := float64(i)
		samples[i] = metricSample{
			Name:  "depth",
			Value: &value,
			Labels: map[string]string{
				"queue":       fmt.Sprintf("queue-%05d", i),
				"region":      "eu-west-1",
				"service":     "payments-api",
				"environment": "production",
			},
		}
	}
	snapshot, err := json.Marshal(response{
		Version: "v1",
		Metrics: samples,
		Checks:  []checkSample{},
	})
	require.NoError(t, err)
	require.Greater(t, len(snapshot), 1<<20, "exercise a real snapshot above the old cap")
	for _, mode := range []string{modeOneshot, modePersistent} {
		t.Run(mode, func(t *testing.T) {
			body := `cat "$(dirname "$0")/response.json"` + "\n"
			var c *Collector
			var dir string
			data := snapshot
			if mode == modePersistent {
				c, dir = persistentCollector(
					t,
					"printf '%s\\n' '{\"version\":\"v1\",\"ready\":true}'\nread -r request\n"+body+"read -r request\n",
				)
				data = append(append([]byte(`{"id":"1","result":`), snapshot...), '}')
			} else {
				c, dir = fixtureCollector(t, body)
			}
			c.Timeout = confopt.Duration(10 * time.Second)
			require.NoError(t, os.WriteFile(filepath.Join(dir, "response.json"), append(data, '\n'), 0644))
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

func TestLargeFunctionAndConfiguration(t *testing.T) {
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
			text := strings.Repeat("x", 2<<20)
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
			result, err := c.ExecuteFunction(ctx, funcapi.RawMethodRequest{
				Method:  "items",
				Args:    []string{"large"},
				Payload: bytes.Repeat([]byte{'x'}, 20<<20),
			})
			require.NoError(t, err)
			require.Equal(t, 200, result.Status)
			rows := result.Data.([][]any)
			require.Len(t, rows, 1)
			require.Len(t, rows[0], 2)
			assert.Equal(t, text, rows[0][0])
			assert.Equal(t, json.Number("20971520"), rows[0][1])
		})
	}
}

func TestConfigurationEnvelopeBoundary(t *testing.T) {
	registry, _ := configuredFixture(t, "exit 0\n", modeOneshot)
	c := registry["native-fixture"].CreateV2().(*Collector)
	c.ScriptConfig = Settings{
		"text": "x",
	}
	frame, err := c.configurationEnvelope()
	require.NoError(t, err)
	// Include the JSON envelope, effective defaults and terminating LF.
	c.ScriptConfig["text"] = strings.Repeat("x", maxMessageBytes-len(frame)+1)
	frame, err = c.configurationEnvelope()
	require.NoError(t, err)
	assert.Len(t, frame, maxMessageBytes)
	c.ScriptConfig["text"] = c.ScriptConfig["text"].(string) + "x"
	_, err = c.configurationEnvelope()
	require.ErrorContains(t, err, "config envelope exceeds 64 MiB")
}
