// SPDX-License-Identifier: GPL-3.0-or-later

package litespeed

import (
	"os"
	"testing"

	"github.com/netdata/netdata/go/go.d.plugin/agent/module"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON": dataConfigJSON,
		"dataConfigYAML": dataConfigYAML,
	} {
		require.NotNil(t, data, name)
	}
}

func TestLitespeed_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &Litespeed{}, dataConfigJSON, dataConfigYAML)
}

func TestLitespeed_Init(t *testing.T) {
	tests := map[string]struct {
		wantFail bool
		config   Config
	}{
		"success with default": {
			wantFail: false,
			config:   New().Config,
		},
		"fails if reports_dir not set": {
			wantFail: true,
			config: Config{
				ReportsDir: "",
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			lite := New()
			lite.Config = test.config

			if test.wantFail {
				assert.Error(t, lite.Init())
			} else {
				assert.NoError(t, lite.Init())
			}
		})
	}
}

func TestLitespeed_Charts(t *testing.T) {
	assert.NotNil(t, New().Charts())
}

func TestLitespeed_Check(t *testing.T) {
	tests := map[string]struct {
		prepareLitespeed func() *Litespeed
		wantFail         bool
	}{
		"success": {
			wantFail:         false,
			prepareLitespeed: prepareLitespeedOk,
		},
		"fails if reports dir not exist": {
			wantFail:         true,
			prepareLitespeed: prepareLitespeedDirNotExist,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			lite := test.prepareLitespeed()

			if test.wantFail {
				assert.Error(t, lite.Check())
			} else {
				assert.NoError(t, lite.Check())
			}
		})
	}
}

func TestLitespeed_Collect(t *testing.T) {
	tests := map[string]struct {
		prepareLitespeed func() *Litespeed
		wantMetrics      map[string]int64
	}{
		"success": {
			prepareLitespeed: prepareLitespeedOk,
			wantMetrics: map[string]int64{
				"availconn":                  3804,
				"availssl":                   3814,
				"bps_in":                     0,
				"bps_out":                    240,
				"plainconn":                  10,
				"private_cache_hits_per_sec": 0,
				"pub_cache_hits_per_sec":     0,
				"req_per_sec":                1560,
				"req_processing":             168,
				"ssl_bps_in":                 16,
				"ssl_bps_out":                3120,
				"sslconn":                    186,
				"static_hits_per_sec":        760,
			},
		},
		"fails if reports dir not exist": {
			prepareLitespeed: prepareLitespeedDirNotExist,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			lite := test.prepareLitespeed()

			mx := lite.Collect()

			assert.Equal(t, test.wantMetrics, mx)
			if len(test.wantMetrics) > 0 {
				testMetricsHasAllChartsDims(t, lite, mx)
			}
		})
	}
}

func testMetricsHasAllChartsDims(t *testing.T, lite *Litespeed, mx map[string]int64) {
	for _, chart := range *lite.Charts() {
		if chart.Obsolete {
			continue
		}
		for _, dim := range chart.Dims {
			_, ok := mx[dim.ID]
			assert.Truef(t, ok, "collected metrics has no data for dim '%s' chart '%s'", dim.ID, chart.ID)
		}
		for _, v := range chart.Vars {
			_, ok := mx[v.ID]
			assert.Truef(t, ok, "collected metrics has no data for var '%s' chart '%s'", v.ID, chart.ID)
		}
	}
}

func prepareLitespeedOk() *Litespeed {
	lite := New()
	lite.ReportsDir = "testdata"
	return lite
}

func prepareLitespeedDirNotExist() *Litespeed {
	lite := prepareLitespeedOk()
	lite.ReportsDir += "!"
	return lite
}
