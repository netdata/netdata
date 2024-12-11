// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package ap

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

	dataIwDevManaged, _ = os.ReadFile("testdata/iw_dev_managed.txt")

	dataIwDevAP, _       = os.ReadFile("testdata/iw_dev_ap.txt")
	dataIwStationDump, _ = os.ReadFile("testdata/station_dump.txt")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON":    dataConfigJSON,
		"dataConfigYAML":    dataConfigYAML,
		"dataIwDevManaged":  dataIwDevManaged,
		"dataIwDevAP":       dataIwDevAP,
		"dataIwStationDump": dataIwStationDump,
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
		"fails if 'binary_path' is not set": {
			wantFail: true,
			config: Config{
				BinaryPath: "",
			},
		},
		"fails if failed to find binary": {
			wantFail: true,
			config: Config{
				BinaryPath: "iw!!!",
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
				collr.exec = prepareMockOk()
				_ = collr.Check(context.Background())
				return collr
			},
		},
		"after collect": {
			prepare: func() *Collector {
				collr := New()
				collr.exec = prepareMockOk()
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
		prepareMock func() *mockIwExec
		wantFail    bool
	}{
		"success case": {
			wantFail:    false,
			prepareMock: prepareMockOk,
		},
		"no ap devices": {
			wantFail:    true,
			prepareMock: prepareMockNoAPDevices,
		},
		"error on devices call": {
			wantFail:    true,
			prepareMock: prepareMockErrOnDevices,
		},
		"error on station stats call": {
			wantFail:    true,
			prepareMock: prepareMockErrOnStationStats,
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
		prepareMock func() *mockIwExec
		wantMetrics map[string]int64
		wantCharts  int
	}{
		"success case": {
			prepareMock: prepareMockOk,
			wantCharts:  len(apChartsTmpl) * 2,
			wantMetrics: map[string]int64{
				"ap_wlp1s0_testing_average_signal":   -34000,
				"ap_wlp1s0_testing_bitrate_receive":  65500,
				"ap_wlp1s0_testing_bitrate_transmit": 65000,
				"ap_wlp1s0_testing_bw_received":      95117,
				"ap_wlp1s0_testing_bw_sent":          8270,
				"ap_wlp1s0_testing_clients":          2,
				"ap_wlp1s0_testing_issues_failures":  1,
				"ap_wlp1s0_testing_issues_retries":   1,
				"ap_wlp1s0_testing_packets_received": 2531,
				"ap_wlp1s0_testing_packets_sent":     38,
				"ap_wlp1s1_testing_average_signal":   -34000,
				"ap_wlp1s1_testing_bitrate_receive":  65500,
				"ap_wlp1s1_testing_bitrate_transmit": 65000,
				"ap_wlp1s1_testing_bw_received":      95117,
				"ap_wlp1s1_testing_bw_sent":          8270,
				"ap_wlp1s1_testing_clients":          2,
				"ap_wlp1s1_testing_issues_failures":  1,
				"ap_wlp1s1_testing_issues_retries":   1,
				"ap_wlp1s1_testing_packets_received": 2531,
				"ap_wlp1s1_testing_packets_sent":     38,
			},
		},
		"no ap devices": {
			prepareMock: prepareMockNoAPDevices,
			wantMetrics: nil,
		},
		"error on devices call": {
			prepareMock: prepareMockErrOnDevices,
			wantMetrics: nil,
		},
		"error on station stats call": {
			prepareMock: prepareMockErrOnStationStats,
			wantMetrics: nil,
		},
		"unexpected response": {
			prepareMock: prepareMockUnexpectedResponse,
			wantMetrics: nil,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := New()
			collr.exec = test.prepareMock()

			mx := collr.Collect(context.Background())

			assert.Equal(t, test.wantMetrics, mx)

			assert.Equal(t, test.wantCharts, len(*collr.Charts()), "wantCharts")

			module.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)
		})
	}
}

func prepareMockOk() *mockIwExec {
	return &mockIwExec{
		devicesData:      dataIwDevAP,
		stationStatsData: dataIwStationDump,
	}
}

func prepareMockNoAPDevices() *mockIwExec {
	return &mockIwExec{
		devicesData: dataIwDevManaged,
	}
}

func prepareMockErrOnDevices() *mockIwExec {
	return &mockIwExec{
		errOnDevices: true,
	}
}

func prepareMockErrOnStationStats() *mockIwExec {
	return &mockIwExec{
		devicesData:       dataIwDevAP,
		errOnStationStats: true,
	}
}

func prepareMockUnexpectedResponse() *mockIwExec {
	return &mockIwExec{
		devicesData: []byte(`
Lorem ipsum dolor sit amet, consectetur adipiscing elit.
Nulla malesuada erat id magna mattis, eu viverra tellus rhoncus.
Fusce et felis pulvinar, posuere sem non, porttitor eros.
`),
	}
}

type mockIwExec struct {
	errOnDevices      bool
	errOnStationStats bool
	devicesData       []byte
	stationStatsData  []byte
}

func (m *mockIwExec) devices() ([]byte, error) {
	if m.errOnDevices {
		return nil, errors.New("mock.devices() error")
	}

	return m.devicesData, nil
}

func (m *mockIwExec) stationStatistics(_ string) ([]byte, error) {
	if m.errOnStationStats {
		return nil, errors.New("mock.stationStatistics() error")
	}
	return m.stationStatsData, nil
}
