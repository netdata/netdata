// SPDX-License-Identifier: GPL-3.0-or-later

package native

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func metricFrame(family string) string { return `{"version":"v1","metrics":[` + family + `]}` }
func checkFrame(family string) string  { return `{"version":"v1","checks":[` + family + `]}` }

// These documents exercise the authoring contract independently of Go DTOs.
var validSnapshots = map[string]string{
	"empty":        `{"version":"v1"}`,
	"empty arrays": `{"version":"v1","metrics":[],"checks":[]}`,
	"defaults":     metricFrame(`{"name":"depth","samples":[{"value":1.5}]}`),
	"empty family": metricFrame(`{"name":"depth","samples":[]}`),
	"counter":      metricFrame(`{"name":"processed","type":"counter","unit":"jobs","samples":[{"value":1e6}]}`),
	"metadata": metricFrame(
		`{"name":"depth","chart_meta":{"title":"Queue 😀","family":"Queues","priority":1000},"samples":[{"value":-1.5e-3,"labels":{"queue":"mail","Version":"λ"}}]}`,
	),
	"enum": metricFrame(
		`{"name":"status","type":"stateset","states":["in progress","unknown","done"],"samples":[{"active":["in progress"]}]}`,
	),
	"bitset": metricFrame(
		`{"name":"flags","type":"stateset","mode":"bitset","states":["read","write"],"samples":[{"active":[]},{"active":["read","write"],"labels":{"queue":"mail"}}]}`,
	),
	"check": checkFrame(`{"id":"probe","title":"Probe","samples":[{"state":"critical"}]}`),
	"identity check": checkFrame(
		`{"id":"probe","title":"Probe","by_labels":["queue"],"samples":[{"state":"warning","labels":{"queue":"mail","region":"east"}}]}`,
	),
	"empty check": checkFrame(`{"id":"probe","title":"Probe","samples":[]}`),
}

var invalidSnapshots = map[string]string{
	"unsupported version":      `{"version":"v2"}`,
	"unknown field":            `{"version":"v1","extra":1}`,
	"case folded field":        `{"version":"v1","Metrics":[]}`,
	"null metrics":             `{"version":"v1","metrics":null}`,
	"null checks":              `{"version":"v1","checks":null}`,
	"old flat sample":          metricFrame(`{"name":"depth","value":1}`),
	"missing name":             metricFrame(`{"samples":[{"value":1}]}`),
	"missing samples":          metricFrame(`{"name":"depth"}`),
	"null samples":             metricFrame(`{"name":"depth","samples":null}`),
	"missing value":            metricFrame(`{"name":"depth","samples":[{}]}`),
	"null value":               metricFrame(`{"name":"depth","samples":[{"value":null}]}`),
	"string value":             metricFrame(`{"name":"depth","samples":[{"value":"1"}]}`),
	"negative counter":         metricFrame(`{"name":"processed","type":"counter","samples":[{"value":-1}]}`),
	"bad name":                 metricFrame(`{"name":"bad-name","samples":[]}`),
	"reserved native prefix":   metricFrame(`{"name":"native.arbitrary","samples":[]}`),
	"reserved chart prefix":    metricFrame(`{"name":"native_check_arbitrary","samples":[]}`),
	"reserved health context":  metricFrame(`{"name":"check_state","samples":[]}`),
	"bad type":                 metricFrame(`{"name":"x","type":"summary","samples":[]}`),
	"blank unit":               metricFrame(`{"name":"x","unit":"   ","samples":[]}`),
	"null labels":              metricFrame(`{"name":"x","samples":[{"value":1,"labels":null}]}`),
	"null label value":         metricFrame(`{"name":"x","samples":[{"value":1,"labels":{"queue":null}}]}`),
	"bad label key":            metricFrame(`{"name":"x","samples":[{"value":1,"labels":{"bad-key":"x"}}]}`),
	"host-owned label":         metricFrame(`{"name":"x","samples":[{"value":1,"labels":{"_collect_job":"x"}}]}`),
	"metadata null":            metricFrame(`{"name":"x","chart_meta":null,"samples":[]}`),
	"metadata unknown":         metricFrame(`{"name":"x","chart_meta":{"units":"x"},"samples":[]}`),
	"zero priority":            metricFrame(`{"name":"x","chart_meta":{"priority":0},"samples":[]}`),
	"fractional priority":      metricFrame(`{"name":"x","chart_meta":{"priority":0.1},"samples":[]}`),
	"scalar active":            metricFrame(`{"name":"x","samples":[{"value":1,"active":[]}]}`),
	"scalar states":            metricFrame(`{"name":"x","states":[],"samples":[]}`),
	"scalar mode":              metricFrame(`{"name":"x","mode":"enum","samples":[]}`),
	"stateset no domain":       metricFrame(`{"name":"x","type":"stateset","samples":[]}`),
	"stateset empty domain":    metricFrame(`{"name":"x","type":"stateset","states":[],"samples":[]}`),
	"stateset blank state":     metricFrame(`{"name":"x","type":"stateset","states":[" "],"samples":[]}`),
	"stateset duplicate state": metricFrame(`{"name":"x","type":"stateset","states":["up","up"],"samples":[]}`),
	"stateset unit": metricFrame(
		`{"name":"x","type":"stateset","unit":"value","states":["up"],"samples":[]}`,
	),
	"stateset bad mode": metricFrame(
		`{"name":"x","type":"stateset","mode":"set","states":["up"],"samples":[]}`,
	),
	"stateset missing active": metricFrame(`{"name":"x","type":"stateset","states":["up"],"samples":[{}]}`),
	"stateset scalar value": metricFrame(
		`{"name":"x","type":"stateset","states":["up"],"samples":[{"value":1,"active":["up"]}]}`,
	),
	"enum empty active": metricFrame(
		`{"name":"x","type":"stateset","states":["up"],"samples":[{"active":[]}]}`,
	),
	"enum multiple active": metricFrame(
		`{"name":"x","type":"stateset","states":["up","down"],"samples":[{"active":["up","down"]}]}`,
	),
	"check missing title":   checkFrame(`{"id":"probe","samples":[]}`),
	"check bad id":          checkFrame(`{"id":"bad-id","title":"Probe","samples":[]}`),
	"check missing samples": checkFrame(`{"id":"probe","title":"Probe"}`),
	"check duplicate identity key": checkFrame(
		`{"id":"probe","title":"Probe","by_labels":["queue","queue"],"samples":[]}`,
	),
	"check reserved identity key": checkFrame(
		`{"id":"probe","title":"Probe","by_labels":["_collect_job"],"samples":[]}`,
	),
	"check bad state": checkFrame(`{"id":"probe","title":"Probe","samples":[{"state":"healthy"}]}`),
}

// These require cross-field/series validation or lexical JSON checks, beyond the schema.
var runtimeInvalidSnapshots = map[string]string{
	"truncated":     `{"version":"v1"`,
	"trailing":      `{"version":"v1"}{}`,
	"invalid utf8":  "{\"version\":\"v1\",\"x\":\"\xff\"}",
	"duplicate key": `{"version":"v1","version":"v1"}`,
	"overflow":      metricFrame(`{"name":"x","samples":[{"value":1e999}]}`),
	"duplicate label key": metricFrame(
		`{"name":"x","samples":[{"value":1,"labels":{"queue":"a","queue":"b"}}]}`,
	),
	"duplicate family": metricFrame(`{"name":"x","samples":[]},{"name":"x","samples":[]}`),
	"duplicate series": metricFrame(`{"name":"x","samples":[{"value":1},{"value":2,"labels":{}}]}`),
	"duplicate reordered labels": metricFrame(
		`{"name":"x","samples":[{"value":1,"labels":{"a":"1","b":"2"}},{"value":2,"labels":{"b":"2","a":"1"}}]}`,
	),
	"stateset label collision": metricFrame(
		`{"name":"x","type":"stateset","states":["up"],"samples":[{"active":["up"],"labels":{"x":"up"}}]}`,
	),
	"unknown active": metricFrame(
		`{"name":"x","type":"stateset","states":["up"],"samples":[{"active":["down"]}]}`,
	),
	"bitset duplicate active": metricFrame(
		`{"name":"x","type":"stateset","mode":"bitset","states":["up"],"samples":[{"active":["up","up"]}]}`,
	),
	"check missing identity value": checkFrame(
		`{"id":"probe","title":"Probe","by_labels":["queue"],"samples":[{"state":"ok"}]}`,
	),
	"check duplicate family": checkFrame(
		`{"id":"probe","title":"Probe","samples":[]},{"id":"probe","title":"Other","samples":[]}`,
	),
	"check duplicate projected identity": checkFrame(
		`{"id":"probe","title":"Probe","by_labels":["queue"],"samples":[{"state":"ok","labels":{"queue":"a","region":"east"}},{"state":"critical","labels":{"queue":"a","region":"west"}}]}`,
	),
}

func TestDecodeSnapshot(t *testing.T) {
	for name, data := range validSnapshots {
		t.Run(name, func(t *testing.T) { _, err := decodeSnapshot([]byte(data)); require.NoError(t, err) })
	}
	for _, cases := range []map[string]string{invalidSnapshots, runtimeInvalidSnapshots} {
		for name, data := range cases {
			t.Run(name, func(t *testing.T) {
				_, err := decodeSnapshot([]byte(data))
				require.Error(t, err)
				assert.NotContains(t, err.Error(), "1e999", "errors must not echo sample values")
			})
		}
	}
}

func TestDecodeSnapshotNormalization(t *testing.T) {
	got, err := decodeSnapshot([]byte(validSnapshots["defaults"]))
	require.NoError(t, err)
	assert.Equal(t, "gauge", got.Metrics[0].Type)
	assert.Equal(t, "value", got.Metrics[0].Unit)
	assert.Equal(t, 1.5, *got.Metrics[0].Samples[0].Value)
	got, err = decodeSnapshot([]byte(validSnapshots["enum"]))
	require.NoError(t, err)
	assert.Equal(t, []string{"done", "in progress", "unknown"}, got.Metrics[0].States)
	assert.Equal(t, "enum", got.Metrics[0].Mode)
	got, err = decodeSnapshot([]byte(validSnapshots["identity check"]))
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"queue": "mail"}, got.Checks[0].Samples[0].Labels)
	data := strings.Replace(validSnapshots["metadata"], "😀", jsonEscape("😀"), 1)
	got, err = decodeSnapshot([]byte(data))
	require.NoError(t, err)
	assert.Equal(t, "Queue 😀", got.Metrics[0].ChartMeta.Title)
}
