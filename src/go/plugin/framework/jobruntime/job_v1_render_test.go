// SPDX-License-Identifier: GPL-3.0-or-later

package jobruntime

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The plugins.d splitter treats a backslash as an escape of the next byte, so a
// label key or value ending in one would consume its closing quote.
func TestV1ChartLabelsAreWireSafe(t *testing.T) {
	charts := collectorapi.Charts{
		{
			ID:    "work",
			Title: "title",
			Units: "units",
			Dims:  collectorapi.Dims{{ID: "value"}},
			Labels: []collectorapi.Label{
				{Key: "if_alias", Value: `C:\share\`},
				{Key: `slot\`, Value: "1"},
			},
		},
	}
	mod := &collectorapi.MockCollectorV1{
		ChartsFunc:  func() *collectorapi.Charts { return &charts },
		CollectFunc: func(context.Context) map[string]int64 { return map[string]int64{"value": 1} },
	}
	var out bytes.Buffer
	job := NewJob(
		JobConfig{
			PluginName: pluginName,
			Name:       `job\`,
			ModuleName: modName,
			FullName:   modName + "_" + jobName,
			Module:     mod,
			Labels:     map[string]string{`zone\`: `rack\`},
			Out:        &out,
		},
	)
	require.NoError(t, job.AutoDetectionManaged(context.Background()))
	job.runOnce()

	var clabels []string
	for line := range strings.Lines(out.String()) {
		if strings.HasPrefix(line, "CLABEL ") {
			clabels = append(clabels, strings.TrimSuffix(line, "\n"))
		}
	}
	// The data chart, then the status and duration self-metrics charts.
	selfLabels := []string{
		"CLABEL 'zone/' 'rack/' '2'",
		"CLABEL '_collect_job' 'job/' '1'",
	}
	want := []string{
		"CLABEL 'if_alias' 'C:/share/' '1'",
		"CLABEL 'slot/' '1' '1'",
		"CLABEL 'zone/' 'rack/' '2'",
		"CLABEL '_collect_job' 'job/' '1'",
	}
	want = append(want, selfLabels...)
	want = append(want, selfLabels...)
	assert.Equal(t, want, clabels)
}
