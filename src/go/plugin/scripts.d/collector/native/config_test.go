// SPDX-License-Identifier: GPL-3.0-or-later

package native

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

// The limit covers the JSON envelope, effective defaults and terminating LF.
func TestConfigEnvelope_SizeLimit(t *testing.T) {
	registry, _ := configuredFixture(t, "exit 0\n", modeOneshot)
	definition := registry["native-fixture"].CreateV2().(*Collector).definition
	settings := Settings{
		"text": "x",
	}
	frame, err := definition.configEnvelope(settings)
	require.NoError(t, err)
	settings["text"] = strings.Repeat("x", maxMessageBytes-len(frame)+1)
	frame, err = definition.configEnvelope(settings)
	require.NoError(t, err)
	assert.Len(t, frame, maxMessageBytes)
	settings["text"] = settings["text"].(string) + "x"
	_, err = definition.configEnvelope(settings)
	require.ErrorContains(t, err, "config envelope exceeds 64 MiB")
}

func TestConfigEnvelope_PackageWithoutForm(t *testing.T) {
	c, _ := fixtureCollector(t, "exit 0\n")
	envelope, err := c.definition.configEnvelope(nil)
	require.NoError(t, err)
	assert.Nil(t, envelope, "schema-free packages receive EOF on stdin")
	_, err = c.definition.configEnvelope(Settings{
		"text": "x",
	})
	require.ErrorContains(t, err, "does not declare config_schema")
}

func TestConfigEnvelope_ConfiguredPeers(t *testing.T) {
	setupRunner(t)
	for _, mode := range []string{modeOneshot, modePersistent} {
		t.Run(mode, func(t *testing.T) {
			registry, _ := configuredFixture(t, configPeer(t), mode)
			c := registry["native-fixture"].CreateV2().(*Collector)
			// Preserve strings, nested values, explicit zero and false through YAML and stdin.
			config := "config:\n  text: 'quotes \" and \\ unicode λ'\n  count: 23\n  enabled: false\n"
			require.NoError(t, yaml.Unmarshal([]byte(config), c))
			out := &wireOutput{}
			job, _ := startTestJob(t, c, out)
			tickUntil(t, job, func() bool { return strings.Contains(out.String(), " = 23") })
			assert.NotContains(t, out.String(), "unicode")
		})
	}
}

// A Python peer receives exactly the envelope Init encoded, in both modes.
func TestConfigEnvelope_RoundTrip(t *testing.T) {
	setupRunner(t)
	python := requireTool(t, "python3")
	const peer = `import json, pathlib, sys
config = json.loads(sys.stdin.readline())
pathlib.Path(__file__).with_suffix('.received').write_text(json.dumps(config))
result = {"version":"v1", "metrics":[{"name":"depth","value":config["config"]["count"],"labels":{"queue":"mail"}}]}
if sys.argv[-1] == 'serve':
    print(json.dumps({"version":"v1","ready":True}), flush=True)
    for line in sys.stdin:
        request = json.loads(line)
        print(json.dumps({"id":request["id"],"result":result}), flush=True)
else:
    assert sys.stdin.read() == ''
    print(json.dumps(result))
`
	for _, mode := range []string{modeOneshot, modePersistent} {
		t.Run(mode, func(t *testing.T) {
			registry, dir := configuredFixture(
				t,
				fmt.Sprintf("exec %q \"$(dirname \"$0\")/peer.py\" \"$@\"\n", python),
				mode,
			)
			require.NoError(t, os.WriteFile(filepath.Join(dir, "peer.py"), []byte(peer), 0644))
			c := registry["native-fixture"].CreateV2().(*Collector)
			c.ScriptConfig = Settings{
				"text":     "line one\nline two\r\tλ\\\"",
				"count":    float64(0),
				"enabled":  false,
				"optional": nil,
			}
			require.NoError(t, c.Init(context.Background()))
			if mode == modePersistent {
				startRuntime(t, c).waitReady(t)
			}
			values, err := collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
			require.NoError(t, err)
			assert.Equal(t, map[string]float64{`depth{queue="mail"}`: 0}, values)
			received, err := os.ReadFile(filepath.Join(dir, "peer.received"))
			require.NoError(t, err)
			var got, want any
			require.NoError(t, json.Unmarshal(received, &got))
			require.NoError(t, json.Unmarshal(c.configEnvelope, &want))
			assert.Equal(t, want, got)
		})
	}
}
