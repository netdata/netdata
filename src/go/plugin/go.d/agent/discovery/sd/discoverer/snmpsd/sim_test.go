// SPDX-License-Identifier: GPL-3.0-or-later

package snmpsd

import (
	"context"
	"errors"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/gosnmp/gosnmp"
	snmpmock "github.com/gosnmp/gosnmp/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/discovery/sd/model"
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

	d.newSnmpClient = func() (gosnmp.Handler, func()) {
		h, cleanup := prepareMockSnmpHandler(t)
		h.setExpectInit()
		h.setExpectSysInfo()
		if sim.updateSnmpHandler != nil {
			sim.updateSnmpHandler(h)
		}
		return h, cleanup
	}

	seen := make(map[string]model.TargetGroup)
	ctx, cancel := context.WithCancel(context.Background())
	in := make(chan []model.TargetGroup)
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		d.Discover(ctx, in)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case tggs := <-in:
				for _, tgg := range tggs {
					seen[tgg.Source()] = tgg
				}
			}
		}
	}()

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

	time.Sleep(time.Second * 2)

	cancel()

	select {
	case <-done:
	case <-time.After(time.Second * 5):
		require.Fail(t, "discovery hasn't finished after cancel")
	}

	var tggs []model.TargetGroup
	for _, tgg := range seen {
		tggs = append(tggs, tgg)
	}

	sortTargetGroups(tggs)
	sortTargetGroups(sim.wantGroups)

	wantLen, gotLen := calcTargets(sim.wantGroups), calcTargets(tggs)
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

type mockSnmpHandler struct {
	*snmpmock.MockHandler
	skipOnConnect func(ip string) bool
}

func (m *mockSnmpHandler) Connect() error {
	if m.skipOnConnect != nil && m.skipOnConnect(m.MockHandler.Target()) {
		return errors.New("mock handler skip connect")
	}
	return m.MockHandler.Connect()
}

func prepareMockSnmpHandler(t *testing.T) (*mockSnmpHandler, func()) {
	mockCtl := gomock.NewController(t)
	cleanup := func() { mockCtl.Finish() }
	mockSNMP := snmpmock.NewMockHandler(mockCtl)
	m := &mockSnmpHandler{MockHandler: mockSNMP}

	return m, cleanup
}

func (m *mockSnmpHandler) setExpectInit() {
	var ip string
	m.EXPECT().Target().DoAndReturn(func() string { return ip }).AnyTimes()
	m.EXPECT().SetTarget(gomock.Any()).Do(func(target string) { ip = target }).AnyTimes()
	m.EXPECT().Port().AnyTimes()
	m.EXPECT().Version().AnyTimes()
	m.EXPECT().Community().AnyTimes()
	m.EXPECT().SetPort(gomock.Any()).AnyTimes()
	m.EXPECT().SetRetries(gomock.Any()).AnyTimes()
	m.EXPECT().SetMaxRepetitions(gomock.Any()).AnyTimes()
	m.EXPECT().SetMaxOids(gomock.Any()).AnyTimes()
	m.EXPECT().SetLogger(gomock.Any()).AnyTimes()
	m.EXPECT().SetTimeout(gomock.Any()).AnyTimes()
	m.EXPECT().SetCommunity(gomock.Any()).AnyTimes()
	m.EXPECT().SetVersion(gomock.Any()).AnyTimes()
	m.EXPECT().SetSecurityModel(gomock.Any()).AnyTimes()
	m.EXPECT().SetMsgFlags(gomock.Any()).AnyTimes()
	m.EXPECT().SetSecurityParameters(gomock.Any()).AnyTimes()
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
	m.EXPECT().WalkAll(snmputils.RootOidMibSystem).Return([]gosnmp.SnmpPDU{
		{Name: snmputils.OidSysDescr, Value: []uint8(mockSysDescr), Type: gosnmp.OctetString},
		{Name: snmputils.OidSysObject, Value: mockSysObject, Type: gosnmp.ObjectIdentifier},
		{Name: snmputils.OidSysContact, Value: []uint8(mockSysContact), Type: gosnmp.OctetString},
		{Name: snmputils.OidSysName, Value: []uint8(mockSysName), Type: gosnmp.OctetString},
		{Name: snmputils.OidSysLocation, Value: []uint8(mockSysLocation), Type: gosnmp.OctetString},
	}, nil).AnyTimes()
}
