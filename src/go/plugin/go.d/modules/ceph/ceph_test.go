// SPDX-License-Identifier: GPL-3.0-or-later

package ceph

import (
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

	dataDf, _           = os.ReadFile("testdata/df.json")
	dataOsdDf, _        = os.ReadFile("testdata/osd_df.json")
	dataOsdPerf, _      = os.ReadFile("testdata/osd_perf.json")
	dataOsdPoolStats, _ = os.ReadFile("testdata/osd_pool_stats.json")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON":   dataConfigJSON,
		"dataConfigYAML":   dataConfigYAML,
		"dataDfStats":      dataDf,
		"dataOsdDf":        dataOsdDf,
		"dataOsdPerf":      dataOsdPerf,
		"dataOsdPoolStats": dataOsdPoolStats,
	} {
		require.NotNil(t, data, name)
	}
}

func TestCeph_Configuration(t *testing.T) {
	module.TestConfigurationSerialize(t, &Ceph{}, dataConfigJSON, dataConfigYAML)
}

func TestCeph_Init(t *testing.T) {
	tests := map[string]struct {
		config   Config
		wantFail bool
	}{
		"fails if 'binary_path' is not set": {
			wantFail: true,
			config: Config{
				BinaryPath: "",
			},
		},
		"fails if failed to find binary": {
			wantFail: true,
			config: Config{
				BinaryPath: "ceph!!!",
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			pf := New()
			pf.Config = test.config

			if test.wantFail {
				assert.Error(t, pf.Init())
			} else {
				assert.NoError(t, pf.Init())
			}
		})
	}
}

func TestCeph_Cleanup(t *testing.T) {
	tests := map[string]struct {
		prepare func() *Ceph
	}{
		"not initialized exec": {
			prepare: func() *Ceph {
				return New()
			},
		},
		"after check": {
			prepare: func() *Ceph {
				ap := New()
				ap.exec = prepareMockOk()
				_ = ap.Check()
				return ap
			},
		},
		"after collect": {
			prepare: func() *Ceph {
				ap := New()
				ap.exec = prepareMockOk()
				_ = ap.Collect()
				return ap
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			pf := test.prepare()

			assert.NotPanics(t, pf.Cleanup)
		})
	}
}

func TestCeph_Charts(t *testing.T) {
	assert.NotNil(t, New().Charts())
}

func TestCeph_Check(t *testing.T) {
	tests := map[string]struct {
		prepareMock func() *mockCephExec
		wantFail    bool
	}{
		"success case": {
			wantFail:    false,
			prepareMock: prepareMockOk,
		},
		"error on df call": {
			wantFail:    true,
			prepareMock: prepareMockErrOnDf,
		},
		"error on osd df call": {
			wantFail:    true,
			prepareMock: prepareMockErrOnOsdDf,
		},
		"error on osd perf call": {
			wantFail:    true,
			prepareMock: prepareMockErrOnOsdPerf,
		},
		"error on osd pool stats call": {
			wantFail:    true,
			prepareMock: prepareMockErrOnOsdPoolStats,
		},
		"unexpected response": {
			wantFail:    true,
			prepareMock: prepareMockUnexpectedResponse,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ap := New()
			ap.exec = test.prepareMock()

			if test.wantFail {
				assert.Error(t, ap.Check())
			} else {
				assert.NoError(t, ap.Check())
			}
		})
	}
}

func TestCeph_Collect(t *testing.T) {
	tests := map[string]struct {
		prepareMock func() *mockCephExec
		wantMetrics map[string]int64
		wantCharts  int
	}{
		"success case": {
			prepareMock: prepareMockOk,
			wantCharts:  len(generalCharts) + len(poolChartsTmpl) + len(osdChartsTmpl),
			wantMetrics: map[string]int64{
				"general_available":         961347,
				"general_usage":             8,
				"general_objects":           2,
				"general_read_bytes":        0,
				"general_write_bytes":       112,
				"general_read_operations":   0,
				"general_write_operations":  0,
				"general_apply_latency":     0,
				"general_commit_latency":    0,
				"testpool_kb_used":          8,
				"testpool_objects":          2,
				"testpool_read_bytes":       0,
				"testpool_write_bytes":      112,
				"testpool_read_operations":  0,
				"testpool_write_operations": 0,
				"osd.0_usage":               34800,
				"osd.0_size":                1048576,
				"osd.0_apply_latency":       0,
				"osd.0_commit_latency":      0,
			},
		},
		"error on df call": {
			prepareMock: prepareMockErrOnDf,
		},
		"error on osd df call": {
			prepareMock: prepareMockErrOnOsdDf,
		},
		"error on osd perf call": {
			prepareMock: prepareMockErrOnOsdPerf,
		},
		"error on osd pool stats call": {
			prepareMock: prepareMockErrOnOsdPoolStats,
		},
		"unexpected response": {
			prepareMock: prepareMockUnexpectedResponse,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ceph := New()
			ceph.exec = test.prepareMock()

			mx := ceph.Collect()

			assert.Equal(t, test.wantMetrics, mx)

			if len(test.wantMetrics) > 0 {
				assert.Equal(t, test.wantCharts, len(*ceph.Charts()))
				module.TestMetricsHasAllChartsDims(t, ceph.Charts(), mx)
			}
		})
	}
}

func prepareMockOk() *mockCephExec {
	return &mockCephExec{
		dfData:           dataDf,
		osdDfData:        dataOsdDf,
		osdPerfData:      dataOsdPerf,
		osdPoolStatsData: dataOsdPoolStats,
	}
}

func prepareMockErrOnDf() *mockCephExec {
	return &mockCephExec{
		errDf: true,
	}
}

func prepareMockErrOnOsdDf() *mockCephExec {
	return &mockCephExec{
		errOsdDf: true,
	}
}

func prepareMockErrOnOsdPerf() *mockCephExec {
	return &mockCephExec{
		errOsdPerf: true,
	}
}

func prepareMockErrOnOsdPoolStats() *mockCephExec {
	return &mockCephExec{
		errOsdPoolStats: true,
	}
}

func prepareMockUnexpectedResponse() *mockCephExec {
	return &mockCephExec{
		dfData: []byte(`
Lorem ipsum dolor sit amet, consectetur adipiscing elit.
Nulla malesuada erat id magna mattis, eu viverra tellus rhoncus.
Fusce et felis pulvinar, posuere sem non, porttitor eros.
`),
	}
}

type mockCephExec struct {
	errDf           bool
	errOsdDf        bool
	errOsdPerf      bool
	errOsdPoolStats bool

	dfData           []byte
	osdDfData        []byte
	osdPerfData      []byte
	osdPoolStatsData []byte
}

func (m *mockCephExec) df() ([]byte, error) {
	if m.errDf {
		return nil, errors.New("mock.df() error")
	}

	return m.dfData, nil
}

func (m *mockCephExec) osdDf() ([]byte, error) {
	if m.errDf {
		return nil, errors.New("mock.osdDf() error")
	}

	return m.osdDfData, nil
}

func (m *mockCephExec) osdPerf() ([]byte, error) {
	if m.errDf {
		return nil, errors.New("mock.osdPerf() error")
	}

	return m.osdPerfData, nil
}

func (m *mockCephExec) osdPoolStats() ([]byte, error) {
	if m.errDf {
		return nil, errors.New("mock.osdPoolStats() error")
	}

	return m.osdPoolStatsData, nil
}
