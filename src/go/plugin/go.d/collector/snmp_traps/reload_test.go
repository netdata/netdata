// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReloadProfileCacheSuccess(t *testing.T) {
	dir := t.TempDir()
	writeProfileYAML(t, dir, "base.yaml", `
traps:
  - oid: 1.3.6.1.6.3.1.1.5.3
    name: IF-MIB::linkDown
    category: state_change
    severity: warning
`)

	setTestDirs(t, dir)
	resetProfileCacheForTest()

	idx, err := AcquireProfileCache()
	require.NoError(t, err)
	defer ReleaseProfileCache()
	assert.NotNil(t, idx.Lookup("1.3.6.1.6.3.1.1.5.3"))
	assert.Nil(t, idx.Lookup("1.3.6.1.6.3.1.1.5.4"))

	writeProfileYAML(t, dir, "new.yaml", `
traps:
  - oid: 1.3.6.1.6.3.1.1.5.4
    name: IF-MIB::linkUp
    category: state_change
    severity: notice
`)

	err = ReloadProfileCache()
	require.NoError(t, err)

	newIdx := CurrentProfileIndex()
	require.NotNil(t, newIdx)
	assert.NotNil(t, newIdx.Lookup("1.3.6.1.6.3.1.1.5.3"))
	assert.NotNil(t, newIdx.Lookup("1.3.6.1.6.3.1.1.5.4"))
	assert.Nil(t, idx.Lookup("1.3.6.1.6.3.1.1.5.4"))
}

func TestReloadProfileCacheBrokenYAMLRetainsPrevious(t *testing.T) {
	dir := t.TempDir()
	writeProfileYAML(t, dir, "good.yaml", `
traps:
  - oid: 1.3.6.1.6.3.1.1.5.3
    name: IF-MIB::linkDown
    category: state_change
    severity: warning
`)

	setTestDirs(t, dir)
	resetProfileCacheForTest()

	idx, err := AcquireProfileCache()
	require.NoError(t, err)
	defer ReleaseProfileCache()

	writeProfileYAML(t, dir, "bad.yaml", `
traps:
  - oid: 1.3.6.1.6.3.1.1.5.4
    name: IF-MIB::linkUp
    category: invalid_category
    severity: notice
`)

	err = ReloadProfileCache()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid category")

	currentIdx := CurrentProfileIndex()
	assert.Same(t, idx, currentIdx, "previous index should be retained on reload failure")
	assert.NotNil(t, currentIdx.Lookup("1.3.6.1.6.3.1.1.5.3"))
	assert.Nil(t, currentIdx.Lookup("1.3.6.1.6.3.1.1.5.4"))
}

func TestReloadProfileCacheDuplicateOIDRetainsPrevious(t *testing.T) {
	dir := t.TempDir()
	writeProfileYAML(t, dir, "one.yaml", `
traps:
  - oid: 1.3.6.1.6.3.1.1.5.3
    name: IF-MIB::linkDown
    category: state_change
    severity: warning
`)

	setTestDirs(t, dir)
	resetProfileCacheForTest()

	idx, err := AcquireProfileCache()
	require.NoError(t, err)
	defer ReleaseProfileCache()

	writeProfileYAML(t, dir, "two.yaml", `
traps:
  - oid: 1.3.6.1.6.3.1.1.5.3
    name: IF-MIB::linkDownDup
    category: security
    severity: warning
`)

	err = ReloadProfileCache()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate trap OID")
	assert.Same(t, idx, CurrentProfileIndex())
}

func TestReloadProfileCacheEmptyDirectoryFails(t *testing.T) {
	dir := t.TempDir()
	writeProfileYAML(t, dir, "good.yaml", `
traps:
  - oid: 1.3.6.1.6.3.1.1.5.3
    name: IF-MIB::linkDown
    category: state_change
    severity: warning
`)

	setTestDirs(t, dir)
	resetProfileCacheForTest()

	idx, err := AcquireProfileCache()
	require.NoError(t, err)
	defer ReleaseProfileCache()

	emptyDir := t.TempDir()
	setTestDirs(t, emptyDir)

	err = ReloadProfileCache()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no trap profiles found")
	assert.Same(t, idx, CurrentProfileIndex())
}

func TestReloadProfileCacheNewOIDsDefaultUnknown(t *testing.T) {
	dir := t.TempDir()
	writeProfileYAML(t, dir, "base.yaml", `
traps:
  - oid: 1.3.6.1.6.3.1.1.5.3
    name: IF-MIB::linkDown
    category: state_change
    severity: warning
`)

	setTestDirs(t, dir)
	resetProfileCacheForTest()

	_, err := AcquireProfileCache()
	require.NoError(t, err)
	defer ReleaseProfileCache()

	writeProfileYAML(t, dir, "new.yaml", `
traps:
  - oid: 1.3.6.1.6.3.1.1.5.4
    name: IF-MIB::linkUp
    category: unknown
    severity: notice
`)

	err = ReloadProfileCache()
	require.NoError(t, err)

	newIdx := CurrentProfileIndex()
	td := newIdx.Lookup("1.3.6.1.6.3.1.1.5.4")
	require.NotNil(t, td)
	assert.Equal(t, "unknown", td.Category)
}

func TestReloadProfileCacheIncrementsErrorMetricForActiveJobs(t *testing.T) {
	dir := t.TempDir()
	writeProfileYAML(t, dir, "good.yaml", `
traps:
  - oid: 1.3.6.1.6.3.1.1.5.3
    name: IF-MIB::linkDown
    category: state_change
    severity: warning
`)

	setTestDirs(t, dir)
	resetProfileCacheForTest()

	const jobA = "test-reload-errors-a"
	const jobB = "test-reload-errors-b"
	removeJobMetrics(jobA)
	removeJobMetrics(jobB)
	mA := getJobMetrics(jobA)
	mB := getJobMetrics(jobB)
	defer removeJobMetrics(jobA)
	defer removeJobMetrics(jobB)

	_, err := AcquireProfileCache()
	require.NoError(t, err)
	defer ReleaseProfileCache()

	writeProfileYAML(t, dir, "bad.yaml", `
traps:
  - oid: 1.3.6.1.6.3.1.1.5.4
    name: IF-MIB::linkUp
    category: bad_cat
    severity: notice
`)

	err = ReloadProfileCache()
	require.Error(t, err)

	assert.Equal(t, uint64(1), atomic.LoadUint64(&mA.errors.profileLoadFailed))
	assert.Equal(t, uint64(1), atomic.LoadUint64(&mB.errors.profileLoadFailed))
}

func TestReloadProfileCacheConcurrentLookups(t *testing.T) {
	dir := t.TempDir()
	writeProfileYAML(t, dir, "base.yaml", `
traps:
  - oid: 1.3.6.1.6.3.1.1.5.3
    name: IF-MIB::linkDown
    category: state_change
    severity: warning
`)

	setTestDirs(t, dir)
	resetProfileCacheForTest()

	_, err := AcquireProfileCache()
	require.NoError(t, err)
	defer ReleaseProfileCache()

	done := make(chan struct{})
	var hits int64
	go func() {
		for range 1000 {
			idx := CurrentProfileIndex()
			if idx != nil {
				if idx.Lookup("1.3.6.1.6.3.1.1.5.3") != nil {
					atomic.AddInt64(&hits, 1)
				}
			}
		}
		close(done)
	}()

	for range 100 {
		require.NoError(t, ReloadProfileCache())
	}

	<-done
	assert.Greater(t, atomic.LoadInt64(&hits), int64(0))
}

func TestCurrentProfileIndexReturnsNilBeforeLoad(t *testing.T) {
	setMinimalProfileDir(t)
	resetProfileCacheForTest()

	assert.Nil(t, CurrentProfileIndex())
}

func TestReloadProfileCacheWithoutActiveJobDoesNotLoad(t *testing.T) {
	dir := t.TempDir()
	writeProfileYAML(t, dir, "test.yaml", `
traps:
  - oid: 1.3.6.1.6.3.1.1.5.3
    name: IF-MIB::linkDown
    category: state_change
    severity: warning
`)

	setTestDirs(t, dir)
	resetProfileCacheForTest()

	err := ReloadProfileCache()
	require.ErrorIs(t, err, errNoActiveProfileJobs)
	assert.Nil(t, CurrentProfileIndex())
}

func TestReloadProfileCacheDoesNotStoreWhenLastJobExitsDuringLoad(t *testing.T) {
	dir := t.TempDir()
	writeProfileYAML(t, dir, "base.yaml", `
traps:
  - oid: 1.3.6.1.6.3.1.1.5.3
    name: IF-MIB::linkDown
    category: state_change
    severity: warning
`)

	setTestDirs(t, dir)
	resetProfileCacheForTest()
	_, err := AcquireProfileCache()
	require.NoError(t, err)

	started := make(chan struct{})
	releaseDone := make(chan struct{})
	reloadDone := make(chan error)
	go func() {
		reloadDone <- reloadProfileCache(func() (*ProfileIndex, error) {
			close(started)
			<-releaseDone
			return &ProfileIndex{trapsByOID: map[string]*TrapDef{
				"1.3.6.1.6.3.1.1.5.4": {
					OID:      "1.3.6.1.6.3.1.1.5.4",
					Name:     "IF-MIB::linkUp",
					Category: "state_change",
					Severity: "notice",
				},
			}}, nil
		})
	}()

	<-started
	ReleaseProfileCache()
	close(releaseDone)

	require.ErrorIs(t, <-reloadDone, errNoActiveProfileJobs)
	assert.Nil(t, CurrentProfileIndex())
}

func TestReloadProfileCacheDoesNotUpdateFailureMetricsWhenLastJobExitsDuringLoad(t *testing.T) {
	dir := t.TempDir()
	writeProfileYAML(t, dir, "base.yaml", `
traps:
  - oid: 1.3.6.1.6.3.1.1.5.3
    name: IF-MIB::linkDown
    category: state_change
    severity: warning
`)

	setTestDirs(t, dir)
	resetProfileCacheForTest()
	_, err := AcquireProfileCache()
	require.NoError(t, err)

	const jobName = "test-reload-error-after-release"
	removeJobMetrics(jobName)
	m := getJobMetrics(jobName)
	defer removeJobMetrics(jobName)

	started := make(chan struct{})
	releaseDone := make(chan struct{})
	reloadDone := make(chan error)
	go func() {
		reloadDone <- reloadProfileCache(func() (*ProfileIndex, error) {
			close(started)
			<-releaseDone
			return nil, assert.AnError
		})
	}()

	<-started
	ReleaseProfileCache()
	close(releaseDone)

	require.ErrorIs(t, <-reloadDone, errNoActiveProfileJobs)
	assert.Equal(t, uint64(0), atomic.LoadUint64(&m.errors.profileLoadFailed))
	assert.Nil(t, CurrentProfileIndex())
}

func TestCollectorHandlePacketUsesReloadedProfileIndex(t *testing.T) {
	packets := readPcapUDPPackets(t, "testdata/v2c_coldstart.pcap.hex")
	require.Len(t, packets, 1)

	dir := t.TempDir()
	writeProfileYAML(t, dir, "coldstart.yaml", `
traps:
  - oid: 1.3.6.1.6.3.1.1.5.1
    name: SNMPv2-MIB::coldStart
    category: state_change
    severity: warning
    description: old profile
`)

	setTestDirs(t, dir)
	resetProfileCacheForTest()
	_, err := AcquireProfileCache()
	require.NoError(t, err)
	defer ReleaseProfileCache()

	writer := &mockTrapWriter{}
	c := &Collector{
		jobName:    "reload-packet",
		trapWriter: writer,
		versions:   map[SnmpVersion]struct{}{SnmpVersionV2c: {}},
		allowlist:  NewAllowlist(nil, []string{"public"}),
	}

	c.handlePacket(packets[0].payload, packets[0].peer, nil, nil)
	require.Len(t, writer.entries, 1)
	assert.Equal(t, Category("state_change"), writer.entries[0].Category)
	assert.Equal(t, "old profile", writer.entries[0].Message)

	writeProfileYAML(t, dir, "coldstart.yaml", `
traps:
  - oid: 1.3.6.1.6.3.1.1.5.1
    name: SNMPv2-MIB::coldStart
    category: security
    severity: crit
    description: new profile
`)
	require.NoError(t, ReloadProfileCache())

	c.handlePacket(packets[0].payload, packets[0].peer, nil, nil)
	require.Len(t, writer.entries, 2)
	assert.Equal(t, Category("security"), writer.entries[1].Category)
	assert.Equal(t, "new profile", writer.entries[1].Message)
}

func TestTrapDeduperSummaryUsesReloadedProfileIndex(t *testing.T) {
	dir := t.TempDir()
	writeProfileYAML(t, dir, "linkdown.yaml", `
traps:
  - oid: 1.3.6.1.6.3.1.1.5.3
    name: OLD-MIB::linkDown
    category: state_change
    severity: warning
`)

	setTestDirs(t, dir)
	resetProfileCacheForTest()
	_, err := AcquireProfileCache()
	require.NoError(t, err)
	defer ReleaseProfileCache()

	writer := &mockTrapWriter{}
	d := newTrapDeduper("local", DedupConfig{Enabled: true}, writer, nil)

	entry := &TrapEntry{SourceIP: "198.51.100.10", TrapOID: "1.3.6.1.6.3.1.1.5.3"}
	_, suppressed := d.Admit(entry, nil, nil)
	require.False(t, suppressed)
	_, suppressed = d.Admit(entry, nil, nil)
	require.True(t, suppressed)

	writeProfileYAML(t, dir, "linkdown.yaml", `
traps:
  - oid: 1.3.6.1.6.3.1.1.5.3
    name: NEW-MIB::linkDown
    category: state_change
    severity: warning
`)
	require.NoError(t, ReloadProfileCache())

	d.emitSummary(time.Now())
	require.Len(t, writer.entries, 1)
	assert.Contains(t, writer.entries[0].Message, "NEW-MIB::linkDown")
	assert.NotContains(t, writer.entries[0].Message, "OLD-MIB::linkDown")
}

func TestProfileReloadHandlerSuccess(t *testing.T) {
	dir := t.TempDir()
	writeProfileYAML(t, dir, "test.yaml", `
traps:
  - oid: 1.3.6.1.6.3.1.1.5.3
    name: IF-MIB::linkDown
    category: state_change
    severity: warning
`)

	setTestDirs(t, dir)
	resetProfileCacheForTest()
	_, err := AcquireProfileCache()
	require.NoError(t, err)
	defer ReleaseProfileCache()

	handler := &profileReloadHandler{}
	resp := handler.Handle(t.Context(), reloadProfilesMethodID, nil)
	require.NotNil(t, resp)
	assert.Equal(t, 200, resp.Status)
	assert.Equal(t, "profile reload successful", resp.Message)
}

func TestProfileReloadHandlerFailure(t *testing.T) {
	dir := t.TempDir()
	writeProfileYAML(t, dir, "test.yaml", `
traps:
  - oid: 1.3.6.1.6.3.1.1.5.3
    name: IF-MIB::linkDown
    category: state_change
    severity: warning
`)

	setTestDirs(t, dir)
	resetProfileCacheForTest()
	_, err := AcquireProfileCache()
	require.NoError(t, err)
	defer ReleaseProfileCache()

	emptyDir := t.TempDir()
	setTestDirs(t, emptyDir)

	handler := &profileReloadHandler{}
	resp := handler.Handle(t.Context(), reloadProfilesMethodID, nil)
	require.NotNil(t, resp)
	assert.Equal(t, 422, resp.Status)
	assert.Contains(t, resp.Message, "no trap profiles found")
}

func TestProfileReloadHandlerUnavailableWithoutActiveJob(t *testing.T) {
	setMinimalProfileDir(t)
	resetProfileCacheForTest()

	handler := &profileReloadHandler{}
	resp := handler.Handle(t.Context(), reloadProfilesMethodID, nil)
	require.NotNil(t, resp)
	assert.Equal(t, 503, resp.Status)
	assert.Contains(t, resp.Message, "requires at least one active job")
}

func TestProfileReloadHandlerUnknownMethod(t *testing.T) {
	handler := &profileReloadHandler{}
	resp := handler.Handle(t.Context(), "unknown-method", nil)
	require.NotNil(t, resp)
	assert.Equal(t, 404, resp.Status)
	assert.Contains(t, resp.Message, "unknown method")
}

func TestProfileReloadHandlerMethodParams(t *testing.T) {
	handler := &profileReloadHandler{}

	params, err := handler.MethodParams(t.Context(), reloadProfilesMethodID)
	require.NoError(t, err)
	assert.Nil(t, params)

	params, err = handler.MethodParams(t.Context(), "other")
	require.NoError(t, err)
	assert.Nil(t, params)
}

func TestSnmpTrapsMethods(t *testing.T) {
	methods := snmpTrapsMethods()
	require.Len(t, methods, 1)
	assert.Equal(t, reloadProfilesMethodID, methods[0].ID)
	assert.True(t, methods[0].AgentWide)
	assert.Equal(t, "text", methods[0].ResponseType)
}
