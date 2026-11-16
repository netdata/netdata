// SPDX-License-Identifier: GPL-3.0-or-later

package wireguard

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")
)

var testNow = time.Date(2026, time.January, 1, 12, 0, 0, 0, time.UTC)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON": dataConfigJSON,
		"dataConfigYAML": dataConfigYAML,
	} {
		assert.NotNil(t, data, name)
	}
}

func TestCollector_ConfigurationSerialize(t *testing.T) {
	collecttest.TestConfigurationSerialize(t, &Collector{}, dataConfigJSON, dataConfigYAML)
}

func TestCollector_Init(t *testing.T) {
	assert.NoError(t, New().Init(context.Background()))
}

func TestArtifacts(t *testing.T) {
	templateYAML := New().ChartTemplateYAML()
	collecttest.AssertChartTemplateSchema(t, templateYAML)

	spec, err := charttpl.DecodeYAML([]byte(templateYAML))
	require.NoError(t, err)
	require.NoError(t, spec.Validate())
	_, err = chartengine.Compile(spec, 1)
	require.NoError(t, err)

	metadata, err := os.ReadFile("metadata.yaml")
	require.NoError(t, err)
	collecttest.AssertMetadataDocumentsChartTemplate(t, metadata, templateYAML, nil)
	collecttest.AssertConfigSchemaMatchesMetadataWith(t, "config_schema.json", "metadata.yaml",
		collecttest.ConfigSchemaCheck{Defaults: true})
}

func TestCollector_Cleanup(t *testing.T) {
	tests := map[string]struct {
		prepare   func(c *Collector)
		wantClose bool
	}{
		"after New": {
			wantClose: false,
			prepare:   func(*Collector) {},
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
			prepare: func(c *Collector) {
				_ = c.Init(context.Background())
				_, _ = collecttest.CollectScalarSeries(c)
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := New()
			m := &mockClient{}
			collr.newWGClient = func() (wgClient, error) { return m, nil }

			test.prepare(collr)

			require.NotPanics(t, func() { collr.Cleanup(context.Background()) })
			assert.Equal(t, test.wantClose, m.closeCalled)

			m.closeCalled = false
			require.NotPanics(t, func() { collr.Cleanup(context.Background()) })
			assert.False(t, m.closeCalled, "second Cleanup must not close the client again")
		})
	}
}

func TestCollector_Check(t *testing.T) {
	tests := map[string]struct {
		wantFail bool
		prepare  func(c *Collector)
	}{
		"success when devices and peers found": {
			wantFail: false,
			prepare: func(c *Collector) {
				c.client = &mockClient{devices: []*wgtypes.Device{
					prepareDevice("wg1", preparePeer("11", 0, 0), preparePeer("12", 0, 0)),
				}}
			},
		},
		"success when devices and no peers found": {
			wantFail: false,
			prepare: func(c *Collector) {
				c.client = &mockClient{devices: []*wgtypes.Device{prepareDevice("wg1")}}
			},
		},
		"fail when no devices and no peers found": {
			wantFail: true,
			prepare: func(c *Collector) {
				c.client = &mockClient{}
			},
		},
		"fail when error on retrieving devices": {
			wantFail: true,
			prepare: func(c *Collector) {
				c.client = &mockClient{errOnDevices: true}
			},
		},
		"fail when error on creating client": {
			wantFail: true,
			prepare: func(c *Collector) {
				c.newWGClient = func() (wgClient, error) { return nil, errors.New("mock.newWGClient() error") }
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := New()
			require.NoError(t, collr.Init(context.Background()))
			test.prepare(collr)

			if test.wantFail {
				assert.Error(t, collr.Check(context.Background()))
			} else {
				assert.NoError(t, collr.Check(context.Background()))
			}
		})
	}
}

func TestCollector_Collect(t *testing.T) {
	key11 := preparePeer("11", 0, 0).PublicKey.String()
	key12 := preparePeer("12", 0, 0).PublicKey.String()
	key21 := preparePeer("21", 0, 0).PublicKey.String()

	type testCaseStep struct {
		prepareMock func(*mockClient)
		wantErr     bool
		wantSeries  map[string]metrix.SampleValue
	}
	tests := map[string][]testCaseStep{
		"several devices no peers": {
			{
				prepareMock: func(m *mockClient) {
					m.devices = []*wgtypes.Device{prepareDevice("wg1"), prepareDevice("wg2")}
				},
				wantSeries: map[string]metrix.SampleValue{
					series("device_peers", "device", "wg1"):    0,
					series("device_receive", "device", "wg1"):  0,
					series("device_transmit", "device", "wg1"): 0,
					series("device_peers", "device", "wg2"):    0,
					series("device_receive", "device", "wg2"):  0,
					series("device_transmit", "device", "wg2"): 0,
				},
			},
		},
		"several devices several peers each": {
			{
				prepareMock: func(m *mockClient) {
					m.devices = []*wgtypes.Device{
						prepareDevice("wg1", preparePeer("11", 100, 200), preparePeer("12", 10, 20)),
						prepareDevice("wg2", preparePeer("21", 1, 2)),
					}
				},
				wantSeries: map[string]metrix.SampleValue{
					series("device_peers", "device", "wg1"):                                   2,
					series("device_receive", "device", "wg1"):                                 110,
					series("device_transmit", "device", "wg1"):                                220,
					series("peer_receive", "device", "wg1", "public_key", key11):              100,
					series("peer_transmit", "device", "wg1", "public_key", key11):             200,
					series("peer_latest_handshake_ago", "device", "wg1", "public_key", key11): 60,
					series("peer_receive", "device", "wg1", "public_key", key12):              10,
					series("peer_transmit", "device", "wg1", "public_key", key12):             20,
					series("peer_latest_handshake_ago", "device", "wg1", "public_key", key12): 60,
					series("device_peers", "device", "wg2"):                                   1,
					series("device_receive", "device", "wg2"):                                 1,
					series("device_transmit", "device", "wg2"):                                2,
					series("peer_receive", "device", "wg2", "public_key", key21):              1,
					series("peer_transmit", "device", "wg2", "public_key", key21):             2,
					series("peer_latest_handshake_ago", "device", "wg2", "public_key", key21): 60,
				},
			},
		},
		"peers without last handshake time": {
			{
				prepareMock: func(m *mockClient) {
					m.devices = []*wgtypes.Device{prepareDevice("wg1",
						preparePeer("11", 100, 200),
						prepareNoLastHandshakePeer("13"),
						prepareNoLastHandshakePeer("14"),
					)}
				},
				wantSeries: map[string]metrix.SampleValue{
					series("device_peers", "device", "wg1"):                                   3,
					series("device_receive", "device", "wg1"):                                 100,
					series("device_transmit", "device", "wg1"):                                200,
					series("peer_receive", "device", "wg1", "public_key", key11):              100,
					series("peer_transmit", "device", "wg1", "public_key", key11):             200,
					series("peer_latest_handshake_ago", "device", "wg1", "public_key", key11): 60,
				},
			},
		},
		"only peers without last handshake time": {
			{
				prepareMock: func(m *mockClient) {
					m.devices = []*wgtypes.Device{prepareDevice("wg1",
						prepareNoLastHandshakePeer("13"),
						prepareNoLastHandshakePeer("14"),
					)}
				},
				wantSeries: map[string]metrix.SampleValue{
					series("device_peers", "device", "wg1"): 2,
				},
			},
		},
		"device added at runtime": {
			{
				prepareMock: func(m *mockClient) {
					m.devices = []*wgtypes.Device{prepareDevice("wg1")}
				},
				wantSeries: map[string]metrix.SampleValue{
					series("device_peers", "device", "wg1"):    0,
					series("device_receive", "device", "wg1"):  0,
					series("device_transmit", "device", "wg1"): 0,
				},
			},
			{
				prepareMock: func(m *mockClient) {
					m.devices = append(m.devices, prepareDevice("wg2"))
				},
				wantSeries: map[string]metrix.SampleValue{
					series("device_peers", "device", "wg1"):    0,
					series("device_receive", "device", "wg1"):  0,
					series("device_transmit", "device", "wg1"): 0,
					series("device_peers", "device", "wg2"):    0,
					series("device_receive", "device", "wg2"):  0,
					series("device_transmit", "device", "wg2"): 0,
				},
			},
		},
		"device removed at runtime": {
			{
				prepareMock: func(m *mockClient) {
					m.devices = []*wgtypes.Device{prepareDevice("wg1"), prepareDevice("wg2")}
				},
				wantSeries: map[string]metrix.SampleValue{
					series("device_peers", "device", "wg1"):    0,
					series("device_receive", "device", "wg1"):  0,
					series("device_transmit", "device", "wg1"): 0,
					series("device_peers", "device", "wg2"):    0,
					series("device_receive", "device", "wg2"):  0,
					series("device_transmit", "device", "wg2"): 0,
				},
			},
			{
				prepareMock: func(m *mockClient) {
					m.devices = m.devices[:1]
				},
				wantSeries: map[string]metrix.SampleValue{
					series("device_peers", "device", "wg1"):    0,
					series("device_receive", "device", "wg1"):  0,
					series("device_transmit", "device", "wg1"): 0,
				},
			},
		},
		"peer added at runtime": {
			{
				prepareMock: func(m *mockClient) {
					m.devices = []*wgtypes.Device{prepareDevice("wg1")}
				},
				wantSeries: map[string]metrix.SampleValue{
					series("device_peers", "device", "wg1"):    0,
					series("device_receive", "device", "wg1"):  0,
					series("device_transmit", "device", "wg1"): 0,
				},
			},
			{
				prepareMock: func(m *mockClient) {
					m.devices[0].Peers = append(m.devices[0].Peers, preparePeer("11", 5, 6))
				},
				wantSeries: map[string]metrix.SampleValue{
					series("device_peers", "device", "wg1"):                                   1,
					series("device_receive", "device", "wg1"):                                 5,
					series("device_transmit", "device", "wg1"):                                6,
					series("peer_receive", "device", "wg1", "public_key", key11):              5,
					series("peer_transmit", "device", "wg1", "public_key", key11):             6,
					series("peer_latest_handshake_ago", "device", "wg1", "public_key", key11): 60,
				},
			},
		},
		"peer removed at runtime": {
			{
				prepareMock: func(m *mockClient) {
					m.devices = []*wgtypes.Device{prepareDevice("wg1", preparePeer("11", 5, 6), preparePeer("12", 7, 8))}
				},
				wantSeries: map[string]metrix.SampleValue{
					series("device_peers", "device", "wg1"):                                   2,
					series("device_receive", "device", "wg1"):                                 12,
					series("device_transmit", "device", "wg1"):                                14,
					series("peer_receive", "device", "wg1", "public_key", key11):              5,
					series("peer_transmit", "device", "wg1", "public_key", key11):             6,
					series("peer_latest_handshake_ago", "device", "wg1", "public_key", key11): 60,
					series("peer_receive", "device", "wg1", "public_key", key12):              7,
					series("peer_transmit", "device", "wg1", "public_key", key12):             8,
					series("peer_latest_handshake_ago", "device", "wg1", "public_key", key12): 60,
				},
			},
			{
				prepareMock: func(m *mockClient) {
					m.devices[0].Peers = m.devices[0].Peers[:1]
				},
				wantSeries: map[string]metrix.SampleValue{
					series("device_peers", "device", "wg1"):                                   1,
					series("device_receive", "device", "wg1"):                                 5,
					series("device_transmit", "device", "wg1"):                                6,
					series("peer_receive", "device", "wg1", "public_key", key11):              5,
					series("peer_transmit", "device", "wg1", "public_key", key11):             6,
					series("peer_latest_handshake_ago", "device", "wg1", "public_key", key11): 60,
				},
			},
		},
		"no devices found": {
			{
				prepareMock: func(*mockClient) {},
				wantSeries:  map[string]metrix.SampleValue{},
			},
		},
		"fails if error on getting devices list": {
			{
				prepareMock: func(m *mockClient) { m.errOnDevices = true },
				wantErr:     true,
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := newTestCollector(t)
			m := collr.client.(*mockClient)

			for i, step := range test {
				t.Run(fmt.Sprintf("step[%d]", i), func(t *testing.T) {
					step.prepareMock(m)

					got, err := collecttest.CollectScalarSeries(collr)
					if step.wantErr {
						assert.Error(t, err)
						return
					}
					require.NoError(t, err)
					assert.Equal(t, step.wantSeries, got)
				})
			}
		})
	}
}

func TestCollector_CollectFailsWhenClientCannotBeCreated(t *testing.T) {
	collr := New()
	collr.newWGClient = func() (wgClient, error) { return nil, errors.New("mock.newWGClient() error") }
	require.NoError(t, collr.Init(context.Background()))

	_, err := collecttest.CollectScalarSeries(collr)
	assert.Error(t, err)
}

func TestCollector_ChartCoverage(t *testing.T) {
	collr := newTestCollector(t)
	collr.client.(*mockClient).devices = []*wgtypes.Device{prepareDevice("wg1", preparePeer("11", 1, 2))}

	_, err := collecttest.CollectScalarSeries(collr)
	require.NoError(t, err)

	collecttest.AssertChartCoverage(t, collr, collecttest.ChartCoverageExpectation{})
}

// Device "wg10" shares the "wg1" prefix: removing "wg1" and one peer of "wg10" must retire only their own charts.
func TestCollector_RemovesOnlyChartsOfGoneDevicesAndPeers(t *testing.T) {
	collr := newTestCollector(t)
	m := collr.client.(*mockClient)

	engine, err := chartengine.New()
	require.NoError(t, err)
	require.NoError(t, engine.LoadYAML([]byte(collr.ChartTemplateYAML()), 1))

	created := make(map[string]map[string]string)
	removed := make(map[string]bool)
	cycle := func() {
		t.Helper()
		_, err := collecttest.CollectScalarSeries(collr)
		require.NoError(t, err)

		attempt, err := engine.PreparePlan(collr.MetricStore().Read(metrix.ReadRaw(), metrix.ReadFlatten()))
		require.NoError(t, err)
		for _, action := range attempt.Plan().Actions {
			switch action := action.(type) {
			case chartengine.CreateChartAction:
				created[action.ChartID] = action.Labels
			case chartengine.RemoveChartAction:
				removed[action.ChartID] = true
			}
		}
		require.NoError(t, attempt.Commit())
	}

	goneKey := preparePeer("102", 0, 0).PublicKey.String()
	m.devices = []*wgtypes.Device{
		prepareDevice("wg1", preparePeer("11", 1, 1)),
		prepareDevice("wg10", preparePeer("101", 1, 1), preparePeer("102", 1, 1)),
	}
	cycle()
	require.Empty(t, removed)

	m.devices = []*wgtypes.Device{
		prepareDevice("wg10", preparePeer("101", 2, 2)),
	}
	// Keep collecting well beyond the chart expiry and the metric-store retention.
	for range 30 {
		cycle()
	}

	wantRemoved := make(map[string]bool)
	for chartID, labels := range created {
		if labels["device"] == "wg1" || labels["public_key"] == goneKey {
			wantRemoved[chartID] = true
		}
	}
	require.NotEmpty(t, wantRemoved)
	assert.Equal(t, wantRemoved, removed)
}

// testKey builds a deterministic public key: seed bytes followed by zeros.
func testKey(seed string) wgtypes.Key {
	var key wgtypes.Key
	copy(key[:], seed)
	return key
}

func newTestCollector(t *testing.T) *Collector {
	t.Helper()

	collr := New()
	collr.now = func() time.Time { return testNow }
	require.NoError(t, collr.Init(context.Background()))
	collr.client = &mockClient{}

	return collr
}

func series(name string, labels ...string) string {
	if len(labels) == 0 {
		return name
	}
	parts := make([]string, 0, len(labels)/2)
	for i := 0; i+1 < len(labels); i += 2 {
		parts = append(parts, labels[i]+"="+strconv.Quote(labels[i+1]))
	}
	return name + "{" + strings.Join(parts, ",") + "}"
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

func prepareDevice(name string, peers ...wgtypes.Peer) *wgtypes.Device {
	return &wgtypes.Device{
		Name:  name,
		Peers: peers,
	}
}

func preparePeer(s string, receive, transmit int64) wgtypes.Peer {
	return wgtypes.Peer{
		PublicKey:         testKey("peer" + s),
		LastHandshakeTime: testNow.Add(-time.Minute),
		ReceiveBytes:      receive,
		TransmitBytes:     transmit,
	}
}

func prepareNoLastHandshakePeer(s string) wgtypes.Peer {
	p := preparePeer(s, 0, 0)
	p.LastHandshakeTime = time.Time{}
	return p
}
