// SPDX-License-Identifier: GPL-3.0-or-later

package snmpsd

import (
	"context"
	"errors"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/gosnmp/gosnmp"
	snmpmock "github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils/snmpmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery/sd/model"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils"
)

type discoverySim struct {
	cfg               Config
	updateSnmpHandler func(m *mockSnmpHandler)
	wantGroups        []model.TargetGroup
}

func (sim *discoverySim) run(t *testing.T) {
	d, err := NewDiscoverer(sim.cfg)
	require.NoError(t, err)

	d.newSnmpClient = func(handler gosnmp.Handler) (snmputils.Session, func()) {
		h, cleanup := prepareMockSnmpHandler(t, handler)
		h.setExpectInit()
		h.setExpectSysInfo()
		if sim.updateSnmpHandler != nil {
			sim.updateSnmpHandler(h)
		}
		return h, cleanup
	}

	seen := make(map[string]model.TargetGroup)
	var seenMu sync.Mutex
	ctx, cancel := context.WithCancel(context.Background())
	in := make(chan []model.TargetGroup)
	var wg sync.WaitGroup

	wg.Go(func() {
		d.Discover(ctx, in)
	})

	wg.Go(func() {
		for {
			select {
			case <-ctx.Done():
				return
			case tggs := <-in:
				seenMu.Lock()
				for _, tgg := range tggs {
					seen[tgg.Source()] = tgg
				}
				seenMu.Unlock()
			}
		}
	})

	done := make(chan struct{})
	go func() {
		defer close(done)
		wg.Wait()
	}()

	select {
	case <-d.started:
	case <-time.After(time.Second * 5):
		require.Fail(t, "discovery failed to start")
	}

	wantLen := calcTargets(sim.wantGroups)
	require.Eventually(t, func() bool {
		seenMu.Lock()
		defer seenMu.Unlock()
		return calcSeenTargets(seen) >= wantLen
	}, time.Second*5, time.Millisecond*10)

	cancel()

	select {
	case <-done:
	case <-time.After(time.Second * 5):
		require.Fail(t, "discovery hasn't finished after cancel")
	}

	var tggs []model.TargetGroup
	seenMu.Lock()
	for _, tgg := range seen {
		tggs = append(tggs, tgg)
	}
	seenMu.Unlock()

	sortTargetGroups(tggs)
	sortTargetGroups(sim.wantGroups)

	gotLen := calcTargets(tggs)
	assert.Equalf(t, wantLen, gotLen, "different len (want %d got %d)", wantLen, gotLen)
	assert.Equal(t, sim.wantGroups, tggs)
}

func calcTargets(tggs []model.TargetGroup) int {
	var n int
	for _, tgg := range tggs {
		n += len(tgg.Targets())
	}
	return n
}

func calcSeenTargets(tggs map[string]model.TargetGroup) int {
	var n int
	for _, tgg := range tggs {
		n += len(tgg.Targets())
	}
	return n
}

func sortTargetGroups(tggs []model.TargetGroup) {
	if len(tggs) == 0 {
		return
	}
	sort.Slice(tggs, func(i, j int) bool { return tggs[i].Source() < tggs[j].Source() })

	for idx := range tggs {
		tgts := tggs[idx].Targets()
		sort.Slice(tgts, func(i, j int) bool { return tgts[i].Hash() < tgts[j].Hash() })
	}
}

// mockSnmpHandler is the session of one probe; handler is the client the
// discoverer configured for it.
type mockSnmpHandler struct {
	*snmpmock.MockSession
	handler       gosnmp.Handler
	skipOnConnect func(ip string) bool
}

func (m *mockSnmpHandler) Connect() error {
	if m.skipOnConnect != nil && m.skipOnConnect(m.handler.Target()) {
		return errors.New("mock handler skip connect")
	}
	return m.MockSession.Connect()
}

func prepareMockSnmpHandler(t *testing.T, handler gosnmp.Handler) (*mockSnmpHandler, func()) {
	mockCtl := gomock.NewController(t)
	cleanup := func() { mockCtl.Finish() }
	mockSNMP := snmpmock.NewMockSession(mockCtl)
	m := &mockSnmpHandler{MockSession: mockSNMP, handler: handler}

	return m, cleanup
}

func (m *mockSnmpHandler) setExpectInit() {
	m.EXPECT().Connect().Return(nil).AnyTimes()
	m.EXPECT().Close().Return(nil).AnyTimes()
}

const (
	mockSysDescr    = "mock sysDescr"
	mockSysObject   = ".1.3.6.1.4.1.8072.3.2.10"
	mockSysContact  = "mock sysContact"
	mockSysName     = "mock sysName"
	mockSysLocation = "mock sysLocation"
)

func (m *mockSnmpHandler) setExpectSysInfo() {
	m.EXPECT().Get(sysInfoOIDsForTest()).Return(&gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{
			{Name: snmputils.OidSysDescr, Value: []uint8(mockSysDescr), Type: gosnmp.OctetString},
			{Name: snmputils.OidSysObject, Value: mockSysObject, Type: gosnmp.ObjectIdentifier},
			{Name: snmputils.OidSysContact, Value: []uint8(mockSysContact), Type: gosnmp.OctetString},
			{Name: snmputils.OidSysName, Value: []uint8(mockSysName), Type: gosnmp.OctetString},
			{Name: snmputils.OidSysLocation, Value: []uint8(mockSysLocation), Type: gosnmp.OctetString},
		},
	}, nil).AnyTimes()
}

func sysInfoOIDsForTest() []string {
	return []string{
		snmputils.OidSysDescr,
		snmputils.OidSysObject,
		snmputils.OidSysContact,
		snmputils.OidSysName,
		snmputils.OidSysLocation,
	}
}
