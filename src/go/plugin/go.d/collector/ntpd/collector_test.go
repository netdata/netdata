// SPDX-License-Identifier: GPL-3.0-or-later

package ntpd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"

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

func TestCollector_Charts(t *testing.T) {
	assert.Equal(t, len(systemCharts), len(*New().Charts()))
}

func TestCollector_Cleanup(t *testing.T) {
	tests := map[string]struct {
		prepare   func(*Collector)
		wantClose bool
	}{
		"after New": {
			wantClose: false,
			prepare:   func(*Collector) {},
		},
		"after Init": {
			wantClose: false,
			prepare:   func(n *Collector) { _ = n.Init(context.Background()) },
		},
		"after Check": {
			wantClose: true,
			prepare:   func(n *Collector) { _ = n.Init(context.Background()); _ = n.Check(context.Background()) },
		},
		"after Collect": {
			wantClose: true,
			prepare:   func(n *Collector) { _ = n.Init(context.Background()); n.Collect(context.Background()) },
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			m := &mockClient{}
			collr := prepareNTPdWithMock(m, true)
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

func TestCollector_Check(t *testing.T) {
	tests := map[string]struct {
		prepare  func() *Collector
		wantFail bool
	}{
		"system: success, peers: success": {
			wantFail: false,
			prepare:  func() *Collector { return prepareNTPdWithMock(&mockClient{}, true) },
		},
		"system: success, list peers: fails": {
			wantFail: false,
			prepare:  func() *Collector { return prepareNTPdWithMock(&mockClient{errOnPeerIDs: true}, true) },
		},
		"system: success, peers info: fails": {
			wantFail: false,
			prepare:  func() *Collector { return prepareNTPdWithMock(&mockClient{errOnPeerInfo: true}, true) },
		},
		"system: fails": {
			wantFail: true,
			prepare:  func() *Collector { return prepareNTPdWithMock(&mockClient{errOnSystemInfo: true}, true) },
		},
		"fail on creating client": {
			wantFail: true,
			prepare:  func() *Collector { return prepareNTPdWithMock(nil, true) },
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

func TestCollector_Collect(t *testing.T) {
	tests := map[string]struct {
		prepare        func() *Collector
		expected       map[string]int64
		expectedCharts int
	}{
		"system: success, peers: success": {
			prepare: func() *Collector { return prepareNTPdWithMock(&mockClient{}, true) },
			expected: map[string]int64{
				"clk_jitter":                  626000,
				"clk_wander":                  81000,
				"mintc":                       3000000,
				"offset":                      -149638,
				"peer_203.0.113.1_delay":      10464000,
				"peer_203.0.113.1_dispersion": 5376000,
				"peer_203.0.113.1_hmode":      3000000,
				"peer_203.0.113.1_hpoll":      7000000,
				"peer_203.0.113.1_jitter":     5204000,
				"peer_203.0.113.1_offset":     312000,
				"peer_203.0.113.1_pmode":      4000000,
				"peer_203.0.113.1_ppoll":      7000000,
				"peer_203.0.113.1_precision":  -21000000,
				"peer_203.0.113.1_rootdelay":  198000,
				"peer_203.0.113.1_rootdisp":   14465000,
				"peer_203.0.113.1_stratum":    2000000,
				"peer_203.0.113.1_xleave":     95000,
				"peer_203.0.113.2_delay":      10464000,
				"peer_203.0.113.2_dispersion": 5376000,
				"peer_203.0.113.2_hmode":      3000000,
				"peer_203.0.113.2_hpoll":      7000000,
				"peer_203.0.113.2_jitter":     5204000,
				"peer_203.0.113.2_offset":     312000,
				"peer_203.0.113.2_pmode":      4000000,
				"peer_203.0.113.2_ppoll":      7000000,
				"peer_203.0.113.2_precision":  -21000000,
				"peer_203.0.113.2_rootdelay":  198000,
				"peer_203.0.113.2_rootdisp":   14465000,
				"peer_203.0.113.2_stratum":    2000000,
				"peer_203.0.113.2_xleave":     95000,
				"peer_203.0.113.3_delay":      10464000,
				"peer_203.0.113.3_dispersion": 5376000,
				"peer_203.0.113.3_hmode":      3000000,
				"peer_203.0.113.3_hpoll":      7000000,
				"peer_203.0.113.3_jitter":     5204000,
				"peer_203.0.113.3_offset":     312000,
				"peer_203.0.113.3_pmode":      4000000,
				"peer_203.0.113.3_ppoll":      7000000,
				"peer_203.0.113.3_precision":  -21000000,
				"peer_203.0.113.3_rootdelay":  198000,
				"peer_203.0.113.3_rootdisp":   14465000,
				"peer_203.0.113.3_stratum":    2000000,
				"peer_203.0.113.3_xleave":     95000,
				"precision":                   -24000000,
				"rootdelay":                   10385000,
				"rootdisp":                    23404000,
				"stratum":                     2000000,
				"sys_jitter":                  1648010,
				"tc":                          7000000,
			},
			expectedCharts: len(systemCharts) + len(peerChartsTmpl)*3,
		},
		"system: success, list peers: fails": {
			prepare: func() *Collector { return prepareNTPdWithMock(&mockClient{errOnPeerIDs: true}, true) },
			expected: map[string]int64{
				"clk_jitter": 626000,
				"clk_wander": 81000,
				"mintc":      3000000,
				"offset":     -149638,
				"precision":  -24000000,
				"rootdelay":  10385000,
				"rootdisp":   23404000,
				"stratum":    2000000,
				"sys_jitter": 1648010,
				"tc":         7000000,
			},
			expectedCharts: len(systemCharts),
		},
		"system: success, peers info: fails": {
			prepare: func() *Collector { return prepareNTPdWithMock(&mockClient{errOnPeerInfo: true}, true) },
			expected: map[string]int64{
				"clk_jitter": 626000,
				"clk_wander": 81000,
				"mintc":      3000000,
				"offset":     -149638,
				"precision":  -24000000,
				"rootdelay":  10385000,
				"rootdisp":   23404000,
				"stratum":    2000000,
				"sys_jitter": 1648010,
				"tc":         7000000,
			},
			expectedCharts: len(systemCharts),
		},
		"system: fails": {
			prepare:        func() *Collector { return prepareNTPdWithMock(&mockClient{errOnSystemInfo: true}, true) },
			expected:       nil,
			expectedCharts: len(systemCharts),
		},
		"fail on creating client": {
			prepare:        func() *Collector { return prepareNTPdWithMock(nil, true) },
			expected:       nil,
			expectedCharts: len(systemCharts),
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := test.prepare()

			require.NoError(t, collr.Init(context.Background()))

			_ = collr.Check(context.Background())

			mx := collr.Collect(context.Background())

			assert.Equal(t, test.expected, mx)
			assert.Equal(t, test.expectedCharts, len(*collr.Charts()))
		})
	}
}

func prepareNTPdWithMock(m *mockClient, collectPeers confopt.FlexBool) *Collector {
	collr := New()
	collr.CollectPeers = collectPeers
	if m == nil {
		collr.newClient = func(_ Config) (ntpConn, error) { return nil, errors.New("mock.newClient error") }
	} else {
		collr.newClient = func(_ Config) (ntpConn, error) { return m, nil }
	}
	return collr
}

type mockClient struct {
	errOnSystemInfo bool
	errOnPeerInfo   bool
	errOnPeerIDs    bool
	closeCalled     bool
}

func (m *mockClient) systemInfo() (map[string]string, error) {
	if m.errOnSystemInfo {
		return nil, errors.New("mockClient.info() error")
	}

	info := map[string]string{
		"rootdelay":  "10.385",
		"tc":         "7",
		"mintc":      "3",
		"processor":  "x86_64",
		"refid":      "194.177.210.54",
		"reftime":    "0xe7504a10.74414244",
		"clock":      "0xe7504e80.8c46aa3f",
		"peer":       "14835",
		"sys_jitter": "1.648010",
		"leapsec":    "201701010000",
		"expire":     "202306280000",
		"leap":       "0",
		"stratum":    "2",
		"precision":  "-24",
		"offset":     "-0.149638",
		"frequency":  "- 7.734",
		"clk_wander": "0.081",
		"tai":        "37",
		"version":    "ntpd 4.2.8p15@1.3728-o Wed Sep 23 11:46:38 UTC 2020 (1)",
		"rootdisp":   "23.404",
		"clk_jitter": "0.626",
		"system":     "Linux/5.10.0-19-amd64",
	}

	return info, nil
}

func (m *mockClient) peerInfo(id uint16) (map[string]string, error) {
	if m.errOnPeerInfo {
		return nil, errors.New("mockClient.peerInfo() error")
	}

	info := map[string]string{
		"delay":      "10.464",
		"dispersion": "5.376",
		"dstadr":     "10.10.10.20",
		"dstport":    "123",
		"filtdelay":  "11.34 10.53 10.49 10.46 10.92 10.56 10.69 37.99",
		"filtdisp":   "0.00 2.01 4.01 5.93 7.89 9.84 11.81 13.73",
		"filtoffset": "0.66 0.32 0.18 0.31 0.33 0.10 0.34 14.07",
		"flash":      "0x0",
		"headway":    "0",
		"hmode":      "3",
		"hpoll":      "7",
		"jitter":     "5.204",
		"keyid":      "0",
		"leap":       "0",
		"offset":     "0.312",
		"pmode":      "4",
		"ppoll":      "7",
		"precision":  "-21",
		"reach":      "0xff",
		"rec":        "0xe7504df8.74802284",
		"refid":      "193.93.164.193",
		"reftime":    "0xe7504b8b.0c98a518",
		"rootdelay":  "0.198",
		"rootdisp":   "14.465",
		"srcadr":     fmt.Sprintf("203.0.113.%d", id),
		"srcport":    "123",
		"stratum":    "2",
		"unreach":    "0",
		"xleave":     "0.095",
	}

	return info, nil
}

func (m *mockClient) peerIDs() ([]uint16, error) {
	if m.errOnPeerIDs {
		return nil, errors.New("mockClient.peerIDs() error")
	}
	return []uint16{1, 2, 3}, nil
}

func (m *mockClient) close() {
	m.closeCalled = true
}
