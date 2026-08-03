// SPDX-License-Identifier: GPL-3.0-or-later

package dedup

import (
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testTrapOID = "1.3.6.1.6.3.1.1.5.3"

func TestNormalize(t *testing.T) {
	tests := map[string]struct {
		cfg     Config
		wantErr string
	}{
		"disabled":          {cfg: Config{}},
		"defaults":          {cfg: Config{Enabled: true}},
		"negative window":   {cfg: Config{Enabled: true, WindowSec: -1}, wantErr: "window_sec"},
		"negative capacity": {cfg: Config{Enabled: true, CacheMaxEntries: -1}, wantErr: "cache_max_entries"},
		"empty key":         {cfg: Config{Enabled: true, KeyVarbinds: []string{"ifIndex", " "}}, wantErr: "key_varbinds[1]"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			policy, err := Normalize(tc.cfg)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.cfg.Enabled, policy.Enabled())
		})
	}
}

func TestNormalizeClonesJobKeyVarbinds(t *testing.T) {
	configured := []string{"ifIndex"}
	policy, err := Normalize(Config{Enabled: true, KeyVarbinds: configured})
	require.NoError(t, err)

	configured[0] = "mutated"
	returned := policy.KeyVarbinds()
	returned[0] = "also-mutated"
	assert.Equal(t, []string{"ifIndex"}, policy.KeyVarbinds())
}

func TestNormalizeWindowBounds(t *testing.T) {
	policy, err := Normalize(Config{Enabled: true, WindowSec: MaxWindowSec})
	require.NoError(t, err)
	assert.Equal(t, time.Duration(MaxWindowSec)*time.Second, policy.window)

	_, err = Normalize(Config{Enabled: true, WindowSec: MaxWindowSec + 1})
	require.ErrorContains(t, err, "window_sec")
}

func TestNormalizeTrimsJobKeyVarbinds(t *testing.T) {
	policy, err := Normalize(Config{Enabled: true, KeyVarbinds: []string{" ifIndex "}})
	require.NoError(t, err)
	assert.Equal(t, []string{"ifIndex"}, policy.KeyVarbinds())

	d := New(policy, Options{})
	_, decision := d.Admit(testEntry("198.51.100.10", "1"), policy.KeyVarbinds())
	assert.Equal(t, DecisionAdmit, decision)
	_, decision = d.Admit(testEntry("198.51.100.10", "2"), policy.KeyVarbinds())
	assert.Equal(t, DecisionAdmit, decision)
}

func TestDefaultFingerprintUsesSourceAndTrap(t *testing.T) {
	d := newTestDeduper(t, Config{Enabled: true}, Options{})
	_, decision := d.Admit(&model.TrapEntry{SourceIP: "198.51.100.10", TrapOID: testTrapOID}, nil)
	assert.Equal(t, DecisionAdmit, decision)
	_, decision = d.Admit(&model.TrapEntry{SourceIP: "198.51.100.10", TrapOID: testTrapOID}, nil)
	assert.Equal(t, DecisionSuppress, decision)
	_, decision = d.Admit(&model.TrapEntry{SourceIP: "198.51.100.11", TrapOID: testTrapOID}, nil)
	assert.Equal(t, DecisionAdmit, decision)
}

func TestSourceIdentityPriority(t *testing.T) {
	d := newTestDeduper(t, Config{Enabled: true}, Options{})
	first := &model.TrapEntry{SourceVnodeID: "vnode-1", SourceIP: "198.51.100.10", TrapOID: testTrapOID}
	_, decision := d.Admit(first, nil)
	assert.Equal(t, DecisionAdmit, decision)

	_, decision = d.Admit(&model.TrapEntry{SourceVnodeID: "vnode-1", SourceIP: "198.51.100.11", TrapOID: testTrapOID}, nil)
	assert.Equal(t, DecisionSuppress, decision)
	_, decision = d.Admit(&model.TrapEntry{SourceVnodeID: "vnode-2", SourceIP: "198.51.100.10", TrapOID: testTrapOID}, nil)
	assert.Equal(t, DecisionAdmit, decision)

	d = newTestDeduper(t, Config{Enabled: true}, Options{})
	_, decision = d.Admit(&model.TrapEntry{
		SourceIP: "198.51.100.10", DeviceHostname: "core-sw-01", TrapOID: testTrapOID,
	}, nil)
	assert.Equal(t, DecisionAdmit, decision)
	_, decision = d.Admit(&model.TrapEntry{
		SourceIP: "198.51.100.10", DeviceHostname: "core-sw-02", TrapOID: testTrapOID,
	}, nil)
	assert.Equal(t, DecisionSuppress, decision)
}

func TestResolvedKeyVarbindNamesNarrowFingerprint(t *testing.T) {
	d := newTestDeduper(t, Config{Enabled: true}, Options{})
	_, decision := d.Admit(testEntry("198.51.100.10", "1"), []string{"ifIndex"})
	assert.Equal(t, DecisionAdmit, decision)
	_, decision = d.Admit(testEntry("198.51.100.10", "2"), []string{"ifIndex"})
	assert.Equal(t, DecisionAdmit, decision)
	_, decision = d.Admit(testEntry("198.51.100.10", "1"), []string{"ifIndex"})
	assert.Equal(t, DecisionSuppress, decision)
}

func TestNumericColumnOIDKeyVarbind(t *testing.T) {
	d := newTestDeduper(t, Config{Enabled: true}, Options{})
	keyOID := "1.3.6.1.2.1.2.2.1.1"
	_, decision := d.Admit(testColumnEntry("1.3.6.1.2.1.2.2.1.1.1", "1"), []string{keyOID})
	assert.Equal(t, DecisionAdmit, decision)
	_, decision = d.Admit(testColumnEntry("1.3.6.1.2.1.2.2.1.1.2", "2"), []string{keyOID})
	assert.Equal(t, DecisionAdmit, decision)
	_, decision = d.Admit(testColumnEntry("1.3.6.1.2.1.2.2.1.1.1", "1"), []string{keyOID})
	assert.Equal(t, DecisionSuppress, decision)
}

func TestNumericInstanceOIDKeyVarbind(t *testing.T) {
	d := newTestDeduper(t, Config{Enabled: true}, Options{})
	keyOID := "1.3.6.1.2.1.2.2.1.1.1"
	_, decision := d.Admit(testEntry("198.51.100.10", "1"), []string{keyOID})
	assert.Equal(t, DecisionAdmit, decision)
	_, decision = d.Admit(testEntry("198.51.100.10", "2"), []string{keyOID})
	assert.Equal(t, DecisionAdmit, decision)
	_, decision = d.Admit(testEntry("198.51.100.10", "1"), []string{keyOID})
	assert.Equal(t, DecisionSuppress, decision)
}

func TestSensitiveKeyVarbindValueIsRedactedFromFingerprint(t *testing.T) {
	d := newTestDeduper(t, Config{Enabled: true}, Options{})
	entry := func(value string) *model.TrapEntry {
		return &model.TrapEntry{
			SourceIP: "198.51.100.10", TrapOID: testTrapOID,
			Varbinds: []model.VarbindValue{{Name: "snmpTrapCommunity", OID: model.SNMPTrapCommunityOID, Type: "OctetString", Value: value}},
		}
	}
	_, decision := d.Admit(entry("private"), []string{"snmpTrapCommunity"})
	assert.Equal(t, DecisionAdmit, decision)
	_, decision = d.Admit(entry("public"), []string{"snmpTrapCommunity"})
	assert.Equal(t, DecisionSuppress, decision)
}

func TestMissingKeyVarbindDiffersFromEmptyString(t *testing.T) {
	d := newTestDeduper(t, Config{Enabled: true}, Options{})
	empty := &model.TrapEntry{
		SourceIP: "198.51.100.10", TrapOID: testTrapOID,
		Varbinds: []model.VarbindValue{{Name: "ifAlias", OID: "1.3.6.1.2.1.31.1.1.1.18.1", Type: "OctetString", Value: ""}},
	}
	_, decision := d.Admit(empty, []string{"ifAlias"})
	assert.Equal(t, DecisionAdmit, decision)
	_, decision = d.Admit(&model.TrapEntry{SourceIP: "198.51.100.10", TrapOID: testTrapOID}, []string{"ifAlias"})
	assert.Equal(t, DecisionAdmit, decision)
	literal := &model.TrapEntry{
		SourceIP: "198.51.100.10", TrapOID: testTrapOID,
		Varbinds: []model.VarbindValue{{
			Name: "ifAlias", OID: "1.3.6.1.2.1.31.1.1.1.18.1", Type: "OctetString", Value: "<missing>",
		}},
	}
	_, decision = d.Admit(literal, []string{"ifAlias"})
	assert.Equal(t, DecisionAdmit, decision)
	_, decision = d.Admit(literal, []string{"ifAlias"})
	assert.Equal(t, DecisionSuppress, decision)
}

func TestFingerprintDistinguishesStringFromBytes(t *testing.T) {
	base := model.TrapEntry{SourceIP: "198.51.100.10", TrapOID: testTrapOID}
	stringEntry := base
	stringEntry.Varbinds = []model.VarbindValue{{Name: "payload", OID: "1.3.6.1.4.1.32473.1.1", Type: "OctetString", Value: "6162"}}
	bytesEntry := base
	bytesEntry.Varbinds = []model.VarbindValue{{Name: "payload", OID: "1.3.6.1.4.1.32473.1.1", Type: "OctetString", Value: []byte{0x61, 0x62}}}
	assert.NotEqual(t, fingerprint(&stringEntry, []string{"payload"}), fingerprint(&bytesEntry, []string{"payload"}))
}

func TestExpiryAndCapacityPreserveInsertionOldestBehavior(t *testing.T) {
	now := time.Unix(10, 0)
	d := newTestDeduper(t, Config{Enabled: true, WindowSec: 1, CacheMaxEntries: 2}, Options{Now: func() time.Time { return now }})
	for _, source := range []string{"198.51.100.10", "198.51.100.11", "198.51.100.12"} {
		_, decision := d.Admit(&model.TrapEntry{SourceIP: source, TrapOID: testTrapOID}, nil)
		assert.Equal(t, DecisionAdmit, decision)
	}
	require.Len(t, d.entries, 2)
	_, decision := d.Admit(&model.TrapEntry{SourceIP: "198.51.100.10", TrapOID: testTrapOID}, nil)
	assert.Equal(t, DecisionAdmit, decision)

	now = now.Add(time.Second)
	_, decision = d.Admit(&model.TrapEntry{SourceIP: "198.51.100.12", TrapOID: testTrapOID}, nil)
	assert.Equal(t, DecisionAdmit, decision)
}

func TestSummaryUsesLiveNameResolverAndRunsCallbackOutsideLock(t *testing.T) {
	now := time.Unix(10, 0)
	name := "OLD-MIB::linkDown"
	var got Summary
	var d *Deduper
	d = newTestDeduper(t, Config{Enabled: true}, Options{
		Now:          func() time.Time { return now },
		MonotonicNow: func() int64 { return 123 },
		ResolveName:  func(string) string { return name },
		OnSummary: func(summary Summary) {
			got = summary
			// This would deadlock if the callback ran under the state mutex.
			d.Admit(&model.TrapEntry{SourceIP: "198.51.100.20", TrapOID: testTrapOID}, nil)
		},
	})
	entry := &model.TrapEntry{SourceIP: "198.51.100.10", TrapOID: testTrapOID}
	d.Admit(entry, nil)
	d.Admit(entry, nil)
	d.Admit(entry, nil)
	name = "NEW-MIB::linkDown"
	d.emitSummary(now)

	assert.Equal(t, now.UnixMicro(), got.ReceivedRealtimeUsec)
	assert.Equal(t, int64(123), got.ReceivedMonotonicUsec)
	require.NotNil(t, got.Counts)
	assert.Equal(t, int64(2), got.Counts.TotalSuppressed)
	assert.Equal(t, int64(1), got.Counts.Fingerprints)
	assert.Equal(t, int64(5), got.Counts.PeriodSec)
	assert.Equal(t, map[string]int64{testTrapOID: 2}, got.Counts.ByTrap)
	assert.Contains(t, got.Message, "NEW-MIB::linkDown 2")
}

func TestSummaryMessageSortsCountsAndFallsBackToOID(t *testing.T) {
	now := time.Unix(10, 0)
	var got Summary
	d := newTestDeduper(t, Config{Enabled: true}, Options{
		Now: func() time.Time { return now },
		ResolveName: func(oid string) string {
			if oid == testTrapOID {
				return "SNMPv2-MIB::linkDown"
			}
			return oid
		},
		OnSummary: func(summary Summary) { got = summary },
	})
	linkDown := &model.TrapEntry{SourceIP: "198.51.100.10", TrapOID: testTrapOID}
	authFailure := &model.TrapEntry{SourceIP: "198.51.100.11", TrapOID: "1.3.6.1.6.3.1.1.5.5"}
	d.Admit(linkDown, nil)
	d.Admit(linkDown, nil)
	d.Admit(linkDown, nil)
	d.Admit(authFailure, nil)
	d.Admit(authFailure, nil)
	d.emitSummary(now)

	assert.Equal(t,
		"DEDUPLICATED TRAPS: 3 events have been deduplicated in the last 5s:\n"+
			"- SNMPv2-MIB::linkDown 2\n"+
			"- 1.3.6.1.6.3.1.1.5.5 1",
		got.Message,
	)
}

func TestCloseWaitsForFinalSummaryCallback(t *testing.T) {
	callbackStarted := make(chan struct{})
	releaseCallback := make(chan struct{})
	d := newTestDeduper(t, Config{Enabled: true}, Options{OnSummary: func(Summary) {
		close(callbackStarted)
		<-releaseCallback
	}})
	d.Start()
	entry := &model.TrapEntry{SourceIP: "198.51.100.10", TrapOID: testTrapOID}
	d.Admit(entry, nil)
	d.Admit(entry, nil)

	closed := make(chan struct{})
	go func() {
		d.Close()
		close(closed)
	}()
	<-callbackStarted
	select {
	case <-closed:
		t.Fatal("Close returned before the final callback completed")
	default:
	}
	close(releaseCallback)
	<-closed
	select {
	case <-d.doneCh:
	default:
		t.Fatal("Close returned before the timer goroutine stopped")
	}
}

func TestConcurrentCloseBeforeStartWaitsForFinalSummaryCallback(t *testing.T) {
	callbackStarted := make(chan struct{})
	releaseCallback := make(chan struct{})
	d := newTestDeduper(t, Config{Enabled: true}, Options{OnSummary: func(Summary) {
		close(callbackStarted)
		<-releaseCallback
	}})
	entry := &model.TrapEntry{SourceIP: "198.51.100.10", TrapOID: testTrapOID}
	d.Admit(entry, nil)
	d.Admit(entry, nil)

	firstClosed := make(chan struct{})
	go func() {
		d.Close()
		close(firstClosed)
	}()
	<-callbackStarted

	secondClosed := make(chan struct{})
	go func() {
		d.Close()
		close(secondClosed)
	}()
	secondReturnedEarly := false
	select {
	case <-secondClosed:
		secondReturnedEarly = true
	case <-time.After(100 * time.Millisecond):
	}

	close(releaseCallback)
	select {
	case <-firstClosed:
	case <-time.After(time.Second):
		t.Fatal("first Close did not return after final callback completed")
	}
	select {
	case <-secondClosed:
	case <-time.After(time.Second):
		t.Fatal("second Close did not return after final callback completed")
	}
	if secondReturnedEarly {
		t.Fatal("concurrent Close returned before the final callback completed")
	}
	select {
	case <-d.doneCh:
	default:
		t.Fatal("Close returned before finalization was signaled complete")
	}
}

func TestCloseBeforeStartSignalsCompletionWhenFinalCallbackPanics(t *testing.T) {
	d := newTestDeduper(t, Config{Enabled: true}, Options{OnSummary: func(Summary) {
		panic("summary callback panic")
	}})
	entry := &model.TrapEntry{SourceIP: "198.51.100.10", TrapOID: testTrapOID}
	d.Admit(entry, nil)
	d.Admit(entry, nil)

	func() {
		defer func() { assert.Equal(t, "summary callback panic", recover()) }()
		d.Close()
	}()
	select {
	case <-d.doneCh:
	default:
		t.Fatal("panicking final callback did not signal finalization complete")
	}
}

func TestPeriodicSummary(t *testing.T) {
	summaries := make(chan Summary, 1)
	policy, err := Normalize(Config{Enabled: true})
	require.NoError(t, err)
	policy.window = 10 * time.Millisecond
	d := New(policy, Options{OnSummary: func(summary Summary) { summaries <- summary }})
	d.Start()
	defer d.Close()
	entry := &model.TrapEntry{SourceIP: "198.51.100.10", TrapOID: testTrapOID}
	d.Admit(entry, nil)
	d.Admit(entry, nil)

	select {
	case summary := <-summaries:
		assert.Equal(t, int64(1), summary.Counts.TotalSuppressed)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for periodic summary")
	}
}

func TestConcurrentAdmissionsAndSummary(t *testing.T) {
	d := newTestDeduper(t, Config{Enabled: true}, Options{OnSummary: func(Summary) {}})
	entry := &model.TrapEntry{SourceIP: "198.51.100.10", TrapOID: testTrapOID}
	d.Admit(entry, nil)

	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			for range 100 {
				d.Admit(entry, nil)
			}
		})
	}
	for range 10 {
		d.emitSummary(time.Now())
	}
	wg.Wait()
}

func newTestDeduper(t testing.TB, cfg Config, opts Options) *Deduper {
	t.Helper()
	policy, err := Normalize(cfg)
	require.NoError(t, err)
	d := New(policy, opts)
	require.NotNil(t, d)
	return d
}

func testEntry(sourceIP, ifIndex string) *model.TrapEntry {
	return &model.TrapEntry{
		SourceIP: sourceIP,
		TrapOID:  testTrapOID,
		Varbinds: []model.VarbindValue{{
			Name: "ifIndex", OID: "1.3.6.1.2.1.2.2.1.1.1", Type: "Integer", Value: ifIndex,
		}},
	}
}

func testColumnEntry(oid, ifIndex string) *model.TrapEntry {
	return &model.TrapEntry{
		SourceIP: "198.51.100.10",
		TrapOID:  testTrapOID,
		Varbinds: []model.VarbindValue{{OID: oid, Type: "Integer", Value: ifIndex}},
	}
}

func TestSortedSummaryItems(t *testing.T) {
	items := sortedSummaryItems(map[string]int64{"1.3.6.2": 1, "1.3.6.1": 1, "1.3.6.3": 2})
	var got []string
	for _, item := range items {
		got = append(got, item.oid)
	}
	assert.Equal(t, []string{"1.3.6.3", "1.3.6.1", "1.3.6.2"}, got)
}
