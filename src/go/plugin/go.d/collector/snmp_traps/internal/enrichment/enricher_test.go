// SPDX-License-Identifier: GPL-3.0-or-later

package enrichment

import (
	"net/netip"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/model"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/reversedns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testReverseDNS struct {
	result        reversedns.Result
	scheduleState reversedns.ScheduleState
	lookups       int
	schedules     int
}

func (d *testReverseDNS) Lookup(netip.Addr) reversedns.Result {
	d.lookups++
	return d.result
}

func (d *testReverseDNS) Schedule(netip.Addr) reversedns.ScheduleState {
	d.schedules++
	return d.scheduleState
}

func TestEnrichReverseDNSCachedPositive(t *testing.T) {
	dns := &testReverseDNS{result: reversedns.Result{State: reversedns.StatePositive, Name: "device.example.test"}}
	entry := &model.TrapEntry{SourceIP: "192.0.2.10"}

	New(nil, nil, dns).Enrich(entry, true)

	assert.Equal(t, "device.example.test", entry.ReverseDNS)
	require.NotNil(t, entry.Enrichment)
	require.NotNil(t, entry.Enrichment.ReverseDNS)
	assert.Equal(t, "matched", entry.Enrichment.ReverseDNS.Status)
	assert.Equal(t, 1, dns.lookups)
	assert.Zero(t, dns.schedules)
}

func TestEnrichReverseDNSMissRemainsPendingWhenScheduleSeesPositive(t *testing.T) {
	dns := &testReverseDNS{
		result:        reversedns.Result{State: reversedns.StateMiss},
		scheduleState: reversedns.SchedulePositive,
	}
	entry := &model.TrapEntry{SourceIP: "192.0.2.10"}

	New(nil, nil, dns).Enrich(entry, true)

	assert.Empty(t, entry.ReverseDNS)
	require.NotNil(t, entry.Enrichment)
	require.NotNil(t, entry.Enrichment.ReverseDNS)
	assert.Equal(t, "pending", entry.Enrichment.ReverseDNS.Status)
	assert.Equal(t, 1, dns.lookups)
	assert.Equal(t, 1, dns.schedules)
}

func TestEnrichReverseDNSDisabledDoesNotUseResolver(t *testing.T) {
	dns := &testReverseDNS{result: reversedns.Result{State: reversedns.StatePositive, Name: "device.example.test"}}
	entry := &model.TrapEntry{SourceIP: "192.0.2.10"}

	New(nil, nil, dns).Enrich(entry, false)

	assert.Empty(t, entry.ReverseDNS)
	assert.Zero(t, dns.lookups)
	assert.Zero(t, dns.schedules)
	require.NotNil(t, entry.Enrichment)
	assert.Nil(t, entry.Enrichment.ReverseDNS)
}

func TestIsUnresolvedSysName(t *testing.T) {
	tests := map[string]struct {
		name string
		want bool
	}{
		"empty":      {want: true},
		"whitespace": {name: " \t", want: true},
		"unknown":    {name: " UNKNOWN ", want: true},
		"resolved":   {name: "switch.example.test"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, IsUnresolvedSysName(tc.name))
		})
	}
}
