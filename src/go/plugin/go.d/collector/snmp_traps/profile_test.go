// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"bufio"
	"compress/gzip"
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/netdata/netdata/go/plugins/pkg/multipath"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// Profile cache lifecycle tests
// =============================================================================

func TestProfileCacheLazyLoad(t *testing.T) {
	setMinimalProfileDir(t)

	idx, err := AcquireProfileCache()
	require.NoError(t, err)
	assert.NotNil(t, idx)

	td := idx.Lookup("1.3.6.1.6.3.1.1.5.1")
	if td != nil {
		assert.Equal(t, "state_change", td.Category)
	}

	ReleaseProfileCache()
	assert.Nil(t, CurrentProfileIndex(), "profile cache should unload after last release")

	idx2, err := AcquireProfileCache()
	require.NoError(t, err)
	assert.NotNil(t, idx2)
	assert.NotNil(t, CurrentProfileIndex())
	ReleaseProfileCache()
	assert.Nil(t, CurrentProfileIndex())
}

func TestProfileCacheSharedAcrossCollectors(t *testing.T) {
	setMinimalProfileDir(t)

	idx1, err1 := AcquireProfileCache()
	require.NoError(t, err1)

	idx2, err2 := AcquireProfileCache()
	require.NoError(t, err2)

	assert.Same(t, idx1, idx2, "second acquire should return the same index")

	ReleaseProfileCache()
	assert.NotNil(t, CurrentProfileIndex(), "cache should stay loaded while one reference remains")

	idx3, err3 := AcquireProfileCache()
	require.NoError(t, err3)
	assert.Same(t, idx1, idx3)

	ReleaseProfileCache()
	assert.NotNil(t, CurrentProfileIndex(), "cache should stay loaded while one reference remains")
	ReleaseProfileCache()
	assert.Nil(t, CurrentProfileIndex(), "cache should unload after last reference")
}

func TestProfileCacheReleaseIdempotent(t *testing.T) {
	setMinimalProfileDir(t)

	_, err := AcquireProfileCache()
	require.NoError(t, err)

	ReleaseProfileCache()
	ReleaseProfileCache()
	ReleaseProfileCache()
	assert.Nil(t, CurrentProfileIndex())
}

func TestCollectorInitAcquiresProfileCache(t *testing.T) {
	setMinimalProfileDir(t)
	withTestCacheDir(t)

	port := freeUDPPort(t)

	c := New()
	c.SetJobName("local")
	c.Listen.Endpoints = []EndpointConfig{{Protocol: "udp", Address: "127.0.0.1", Port: port}}

	err := c.Init(context.Background())
	require.NoError(t, err)
	assert.NotNil(t, CurrentProfileIndex())

	c.Cleanup(context.Background())
	assert.Nil(t, CurrentProfileIndex())
}

func TestMultipleCollectorsShareSameCache(t *testing.T) {
	setMinimalProfileDir(t)
	withTestCacheDir(t)

	port1 := freeUDPPort(t)
	port2 := freeUDPPort(t)

	c1 := New()
	c1.SetJobName("job1")
	c1.Listen.Endpoints = []EndpointConfig{{Protocol: "udp", Address: "127.0.0.1", Port: port1}}

	c2 := New()
	c2.SetJobName("job2")
	c2.Listen.Endpoints = []EndpointConfig{{Protocol: "udp", Address: "127.0.0.1", Port: port2}}

	err := c1.Init(context.Background())
	require.NoError(t, err)

	err = c2.Init(context.Background())
	require.NoError(t, err)

	sharedIndex := CurrentProfileIndex()
	require.NotNil(t, sharedIndex)

	c2.Cleanup(context.Background())
	assert.Same(t, sharedIndex, CurrentProfileIndex(), "cache should still use the shared index after one collector cleans up")
	assert.NotNil(t, CurrentProfileIndex(), "cache should still be alive after second collector cleans up")

	c1.Cleanup(context.Background())
	assert.Nil(t, CurrentProfileIndex(), "cache should unload after all collectors clean up")
}

func TestInitBindFailureReleasesProfileRef(t *testing.T) {
	setMinimalProfileDir(t)
	withTestCacheDir(t)

	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	require.NoError(t, err)
	defer conn.Close()

	c := New()
	c.SetJobName("local")
	c.Listen.Endpoints = []EndpointConfig{{Protocol: "udp", Address: "127.0.0.1", Port: conn.LocalAddr().(*net.UDPAddr).Port}}

	err = c.Init(context.Background())
	require.Error(t, err)
	assert.Nil(t, CurrentProfileIndex(), "profile ref should be released on bind failure")

	// Verify cache can still be acquired after bind failure
	idx, err := AcquireProfileCache()
	require.NoError(t, err)
	ReleaseProfileCache()
	assert.Nil(t, CurrentProfileIndex())
	_ = idx
}

// =============================================================================
// Profile loading tests (using temp dir overrides)
// =============================================================================

func TestProfileDirPathBuilders(t *testing.T) {
	assert.Equal(t, "/etc/netdata/go.d/snmp.trap-profiles", trapProfilesUserDir("/etc/netdata/go.d"))
	assert.Equal(t, "/usr/lib/netdata/conf.d/go.d/snmp.trap-profiles/default", trapProfilesStockDir("/usr/lib/netdata/conf.d/go.d"))
}

func setTestDirs(t *testing.T, dirs ...string) {
	t.Helper()
	testDirsOverride = dirs
	t.Cleanup(func() {
		testDirsOverride = nil
	})
}

func setMinimalProfileDir(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	writeProfileYAML(t, dir, "minimal.yaml", `
traps:
  - oid: 1.3.6.1.6.3.1.1.5.1
    name: SNMPv2-MIB::coldStart
    category: state_change
    severity: notice
`)
	setTestDirs(t, dir)
	resetProfileCacheForTest()
}

func TestProfileLoadValid(t *testing.T) {
	dir := t.TempDir()
	writeProfileYAML(t, dir, "test.yaml", `
varbinds:
  ifIndex:
    oid: 1.3.6.1.2.1.2.2.1.1
    type: INTEGER
  ifDescr:
    oid: 1.3.6.1.2.1.31.1.1.1.1
    type: OctetString

traps:
  - oid: 1.3.6.1.6.3.1.1.5.3
    name: IF-MIB::linkDown
    category: state_change
    severity: warning
    description: "Interface {ifDescr} down"
    varbinds: [ifIndex, ifDescr]
`)

	setTestDirs(t, dir)
	resetProfileCacheForTest()

	idx, err := AcquireProfileCache()
	require.NoError(t, err)
	defer ReleaseProfileCache()

	td := idx.Lookup("1.3.6.1.6.3.1.1.5.3")
	require.NotNil(t, td)
	assert.Equal(t, "IF-MIB::linkDown", td.Name)
	assert.Equal(t, "state_change", td.Category)
	assert.Equal(t, "warning", td.Severity)
	assert.NotNil(t, td.sharedVarbinds)
}

func TestProfileLoadGzipYAML(t *testing.T) {
	dir := t.TempDir()
	writeProfileYAMLGzip(t, dir, "test.yaml.gz", `
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

	td := idx.Lookup("1.3.6.1.6.3.1.1.5.3")
	require.NotNil(t, td)
	assert.Equal(t, "IF-MIB::linkDown", td.Name)
}

func TestStockProfileStoreLazyLoadsRoutedGzip(t *testing.T) {
	stockDir := t.TempDir()
	writeProfileYAMLGzip(t, stockDir, "ciscosystems.yaml.gz", `
traps:
  - oid: 1.3.6.1.4.1.9.1.0.1
    name: TEST-CISCO-MIB::testTrap
    category: diagnostic
    severity: warning
`)

	idx := &ProfileIndex{
		trapsByOID:      make(map[string]*TrapDef),
		namesByTrapName: make(map[string]*TrapDef),
	}
	store, err := buildStockProfileStore(stockDir, multipath.New(stockDir), nil, idx)
	require.NoError(t, err)
	idx.stock = store

	assert.Empty(t, idx.trapsByOID)
	td := idx.Lookup("1.3.6.1.4.1.9.1.0.1")
	require.NotNil(t, td)
	assert.Equal(t, "TEST-CISCO-MIB::testTrap", td.Name)
	assert.True(t, store.loaded["ciscosystems.yaml"])
	assert.Len(t, idx.trapsByOID, 1)
}

func TestReadMaybeGzipFileRejectsOversizedDecompressedProfile(t *testing.T) {
	dir := t.TempDir()
	writeProfileYAMLGzip(t, dir, "oversized.yaml.gz", strings.Repeat("x", 32))

	oldLimit := maxProfileFileBytes
	maxProfileFileBytes = 16
	t.Cleanup(func() { maxProfileFileBytes = oldLimit })

	_, err := readMaybeGzipFile(filepath.Join(dir, "oversized.yaml.gz"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds maximum decompressed size")
}

func TestStockProfileStoreUsesCatalogueWithoutParsingProfiles(t *testing.T) {
	rootDir := t.TempDir()
	stockDir := filepath.Join(rootDir, "default")
	require.NoError(t, os.MkdirAll(stockDir, 0o755))
	writeProfileYAMLGzip(t, stockDir, "ciscosystems.yaml.gz", `not: [valid`)
	writeProfileYAMLGzip(t, rootDir, "catalogue.json.gz", `{
  "ciscosystems": {
    "file": "ciscosystems.yaml",
    "trap_count": 1,
    "trap_oids": ["1.3.6.1.4.1.9.1.0.1"]
  }
}`)

	idx := &ProfileIndex{
		trapsByOID:      make(map[string]*TrapDef),
		namesByTrapName: make(map[string]*TrapDef),
	}
	store, err := buildStockProfileStore(stockDir, multipath.New(stockDir), nil, idx)
	require.NoError(t, err)
	idx.stock = store

	assert.Empty(t, idx.trapsByOID)
	assert.Equal(t, "ciscosystems.yaml", store.route("1.3.6.1.4.1.9.1.0.1"))

	td, err := idx.LookupWithError("1.3.6.1.4.1.9.1.0.1")
	require.Error(t, err)
	assert.Nil(t, td)
	assert.Contains(t, err.Error(), "failed to lazy-load stock trap profile")
}

func TestOverridesApplyToLazyLoadedStockProfiles(t *testing.T) {
	stockDir := t.TempDir()
	const oid = "1.3.6.1.4.1.9.1.0.1"
	writeProfileYAMLGzip(t, stockDir, "ciscosystems.yaml.gz", `
traps:
  - oid: 1.3.6.1.4.1.9.1.0.1
    name: TEST-CISCO-MIB::testTrap
    category: diagnostic
    severity: warning
    labels:
      vendor: stock
`)

	idx := &ProfileIndex{
		trapsByOID:      make(map[string]*TrapDef),
		namesByTrapName: make(map[string]*TrapDef),
	}
	store, err := buildStockProfileStore(stockDir, multipath.New(stockDir), nil, idx)
	require.NoError(t, err)
	idx.stock = store

	td := idx.Lookup(oid)
	require.NotNil(t, td)
	require.Equal(t, "diagnostic", td.Category)
	require.Equal(t, "warning", td.Severity)

	c := New()
	c.overrides = buildOverrideMap([]OverrideConfig{
		{
			OID:      oid,
			Category: "security",
			Severity: "crit",
			Labels:   map[string]string{"site": "edge"},
		},
	})

	overridden := c.applyOverrides(td)
	require.NotSame(t, td, overridden)
	assert.Equal(t, "security", overridden.Category)
	assert.Equal(t, "crit", overridden.Severity)
	assert.Equal(t, map[string]string{"vendor": "stock", "site": "edge"}, overridden.Labels)

	assert.Equal(t, "diagnostic", td.Category)
	assert.Equal(t, "warning", td.Severity)
	assert.Equal(t, map[string]string{"vendor": "stock"}, td.Labels)
}

func TestStockProfileStoreLazyLoadErrorIsReported(t *testing.T) {
	stockDir := t.TempDir()
	writeProfileYAMLGzip(t, stockDir, "ciscosystems.yaml.gz", `
traps:
  - oid: 1.3.6.1.4.1.9.1.0.1
    name: TEST-CISCO-MIB::testTrap
    category: diagnostic
    severity: warning
`)

	idx := &ProfileIndex{
		trapsByOID:      make(map[string]*TrapDef),
		namesByTrapName: make(map[string]*TrapDef),
	}
	store, err := buildStockProfileStore(stockDir, multipath.New(stockDir), nil, idx)
	require.NoError(t, err)
	idx.stock = store
	require.NoError(t, os.Remove(filepath.Join(stockDir, "ciscosystems.yaml.gz")))

	td, err := idx.LookupWithError("1.3.6.1.4.1.9.1.0.1")
	require.Error(t, err)
	assert.Nil(t, td)
	assert.Contains(t, err.Error(), "failed to lazy-load stock trap profile")
}

func TestStockProfileStoreSkipsSameBasenameUserReplacement(t *testing.T) {
	userDir := t.TempDir()
	stockDir := t.TempDir()
	writeProfileYAML(t, userDir, "ciscosystems.yaml", `
traps:
  - oid: 1.3.6.1.4.1.9.1.0.1
    name: TEST-CISCO-MIB::userTrap
    category: security
    severity: warning
`)
	writeProfileYAMLGzip(t, stockDir, "ciscosystems.yaml.gz", `
traps:
  - oid: 1.3.6.1.4.1.9.1.0.2
    name: TEST-CISCO-MIB::stockTrap
    category: diagnostic
    severity: warning
`)

	idx := &ProfileIndex{
		trapsByOID:      make(map[string]*TrapDef),
		namesByTrapName: make(map[string]*TrapDef),
	}
	seen := map[string]bool{}
	files, err := loadProfilesFromDir(userDir, multipath.New(userDir, stockDir))
	require.NoError(t, err)
	for _, file := range files {
		seen[file.name] = true
		require.NoError(t, idx.addTraps(file.traps))
	}

	store, err := buildStockProfileStore(stockDir, multipath.New(userDir, stockDir), seen, idx)
	require.NoError(t, err)
	idx.stock = store

	assert.NotNil(t, idx.Lookup("1.3.6.1.4.1.9.1.0.1"))
	assert.Nil(t, idx.Lookup("1.3.6.1.4.1.9.1.0.2"))
	assert.Empty(t, store.files)
}

func TestProfileIndexLookupTrapOIDTolerance(t *testing.T) {
	withZero := &TrapDef{OID: "1.3.6.1.2.1.33.2.0.1", Name: "UPS-MIB::upsTrapOnBattery"}
	withoutZero := &TrapDef{OID: "1.3.6.1.4.1.14179.2.6.3.24", Name: "AIRESPACE-WIRELESS-MIB::bsnAPFunctionalityDisabled"}
	exact := &TrapDef{OID: "1.3.6.1.4.1.14179.2.6.3.0.24", Name: "AIRESPACE-WIRELESS-MIB::exactZeroForm"}
	alt := &TrapDef{OID: "1.3.6.1.4.1.14179.2.6.3.24", Name: "AIRESPACE-WIRELESS-MIB::alternateNoZeroForm"}

	tests := map[string]struct {
		lookup string
		traps  map[string]*TrapDef
		want   *TrapDef
	}{
		"decoded_with_zero_matches_profile_without_zero": {
			lookup: "1.3.6.1.4.1.14179.2.6.3.0.24",
			traps: map[string]*TrapDef{
				withoutZero.OID: withoutZero,
			},
			want: withoutZero,
		},
		"decoded_without_zero_matches_profile_with_zero": {
			lookup: "1.3.6.1.2.1.33.2.1",
			traps: map[string]*TrapDef{
				withZero.OID: withZero,
			},
			want: withZero,
		},
		"exact_match_wins_when_both_forms_exist": {
			lookup: "1.3.6.1.4.1.14179.2.6.3.0.24",
			traps: map[string]*TrapDef{
				exact.OID: exact,
				alt.OID:   alt,
			},
			want: exact,
		},
		"too_short_oid_does_not_match_alternate": {
			lookup: "1.3.6",
			traps: map[string]*TrapDef{
				"1.3.0.6": {OID: "1.3.0.6", Name: "TEST-MIB::tooShortAlternate"},
			},
		},
		"leading_dot_oid_does_not_match_alternate": {
			lookup: ".1.3.6.1.4",
			traps: map[string]*TrapDef{
				"1.3.6.1.0.4": {OID: "1.3.6.1.0.4", Name: "TEST-MIB::leadingDotAlternate"},
			},
		},
		"true_miss_returns_nil": {
			lookup: "1.3.6.1.4.1.9999.1",
			traps: map[string]*TrapDef{
				"1.3.6.1.4.1.8888.1": {OID: "1.3.6.1.4.1.8888.1", Name: "TEST-MIB::differentTrap"},
			},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			idx := &ProfileIndex{trapsByOID: tt.traps}

			got := idx.Lookup(tt.lookup)
			if tt.want == nil {
				assert.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			assert.Same(t, tt.want, got)
		})
	}
}

func TestAlternateTrapOID(t *testing.T) {
	tests := map[string]struct {
		oid  string
		want string
	}{
		"insert_zero_before_final_arc": {
			oid:  "1.3.6.1.2.1.33.2.1",
			want: "1.3.6.1.2.1.33.2.0.1",
		},
		"remove_zero_before_final_arc": {
			oid:  "1.3.6.1.4.1.14179.2.6.3.0.24",
			want: "1.3.6.1.4.1.14179.2.6.3.24",
		},
		"only_last_position_zero_is_flipped": {
			oid:  "1.3.0.6.1.0.24",
			want: "1.3.0.6.1.24",
		},
		"internal_zero_is_not_removed": {
			oid:  "1.0.3.4.5",
			want: "1.0.3.4.0.5",
		},
		"empty_oid_is_unchanged": {
			oid:  "",
			want: "",
		},
		"single_arc_oid_is_unchanged": {
			oid:  "1",
			want: "1",
		},
		"too_short_oid_is_unchanged": {
			oid:  "1.3.6",
			want: "1.3.6",
		},
		"leading_dot_oid_is_unchanged": {
			oid:  ".1.3.6.1",
			want: ".1.3.6.1",
		},
		"trailing_dot_oid_is_unchanged": {
			oid:  "1.3.6.1.",
			want: "1.3.6.1.",
		},
		"empty_segment_oid_is_unchanged": {
			oid:  "1.3..6.1",
			want: "1.3..6.1",
		},
		"non_numeric_oid_is_unchanged": {
			oid:  "1.3.6.foo.1",
			want: "1.3.6.foo.1",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tt.want, alternateTrapOID(tt.oid))
		})
	}
}

func TestProfileLoadEmptyDirFails(t *testing.T) {
	dir := t.TempDir()

	setTestDirs(t, dir)
	resetProfileCacheForTest()

	_, err := AcquireProfileCache()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no trap profiles found")
}

func TestProfileLoadMissingName(t *testing.T) {
	dir := t.TempDir()
	writeProfileYAML(t, dir, "bad.yaml", `
traps:
  - oid: 1.3.6.1.6.3.1.1.5.3
    category: state_change
    severity: warning
`)

	setTestDirs(t, dir)
	resetProfileCacheForTest()

	_, err := AcquireProfileCache()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing required field 'name'")
}

func TestProfileLoadNonMIBQualifiedName(t *testing.T) {
	dir := t.TempDir()
	writeProfileYAML(t, dir, "bad.yaml", `
traps:
  - oid: 1.3.6.1.6.3.1.1.5.3
    name: linkDown
    category: state_change
    severity: warning
`)

	setTestDirs(t, dir)
	resetProfileCacheForTest()

	_, err := AcquireProfileCache()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not MIB-qualified")
}

func TestProfileLoadInvalidCategory(t *testing.T) {
	dir := t.TempDir()
	writeProfileYAML(t, dir, "bad.yaml", `
traps:
  - oid: 1.3.6.1.6.3.1.1.5.3
    name: IF-MIB::linkDown
    category: nonexistent
    severity: warning
`)

	setTestDirs(t, dir)
	resetProfileCacheForTest()

	_, err := AcquireProfileCache()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid category")
}

func TestProfileLoadInvalidSeverity(t *testing.T) {
	dir := t.TempDir()
	writeProfileYAML(t, dir, "bad.yaml", `
traps:
  - oid: 1.3.6.1.6.3.1.1.5.3
    name: IF-MIB::linkDown
    category: state_change
    severity: bad
`)

	setTestDirs(t, dir)
	resetProfileCacheForTest()

	_, err := AcquireProfileCache()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid severity")
}

func TestProfileLoadInvalidStatus(t *testing.T) {
	dir := t.TempDir()
	writeProfileYAML(t, dir, "bad.yaml", `
traps:
  - oid: 1.3.6.1.6.3.1.1.5.3
    name: IF-MIB::linkDown
    category: state_change
    severity: warning
    status: bad
`)

	setTestDirs(t, dir)
	resetProfileCacheForTest()

	_, err := AcquireProfileCache()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid status")
}

func TestProfileLoadDanglingVarbind(t *testing.T) {
	dir := t.TempDir()
	writeProfileYAML(t, dir, "bad.yaml", `
traps:
  - oid: 1.3.6.1.6.3.1.1.5.3
    name: IF-MIB::linkDown
    category: state_change
    severity: warning
    varbinds: [nonexistent]
`)

	setTestDirs(t, dir)
	resetProfileCacheForTest()

	_, err := AcquireProfileCache()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found in file-scoped varbinds table")
}

func TestProfileLoadInvalidFileVarbind(t *testing.T) {
	dir := t.TempDir()
	writeProfileYAML(t, dir, "bad.yaml", `
varbinds:
  ifIndex:
    oid: not.an.oid
    type: INTEGER

traps:
  - oid: 1.3.6.1.6.3.1.1.5.3
    name: IF-MIB::linkDown
    category: state_change
    severity: warning
    varbinds: [ifIndex]
`)

	setTestDirs(t, dir)
	resetProfileCacheForTest()

	_, err := AcquireProfileCache()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid oid")
}

func TestProfileLoadInlineVarbind(t *testing.T) {
	dir := t.TempDir()
	writeProfileYAML(t, dir, "test.yaml", `
traps:
  - oid: 1.3.6.1.6.3.1.1.5.3
    name: IF-MIB::linkDown
    category: state_change
    severity: warning
    varbinds:
      - oid: 1.3.6.1.2.1.31.1.1.1.1
        name: ifDescr
        type: OctetString
`)

	setTestDirs(t, dir)
	resetProfileCacheForTest()

	idx, err := AcquireProfileCache()
	require.NoError(t, err)
	defer ReleaseProfileCache()

	td := idx.Lookup("1.3.6.1.6.3.1.1.5.3")
	require.NotNil(t, td)
	require.NotNil(t, td.varbindByName("ifDescr"))
}

func TestProfileLoadInvalidInlineVarbind(t *testing.T) {
	dir := t.TempDir()
	writeProfileYAML(t, dir, "bad.yaml", `
traps:
  - oid: 1.3.6.1.6.3.1.1.5.3
    name: IF-MIB::linkDown
    category: state_change
    severity: warning
    varbinds:
      - oid: 1.3.6.1.2.1.31.1.1.1.1
        name: ifDescr
`)

	setTestDirs(t, dir)
	resetProfileCacheForTest()

	_, err := AcquireProfileCache()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing required field 'type'")
}

func TestProfileLoadDanglingDedupKey(t *testing.T) {
	dir := t.TempDir()
	writeProfileYAML(t, dir, "bad.yaml", `
traps:
  - oid: 1.3.6.1.6.3.1.1.5.3
    name: IF-MIB::linkDown
    category: state_change
    severity: warning
    dedup_key_varbinds: [nonexistent]
`)

	setTestDirs(t, dir)
	resetProfileCacheForTest()

	_, err := AcquireProfileCache()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dedup_key_varbind")
}

func TestProfileLoadInvalidLabelKey(t *testing.T) {
	dir := t.TempDir()
	writeProfileYAML(t, dir, "bad.yaml", `
traps:
  - oid: 1.3.6.1.6.3.1.1.5.3
    name: IF-MIB::linkDown
    category: state_change
    severity: warning
    labels:
      "BAD_KEY": "value"
`)

	setTestDirs(t, dir)
	resetProfileCacheForTest()

	_, err := AcquireProfileCache()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "label key")
}

func TestProfileLoadDuplicateOID(t *testing.T) {
	dir := t.TempDir()
	writeProfileYAML(t, dir, "one.yaml", `
traps:
  - oid: 1.3.6.1.6.3.1.1.5.3
    name: IF-MIB::linkDown
    category: state_change
    severity: warning
`)
	writeProfileYAML(t, dir, "two.yaml", `
traps:
  - oid: 1.3.6.1.6.3.1.1.5.3
    name: IF-MIB::linkDown
    category: state_change
    severity: warning
`)

	setTestDirs(t, dir)
	resetProfileCacheForTest()

	_, err := AcquireProfileCache()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate trap OID")
}

func TestProfileLoadDuplicateName(t *testing.T) {
	dir := t.TempDir()
	writeProfileYAML(t, dir, "one.yaml", `
traps:
  - oid: 1.3.6.1.6.3.1.1.5.3
    name: IF-MIB::linkDown
    category: state_change
    severity: warning
`)
	writeProfileYAML(t, dir, "two.yaml", `
traps:
  - oid: 1.3.6.1.6.3.1.1.5.4
    name: IF-MIB::linkDown
    category: state_change
    severity: warning
`)

	setTestDirs(t, dir)
	resetProfileCacheForTest()

	_, err := AcquireProfileCache()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate trap name")
}

func TestProfileLoadFilenameDedup(t *testing.T) {
	userDir := t.TempDir()
	stockDir := t.TempDir()

	writeProfileYAML(t, userDir, "same.yaml", `
traps:
  - oid: 1.3.6.1.6.3.1.1.5.3
    name: IF-MIB::linkDown
    category: auth
    severity: crit
`)

	writeProfileYAML(t, stockDir, "same.yaml", `
traps:
  - oid: 1.3.6.1.6.3.1.1.5.3
    name: IF-MIB::linkDown
    category: state_change
    severity: warning
`)

	setTestDirs(t, userDir, stockDir)
	resetProfileCacheForTest()

	idx, err := AcquireProfileCache()
	require.NoError(t, err)
	defer ReleaseProfileCache()

	td := idx.Lookup("1.3.6.1.6.3.1.1.5.3")
	require.NotNil(t, td)
	assert.Equal(t, "auth", td.Category)
	assert.Equal(t, "crit", td.Severity)
}

func TestProfileLoadFilenameDedupKeepsAllTrapsFromWinningFile(t *testing.T) {
	userDir := t.TempDir()
	stockDir := t.TempDir()

	writeProfileYAML(t, userDir, "same.yaml", `
traps:
  - oid: 1.3.6.1.6.3.1.1.5.2
    name: IF-MIB::warmStart
    category: state_change
    severity: notice
  - oid: 1.3.6.1.6.3.1.1.5.3
    name: IF-MIB::linkDown
    category: auth
    severity: crit
`)

	writeProfileYAML(t, stockDir, "same.yaml", `
traps:
  - oid: 1.3.6.1.6.3.1.1.5.4
    name: IF-MIB::linkUp
    category: state_change
    severity: notice
`)

	setTestDirs(t, userDir, stockDir)
	resetProfileCacheForTest()

	idx, err := AcquireProfileCache()
	require.NoError(t, err)
	defer ReleaseProfileCache()

	assert.NotNil(t, idx.Lookup("1.3.6.1.6.3.1.1.5.2"))
	assert.NotNil(t, idx.Lookup("1.3.6.1.6.3.1.1.5.3"))
	assert.Nil(t, idx.Lookup("1.3.6.1.6.3.1.1.5.4"))
}

func TestProfileLoadExtendsMerge(t *testing.T) {
	dir := t.TempDir()

	writeProfileYAML(t, dir, "_base.yaml", `
varbinds:
  ifIndex:
    oid: 1.3.6.1.2.1.2.2.1.1
    type: INTEGER
  ifDescr:
    oid: 1.3.6.1.2.1.31.1.1.1.1
    type: OctetString

traps:
  - oid: 1.3.6.1.6.3.1.1.5.3
    name: IF-MIB::linkDown
    category: state_change
    severity: info
    description: "Link down: {ifDescr}"
    varbinds: [ifIndex, ifDescr]
`)

	writeProfileYAML(t, dir, "override.yaml", `
extends: [_base.yaml]

traps:
  - oid: 1.3.6.1.6.3.1.1.5.3
    name: IF-MIB::linkDown
    category: security
    severity: warning
    varbinds: [ifIndex]
`)

	setTestDirs(t, dir)
	resetProfileCacheForTest()

	idx, err := AcquireProfileCache()
	require.NoError(t, err)
	defer ReleaseProfileCache()

	td := idx.Lookup("1.3.6.1.6.3.1.1.5.3")
	require.NotNil(t, td)
	assert.Equal(t, "security", td.Category)
	assert.Equal(t, "warning", td.Severity)
	assert.Equal(t, "Link down: {ifDescr}", td.Description)
}

func TestProfileLoadExtendsLaterBaseOverridesEarlier(t *testing.T) {
	dir := t.TempDir()

	writeProfileYAML(t, dir, "_base1.yaml", `
traps:
  - oid: 1.3.6.1.6.3.1.1.5.3
    name: IF-MIB::linkDown
    category: state_change
    severity: info
`)
	writeProfileYAML(t, dir, "_base2.yaml", `
traps:
  - oid: 1.3.6.1.6.3.1.1.5.3
    name: IF-MIB::linkDown
    category: security
    severity: warning
`)
	writeProfileYAML(t, dir, "derived.yaml", `
extends: [_base1.yaml, _base2.yaml]
`)

	setTestDirs(t, dir)
	resetProfileCacheForTest()

	idx, err := AcquireProfileCache()
	require.NoError(t, err)
	defer ReleaseProfileCache()

	td := idx.Lookup("1.3.6.1.6.3.1.1.5.3")
	require.NotNil(t, td)
	assert.Equal(t, "security", td.Category)
	assert.Equal(t, "warning", td.Severity)
}

func TestProfileLoadExtendsPartialTrapOverride(t *testing.T) {
	dir := t.TempDir()

	writeProfileYAML(t, dir, "_base.yaml", `
varbinds:
  ifIndex:
    oid: 1.3.6.1.2.1.2.2.1.1
    type: INTEGER

traps:
  - oid: 1.3.6.1.6.3.1.1.5.3
    name: IF-MIB::linkDown
    category: state_change
    severity: info
    description: "Link down"
    varbinds: [ifIndex]
`)
	writeProfileYAML(t, dir, "derived.yaml", `
extends: [_base.yaml]

traps:
  - oid: 1.3.6.1.6.3.1.1.5.3
    severity: warning
`)

	setTestDirs(t, dir)
	resetProfileCacheForTest()

	idx, err := AcquireProfileCache()
	require.NoError(t, err)
	defer ReleaseProfileCache()

	td := idx.Lookup("1.3.6.1.6.3.1.1.5.3")
	require.NotNil(t, td)
	assert.Equal(t, "IF-MIB::linkDown", td.Name)
	assert.Equal(t, "state_change", td.Category)
	assert.Equal(t, "warning", td.Severity)
	assert.Equal(t, "Link down", td.Description)
	assert.NotNil(t, td.varbindByName("ifIndex"))
}

func TestProfileLoadRejectsUnsafeExtendsName(t *testing.T) {
	dir := t.TempDir()
	writeProfileYAML(t, dir, "bad.yaml", `
extends: [../../outside.yaml]
`)

	setTestDirs(t, dir)
	resetProfileCacheForTest()

	_, err := AcquireProfileCache()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid extends entry")
}

// =============================================================================
// Template rendering tests
// =============================================================================

func TestRenderMessageDefault(t *testing.T) {
	entry := &TrapEntry{
		TrapName: "IF-MIB::linkDown",
		SourceIP: "10.0.0.1",
	}
	td := &TrapDef{}

	msg := renderMessage(entry, td)
	assert.Equal(t, "IF-MIB::linkDown on 10.0.0.1.", msg)
}

func TestRenderMessageWithVarbinds(t *testing.T) {
	dir := t.TempDir()
	writeProfileYAML(t, dir, "test.yaml", `
varbinds:
  ifIndex:
    oid: 1.3.6.1.2.1.2.2.1.1
    type: INTEGER
  ifDescr:
    oid: 1.3.6.1.2.1.31.1.1.1.1
    type: OctetString

traps:
  - oid: 1.3.6.1.6.3.1.1.5.3
    name: IF-MIB::linkDown
    category: state_change
    severity: warning
    description: "Interface {ifDescr} (index {ifIndex}) went down on {_HOSTNAME}"
    varbinds: [ifIndex, ifDescr]
`)

	setTestDirs(t, dir)
	resetProfileCacheForTest()

	idx, err := AcquireProfileCache()
	require.NoError(t, err)
	defer ReleaseProfileCache()

	td := idx.Lookup("1.3.6.1.6.3.1.1.5.3")
	require.NotNil(t, td)

	entry := &TrapEntry{
		TrapName: "IF-MIB::linkDown",
		SourceIP: "10.0.0.1",
		Varbinds: []VarbindValue{
			{OID: "1.3.6.1.2.1.2.2.1.1", Type: "INTEGER", Value: int64(42)},
			{OID: "1.3.6.1.2.1.31.1.1.1.1", Type: "OctetString", Value: "Gi0/1"},
		},
	}

	msg := renderMessage(entry, td)
	assert.Contains(t, msg, "Gi0/1")
	assert.Contains(t, msg, "42")
	assert.Contains(t, msg, "10.0.0.1")
}

func TestRenderMessageResolvesTabularVarbindInstances(t *testing.T) {
	td := testIFMIBLinkDownTrapDef()
	entry := testIFMIBLinkDownEntry()

	msg := renderMessage(entry, td)

	assert.Equal(t, "Link 1 operational state changed to down on 198.51.100.10.", msg)
}

func TestRenderMessageMissingVarbind(t *testing.T) {
	dir := t.TempDir()
	writeProfileYAML(t, dir, "test.yaml", `
varbinds:
  ifDescr:
    oid: 1.3.6.1.2.1.31.1.1.1.1
    type: OctetString

traps:
  - oid: 1.3.6.1.6.3.1.1.5.3
    name: IF-MIB::linkDown
    category: state_change
    severity: warning
    description: "Test {ifDescr}"
    varbinds: [ifDescr]
`)

	setTestDirs(t, dir)
	resetProfileCacheForTest()

	idx, err := AcquireProfileCache()
	require.NoError(t, err)
	defer ReleaseProfileCache()

	td := idx.Lookup("1.3.6.1.6.3.1.1.5.3")
	require.NotNil(t, td)

	entry := &TrapEntry{
		TrapName: "IF-MIB::linkDown",
		SourceIP: "10.0.0.1",
		Varbinds: []VarbindValue{},
	}

	msg := renderMessage(entry, td)
	assert.Contains(t, msg, "<missing>")
}

func TestRenderMessageGoTemplateFirstFallbackSkipsMissingVarbinds(t *testing.T) {
	dir := t.TempDir()
	writeProfileYAML(t, dir, "test.yaml", `
varbinds:
  ifIndex:
    oid: 1.3.6.1.2.1.2.2.1.1
    type: INTEGER
  ifName:
    oid: 1.3.6.1.2.1.31.1.1.1.1
    type: OctetString
  ifDescr:
    oid: 1.3.6.1.2.1.2.2.1.2
    type: OctetString

traps:
  - oid: 1.3.6.1.6.3.1.1.5.3
    name: IF-MIB::linkDown
    category: state_change
    severity: warning
    description: '{{first (value "ifDescr") (value "ifName") (value "ifIndex") "Interface"}} went down on {{hostname}}.'
    varbinds: [ifIndex, ifName, ifDescr]
`)

	setTestDirs(t, dir)
	resetProfileCacheForTest()

	idx, err := AcquireProfileCache()
	require.NoError(t, err)
	defer ReleaseProfileCache()

	td := idx.Lookup("1.3.6.1.6.3.1.1.5.3")
	require.NotNil(t, td)

	entry := &TrapEntry{
		TrapName: "IF-MIB::linkDown",
		SourceIP: "198.51.100.10",
		Varbinds: []VarbindValue{
			{OID: "1.3.6.1.2.1.2.2.1.1.7", Type: "INTEGER", Value: int64(7)},
		},
	}

	msg := renderMessage(entry, td)
	assert.Equal(t, "7 went down on 198.51.100.10.", msg)
	assert.NotContains(t, msg, "<missing>")
}

func TestRenderMessageGoTemplateWithFallback(t *testing.T) {
	dir := t.TempDir()
	writeProfileYAML(t, dir, "test.yaml", `
varbinds:
  ifDescr:
    oid: 1.3.6.1.2.1.2.2.1.2
    type: OctetString

traps:
  - oid: 1.3.6.1.6.3.1.1.5.4
    name: IF-MIB::linkUp
    category: state_change
    severity: notice
    description: '{{with value "ifDescr"}}Interface {{.}}{{else}}Interface{{end}} came up on {{hostname}}.'
    varbinds: [ifDescr]
`)

	setTestDirs(t, dir)
	resetProfileCacheForTest()

	idx, err := AcquireProfileCache()
	require.NoError(t, err)
	defer ReleaseProfileCache()

	td := idx.Lookup("1.3.6.1.6.3.1.1.5.4")
	require.NotNil(t, td)

	entry := &TrapEntry{TrapName: "IF-MIB::linkUp", SourceIP: "198.51.100.10"}
	assert.Equal(t, "Interface came up on 198.51.100.10.", renderMessage(entry, td))

	entry.Varbinds = []VarbindValue{{OID: "1.3.6.1.2.1.2.2.1.2.7", Type: "OctetString", Value: "Gi0/7"}}
	assert.Equal(t, "Interface Gi0/7 came up on 198.51.100.10.", renderMessage(entry, td))
}

func TestRenderMessageGoTemplateWithBlockWithoutElse(t *testing.T) {
	dir := t.TempDir()
	writeProfileYAML(t, dir, "test.yaml", `
varbinds:
  ccmHistoryEventTerminalUser:
    oid: 1.3.6.1.4.1.9.9.43.1.1.6.1.8
    type: DisplayString

traps:
  - oid: 1.3.6.1.4.1.9.9.43.2.0.2
    name: CISCO-CONFIG-MAN-MIB::ccmCLIRunningConfigChanged
    category: config_change
    severity: notice
    description: 'Running configuration changed{{with value "ccmHistoryEventTerminalUser"}} by {{.}}{{end}} on {{hostname}}.'
    varbinds: [ccmHistoryEventTerminalUser]
`)

	setTestDirs(t, dir)
	resetProfileCacheForTest()

	idx, err := AcquireProfileCache()
	require.NoError(t, err)
	defer ReleaseProfileCache()

	td := idx.Lookup("1.3.6.1.4.1.9.9.43.2.0.2")
	require.NotNil(t, td)

	entry := &TrapEntry{TrapName: "CISCO-CONFIG-MAN-MIB::ccmCLIRunningConfigChanged", SourceIP: "198.51.100.10"}
	assert.Equal(t, "Running configuration changed on 198.51.100.10.", renderMessage(entry, td))

	entry.Varbinds = []VarbindValue{{OID: "1.3.6.1.4.1.9.9.43.1.1.6.1.8.1", Type: "DisplayString", Value: "admin"}}
	assert.Equal(t, "Running configuration changed by admin on 198.51.100.10.", renderMessage(entry, td))
}

func TestRenderMessageAndLabelsRedactSensitiveCommunityVarbind(t *testing.T) {
	entry := &TrapEntry{
		SourceIP: "198.51.100.10",
		Varbinds: []VarbindValue{
			{OID: snmpTrapCommunityOID, Name: "snmpTrapCommunity.0", Type: "OctetString", Value: "private-community"},
		},
	}
	td := testSensitiveCommunityTrapDef(
		"Community {snmpTrapCommunity} raw {snmpTrapCommunity.raw} oid {1.3.6.1.6.3.18.1.4}",
		map[string]string{"community": "{snmpTrapCommunity}"},
	)

	msg := renderMessage(entry, td)
	labels := renderLabels(entry, td)

	assert.NotContains(t, msg, "private-community")
	assert.Contains(t, msg, redactedTrapVarbind)
	assert.Equal(t, redactedTrapVarbind, labels["community"])
}

func TestRenderGoTemplateMessageAndLabelsRedactSensitiveCommunityVarbind(t *testing.T) {
	entry := &TrapEntry{
		SourceIP: "198.51.100.10",
		Varbinds: []VarbindValue{
			{OID: snmpTrapCommunityOID, Name: "snmpTrapCommunity.0", Type: "OctetString", Value: "private-community"},
		},
	}
	td := testSensitiveCommunityTrapDef(
		`Community {{value "snmpTrapCommunity"}} raw {{raw "snmpTrapCommunity"}}`,
		map[string]string{"community": `{{value "snmpTrapCommunity"}}`},
	)

	msg := renderMessage(entry, td)
	labels := renderLabels(entry, td)

	assert.NotContains(t, msg, "private-community")
	assert.Contains(t, msg, redactedTrapVarbind)
	assert.Equal(t, redactedTrapVarbind, labels["community"])
}

func TestLoadProfileRejectsGoTemplateIfBlock(t *testing.T) {
	dir := t.TempDir()
	writeProfileYAML(t, dir, "test.yaml", `
varbinds:
  ifName:
    oid: 1.3.6.1.2.1.31.1.1.1.1
    type: OctetString

traps:
  - oid: 1.3.6.1.6.3.1.1.5.4
    name: IF-MIB::linkUp
    category: state_change
    severity: notice
    description: 'Interface{{if value "ifName"}} {{value "ifName"}}{{end}} came up on {{hostname}}.'
    varbinds: [ifName]
`)

	setTestDirs(t, dir)
	resetProfileCacheForTest()

	_, err := AcquireProfileCache()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "if template actions are not allowed")
}

func TestLoadProfileRejectsInvalidGoTemplates(t *testing.T) {
	tests := map[string]string{
		"unknown function": `
varbinds:
  ifIndex:
    oid: 1.3.6.1.2.1.2.2.1.1
    type: INTEGER
traps:
  - oid: 1.3.6.1.6.3.1.1.5.3
    name: IF-MIB::linkDown
    category: state_change
    severity: warning
    description: '{{badfunc "ifIndex"}} on {{hostname}}.'
    varbinds: [ifIndex]
`,
		"unknown varbind": `
varbinds:
  ifIndex:
    oid: 1.3.6.1.2.1.2.2.1.1
    type: INTEGER
traps:
  - oid: 1.3.6.1.6.3.1.1.5.3
    name: IF-MIB::linkDown
    category: state_change
    severity: warning
    description: '{{value "ifName"}} changed on {{hostname}}.'
    varbinds: [ifIndex]
`,
		"forbidden range": `
varbinds:
  ifIndex:
    oid: 1.3.6.1.2.1.2.2.1.1
    type: INTEGER
traps:
  - oid: 1.3.6.1.6.3.1.1.5.3
    name: IF-MIB::linkDown
    category: state_change
    severity: warning
    description: '{{range value "ifIndex"}}x{{end}} on {{hostname}}.'
    varbinds: [ifIndex]
`,
	}

	for name, profile := range tests {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			writeProfileYAML(t, dir, "test.yaml", profile)
			setTestDirs(t, dir)
			resetProfileCacheForTest()
			_, err := AcquireProfileCache()
			require.Error(t, err)
		})
	}
}

func TestLoadProfileRejectsUnboundedGoTemplateLabel(t *testing.T) {
	dir := t.TempDir()
	writeProfileYAML(t, dir, "test.yaml", `
varbinds:
  ifDescr:
    oid: 1.3.6.1.2.1.2.2.1.2
    type: OctetString
traps:
  - oid: 1.3.6.1.6.3.1.1.5.3
    name: IF-MIB::linkDown
    category: state_change
    severity: warning
    description: 'Interface changed on {{hostname}}.'
    labels:
      iface: '{{value "ifDescr"}}'
    varbinds: [ifDescr]
`)

	setTestDirs(t, dir)
	resetProfileCacheForTest()
	_, err := AcquireProfileCache()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unbounded varbind")
}

func TestRenderMessageUnresolvedRef(t *testing.T) {
	entry := &TrapEntry{
		TrapName: "IF-MIB::linkDown",
		SourceIP: "10.0.0.1",
		Varbinds: []VarbindValue{},
	}
	td := &TrapDef{
		Description: "Test {nonexistent}",
	}

	msg := renderMessage(entry, td)
	assert.Contains(t, msg, "<unresolved:nonexistent>")
}

func TestRenderMessageNumericOIDRef(t *testing.T) {
	entry := &TrapEntry{
		TrapName: "IF-MIB::linkDown",
		SourceIP: "10.0.0.1",
		Varbinds: []VarbindValue{
			{OID: "1.3.6.1.2.1.2.2.1.1", Type: "INTEGER", Value: int64(99)},
		},
	}
	td := &TrapDef{
		Description: "Index {1.3.6.1.2.1.2.2.1.1}",
	}

	msg := renderMessage(entry, td)
	assert.Contains(t, msg, "99")
}

func TestRenderMessageNumericOIDRawRef(t *testing.T) {
	entry := &TrapEntry{
		TrapName: "IF-MIB::linkDown",
		SourceIP: "10.0.0.1",
		Varbinds: []VarbindValue{
			{OID: "1.3.6.1.2.1.2.2.1.8", Type: "INTEGER", Value: int64(2), Enum: "down"},
		},
	}
	td := &TrapDef{
		Description: "Raw status {1.3.6.1.2.1.2.2.1.8.raw}",
	}

	msg := renderMessage(entry, td)
	assert.Contains(t, msg, "2")
}

func testSensitiveCommunityTrapDef(description string, labels map[string]string) *TrapDef {
	return &TrapDef{
		Description: description,
		Labels:      labels,
		sharedVarbinds: map[string]*VarbindDef{
			strings.TrimSuffix(snmpTrapCommunityOID, ".0"): {
				OID:     strings.TrimSuffix(snmpTrapCommunityOID, ".0"),
				rawName: "snmpTrapCommunity",
			},
		},
	}
}

func TestResolveVarbindRawResolvesTabularVarbindInstance(t *testing.T) {
	td := testIFMIBLinkDownTrapDef()
	entry := testIFMIBLinkDownEntry()

	got := resolveVarbindRaw("ifOperStatus", entry, td)

	assert.Equal(t, "2", got)
}

func TestFindVarbindForProfileOIDExactMatchWins(t *testing.T) {
	entry := &TrapEntry{
		Varbinds: []VarbindValue{
			{OID: testIFMIBIfIndexOID + ".1", Type: "INTEGER", Value: int64(1)},
			{OID: testIFMIBIfIndexOID, Type: "INTEGER", Value: int64(99)},
		},
	}

	got, ok := findVarbindForProfileOID(entry, testIFMIBIfIndexOID)

	require.True(t, ok)
	assert.Equal(t, testIFMIBIfIndexOID, got.OID)
	assert.Equal(t, int64(99), got.Value)
}

func TestOIDMatchesColumnRequiresArcBoundary(t *testing.T) {
	assert.True(t, oidMatchesColumn(testIFMIBIfOperStatusOID, testIFMIBIfOperStatusOID+".1"))
	assert.False(t, oidMatchesColumn(testIFMIBIfOperStatusOID, testIFMIBIfOperStatusOID))
	assert.False(t, oidMatchesColumn(testIFMIBIfOperStatusOID, testIFMIBIfOperStatusOID+"0.1"))
}

func TestFindVarbindForProfileOIDFirstMatchingInstanceWins(t *testing.T) {
	entry := &TrapEntry{
		Varbinds: []VarbindValue{
			{OID: testIFMIBIfIndexOID + ".2", Type: "INTEGER", Value: int64(2)},
			{OID: testIFMIBIfIndexOID + ".1", Type: "INTEGER", Value: int64(1)},
		},
	}

	got, ok := findVarbindForProfileOID(entry, testIFMIBIfIndexOID)

	require.True(t, ok)
	assert.Equal(t, testIFMIBIfIndexOID+".2", got.OID)
	assert.Equal(t, int64(2), got.Value)
}

func TestFindVarbindForProfileOIDMatchesScalarZeroInstance(t *testing.T) {
	const sysNameOID = "1.3.6.1.2.1.1.5"
	entry := &TrapEntry{
		Varbinds: []VarbindValue{
			{OID: sysNameOID + ".0", Type: "OctetString", Value: "switch01"},
		},
	}

	got, ok := findVarbindForProfileOID(entry, sysNameOID)

	require.True(t, ok)
	assert.Equal(t, sysNameOID+".0", got.OID)
	assert.Equal(t, "switch01", got.Value)
}

func TestFindVarbindDefForObservedOIDUsesLongestColumnPrefix(t *testing.T) {
	td := &TrapDef{
		sharedVarbinds: map[string]*VarbindDef{
			"1.3.6.1.4.1.999.1": {
				OID:     "1.3.6.1.4.1.999.1",
				Type:    "INTEGER",
				rawName: "shortColumn",
			},
			"1.3.6.1.4.1.999.1.1": {
				OID:     "1.3.6.1.4.1.999.1.1",
				Type:    "INTEGER",
				rawName: "longColumn",
			},
		},
	}

	got := findVarbindDefForObservedOID(td, "1.3.6.1.4.1.999.1.1.7")

	require.NotNil(t, got)
	assert.Equal(t, "longColumn", got.rawName)
}

func TestRenderMessageMalformedNumericOIDRawRef(t *testing.T) {
	entry := &TrapEntry{
		TrapName: "IF-MIB::linkDown",
		SourceIP: "10.0.0.1",
	}
	td := &TrapDef{
		Description: "Bad {1.bad.raw}",
	}

	msg := renderMessage(entry, td)
	assert.Contains(t, msg, "<unresolved:1.bad>")
}

func TestRenderMessageEmptyStringVarbindPresent(t *testing.T) {
	dir := t.TempDir()
	writeProfileYAML(t, dir, "test.yaml", `
varbinds:
  ifAlias:
    oid: 1.3.6.1.2.1.31.1.1.1.18
    type: OctetString

traps:
  - oid: 1.3.6.1.6.3.1.1.5.3
    name: IF-MIB::linkDown
    category: state_change
    severity: warning
    description: "Alias [{ifAlias}]"
    varbinds: [ifAlias]
`)

	setTestDirs(t, dir)
	resetProfileCacheForTest()

	idx, err := AcquireProfileCache()
	require.NoError(t, err)
	defer ReleaseProfileCache()

	td := idx.Lookup("1.3.6.1.6.3.1.1.5.3")
	require.NotNil(t, td)

	entry := &TrapEntry{
		TrapName: "IF-MIB::linkDown",
		SourceIP: "10.0.0.1",
		Varbinds: []VarbindValue{
			{OID: "1.3.6.1.2.1.31.1.1.1.18", Type: "OctetString", Value: ""},
		},
	}

	msg := renderMessage(entry, td)
	assert.Equal(t, "Alias []", msg)
}

func TestRenderMessageEnumSubstitution(t *testing.T) {
	dir := t.TempDir()
	writeProfileYAML(t, dir, "test.yaml", `
varbinds:
  ifOperStatus:
    oid: 1.3.6.1.2.1.2.2.1.8
    type: INTEGER
    enum:
      '1': up
      '2': down
      '3': testing

traps:
  - oid: 1.3.6.1.6.3.1.1.5.3
    name: IF-MIB::linkDown
    category: state_change
    severity: warning
    description: "OperStatus is {ifOperStatus}"
    varbinds: [ifOperStatus]
`)

	setTestDirs(t, dir)
	resetProfileCacheForTest()

	idx, err := AcquireProfileCache()
	require.NoError(t, err)
	defer ReleaseProfileCache()

	td := idx.Lookup("1.3.6.1.6.3.1.1.5.3")
	require.NotNil(t, td)

	entry := &TrapEntry{
		TrapName: "IF-MIB::linkDown",
		SourceIP: "10.0.0.1",
		Varbinds: []VarbindValue{
			{OID: "1.3.6.1.2.1.2.2.1.8", Type: "INTEGER", Value: int64(2)},
		},
	}

	msg := renderMessage(entry, td)
	assert.Contains(t, msg, "down")
}

func TestRenderMessageRawEnumValue(t *testing.T) {
	dir := t.TempDir()
	writeProfileYAML(t, dir, "test.yaml", `
varbinds:
  ifOperStatus:
    oid: 1.3.6.1.2.1.2.2.1.8
    type: INTEGER
    enum:
      '1': up
      '2': down

traps:
  - oid: 1.3.6.1.6.3.1.1.5.3
    name: IF-MIB::linkDown
    category: state_change
    severity: warning
    description: "Raw: {ifOperStatus.raw}"
    varbinds: [ifOperStatus]
`)

	setTestDirs(t, dir)
	resetProfileCacheForTest()

	idx, err := AcquireProfileCache()
	require.NoError(t, err)
	defer ReleaseProfileCache()

	td := idx.Lookup("1.3.6.1.6.3.1.1.5.3")
	require.NotNil(t, td)

	entry := &TrapEntry{
		TrapName: "IF-MIB::linkDown",
		SourceIP: "10.0.0.1",
		Varbinds: []VarbindValue{
			{OID: "1.3.6.1.2.1.2.2.1.8", Type: "INTEGER", Value: int64(2)},
		},
	}

	msg := renderMessage(entry, td)
	assert.Contains(t, msg, "2")
}

func TestRenderMessageTruncation(t *testing.T) {
	entry := &TrapEntry{TrapName: "X::Y", SourceIP: "1.1.1.1"}
	td := &TrapDef{}

	long := make([]byte, 600)
	for i := range long {
		long[i] = 'x'
	}
	td.Description = string(long)

	msg := renderMessage(entry, td)
	assert.LessOrEqual(t, len(msg), maxMessageLen)
}

func TestRenderMessageTruncationKeepsValidUTF8(t *testing.T) {
	entry := &TrapEntry{TrapName: "X::Y", SourceIP: "1.1.1.1"}
	td := &TrapDef{Description: strings.Repeat("é", 300)}

	msg := renderMessage(entry, td)
	assert.LessOrEqual(t, len(msg), maxMessageLen)
	assert.True(t, utf8.ValidString(msg))
	assert.True(t, strings.HasSuffix(msg, "..."))
}

func TestRenderLabels(t *testing.T) {
	dir := t.TempDir()
	writeProfileYAML(t, dir, "test.yaml", `
varbinds:
  ifOperStatus:
    oid: 1.3.6.1.2.1.2.2.1.8
    type: INTEGER
    enum:
      '1': up
      '2': down

traps:
  - oid: 1.3.6.1.6.3.1.1.5.3
    name: IF-MIB::linkDown
    category: state_change
    severity: warning
    labels:
      oper_status: "{ifOperStatus}"
      severity: "warning"
    varbinds: [ifOperStatus]
`)

	setTestDirs(t, dir)
	resetProfileCacheForTest()

	idx, err := AcquireProfileCache()
	require.NoError(t, err)
	defer ReleaseProfileCache()

	td := idx.Lookup("1.3.6.1.6.3.1.1.5.3")
	require.NotNil(t, td)

	entry := &TrapEntry{
		TrapName: "IF-MIB::linkDown",
		SourceIP: "10.0.0.1",
		Varbinds: []VarbindValue{
			{OID: "1.3.6.1.2.1.2.2.1.8", Type: "INTEGER", Value: int64(2)},
		},
	}

	labels := renderLabels(entry, td)
	assert.Equal(t, "down", labels["oper_status"])
	assert.Equal(t, "warning", labels["severity"])
}

func TestProfileLoadRejectsUnboundedLabelVarbind(t *testing.T) {
	dir := t.TempDir()
	writeProfileYAML(t, dir, "bad.yaml", `
varbinds:
  ifDescr:
    oid: 1.3.6.1.2.1.31.1.1.1.1
    type: OctetString

traps:
  - oid: 1.3.6.1.6.3.1.1.5.3
    name: IF-MIB::linkDown
    category: state_change
    severity: warning
    labels:
      interface: "{ifDescr}"
    varbinds: [ifDescr]
`)

	setTestDirs(t, dir)
	resetProfileCacheForTest()

	_, err := AcquireProfileCache()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unbounded varbind")
}

func TestRenderMessageSpecialVars(t *testing.T) {
	entry := &TrapEntry{
		TrapName:          "IF-MIB::linkDown",
		SourceIP:          "10.0.0.1",
		DeviceHostname:    "switch01",
		DeviceVendor:      "cisco",
		TopologyInterface: "Gi0/1",
		TopologyNeighbors: "leaf01,leaf02",
	}
	td := &TrapDef{
		Description: "Host: {_HOSTNAME}, IP: {TRAP_SOURCE_IP}, Name: {TRAP_NAME}, Vendor: {TRAP_DEVICE_VENDOR}, Interface: {TRAP_INTERFACE}, Neighbors: {TRAP_NEIGHBORS}",
	}

	msg := renderMessage(entry, td)
	assert.Contains(t, msg, "switch01")
	assert.Contains(t, msg, "10.0.0.1")
	assert.Contains(t, msg, "IF-MIB::linkDown")
	assert.Contains(t, msg, "cisco")
	assert.Contains(t, msg, "Gi0/1")
	assert.Contains(t, msg, "leaf01,leaf02")
}

func TestRenderMessageHostnameFallback(t *testing.T) {
	tests := map[string]struct {
		entry *TrapEntry
		want  string
	}{
		"source_ip": {
			entry: &TrapEntry{
				TrapName: "IF-MIB::linkDown",
				SourceIP: "10.0.0.1",
			},
			want: "10.0.0.1",
		},
		"udp_peer": {
			entry: &TrapEntry{
				TrapName:      "IF-MIB::linkDown",
				SourceUDPPeer: "10.0.0.2",
			},
			want: "10.0.0.2",
		},
	}
	td := &TrapDef{
		Description: "Host: {_HOSTNAME}",
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			msg := renderMessage(tc.entry, td)
			assert.Contains(t, msg, tc.want)
		})
	}
}

// =============================================================================
// 2-tier varbind resolution tests
// =============================================================================

func TestResolve2TierProfileFirst(t *testing.T) {
	dir := t.TempDir()
	writeProfileYAML(t, dir, "test.yaml", `
varbinds:
  ifDescr:
    oid: 1.3.6.1.2.1.31.1.1.1.1
    type: OctetString

traps:
  - oid: 1.3.6.1.6.3.1.1.5.3
    name: IF-MIB::linkDown
    category: state_change
    severity: warning
    varbinds: [ifDescr]
`)

	setTestDirs(t, dir)
	resetProfileCacheForTest()

	idx, err := AcquireProfileCache()
	require.NoError(t, err)
	defer ReleaseProfileCache()

	td := idx.Lookup("1.3.6.1.6.3.1.1.5.3")
	require.NotNil(t, td)

	raw := VarbindValue{OID: "1.3.6.1.2.1.31.1.1.1.1", Type: "OctetString", Value: "Gi0/1"}
	resolved := resolve2TierVarbind("1.3.6.1.2.1.31.1.1.1.1", raw, td)
	assert.Equal(t, "ifDescr", resolved.Name)
	assert.Equal(t, ASN1Type("OctetString"), resolved.Type)
}

func TestResolve2TierRawFallback(t *testing.T) {
	raw := VarbindValue{
		Name:  "customVarbind",
		OID:   "1.3.6.1.4.1.99999.1.1",
		Type:  "Counter32",
		Value: int64(12345),
	}

	resolved := resolve2TierVarbind("1.3.6.1.4.1.99999.1.1", raw, nil)
	assert.Equal(t, "customVarbind", resolved.Name)
	assert.Equal(t, ASN1Type("Counter32"), resolved.Type)
}

func TestResolve2TierRawFallbackNoName(t *testing.T) {
	raw := VarbindValue{
		OID:   "1.3.6.1.4.1.99999.1.1",
		Type:  "Counter32",
		Value: int64(12345),
	}

	resolved := resolve2TierVarbind("1.3.6.1.4.1.99999.1.1", raw, nil)
	assert.Equal(t, "1.3.6.1.4.1.99999.1.1", resolved.OID)
	assert.Equal(t, ASN1Type("Counter32"), resolved.Type)
}

func TestResolve2TierEnum(t *testing.T) {
	dir := t.TempDir()
	writeProfileYAML(t, dir, "test.yaml", `
varbinds:
  ifOperStatus:
    oid: 1.3.6.1.2.1.2.2.1.8
    type: INTEGER
    enum:
      '1': up
      '2': down

traps:
  - oid: 1.3.6.1.6.3.1.1.5.3
    name: IF-MIB::linkDown
    category: state_change
    severity: warning
    varbinds: [ifOperStatus]
`)

	setTestDirs(t, dir)
	resetProfileCacheForTest()

	idx, err := AcquireProfileCache()
	require.NoError(t, err)
	defer ReleaseProfileCache()

	td := idx.Lookup("1.3.6.1.6.3.1.1.5.3")
	require.NotNil(t, td)

	raw := VarbindValue{OID: "1.3.6.1.2.1.2.2.1.8", Type: "INTEGER", Value: int64(2)}
	resolved := resolve2TierVarbind("1.3.6.1.2.1.2.2.1.8", raw, td)
	assert.Equal(t, "down", resolved.Enum)
}

func TestResolve2TierResolvesTabularVarbindInstance(t *testing.T) {
	td := testIFMIBLinkDownTrapDef()
	raw := VarbindValue{OID: testIFMIBIfOperStatusOID + ".1", Type: "INTEGER", Value: int64(2)}

	resolved := resolve2TierVarbind(raw.OID, raw, td)

	assert.Equal(t, "ifOperStatus", resolved.Name)
	assert.Equal(t, testIFMIBIfOperStatusOID+".1", resolved.OID)
	assert.Equal(t, ASN1Type("INTEGER"), resolved.Type)
	assert.Equal(t, int64(2), resolved.Value)
	assert.Equal(t, "down", resolved.Enum)
}

// =============================================================================
// Stock profile index verification
// =============================================================================

func TestStockProfileIndexLoads(t *testing.T) {
	resetProfileCacheForTest()

	idx, err := AcquireProfileCache()
	if err != nil {
		t.Skipf("no stock profiles available: %v", err)
	}
	defer ReleaseProfileCache()

	assert.NotNil(t, idx)
	assert.Empty(t, idx.trapsByOID, "stock profiles should be routed lazily, not retained at startup")
	require.NotNil(t, idx.stock)
	assert.NotEmpty(t, idx.stock.exactRoutes)

	// Verify known IETF standard trap OIDs
	td := idx.Lookup("1.3.6.1.6.3.1.1.5.1")
	require.NotNil(t, td)
	assert.Equal(t, "state_change", td.Category)

	td = idx.Lookup("1.3.6.1.6.3.1.1.5.3")
	require.NotNil(t, td)
	assert.Equal(t, "state_change", td.Category)
	assert.NotEmpty(t, td.Name)
	assert.NotEmpty(t, idx.trapsByOID, "first stock lookup should retain the routed profile file")
}

func TestStockProfileCatalogueMatchesDefaultFiles(t *testing.T) {
	stockDir := trapProfilesDirFromThisFile()
	if stockDir == "" {
		t.Skip("no stock profiles available")
	}
	cataloguePath := filepath.Join(filepath.Dir(stockDir), "catalogue.json")
	data, err := os.ReadFile(cataloguePath)
	require.NoError(t, err)

	var catalogue map[string]struct {
		File      string   `json:"file"`
		TrapCount int      `json:"trap_count"`
		TrapOIDs  []string `json:"trap_oids"`
	}
	require.NoError(t, json.Unmarshal(data, &catalogue))
	require.NotEmpty(t, catalogue)

	files, err := profileFilesInDir(stockDir)
	require.NoError(t, err)
	require.NotEmpty(t, files)

	routesByFile := make(map[string]profileTrapRoutes, len(files))
	totalFiles := 0
	for _, path := range files {
		name := filepath.Base(path)
		require.False(t, strings.HasSuffix(name, ".gz"), "stock profiles must stay uncompressed in the repository")
		routes := profileTrapRoutesFromFile(t, path)
		routesByFile[name] = routes
		totalFiles += len(routes.oids)
	}
	require.Len(t, routesByFile, len(catalogue), "catalogue and default profile file count must match")

	totalCatalogue := 0
	for vendor, entry := range catalogue {
		require.NotEmpty(t, entry.File, "catalogue entry %q has no file", vendor)
		routes, ok := routesByFile[entry.File]
		require.True(t, ok, "catalogue entry %q references missing profile file %q", vendor, entry.File)
		assert.Equal(t, entry.TrapCount, len(routes.oids), "catalogue trap_count mismatch for %s", entry.File)
		assert.Equal(t, routes.oids, entry.TrapOIDs, "catalogue trap_oids mismatch for %s", entry.File)
		totalCatalogue += entry.TrapCount
	}
	assert.Equal(t, totalFiles, totalCatalogue, "catalogue trap_count sum must match profile files")
}

func TestStockIFMIBLinkMessagesDoNotDependOnIfOperStatus(t *testing.T) {
	resetProfileCacheForTest()

	idx, err := AcquireProfileCache()
	if err != nil {
		t.Skipf("no stock profiles available: %v", err)
	}
	defer ReleaseProfileCache()

	tests := map[string]struct {
		oid          string
		wantContains []string
		stateWord    string
	}{
		"linkDown": {
			oid:          testIFMIBLinkDownOID,
			wantContains: []string{"1", "down", "lab-switch"},
			stateWord:    "down",
		},
		"linkUp": {
			oid:          testIFMIBLinkUpOID,
			wantContains: []string{"1", "up", "lab-switch"},
			stateWord:    "up",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			td := idx.Lookup(tc.oid)
			require.NotNil(t, td)

			entry := trapEntryFromPDU("local", &TrapPDU{
				OID:      tc.oid,
				SourceIP: "198.51.100.10",
				PeerIP:   "198.51.100.10",
				Version:  SnmpVersionV2c,
				PduType:  PduTypeTrap,
				Varbinds: []VarbindValue{
					{OID: testIFMIBIfIndexOID + ".1", Type: "INTEGER", Value: int64(1)},
				},
			}, td, 1000000, 1000)
			entry.DeviceHostname = "lab-switch"

			renderTrapEntryTemplates(entry, td)

			for _, want := range tc.wantContains {
				assert.Contains(t, entry.Message, want)
			}
			assert.NotContains(t, entry.Message, "ifOperStatus")
			assert.NotContains(t, entry.Message, "ifAdminStatus")
			assert.NotContains(t, entry.Message, "changed to")
			assert.False(t, hasUnresolvedTemplateMarker(entry.Message))

			entry = trapEntryFromPDU("local", &TrapPDU{
				OID:      tc.oid,
				SourceIP: "198.51.100.10",
				PeerIP:   "198.51.100.10",
				Version:  SnmpVersionV2c,
				PduType:  PduTypeTrap,
			}, td, 1000000, 1000)
			entry.DeviceHostname = "lab-switch"

			renderTrapEntryTemplates(entry, td)

			assert.Contains(t, entry.Message, tc.stateWord)
			assert.Contains(t, entry.Message, "lab-switch")
			assert.NotContains(t, entry.Message, "ifOperStatus")
			assert.NotContains(t, entry.Message, "ifAdminStatus")
			assert.NotContains(t, entry.Message, "changed to")
			assert.False(t, hasUnresolvedTemplateMarker(entry.Message))
		})
	}
}

// =============================================================================
// Test helpers
// =============================================================================

const (
	testIFMIBLinkDownOID      = "1.3.6.1.6.3.1.1.5.3"
	testIFMIBLinkUpOID        = "1.3.6.1.6.3.1.1.5.4"
	testIFMIBIfIndexOID       = "1.3.6.1.2.1.2.2.1.1"
	testIFMIBIfAdminStatusOID = "1.3.6.1.2.1.2.2.1.7"
	testIFMIBIfOperStatusOID  = "1.3.6.1.2.1.2.2.1.8"
)

func testIFMIBLinkDownTrapDef() *TrapDef {
	return &TrapDef{
		OID:         testIFMIBLinkDownOID,
		Name:        "IF-MIB::linkDown",
		Category:    "state_change",
		Severity:    "warning",
		Description: "Link {ifIndex} operational state changed to {ifOperStatus} on {_HOSTNAME}.",
		VarbindRefs: []any{"ifIndex", "ifAdminStatus", "ifOperStatus"},
		sharedVarbinds: map[string]*VarbindDef{
			testIFMIBIfIndexOID: {
				OID:     testIFMIBIfIndexOID,
				Type:    "INTEGER",
				rawName: "ifIndex",
			},
			testIFMIBIfAdminStatusOID: {
				OID:     testIFMIBIfAdminStatusOID,
				Type:    "INTEGER",
				rawName: "ifAdminStatus",
				Enum: map[string]string{
					"1": "up",
					"2": "down",
					"3": "testing",
				},
			},
			testIFMIBIfOperStatusOID: {
				OID:     testIFMIBIfOperStatusOID,
				Type:    "INTEGER",
				rawName: "ifOperStatus",
				Enum: map[string]string{
					"1": "up",
					"2": "down",
					"3": "testing",
				},
			},
		},
	}
}

func testIFMIBLinkDownEntry() *TrapEntry {
	return &TrapEntry{
		TrapOID:  testIFMIBLinkDownOID,
		TrapName: "IF-MIB::linkDown",
		SourceIP: "198.51.100.10",
		Varbinds: []VarbindValue{
			{OID: testIFMIBIfIndexOID + ".1", Type: "INTEGER", Value: int64(1)},
			{OID: testIFMIBIfAdminStatusOID + ".1", Type: "INTEGER", Value: int64(1)},
			{OID: testIFMIBIfOperStatusOID + ".1", Type: "INTEGER", Value: int64(2)},
		},
	}
}

func testIFMIBLinkDownPDU() *TrapPDU {
	return &TrapPDU{
		OID:      testIFMIBLinkDownOID,
		SourceIP: "198.51.100.10",
		PeerIP:   "198.51.100.10",
		Version:  SnmpVersionV2c,
		PduType:  PduTypeTrap,
		Varbinds: []VarbindValue{
			{OID: testIFMIBIfIndexOID + ".1", Type: "INTEGER", Value: int64(1)},
			{OID: testIFMIBIfAdminStatusOID + ".1", Type: "INTEGER", Value: int64(1)},
			{OID: testIFMIBIfOperStatusOID + ".1", Type: "INTEGER", Value: int64(2)},
		},
	}
}

func writeProfileYAML(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	err := os.WriteFile(path, []byte(content), 0644)
	require.NoError(t, err)
}

func writeProfileYAMLGzip(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	file, err := os.Create(path)
	require.NoError(t, err)
	defer file.Close()

	zw := gzip.NewWriter(file)
	_, err = zw.Write([]byte(content))
	require.NoError(t, err)
	require.NoError(t, zw.Close())
}

type profileTrapRoutes struct {
	oids  []string
	names []string
}

func profileTrapRoutesFromFile(t *testing.T, path string) profileTrapRoutes {
	t.Helper()
	file, err := os.Open(path)
	require.NoError(t, err)
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var routes profileTrapRoutes
	for scanner.Scan() {
		line := scanner.Text()
		if after, ok := strings.CutPrefix(line, "  - oid: "); ok {
			routes.oids = append(routes.oids, strings.TrimSpace(after))
			continue
		}
		if after, ok := strings.CutPrefix(line, "    name: "); ok {
			routes.names = append(routes.names, strings.TrimSpace(after))
		}
	}
	require.NoError(t, scanner.Err())
	require.Len(t, routes.names, len(routes.oids), "%s has mismatched trap OID/name counts", path)
	return routes
}
