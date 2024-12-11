// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || freebsd || openbsd || netbsd || dragonfly

package litespeed

import (
	"context"
	"os"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

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

func TestCollector_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &Collector{}, dataConfigJSON, dataConfigYAML)
}

func TestCollector_Init(t *testing.T) {
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
			collr := New()
			collr.Config = test.config

			if test.wantFail {
				assert.Error(t, collr.Init(context.Background()))
			} else {
				assert.NoError(t, collr.Init(context.Background()))
			}
		})
	}
}

func TestCollector_Charts(t *testing.T) {
	assert.NotNil(t, New().Charts())
}

func TestCollector_Check(t *testing.T) {
	tests := map[string]struct {
		prepareLitespeed func() *Collector
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
			collr := test.prepareLitespeed()

			if test.wantFail {
				assert.Error(t, collr.Check(context.Background()))
			} else {
				assert.NoError(t, collr.Check(context.Background()))
			}
		})
	}
}

func TestCollector_Collect(t *testing.T) {
	tests := map[string]struct {
		prepareLitespeed func() *Collector
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
			collr := test.prepareLitespeed()

			mx := collr.Collect(context.Background())

			assert.Equal(t, test.wantMetrics, mx)

			if len(test.wantMetrics) > 0 {
				module.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)
			}
		})
	}
}

func prepareLitespeedOk() *Collector {
	collr := New()
	collr.ReportsDir = "testdata"
	return collr
}

func prepareLitespeedDirNotExist() *Collector {
	collr := prepareLitespeedOk()
	collr.ReportsDir += "!"
	return collr
}
