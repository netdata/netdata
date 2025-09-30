// SPDX-License-Identifier: GPL-3.0-or-later

package varnish

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")

	dataVer71Varnishstat, _ = os.ReadFile("testdata/v7.1/varnishstat.txt")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON":       dataConfigJSON,
		"dataConfigYAML":       dataConfigYAML,
		"dataVer71Varnishstat": dataVer71Varnishstat,
	} {
		require.NotNil(t, data, name)
	}
}

func TestCollector_Configuration(t *testing.T) {
	module.TestConfigurationSerialize(t, &Collector{}, dataConfigJSON, dataConfigYAML)
}

func TestCollector_Init(t *testing.T) {
	tests := map[string]struct {
		config   Config
		wantFail bool
	}{
		"success with default config": {
			wantFail: false,
			config:   New().Config,
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

func TestCollector_Cleanup(t *testing.T) {
	tests := map[string]struct {
		prepare func() *Collector
	}{
		"not initialized exec": {
			prepare: func() *Collector {
				return New()
			},
		},
		"after check": {
			prepare: func() *Collector {
				collr := New()
				collr.exec = prepareMockOkVer71()
				_ = collr.Check(context.Background())
				return collr
			},
		},
		"after collect": {
			prepare: func() *Collector {
				collr := New()
				collr.exec = prepareMockOkVer71()
				_ = collr.Collect(context.Background())
				return collr
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := test.prepare()

			assert.NotPanics(t, func() { collr.Cleanup(context.Background()) })
		})
	}
}

func TestCollector_Charts(t *testing.T) {
	assert.NotNil(t, New().Charts())
}

func TestCollector_Check(t *testing.T) {
	tests := map[string]struct {
		prepareMock func() *mockVarnishstatExec
		wantFail    bool
	}{
		"success case": {
			wantFail:    false,
			prepareMock: prepareMockOkVer71,
		},
		"error on varnishstat call": {
			wantFail:    true,
			prepareMock: prepareMockErrOnVarnishstatCall,
		},
		"unexpected response": {
			wantFail:    true,
			prepareMock: prepareMockUnexpectedResponse,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := New()
			collr.exec = test.prepareMock()

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
		prepareMock func() *mockVarnishstatExec
		wantMetrics map[string]int64
		wantCharts  int
	}{
		"success case varnish v7.1": {
			prepareMock: prepareMockOkVer71,
			wantCharts:  len(varnishCharts) + len(backendChartsTmpl)*2 + len(storageChartsTmpl)*2,
			wantMetrics: map[string]int64{
				"MAIN.backend_busy":                 0,
				"MAIN.backend_conn":                 2,
				"MAIN.backend_fail":                 0,
				"MAIN.backend_recycle":              4,
				"MAIN.backend_req":                  4,
				"MAIN.backend_retry":                0,
				"MAIN.backend_reuse":                2,
				"MAIN.backend_unhealthy":            0,
				"MAIN.cache_hit":                    0,
				"MAIN.cache_hitmiss":                0,
				"MAIN.cache_hitpass":                0,
				"MAIN.cache_miss":                   0,
				"MAIN.client_req":                   4,
				"MAIN.esi_errors":                   0,
				"MAIN.esi_warnings":                 0,
				"MAIN.n_expired":                    0,
				"MAIN.n_lru_limited":                0,
				"MAIN.n_lru_moved":                  0,
				"MAIN.n_lru_nuked":                  0,
				"MAIN.sess_conn":                    4,
				"MAIN.sess_dropped":                 0,
				"MAIN.thread_queue_len":             0,
				"MAIN.threads":                      200,
				"MAIN.threads_created":              200,
				"MAIN.threads_destroyed":            0,
				"MAIN.threads_failed":               0,
				"MAIN.threads_limited":              0,
				"MAIN.uptime":                       33834,
				"MGT.child_died":                    0,
				"MGT.child_dump":                    0,
				"MGT.child_exit":                    0,
				"MGT.child_panic":                   0,
				"MGT.child_start":                   1,
				"MGT.child_stop":                    0,
				"MGT.uptime":                        33833,
				"SMA.Transient.g_alloc":             0,
				"SMA.Transient.g_bytes":             0,
				"SMA.Transient.g_space":             0,
				"SMA.s0.g_alloc":                    0,
				"SMA.s0.g_bytes":                    0,
				"SMA.s0.g_space":                    268435456,
				"VBE.boot.default.bereq_bodybytes":  0,
				"VBE.boot.default.bereq_hdrbytes":   5214,
				"VBE.boot.default.beresp_bodybytes": 1170,
				"VBE.boot.default.beresp_hdrbytes":  753,
				"VBE.boot.nginx2.bereq_bodybytes":   0,
				"VBE.boot.nginx2.bereq_hdrbytes":    0,
				"VBE.boot.nginx2.beresp_bodybytes":  0,
				"VBE.boot.nginx2.beresp_hdrbytes":   0,
			},
		},
		"error on varnishstat call": {
			prepareMock: prepareMockErrOnVarnishstatCall,
		},
		"unexpected response": {
			prepareMock: prepareMockUnexpectedResponse,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := New()
			collr.exec = test.prepareMock()

			mx := collr.Collect(context.Background())

			assert.Equal(t, test.wantMetrics, mx)

			if len(test.wantMetrics) > 0 {
				assert.Equal(t, test.wantCharts, len(*collr.Charts()))
				module.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)
			}
		})
	}
}

func prepareMockOkVer71() *mockVarnishstatExec {
	return &mockVarnishstatExec{
		dataVarnishstat: dataVer71Varnishstat,
	}
}

func prepareMockErrOnVarnishstatCall() *mockVarnishstatExec {
	return &mockVarnishstatExec{
		dataVarnishstat:      nil,
		errOnVarnishstatCall: true,
	}
}

func prepareMockUnexpectedResponse() *mockVarnishstatExec {
	return &mockVarnishstatExec{
		dataVarnishstat: []byte(`
Lorem ipsum dolor sit amet, consectetur adipiscing elit.
Nulla malesuada erat id magna mattis, eu viverra tellus rhoncus.
Fusce et felis pulvinar, posuere sem non, porttitor eros.
`),
	}
}

type mockVarnishstatExec struct {
	errOnVarnishstatCall bool
	dataVarnishstat      []byte
}

func (m *mockVarnishstatExec) statistics() ([]byte, error) {
	if m.errOnVarnishstatCall {
		return nil, errors.New("mock statistics() error")
	}

	return m.dataVarnishstat, nil
}
