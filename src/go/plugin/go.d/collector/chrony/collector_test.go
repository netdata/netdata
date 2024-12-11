// SPDX-License-Identifier: GPL-3.0-or-later

package chrony

import (
	"context"
	"errors"
	"net"
	"os"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

	"github.com/facebook/time/ntp/chrony"
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
		assert.NotNil(t, data, name)
	}
}

func TestCollector_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &Collector{}, dataConfigJSON, dataConfigYAML)
}

func TestCollector_Init(t *testing.T) {
	tests := map[string]struct {
		config   Config
		wantFail bool
	}{
		"default config": {
			config: New().Config,
		},
		"unset 'address'": {
			wantFail: true,
			config: Config{
				Address: "",
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

func TestCollector_Check(t *testing.T) {
	tests := map[string]struct {
		prepare  func() *Collector
		wantFail bool
	}{
		"tracking: success, activity: success": {
			wantFail: false,
			prepare:  func() *Collector { return prepareChronyWithMock(&mockClient{}) },
		},
		"tracking: success, activity: fail": {
			wantFail: true,
			prepare:  func() *Collector { return prepareChronyWithMock(&mockClient{errOnActivity: true}) },
		},
		"tracking: fail, activity: success": {
			wantFail: true,
			prepare:  func() *Collector { return prepareChronyWithMock(&mockClient{errOnTracking: true}) },
		},
		"tracking: fail, activity: fail": {
			wantFail: true,
			prepare:  func() *Collector { return prepareChronyWithMock(&mockClient{errOnTracking: true}) },
		},
		"fail on creating client": {
			wantFail: true,
			prepare:  func() *Collector { return prepareChronyWithMock(nil) },
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := test.prepare()

			require.NoError(t, collr.Init(context.Background()))

			if test.wantFail {
				assert.Error(t, collr.Check(context.Background()))
			} else {
				assert.NoError(t, collr.Check(context.Background()))
			}
		})
	}
}

func TestCollector_Charts(t *testing.T) {
	assert.Equal(t, len(charts), len(*New().Charts()))
}

func TestCollector_Cleanup(t *testing.T) {
	tests := map[string]struct {
		prepare   func(c *Collector)
		wantClose bool
	}{
		"after New": {
			wantClose: false,
			prepare:   func(c *Collector) {},
		},
		"after Init": {
			wantClose: false,
			prepare:   func(c *Collector) { _ = c.Init(context.Background()) },
		},
		"after Check": {
			wantClose: true,
			prepare:   func(c *Collector) { _ = c.Init(context.Background()); _ = c.Check(context.Background()) },
		},
		"after Collect": {
			wantClose: true,
			prepare:   func(c *Collector) { _ = c.Init(context.Background()); _ = c.Collect(context.Background()) },
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			m := &mockClient{}
			collr := prepareChronyWithMock(m)
			test.prepare(collr)

			require.NotPanics(t, func() { collr.Cleanup(context.Background()) })

			if test.wantClose {
				assert.True(t, m.closeCalled)
			} else {
				assert.False(t, m.closeCalled)
			}
		})
	}
}

func TestCollector_Collect(t *testing.T) {
	tests := map[string]struct {
		prepare  func() *Collector
		expected map[string]int64
	}{
		"tracking: success, activity: success, serverstats: success": {
			prepare: func() *Collector { return prepareChronyWithMock(&mockClient{}) },
			expected: map[string]int64{
				"burst_offline_sources":      3,
				"burst_online_sources":       4,
				"command_packets_dropped":    1,
				"command_packets_received":   652,
				"current_correction":         154872,
				"frequency":                  51051185607,
				"last_offset":                3095,
				"leap_status_delete_second":  0,
				"leap_status_insert_second":  1,
				"leap_status_normal":         0,
				"leap_status_unsynchronised": 0,
				"ntp_packets_dropped":        1,
				"ntp_packets_received":       1,
				"offline_sources":            2,
				"online_sources":             8,
				"ref_measurement_time":       63793323616,
				"residual_frequency":         -571789,
				"rms_offset":                 130089,
				"root_delay":                 59576179,
				"root_dispersion":            1089275,
				"skew":                       41821926,
				"stratum":                    4,
				"unresolved_sources":         1,
				"update_interval":            1044219238281,
			},
		},
		"tracking: success, activity: fail": {
			prepare: func() *Collector { return prepareChronyWithMock(&mockClient{errOnActivity: true}) },
			expected: map[string]int64{
				"current_correction":         154872,
				"frequency":                  51051185607,
				"last_offset":                3095,
				"leap_status_delete_second":  0,
				"leap_status_insert_second":  1,
				"leap_status_normal":         0,
				"leap_status_unsynchronised": 0,
				"ref_measurement_time":       63793323586,
				"residual_frequency":         -571789,
				"rms_offset":                 130089,
				"root_delay":                 59576179,
				"root_dispersion":            1089275,
				"skew":                       41821926,
				"stratum":                    4,
				"update_interval":            1044219238281,
			},
		},
		"tracking: fail, activity: success": {
			prepare:  func() *Collector { return prepareChronyWithMock(&mockClient{errOnTracking: true}) },
			expected: nil,
		},
		"tracking: fail, activity: fail": {
			prepare:  func() *Collector { return prepareChronyWithMock(&mockClient{errOnTracking: true}) },
			expected: nil,
		},
		"fail on creating client": {
			prepare:  func() *Collector { return prepareChronyWithMock(nil) },
			expected: nil,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := test.prepare()

			require.NoError(t, collr.Init(context.Background()))
			collr.exec = &mockChronyc{}
			_ = collr.Check(context.Background())

			mx := collr.Collect(context.Background())
			copyRefMeasurementTime(mx, test.expected)

			assert.Equal(t, test.expected, mx)
		})
	}
}

func prepareChronyWithMock(m *mockClient) *Collector {
	c := New()
	if m == nil {
		c.newConn = func(_ Config) (chronyConn, error) { return nil, errors.New("mock.newClient error") }
	} else {
		c.newConn = func(_ Config) (chronyConn, error) { return m, nil }
	}
	return c
}

type mockChronyc struct{}

func (m *mockChronyc) serverStats() ([]byte, error) {
	data := `
NTP packets received       : 1
NTP packets dropped        : 1
Command packets received   : 652
Command packets dropped    : 1
Client log records dropped : 1
NTS-KE connections accepted: 1
NTS-KE connections dropped : 1
Authenticated NTP packets  : 1
Interleaved NTP packets    : 1
NTP timestamps held        : 1
NTP timestamp span         : 0
`
	return []byte(data), nil
}

type mockClient struct {
	errOnTracking    bool
	errOnActivity    bool
	errOnServerStats bool
	closeCalled      bool
}

func (m *mockClient) tracking() (*chrony.ReplyTracking, error) {
	if m.errOnTracking {
		return nil, errors.New("mockClient.Tracking call error")
	}
	reply := chrony.ReplyTracking{
		Tracking: chrony.Tracking{
			RefID:              2728380539,
			IPAddr:             net.IP("192.0.2.0"),
			Stratum:            4,
			LeapStatus:         1,
			RefTime:            time.Time{},
			CurrentCorrection:  0.00015487267228309065,
			LastOffset:         3.0953951863921247e-06,
			RMSOffset:          0.00013008920359425247,
			FreqPPM:            -51.051185607910156,
			ResidFreqPPM:       -0.0005717896274290979,
			SkewPPM:            0.0418219268321991,
			RootDelay:          0.05957617983222008,
			RootDispersion:     0.0010892755817621946,
			LastUpdateInterval: 1044.21923828125,
		},
	}
	return &reply, nil
}

func (m *mockClient) activity() (*chrony.ReplyActivity, error) {
	if m.errOnActivity {
		return nil, errors.New("mockClient.Activity call error")
	}
	reply := chrony.ReplyActivity{
		Activity: chrony.Activity{
			Online:       8,
			Offline:      2,
			BurstOnline:  4,
			BurstOffline: 3,
			Unresolved:   1,
		},
	}
	return &reply, nil
}

func (m *mockClient) close() {
	m.closeCalled = true
}

func copyRefMeasurementTime(dst, src map[string]int64) {
	if _, ok := dst["ref_measurement_time"]; !ok {
		return
	}
	if _, ok := src["ref_measurement_time"]; !ok {
		return
	}
	dst["ref_measurement_time"] = src["ref_measurement_time"]
}
