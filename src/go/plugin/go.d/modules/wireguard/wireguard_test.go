// SPDX-License-Identifier: GPL-3.0-or-later

package wireguard

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
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

func TestWireGuard_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &WireGuard{}, dataConfigJSON, dataConfigYAML)
}

func TestWireGuard_Init(t *testing.T) {
	assert.NoError(t, New().Init())
}

func TestWireGuard_Charts(t *testing.T) {
	assert.Len(t, *New().Charts(), 0)

}

func TestWireGuard_Cleanup(t *testing.T) {
	tests := map[string]struct {
		prepare   func(w *WireGuard)
		wantClose bool
	}{
		"after New": {
			wantClose: false,
			prepare:   func(w *WireGuard) {},
		},
		"after Init": {
			wantClose: false,
			prepare:   func(w *WireGuard) { _ = w.Init() },
		},
		"after Check": {
			wantClose: true,
			prepare:   func(w *WireGuard) { _ = w.Init(); _ = w.Check() },
		},
		"after Collect": {
			wantClose: true,
			prepare:   func(w *WireGuard) { _ = w.Init(); _ = w.Collect() },
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			w := New()
			m := &mockClient{}
			w.newWGClient = func() (wgClient, error) { return m, nil }

			test.prepare(w)

			require.NotPanics(t, w.Cleanup)

			if test.wantClose {
				assert.True(t, m.closeCalled)
			} else {
				assert.False(t, m.closeCalled)
			}
		})
	}
}

func TestWireGuard_Check(t *testing.T) {
	tests := map[string]struct {
		wantFail bool
		prepare  func(w *WireGuard)
	}{
		"success when devices and peers found": {
			wantFail: false,
			prepare: func(w *WireGuard) {
				m := &mockClient{}
				d1 := prepareDevice(1)
				d1.Peers = append(d1.Peers, preparePeer("11"))
				d1.Peers = append(d1.Peers, preparePeer("12"))
				m.devices = append(m.devices, d1)
				w.client = m
			},
		},
		"success when devices and no peers found": {
			wantFail: false,
			prepare: func(w *WireGuard) {
				m := &mockClient{}
				m.devices = append(m.devices, prepareDevice(1))
				w.client = m
			},
		},
		"fail when no devices and no peers found": {
			wantFail: true,
			prepare: func(w *WireGuard) {
				w.client = &mockClient{}
			},
		},
		"fail when error on retrieving devices": {
			wantFail: true,
			prepare: func(w *WireGuard) {
				w.client = &mockClient{errOnDevices: true}
			},
		},
		"fail when error on creating client": {
			wantFail: true,
			prepare: func(w *WireGuard) {
				w.newWGClient = func() (wgClient, error) { return nil, errors.New("mock.newWGClient() error") }
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			w := New()
			require.NoError(t, w.Init())
			test.prepare(w)

			if test.wantFail {
				assert.Error(t, w.Check())
			} else {
				assert.NoError(t, w.Check())
			}
		})
	}
}

func TestWireGuard_Collect(t *testing.T) {
	type testCaseStep struct {
		prepareMock func(m *mockClient)
		check       func(t *testing.T, w *WireGuard)
	}
	tests := map[string][]testCaseStep{
		"several devices no peers": {
			{
				prepareMock: func(m *mockClient) {
					m.devices = append(m.devices, prepareDevice(1))
					m.devices = append(m.devices, prepareDevice(2))
				},
				check: func(t *testing.T, w *WireGuard) {
					mx := w.Collect()

					expected := map[string]int64{
						"device_wg1_peers":    0,
						"device_wg1_receive":  0,
						"device_wg1_transmit": 0,
						"device_wg2_peers":    0,
						"device_wg2_receive":  0,
						"device_wg2_transmit": 0,
					}

					copyLatestHandshake(mx, expected)
					assert.Equal(t, expected, mx)
					assert.Equal(t, len(deviceChartsTmpl)*2, len(*w.Charts()))
				},
			},
		},
		"several devices several peers each": {
			{
				prepareMock: func(m *mockClient) {
					d1 := prepareDevice(1)
					d1.Peers = append(d1.Peers, preparePeer("11"))
					d1.Peers = append(d1.Peers, preparePeer("12"))
					m.devices = append(m.devices, d1)

					d2 := prepareDevice(2)
					d2.Peers = append(d2.Peers, preparePeer("21"))
					d2.Peers = append(d2.Peers, preparePeer("22"))
					m.devices = append(m.devices, d2)
				},
				check: func(t *testing.T, w *WireGuard) {
					mx := w.Collect()

					expected := map[string]int64{
						"device_wg1_peers":    2,
						"device_wg1_receive":  0,
						"device_wg1_transmit": 0,
						"device_wg2_peers":    2,
						"device_wg2_receive":  0,
						"device_wg2_transmit": 0,
						"peer_wg1_cGVlcjExAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=_latest_handshake_ago": 60,
						"peer_wg1_cGVlcjExAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=_receive":              0,
						"peer_wg1_cGVlcjExAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=_transmit":             0,
						"peer_wg1_cGVlcjEyAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=_latest_handshake_ago": 60,
						"peer_wg1_cGVlcjEyAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=_receive":              0,
						"peer_wg1_cGVlcjEyAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=_transmit":             0,
						"peer_wg2_cGVlcjIxAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=_latest_handshake_ago": 60,
						"peer_wg2_cGVlcjIxAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=_receive":              0,
						"peer_wg2_cGVlcjIxAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=_transmit":             0,
						"peer_wg2_cGVlcjIyAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=_latest_handshake_ago": 60,
						"peer_wg2_cGVlcjIyAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=_receive":              0,
						"peer_wg2_cGVlcjIyAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=_transmit":             0,
					}

					copyLatestHandshake(mx, expected)
					assert.Equal(t, expected, mx)
					assert.Equal(t, len(deviceChartsTmpl)*2+len(peerChartsTmpl)*4, len(*w.Charts()))
				},
			},
		},
		"peers without last handshake time": {
			{
				prepareMock: func(m *mockClient) {
					d1 := prepareDevice(1)
					d1.Peers = append(d1.Peers, preparePeer("11"))
					d1.Peers = append(d1.Peers, preparePeer("12"))
					d1.Peers = append(d1.Peers, prepareNoLastHandshakePeer("13"))
					d1.Peers = append(d1.Peers, prepareNoLastHandshakePeer("14"))
					m.devices = append(m.devices, d1)
				},
				check: func(t *testing.T, w *WireGuard) {
					mx := w.Collect()

					expected := map[string]int64{
						"device_wg1_peers":    4,
						"device_wg1_receive":  0,
						"device_wg1_transmit": 0,
						"peer_wg1_cGVlcjExAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=_latest_handshake_ago": 60,
						"peer_wg1_cGVlcjExAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=_receive":              0,
						"peer_wg1_cGVlcjExAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=_transmit":             0,
						"peer_wg1_cGVlcjEyAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=_latest_handshake_ago": 60,
						"peer_wg1_cGVlcjEyAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=_receive":              0,
						"peer_wg1_cGVlcjEyAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=_transmit":             0,
					}

					copyLatestHandshake(mx, expected)
					assert.Equal(t, expected, mx)
					assert.Equal(t, len(deviceChartsTmpl)+len(peerChartsTmpl)*2, len(*w.Charts()))
				},
			},
		},
		"device added at runtime": {
			{
				prepareMock: func(m *mockClient) {
					m.devices = append(m.devices, prepareDevice(1))
				},
				check: func(t *testing.T, w *WireGuard) {
					_ = w.Collect()
					assert.Equal(t, len(deviceChartsTmpl)*1, len(*w.Charts()))
				},
			},
			{
				prepareMock: func(m *mockClient) {
					m.devices = append(m.devices, prepareDevice(2))
				},
				check: func(t *testing.T, w *WireGuard) {
					mx := w.Collect()

					expected := map[string]int64{
						"device_wg1_peers":    0,
						"device_wg1_receive":  0,
						"device_wg1_transmit": 0,
						"device_wg2_peers":    0,
						"device_wg2_receive":  0,
						"device_wg2_transmit": 0,
					}
					copyLatestHandshake(mx, expected)
					assert.Equal(t, expected, mx)
					assert.Equal(t, len(deviceChartsTmpl)*2, len(*w.Charts()))

				},
			},
		},
		"device removed at run time, no cleanup occurred": {
			{
				prepareMock: func(m *mockClient) {
					m.devices = append(m.devices, prepareDevice(1))
					m.devices = append(m.devices, prepareDevice(2))
				},
				check: func(t *testing.T, w *WireGuard) {
					_ = w.Collect()
				},
			},
			{
				prepareMock: func(m *mockClient) {
					m.devices = m.devices[:len(m.devices)-1]
				},
				check: func(t *testing.T, w *WireGuard) {
					_ = w.Collect()
					assert.Equal(t, len(deviceChartsTmpl)*2, len(*w.Charts()))
					assert.Equal(t, 0, calcObsoleteCharts(w.Charts()))
				},
			},
		},
		"device removed at run time, cleanup occurred": {
			{
				prepareMock: func(m *mockClient) {
					m.devices = append(m.devices, prepareDevice(1))
					m.devices = append(m.devices, prepareDevice(2))
				},
				check: func(t *testing.T, w *WireGuard) {
					_ = w.Collect()
				},
			},
			{
				prepareMock: func(m *mockClient) {
					m.devices = m.devices[:len(m.devices)-1]
				},
				check: func(t *testing.T, w *WireGuard) {
					w.cleanupEvery = time.Second
					time.Sleep(time.Second)
					_ = w.Collect()
					assert.Equal(t, len(deviceChartsTmpl)*2, len(*w.Charts()))
					assert.Equal(t, len(deviceChartsTmpl)*1, calcObsoleteCharts(w.Charts()))
				},
			},
		},
		"peer added at runtime": {
			{
				prepareMock: func(m *mockClient) {
					m.devices = append(m.devices, prepareDevice(1))
				},
				check: func(t *testing.T, w *WireGuard) {
					_ = w.Collect()
					assert.Equal(t, len(deviceChartsTmpl)*1, len(*w.Charts()))
				},
			},
			{
				prepareMock: func(m *mockClient) {
					d1 := m.devices[0]
					d1.Peers = append(d1.Peers, preparePeer("11"))
				},
				check: func(t *testing.T, w *WireGuard) {
					mx := w.Collect()

					expected := map[string]int64{
						"device_wg1_peers":    1,
						"device_wg1_receive":  0,
						"device_wg1_transmit": 0,
						"peer_wg1_cGVlcjExAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=_latest_handshake_ago": 60,
						"peer_wg1_cGVlcjExAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=_receive":              0,
						"peer_wg1_cGVlcjExAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=_transmit":             0,
					}
					copyLatestHandshake(mx, expected)
					assert.Equal(t, expected, mx)
					assert.Equal(t, len(deviceChartsTmpl)*1+len(peerChartsTmpl)*1, len(*w.Charts()))
				},
			},
		},
		"peer removed at run time, no cleanup occurred": {
			{
				prepareMock: func(m *mockClient) {
					d1 := prepareDevice(1)
					d1.Peers = append(d1.Peers, preparePeer("11"))
					d1.Peers = append(d1.Peers, preparePeer("12"))
					m.devices = append(m.devices, d1)
				},
				check: func(t *testing.T, w *WireGuard) {
					_ = w.Collect()
				},
			},
			{
				prepareMock: func(m *mockClient) {
					d1 := m.devices[0]
					d1.Peers = d1.Peers[:len(d1.Peers)-1]
				},
				check: func(t *testing.T, w *WireGuard) {
					_ = w.Collect()
					assert.Equal(t, len(deviceChartsTmpl)*1+len(peerChartsTmpl)*2, len(*w.Charts()))
					assert.Equal(t, 0, calcObsoleteCharts(w.Charts()))
				},
			},
		},
		"peer removed at run time, cleanup occurred": {
			{
				prepareMock: func(m *mockClient) {
					d1 := prepareDevice(1)
					d1.Peers = append(d1.Peers, preparePeer("11"))
					d1.Peers = append(d1.Peers, preparePeer("12"))
					m.devices = append(m.devices, d1)
				},
				check: func(t *testing.T, w *WireGuard) {
					_ = w.Collect()
				},
			},
			{
				prepareMock: func(m *mockClient) {
					d1 := m.devices[0]
					d1.Peers = d1.Peers[:len(d1.Peers)-1]
				},
				check: func(t *testing.T, w *WireGuard) {
					w.cleanupEvery = time.Second
					time.Sleep(time.Second)
					_ = w.Collect()
					assert.Equal(t, len(deviceChartsTmpl)*1+len(peerChartsTmpl)*2, len(*w.Charts()))
					assert.Equal(t, len(peerChartsTmpl)*1, calcObsoleteCharts(w.Charts()))
				},
			},
		},
		"fails if no devices found": {
			{
				prepareMock: func(m *mockClient) {},
				check: func(t *testing.T, w *WireGuard) {
					assert.Equal(t, map[string]int64(nil), w.Collect())
				},
			},
		},
		"fails if error on getting devices list": {
			{
				prepareMock: func(m *mockClient) {
					m.errOnDevices = true
				},
				check: func(t *testing.T, w *WireGuard) {
					assert.Equal(t, map[string]int64(nil), w.Collect())
				},
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			w := New()
			require.NoError(t, w.Init())
			m := &mockClient{}
			w.client = m

			for i, step := range test {
				t.Run(fmt.Sprintf("step[%d]", i), func(t *testing.T) {
					step.prepareMock(m)
					step.check(t, w)
				})
			}
		})
	}
}

type mockClient struct {
	devices      []*wgtypes.Device
	errOnDevices bool
	closeCalled  bool
}

func (m *mockClient) Devices() ([]*wgtypes.Device, error) {
	if m.errOnDevices {
		return nil, errors.New("mock.Devices() error")
	}
	return m.devices, nil
}

func (m *mockClient) Close() error {
	m.closeCalled = true
	return nil
}

func prepareDevice(num uint8) *wgtypes.Device {
	return &wgtypes.Device{
		Name: fmt.Sprintf("wg%d", num),
	}
}

func preparePeer(s string) wgtypes.Peer {
	b := make([]byte, 32)
	b = append(b[:0], fmt.Sprintf("peer%s", s)...)
	k, _ := wgtypes.NewKey(b[:32])

	return wgtypes.Peer{
		PublicKey:         k,
		LastHandshakeTime: time.Now().Add(-time.Minute),
		ReceiveBytes:      0,
		TransmitBytes:     0,
	}
}

func prepareNoLastHandshakePeer(s string) wgtypes.Peer {
	p := preparePeer(s)
	var lh time.Time
	p.LastHandshakeTime = lh
	return p
}

func copyLatestHandshake(dst, src map[string]int64) {
	for k, v := range src {
		if strings.HasSuffix(k, "latest_handshake_ago") {
			if _, ok := dst[k]; ok {
				dst[k] = v
			}
		}
	}
}

func calcObsoleteCharts(charts *module.Charts) int {
	var num int
	for _, c := range *charts {
		if c.Obsolete {
			num++
		}
	}
	return num
}
