// SPDX-License-Identifier: GPL-3.0-or-later

package catalog

import (
	"bufio"
	"compress/gzip"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/klauspost/compress/zstd"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	testProfileManager *Manager
	testProfileLeases  []*Lease
)

func TestManagerSharesOneEpochUntilFinalRelease(t *testing.T) {
	stockDir := writeTestStockCatalog(t)
	manager := NewManager(Paths{StockDir: stockDir})

	first, err := manager.Acquire()
	require.NoError(t, err)
	second, err := manager.Acquire()
	require.NoError(t, err)
	assert.Same(t, first.Epoch(), second.Epoch())

	first.Close()
	first.Close()
	third, err := manager.Acquire()
	require.NoError(t, err)
	assert.Same(t, second.Epoch(), third.Epoch())

	second.Close()
	oldEpoch := third.Epoch()
	third.Close()
	fourth, err := manager.Acquire()
	require.NoError(t, err)
	t.Cleanup(fourth.Close)
	assert.NotSame(t, oldEpoch, fourth.Epoch())
}

func TestManagerConcurrentAcquireSharesOneEpoch(t *testing.T) {
	manager := NewManager(Paths{StockDir: writeTestStockCatalog(t)})
	leases := make([]*Lease, 32)
	errs := make([]error, len(leases))
	var wg sync.WaitGroup
	for i := range leases {
		wg.Go(func() {
			leases[i], errs[i] = manager.Acquire()
		})
	}
	wg.Wait()
	for i, err := range errs {
		require.NoError(t, err)
		assert.Same(t, leases[0].Epoch(), leases[i].Epoch())
	}
	for _, lease := range leases {
		lease.Close()
	}
}

func TestManagerRetriesLoadAfterFailure(t *testing.T) {
	root := t.TempDir()
	stockDir := filepath.Join(root, "default")
	manager := NewManager(Paths{StockDir: stockDir})

	_, err := manager.Acquire()
	require.Error(t, err)

	require.NoError(t, os.MkdirAll(stockDir, 0o755))
	writeStockProfileAndCatalogue(t, stockDir, "test", "1.3.6.1.4.1.99999.1")
	lease, err := manager.Acquire()
	require.NoError(t, err)
	t.Cleanup(lease.Close)
	assert.NotNil(t, lease.Epoch())
}

func acquireTestEpoch() (*Epoch, error) {
	if testProfileManager == nil {
		return nil, fmt.Errorf("test profile manager is not configured")
	}
	lease, err := testProfileManager.Acquire()
	if err != nil {
		return nil, err
	}
	testProfileLeases = append(testProfileLeases, lease)
	return lease.Epoch(), nil
}

func releaseTestEpoch() {
	if len(testProfileLeases) == 0 {
		return
	}
	last := len(testProfileLeases) - 1
	lease := testProfileLeases[last]
	testProfileLeases = testProfileLeases[:last]
	lease.Close()
}

func resetTestEpoch() {
	for len(testProfileLeases) > 0 {
		releaseTestEpoch()
	}
}

func setTestDirs(t *testing.T, dirs ...string) {
	t.Helper()
	resetTestEpoch()
	testProfileManager = NewManager(Paths{UserDirs: append([]string(nil), dirs...), StockDir: writeTestStockCatalog(t)})
	t.Cleanup(func() {
		resetTestEpoch()
		testProfileManager = nil
	})
}

func writeTestStockCatalog(t *testing.T) string {
	t.Helper()
	stockDir := filepath.Join(t.TempDir(), "default")
	require.NoError(t, os.MkdirAll(stockDir, 0o755))
	writeStockProfileAndCatalogue(t, stockDir, "catalog_base", "1.3.6.1.4.1.99999.999")
	return stockDir
}

func writeStockProfileAndCatalogue(t *testing.T, stockDir, name, oid string) {
	t.Helper()
	writeProfileYAML(t, stockDir, name+".yaml", fmt.Sprintf(`
traps:
  - oid: %s
    name: TEST-CATALOG-MIB::%s
    category: diagnostic
    severity: info
`, oid, name))
	catalogue := stockProfileCatalogue{
		name: {
			File:     name + ".yaml",
			MIBs:     []string{"TEST-CATALOG-MIB"},
			TrapOIDs: []string{oid},
		},
	}
	writeStockCatalogue(t, stockDir, catalogue, false)
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
    description: 'Interface {{value "ifDescr"}} down'
    varbinds: [ifIndex, ifDescr]
`)

	setTestDirs(t, dir)
	resetTestEpoch()

	idx, err := acquireTestEpoch()
	require.NoError(t, err)
	defer releaseTestEpoch()

	td := idx.Lookup("1.3.6.1.6.3.1.1.5.3")
	require.NotNil(t, td)
	assert.Equal(t, "IF-MIB::linkDown", td.Name)
	assert.Equal(t, "state_change", td.Category)
	assert.Equal(t, "warning", td.Severity)
	assert.NotNil(t, td.SharedVarbinds)
}

func TestBuildSharedVarbindsUsesOnlyExplicitReferences(t *testing.T) {
	fileVarbinds := map[string]VarbindDef{
		"referenced":   {OID: "1.3.6.1.4.1.99999.1", Type: "INTEGER"},
		"unreferenced": {OID: "1.3.6.1.4.1.99999.2", Type: "INTEGER"},
	}
	trap := &TrapDef{VarbindRefs: []any{"referenced"}}

	shared := buildSharedVarbinds(trap, fileVarbinds)
	require.Len(t, shared, 1)
	require.Equal(t, "referenced", shared["1.3.6.1.4.1.99999.1"].RawName)
	require.NotContains(t, shared, "1.3.6.1.4.1.99999.2")
}

func TestProfileLoadSupportedFormats(t *testing.T) {
	for _, tc := range []struct {
		name  string
		write func(*testing.T, string, string, string)
	}{
		{name: "test.yaml", write: writeProfileYAML},
		{name: "test.yml", write: writeProfileYAML},
		{name: "test.yaml.zst", write: writeProfileYAMLZstd},
		{name: "test.yml.zst", write: writeProfileYAMLZstd},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			tc.write(t, dir, tc.name, `
traps:
  - oid: 1.3.6.1.6.3.1.1.5.3
    name: IF-MIB::linkDown
    category: state_change
    severity: warning
`)

			setTestDirs(t, dir)
			resetTestEpoch()

			idx, err := acquireTestEpoch()
			require.NoError(t, err)
			defer releaseTestEpoch()

			td := idx.Lookup("1.3.6.1.6.3.1.1.5.3")
			require.NotNil(t, td)
			assert.Equal(t, "IF-MIB::linkDown", td.Name)
		})
	}
}

func TestInvalidUserProfileErrorNamesTheFile(t *testing.T) {
	userDir := t.TempDir()
	path := filepath.Join(userDir, "broken.yaml")
	writeProfileYAML(t, userDir, "broken.yaml", "traps: [")

	_, err := NewManager(Paths{UserDirs: []string{userDir}, StockDir: writeTestStockCatalog(t)}).Acquire()
	require.ErrorContains(t, err, path)
}

func TestProfileLoadRejectsAlternateOIDDuplicatesAcrossUserProfiles(t *testing.T) {
	userDir := t.TempDir()
	writeProfileYAML(t, userDir, "first.yaml", `
traps:
  - oid: 1.3.6.1.4.1.99998.0.10
    name: USER-MIB::first
    category: diagnostic
    severity: info
`)
	writeProfileYAML(t, userDir, "second.yaml", `
traps:
  - oid: 1.3.6.1.4.1.99998.10
    name: USER-MIB::second
    category: diagnostic
    severity: info
`)

	_, err := NewManager(Paths{UserDirs: []string{userDir}, StockDir: writeTestStockCatalog(t)}).Acquire()
	require.ErrorContains(t, err, "duplicate trap OID")
}

func TestProfileLoadIgnoresGzipProfiles(t *testing.T) {
	userDir := t.TempDir()
	writeGzipFile(t, userDir, "ignored.yaml.gz", `
traps:
  - oid: 1.3.6.1.4.1.99998.1
    name: IGNORED-MIB::ignored
    category: diagnostic
    severity: info
`)
	setTestDirs(t, userDir)
	idx, err := acquireTestEpoch()
	require.NoError(t, err)
	t.Cleanup(releaseTestEpoch)
	assert.Nil(t, idx.Lookup("1.3.6.1.4.1.99998.1"))
}

func TestProfileLoadRejectsRemovedExtendsKey(t *testing.T) {
	userDir := t.TempDir()
	writeProfileYAML(t, userDir, "_base.yaml", `
traps:
  - oid: 1.3.6.1.4.1.99998.1
    name: USER-MIB::base
    category: diagnostic
    severity: info
`)
	writeProfileYAML(t, userDir, "site.yaml", "extends: [_base.yaml]\n")

	_, err := NewManager(Paths{UserDirs: []string{userDir}, StockDir: writeTestStockCatalog(t)}).Acquire()
	require.ErrorContains(t, err, `unknown config key "extends"`)
}

func TestStockProfileCatalogueRequiresValidSHA256(t *testing.T) {
	content := []byte(`
traps:
  - oid: 1.3.6.1.4.1.99998.2
    name: VENDOR-MIB::event
    category: diagnostic
    severity: info
`)
	valid := fmt.Sprintf("%x", sha256.Sum256(content))
	tests := map[string]struct {
		sha256  string
		include bool
	}{
		"missing":   {},
		"empty":     {include: true},
		"short":     {sha256: "abc", include: true},
		"non_hex":   {sha256: strings.Repeat("g", 64), include: true},
		"uppercase": {sha256: strings.ToUpper(valid), include: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			stockDir := filepath.Join(root, "default")
			require.NoError(t, os.MkdirAll(stockDir, 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(stockDir, "vendor.yaml"), content, 0o644))
			entry := map[string]any{
				"file":      "vendor.yaml",
				"mibs":      []string{"VENDOR-MIB"},
				"trap_oids": []string{"1.3.6.1.4.1.99998.2"},
			}
			if tc.include {
				entry["sha256"] = tc.sha256
			}
			data, err := json.Marshal(map[string]any{"vendor": entry})
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(filepath.Join(root, "catalogue.json"), data, 0o644))

			_, err = NewManager(Paths{StockDir: stockDir}).Acquire()
			require.ErrorContains(t, err, "invalid sha256")
		})
	}
}

func TestStockProfileCatalogueValidatesSHA256ForOperatorOverride(t *testing.T) {
	root := t.TempDir()
	stockDir := filepath.Join(root, "default")
	userDir := t.TempDir()
	require.NoError(t, os.MkdirAll(stockDir, 0o755))
	writeProfileYAML(t, stockDir, "vendor.yaml", `
traps:
  - oid: 1.3.6.1.4.1.99998.2
    name: VENDOR-MIB::stock
    category: diagnostic
    severity: info
`)
	writeProfileYAML(t, userDir, "vendor.yaml", `
traps:
  - oid: 1.3.6.1.4.1.99998.3
    name: VENDOR-MIB::operator
    category: diagnostic
    severity: warning
`)
	data, err := json.Marshal(map[string]any{
		"vendor": map[string]any{
			"file":      "vendor.yaml",
			"sha256":    "invalid",
			"mibs":      []string{"VENDOR-MIB"},
			"trap_oids": []string{"1.3.6.1.4.1.99998.2"},
		},
	})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(root, "catalogue.json"), data, 0o644))

	_, err = NewManager(Paths{UserDirs: []string{userDir}, StockDir: stockDir}).Acquire()
	require.ErrorContains(t, err, "invalid sha256")
}

func TestStockProfileEpochBindsLazyHydrationToManifestContent(t *testing.T) {
	const oid = "1.3.6.1.4.1.99998.3"
	root := t.TempDir()
	stockDir := filepath.Join(root, "default")
	require.NoError(t, os.MkdirAll(stockDir, 0o755))

	writeGeneration := func(content string) {
		t.Helper()
		writeProfileYAML(t, stockDir, "vendor.yaml", content)
		data, err := json.Marshal(map[string]any{
			"vendor": map[string]any{
				"file":      "vendor.yaml",
				"sha256":    fmt.Sprintf("%x", sha256.Sum256([]byte(content))),
				"mibs":      []string{"VENDOR-MIB"},
				"trap_oids": []string{oid},
			},
		})
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(root, "catalogue.json"), data, 0o644))
	}

	generationA := `
traps:
  - oid: 1.3.6.1.4.1.99998.3
    name: VENDOR-MIB::event
    category: diagnostic
    severity: info
`
	generationB := `
traps:
  - oid: 1.3.6.1.4.1.99998.3
    name: VENDOR-MIB::event
    category: diagnostic
    severity: warning
`
	writeGeneration(generationA)

	manager := NewManager(Paths{StockDir: stockDir})
	first, err := manager.Acquire()
	require.NoError(t, err)
	writeGeneration(generationB)
	second, err := manager.Acquire()
	require.NoError(t, err)
	assert.Same(t, first.Epoch(), second.Epoch())

	td, firstErr := first.Epoch().LookupWithError(oid)
	assert.Nil(t, td)
	require.ErrorContains(t, firstErr, "content sha256 mismatch")
	td, secondErr := second.Epoch().LookupWithError(oid)
	assert.Nil(t, td)
	assert.EqualError(t, secondErr, firstErr.Error(), "a mismatch must be cached for the epoch")

	first.Close()
	second.Close()
	third, err := manager.Acquire()
	require.NoError(t, err)
	t.Cleanup(third.Close)
	td, err = third.Epoch().LookupWithError(oid)
	require.NoError(t, err)
	require.NotNil(t, td)
	assert.Equal(t, "warning", td.Severity)
}

func TestStockProfileDigestMismatchPrecedesYAMLParsing(t *testing.T) {
	root := t.TempDir()
	stockDir := filepath.Join(root, "default")
	require.NoError(t, os.MkdirAll(stockDir, 0o755))
	writeProfileYAML(t, stockDir, "vendor.yaml", `not: [valid`)
	data, err := json.Marshal(map[string]any{
		"vendor": map[string]any{
			"file":      "vendor.yaml",
			"sha256":    strings.Repeat("0", 64),
			"mibs":      []string{"VENDOR-MIB"},
			"trap_oids": []string{"1.3.6.1.4.1.99998.4"},
		},
	})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(root, "catalogue.json"), data, 0o644))

	lease, err := NewManager(Paths{StockDir: stockDir}).Acquire()
	require.NoError(t, err)
	t.Cleanup(lease.Close)
	_, err = lease.Epoch().LookupWithError("1.3.6.1.4.1.99998.4")
	require.ErrorContains(t, err, "content sha256 mismatch")
}

func TestStockProfileSHA256UsesDecompressedBytes(t *testing.T) {
	const oid = "1.3.6.1.4.1.99998.5"
	stockDir := filepath.Join(t.TempDir(), "default")
	require.NoError(t, os.MkdirAll(stockDir, 0o755))
	writeProfileYAMLZstd(t, stockDir, "vendor.yaml.zst", `
traps:
  - oid: 1.3.6.1.4.1.99998.5
    name: VENDOR-MIB::compressed
    category: diagnostic
    severity: info
`)
	writeStockCatalogue(t, stockDir, stockProfileCatalogue{
		"vendor": {File: "vendor.yaml", MIBs: []string{"VENDOR-MIB"}, TrapOIDs: []string{oid}},
	}, false)

	lease, err := NewManager(Paths{StockDir: stockDir}).Acquire()
	require.NoError(t, err)
	t.Cleanup(lease.Close)
	td, err := lease.Epoch().LookupWithError(oid)
	require.NoError(t, err)
	require.NotNil(t, td)
	assert.Equal(t, "VENDOR-MIB::compressed", td.Name)
}

func TestStockProfileLazyHydrationAndNegativeCache(t *testing.T) {
	root := t.TempDir()
	stockDir := filepath.Join(root, "default")
	require.NoError(t, os.MkdirAll(stockDir, 0o755))
	writeProfileYAMLZstd(t, stockDir, "vendor.yaml.zst", `not: [valid`)
	catalogue := stockProfileCatalogue{
		"vendor": {File: "vendor.yaml", MIBs: []string{"VENDOR-MIB"}, TrapOIDs: []string{"1.3.6.1.4.1.99998.2"}},
	}
	writeStockCatalogue(t, stockDir, catalogue, false)

	manager := NewManager(Paths{StockDir: stockDir})
	lease, err := manager.Acquire()
	require.NoError(t, err, "stock profile bodies must remain lazy")
	idx := lease.Epoch()

	td, firstErr := idx.LookupWithError("1.3.6.1.4.1.99998.2")
	require.Error(t, firstErr)
	assert.Nil(t, td)
	writeProfileYAMLZstd(t, stockDir, "vendor.yaml.zst", `
traps:
  - oid: 1.3.6.1.4.1.99998.2
    name: VENDOR-MIB::recovered
    category: diagnostic
    severity: info
`)
	for owner, entry := range catalogue {
		entry.SHA256 = ""
		catalogue[owner] = entry
	}
	writeStockCatalogue(t, stockDir, catalogue, false)
	td, secondErr := idx.LookupWithError("1.3.6.1.4.1.99998.2")
	assert.Nil(t, td)
	assert.EqualError(t, secondErr, firstErr.Error(), "a failed hydration must be cached for the epoch")

	lease.Close()
	newLease, err := manager.Acquire()
	require.NoError(t, err)
	t.Cleanup(newLease.Close)
	td, err = newLease.Epoch().LookupWithError("1.3.6.1.4.1.99998.2")
	require.NoError(t, err)
	require.NotNil(t, td)
	assert.Equal(t, "VENDOR-MIB::recovered", td.Name)
}

func TestStockHydrationPublishesBundleAtomically(t *testing.T) {
	root := t.TempDir()
	stockDir := filepath.Join(root, "default")
	require.NoError(t, os.MkdirAll(stockDir, 0o755))
	writeProfileYAML(t, stockDir, "vendor.yaml", `
traps:
  - oid: 1.3.6.1.4.1.99998.3
    name: VENDOR-MIB::unpublished
    category: diagnostic
    severity: info
metrics:
  - name: vendor.atomic
    type: counter
    on_trap: VENDOR-MIB::unpublished
    output:
      metric: vendor_atomic_events
      dimension: events
      chart: missing_chart
`)
	writeStockCatalogue(t, stockDir, stockProfileCatalogue{
		"vendor": {
			File:            "vendor.yaml",
			MIBs:            []string{"VENDOR-MIB"},
			TrapOIDs:        []string{"1.3.6.1.4.1.99998.3"},
			MetricRuleNames: []string{"vendor.atomic"},
		},
	}, false)

	lease, err := NewManager(Paths{StockDir: stockDir}).Acquire()
	require.NoError(t, err)
	t.Cleanup(lease.Close)
	idx := lease.Epoch()
	_, err = idx.Definitions([]string{"vendor.atomic"})
	require.Error(t, err)
	require.ErrorContains(t, err, `references unknown chart "missing_chart"`)
	assert.Nil(t, idx.lookupLoaded("1.3.6.1.4.1.99998.3"))
	assert.Nil(t, idx.lookupMetricRule("vendor.atomic"))
	assert.Nil(t, idx.lookupMetricChart("missing_chart"))
}

func TestStockHydrationPublishesOnlyBundleDelta(t *testing.T) {
	idx := newEpoch()
	first := &TrapDef{OID: "1.3.6.1.4.1.99998.11", Name: "DELTA-MIB::first"}
	require.NoError(t, idx.addTraps([]*TrapDef{first}))
	original := idx.trapsByOID

	second := &TrapDef{OID: "1.3.6.1.4.1.99998.12", Name: "DELTA-MIB::second"}
	require.NoError(t, idx.addBundleAtomic(profileLoadBundle{traps: []*TrapDef{second}}))

	probe := &TrapDef{OID: "1.3.6.1.4.1.99998.13", Name: "DELTA-MIB::probe"}
	original[probe.OID] = probe
	t.Cleanup(func() { delete(original, probe.OID) })
	assert.Same(t, probe, idx.lookupLoaded(probe.OID), "publishing one bundle must not replace the complete epoch maps")
}

func TestAddTrapsRejectsNilDefinitionAtomically(t *testing.T) {
	idx := newEpoch()
	valid := &TrapDef{OID: "1.3.6.1.4.1.99998.14", Name: "NIL-MIB::valid"}

	err := idx.addTraps([]*TrapDef{valid, nil})
	require.ErrorContains(t, err, "trap definition at index 1 is nil")
	assert.Nil(t, idx.Lookup(valid.OID))
}

func TestStockHydrationRejectsManifestRouteMismatch(t *testing.T) {
	const actualOID = "1.3.6.1.4.1.99998.20"

	for _, tc := range []struct {
		name      string
		catalogue stockProfileCatalogueEntry
		hydrate   func(*Epoch) error
	}{
		{
			name: "trap_oid",
			catalogue: stockProfileCatalogueEntry{
				File:     "vendor.yaml",
				MIBs:     []string{"VENDOR-MIB"},
				TrapOIDs: []string{"1.3.6.1.4.1.99998.21"},
			},
			hydrate: func(idx *Epoch) error {
				_, err := idx.LookupWithError("1.3.6.1.4.1.99998.21")
				return err
			},
		},
		{
			name: "mib",
			catalogue: stockProfileCatalogueEntry{
				File:     "vendor.yaml",
				MIBs:     []string{"OTHER-MIB"},
				TrapOIDs: []string{actualOID},
			},
			hydrate: func(idx *Epoch) error {
				_, err := idx.LookupWithError(actualOID)
				return err
			},
		},
		{
			name: "metric_rule",
			catalogue: stockProfileCatalogueEntry{
				File:            "vendor.yaml",
				MIBs:            []string{"VENDOR-MIB"},
				MetricRuleNames: []string{"vendor.missing"},
				TrapOIDs:        []string{actualOID},
			},
			hydrate: func(idx *Epoch) error {
				_, err := idx.Definitions([]string{"vendor.missing"})
				return err
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stockDir := filepath.Join(t.TempDir(), "default")
			require.NoError(t, os.MkdirAll(stockDir, 0o755))
			writeProfileYAML(t, stockDir, "vendor.yaml", `
traps:
  - oid: 1.3.6.1.4.1.99998.20
    name: VENDOR-MIB::actual
    category: diagnostic
    severity: info
`)
			writeStockCatalogue(t, stockDir, stockProfileCatalogue{"vendor": tc.catalogue}, false)

			lease, err := NewManager(Paths{StockDir: stockDir}).Acquire()
			require.NoError(t, err)
			t.Cleanup(lease.Close)
			err = tc.hydrate(lease.Epoch())
			require.ErrorContains(t, err, "does not match hydrated profile")
			assert.Nil(t, lease.Epoch().lookupLoaded(actualOID), "a route mismatch must publish none of the bundle")
		})
	}
}

func TestResolveTrapHydratesEveryMIBCandidateForExactName(t *testing.T) {
	stockDir := filepath.Join(t.TempDir(), "default")
	require.NoError(t, os.MkdirAll(stockDir, 0o755))
	writeProfileYAML(t, stockDir, "first.yaml", `
traps:
  - oid: 1.3.6.1.4.1.99998.22
    name: SHARED-MIB::first
    category: diagnostic
    severity: info
`)
	writeProfileYAML(t, stockDir, "second.yaml", `
traps:
  - oid: 1.3.6.1.4.1.99998.23
    name: SHARED-MIB::second
    category: diagnostic
    severity: info
`)
	writeStockCatalogue(t, stockDir, stockProfileCatalogue{
		"first": {
			File:     "first.yaml",
			MIBs:     []string{"SHARED-MIB"},
			TrapOIDs: []string{"1.3.6.1.4.1.99998.22"},
		},
		"second": {
			File:     "second.yaml",
			MIBs:     []string{"SHARED-MIB"},
			TrapOIDs: []string{"1.3.6.1.4.1.99998.23"},
		},
	}, false)

	lease, err := NewManager(Paths{StockDir: stockDir}).Acquire()
	require.NoError(t, err)
	t.Cleanup(lease.Close)
	td, err := lease.Epoch().ResolveTrap("SHARED-MIB::second")
	require.NoError(t, err)
	require.NotNil(t, td)
	assert.Equal(t, "1.3.6.1.4.1.99998.23", td.OID)
}

func TestResolveTrapRejectsLoadedNameThatConflictsWithLazyStockCandidate(t *testing.T) {
	stockDir := filepath.Join(t.TempDir(), "default")
	userDir := t.TempDir()
	require.NoError(t, os.MkdirAll(stockDir, 0o755))
	writeProfileYAML(t, userDir, "site.yaml", `
traps:
  - oid: 1.3.6.1.4.1.99998.24
    name: SHARED-MIB::same
    category: diagnostic
    severity: info
`)
	writeProfileYAML(t, stockDir, "vendor.yaml", `
traps:
  - oid: 1.3.6.1.4.1.99998.25
    name: SHARED-MIB::same
    category: diagnostic
    severity: warning
`)
	writeStockCatalogue(t, stockDir, stockProfileCatalogue{
		"vendor": {
			File:     "vendor.yaml",
			MIBs:     []string{"SHARED-MIB"},
			TrapOIDs: []string{"1.3.6.1.4.1.99998.25"},
		},
	}, false)

	lease, err := NewManager(Paths{UserDirs: []string{userDir}, StockDir: stockDir}).Acquire()
	require.NoError(t, err)
	t.Cleanup(lease.Close)
	_, err = lease.Epoch().ResolveTrap("SHARED-MIB::same")
	require.ErrorContains(t, err, "duplicate trap name")
}

func TestStockMetricCanReferenceTrapInAnotherLazyStockProfile(t *testing.T) {
	for _, prehydrate := range []bool{false, true} {
		t.Run(fmt.Sprintf("prehydrated_%v", prehydrate), func(t *testing.T) {
			stockDir := filepath.Join(t.TempDir(), "default")
			require.NoError(t, os.MkdirAll(stockDir, 0o755))
			writeProfileYAML(t, stockDir, "target.yaml", `
traps:
  - oid: 1.3.6.1.4.1.99998.26
    name: TARGET-MIB::target
    category: diagnostic
    severity: info
`)
			writeProfileYAML(t, stockDir, "metric.yaml", `
charts:
  - id: cross_file
    title: Cross-file metric
    context: snmp.trap.cross_file
    units: events/s
    algorithm: incremental
metrics:
  - name: stock.cross_file
    type: counter
    on_trap: TARGET-MIB::target
    output:
      metric: snmp_trap_cross_file_events
      dimension: events
      chart: cross_file
`)
			writeStockCatalogue(t, stockDir, stockProfileCatalogue{
				"metric": {
					File:            "metric.yaml",
					MetricRuleNames: []string{"stock.cross_file"},
				},
				"target": {
					File:     "target.yaml",
					MIBs:     []string{"TARGET-MIB"},
					TrapOIDs: []string{"1.3.6.1.4.1.99998.26"},
				},
			}, false)

			lease, err := NewManager(Paths{StockDir: stockDir}).Acquire()
			require.NoError(t, err)
			t.Cleanup(lease.Close)
			if prehydrate {
				_, err = lease.Epoch().LookupWithError("1.3.6.1.4.1.99998.26")
				require.NoError(t, err)
			}
			defs, err := lease.Epoch().Definitions([]string{"stock.cross_file"})
			require.NoError(t, err)
			assert.NotNil(t, defs.RulesByName["stock.cross_file"])
		})
	}
}

func TestStockHydrationCoalescesPerFileWithoutBlockingOtherFiles(t *testing.T) {
	const (
		slowOID = "1.3.6.1.4.1.99998.30"
		fastOID = "1.3.6.1.4.1.99998.31"
	)
	idx := newEpoch()
	started := make(chan struct{})
	release := make(chan struct{})
	var slowLoads atomic.Int64
	var fastLoads atomic.Int64
	store := &stockProfileStore{
		files: map[string]stockProfileFile{"slow": {path: "slow.yaml"}, "fast": {path: "fast.yaml"}},
		routes: map[string]stockProfileRoutes{
			"slow": {trapOIDs: []string{slowOID}, mibs: []string{"TEST-MIB"}},
			"fast": {trapOIDs: []string{fastOID}, mibs: []string{"TEST-MIB"}},
		},
		exactRoutes: map[string]string{slowOID: "slow", fastOID: "fast"},
		hydration: map[string]*profileHydration{
			"slow": {},
			"fast": {},
		},
		loadBundle: func(path string, _ [sha256.Size]byte) (profileLoadBundle, error) {
			switch path {
			case "slow.yaml":
				slowLoads.Add(1)
				close(started)
				<-release
				return profileLoadBundle{traps: []*TrapDef{{OID: slowOID, Name: "TEST-MIB::slow"}}}, nil
			case "fast.yaml":
				fastLoads.Add(1)
				return profileLoadBundle{traps: []*TrapDef{{OID: fastOID, Name: "TEST-MIB::fast"}}}, nil
			default:
				return profileLoadBundle{}, fmt.Errorf("unexpected path %q", path)
			}
		},
	}
	idx.stock = store

	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			_, _ = idx.LookupWithError(slowOID)
		})
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("slow hydration did not start")
	}

	fastDone := make(chan error, 1)
	go func() {
		_, err := idx.LookupWithError(fastOID)
		fastDone <- err
	}()
	select {
	case err := <-fastDone:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("independent profile hydration was blocked")
	}
	close(release)
	wg.Wait()

	assert.EqualValues(t, 1, slowLoads.Load())
	assert.EqualValues(t, 1, fastLoads.Load())
	assert.NotNil(t, idx.lookupLoaded(slowOID))
	assert.NotNil(t, idx.lookupLoaded(fastOID))
}

func TestStockDependencyParsingDoesNotBlockUnrelatedPublication(t *testing.T) {
	const (
		dependencyOID = "1.3.6.1.4.1.99998.32"
		fastOID       = "1.3.6.1.4.1.99998.33"
	)
	idx := newEpoch()
	dependencyStarted := make(chan struct{})
	releaseDependency := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseDependency) }) }
	defer release()

	metricBundle := profileLoadBundle{
		metrics: []profileMetricRule{{
			Name:       "stock.dependency",
			Type:       profileMetricTypeCounter,
			OnTrap:     "DEPENDENCY-MIB::target",
			Output:     profileMetricOutput{Metric: "vendor_dependency_events", Dimension: "events", Chart: "dependency_chart"},
			SourceFile: "metric.yaml",
		}},
		charts: []profileMetricChart{{
			ID: "dependency_chart", Title: "Dependency", Context: "snmp.trap.dependency", Units: "events/s", Algorithm: "incremental", SourceFile: "metric.yaml",
		}},
	}
	store := &stockProfileStore{
		files: map[string]stockProfileFile{
			"metric":     {path: "metric.yaml"},
			"dependency": {path: "dependency.yaml"},
			"fast":       {path: "fast.yaml"},
		},
		routes: map[string]stockProfileRoutes{
			"metric":     {metricRuleNames: []string{"stock.dependency"}},
			"dependency": {trapOIDs: []string{dependencyOID}, mibs: []string{"DEPENDENCY-MIB"}},
			"fast":       {trapOIDs: []string{fastOID}, mibs: []string{"FAST-MIB"}},
		},
		exactRoutes:  map[string]string{dependencyOID: "dependency", fastOID: "fast"},
		mibRoutes:    map[string][]string{"DEPENDENCY-MIB": {"dependency"}, "FAST-MIB": {"fast"}},
		metricRoutes: map[string]string{"stock.dependency": "metric"},
		hydration: map[string]*profileHydration{
			"metric":     {},
			"dependency": {},
			"fast":       {},
		},
		loadBundle: func(path string, _ [sha256.Size]byte) (profileLoadBundle, error) {
			switch path {
			case "metric.yaml":
				return metricBundle, nil
			case "dependency.yaml":
				close(dependencyStarted)
				<-releaseDependency
				return profileLoadBundle{traps: []*TrapDef{{
					OID: dependencyOID, Name: "DEPENDENCY-MIB::target", SourceFile: path,
				}}}, nil
			case "fast.yaml":
				return profileLoadBundle{traps: []*TrapDef{{
					OID: fastOID, Name: "FAST-MIB::event", SourceFile: path,
				}}}, nil
			default:
				return profileLoadBundle{}, fmt.Errorf("unexpected path %q", path)
			}
		},
	}
	idx.stock = store

	metricDone := make(chan error, 1)
	go func() {
		_, err := idx.Definitions([]string{"stock.dependency"})
		metricDone <- err
	}()
	select {
	case <-dependencyStarted:
	case <-time.After(time.Second):
		t.Fatal("metric dependency parsing did not start")
	}

	fastDone := make(chan error, 1)
	go func() {
		_, err := idx.LookupWithError(fastOID)
		fastDone <- err
	}()
	blocked := false
	select {
	case err := <-fastDone:
		require.NoError(t, err)
	case <-time.After(time.Second):
		blocked = true
	}

	release()
	require.NoError(t, <-metricDone)
	if blocked {
		require.NoError(t, <-fastDone)
		t.Fatal("dependency parsing held the global publication lock")
	}
}

func TestStockCatalogueIsRequiredAndUnambiguous(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(*testing.T, string)
		want  string
	}{
		{name: "missing", want: "catalogue is missing"},
		{name: "raw_and_zstd", setup: func(t *testing.T, stockDir string) {
			writeStockCatalogue(t, stockDir, stockProfileCatalogue{}, false)
			writeStockCatalogue(t, stockDir, stockProfileCatalogue{}, true)
		}, want: "ambiguous"},
		{name: "gzip", setup: func(t *testing.T, stockDir string) {
			writeGzipFile(t, filepath.Dir(stockDir), "catalogue.json.gz", `{}`)
		}, want: "unsupported gzip"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stockDir := filepath.Join(t.TempDir(), "default")
			require.NoError(t, os.MkdirAll(stockDir, 0o755))
			if tc.setup != nil {
				tc.setup(t, stockDir)
			}
			_, err := NewManager(Paths{StockDir: stockDir}).Acquire()
			require.ErrorContains(t, err, tc.want)
		})
	}
}

func TestStockCatalogueReconcilesPhysicalInventory(t *testing.T) {
	stockDir := filepath.Join(t.TempDir(), "default")
	require.NoError(t, os.MkdirAll(stockDir, 0o755))
	writeProfileYAML(t, stockDir, "orphan.yaml", "traps: []\n")
	writeStockCatalogue(t, stockDir, stockProfileCatalogue{
		"missing": {File: "missing.yaml", MIBs: []string{"MISSING-MIB"}, TrapOIDs: []string{"1.3.6.1.4.1.99998.5"}},
	}, false)

	_, err := NewManager(Paths{StockDir: stockDir}).Acquire()
	require.ErrorContains(t, err, "references missing profile")

	writeProfileYAML(t, stockDir, "missing.yaml", "traps: []\n")
	_, err = NewManager(Paths{StockDir: stockDir}).Acquire()
	require.ErrorContains(t, err, `profile "orphan" is missing from the stock catalogue`)
}

func TestStockCatalogueRejectsAlternateOIDRouteDuplicates(t *testing.T) {
	stockDir := filepath.Join(t.TempDir(), "default")
	require.NoError(t, os.MkdirAll(stockDir, 0o755))
	writeProfileYAML(t, stockDir, "first.yaml", "traps: []\n")
	writeProfileYAML(t, stockDir, "second.yaml", "traps: []\n")
	writeStockCatalogue(t, stockDir, stockProfileCatalogue{
		"first":  {File: "first.yaml", MIBs: []string{"FIRST-MIB"}, TrapOIDs: []string{"1.3.6.1.4.1.99998.7.1"}},
		"second": {File: "second.yaml", MIBs: []string{"SECOND-MIB"}, TrapOIDs: []string{"1.3.6.1.4.1.99998.7.0.1"}},
	}, false)

	_, err := NewManager(Paths{StockDir: stockDir}).Acquire()
	require.ErrorContains(t, err, "routes OID")
}

func TestStockCatalogueRequiresCanonicalRawProfileFilename(t *testing.T) {
	stockDir := filepath.Join(t.TempDir(), "default")
	require.NoError(t, os.MkdirAll(stockDir, 0o755))
	writeProfileYAMLZstd(t, stockDir, "vendor.yaml.zst", "traps: []\n")
	writeStockCatalogue(t, stockDir, stockProfileCatalogue{
		"vendor": {File: "vendor.yaml.zst"},
	}, false)

	_, err := NewManager(Paths{StockDir: stockDir}).Acquire()
	require.ErrorContains(t, err, `invalid file "vendor.yaml.zst"`)
}

func TestUserProfileReplacesCompressedStockByExtensionlessIdentity(t *testing.T) {
	root := t.TempDir()
	stockDir := filepath.Join(root, "default")
	userDir := t.TempDir()
	require.NoError(t, os.MkdirAll(stockDir, 0o755))
	writeProfileYAMLZstd(t, stockDir, "vendor.yaml.zst", `
traps:
  - oid: 1.3.6.1.4.1.99998.6
    name: VENDOR-MIB::stock
    category: diagnostic
    severity: info
`)
	writeStockCatalogue(t, stockDir, stockProfileCatalogue{
		"vendor": {File: "vendor.yaml", MIBs: []string{"VENDOR-MIB"}, TrapOIDs: []string{"1.3.6.1.4.1.99998.6"}},
	}, false)
	writeProfileYAML(t, userDir, "vendor.yaml", `
traps:
  - oid: 1.3.6.1.4.1.99998.7
    name: VENDOR-MIB::user
    category: security
    severity: warning
`)

	lease, err := NewManager(Paths{UserDirs: []string{userDir}, StockDir: stockDir}).Acquire()
	require.NoError(t, err)
	t.Cleanup(lease.Close)
	idx := lease.Epoch()
	assert.Nil(t, idx.Lookup("1.3.6.1.4.1.99998.6"))
	require.NotNil(t, idx.Lookup("1.3.6.1.4.1.99998.7"))
}

func TestUserProfileWithDistinctIdentityAddsAlongsideStock(t *testing.T) {
	stockDir := filepath.Join(t.TempDir(), "default")
	userDir := t.TempDir()
	require.NoError(t, os.MkdirAll(stockDir, 0o755))
	writeProfileYAML(t, stockDir, "vendor.yaml", `
traps:
  - oid: 1.3.6.1.4.1.99998.27
    name: VENDOR-MIB::base
    category: diagnostic
    severity: info
`)
	writeStockCatalogue(t, stockDir, stockProfileCatalogue{
		"vendor": {
			File:     "vendor.yaml",
			MIBs:     []string{"VENDOR-MIB"},
			TrapOIDs: []string{"1.3.6.1.4.1.99998.27"},
		},
	}, false)
	writeProfileYAML(t, userDir, "site.yaml", `
traps:
  - oid: 1.3.6.1.4.1.99998.28
    name: SITE-MIB::addition
    category: security
    severity: warning
`)

	lease, err := NewManager(Paths{UserDirs: []string{userDir}, StockDir: stockDir}).Acquire()
	require.NoError(t, err)
	t.Cleanup(lease.Close)
	assert.NotNil(t, lease.Epoch().Lookup("1.3.6.1.4.1.99998.27"))
	assert.NotNil(t, lease.Epoch().Lookup("1.3.6.1.4.1.99998.28"))
}

func TestUserMetricCollisionWithReferencedStockProfileIsRejected(t *testing.T) {
	stockDir := filepath.Join(t.TempDir(), "default")
	userDir := t.TempDir()
	require.NoError(t, os.MkdirAll(stockDir, 0o755))
	writeProfileYAML(t, stockDir, "vendor.yaml", `
traps:
  - oid: 1.3.6.1.4.1.99998.28
    name: VENDOR-MIB::event
    category: diagnostic
    severity: info
charts:
  - id: stock_chart
    title: Stock chart
    context: snmp.trap.stock_chart
    units: events/s
    algorithm: incremental
metrics:
  - name: shared.rule
    type: counter
    on_trap: VENDOR-MIB::event
    output:
      metric: snmp_trap_stock_events
      dimension: events
      chart: stock_chart
`)
	writeStockCatalogue(t, stockDir, stockProfileCatalogue{
		"vendor": {
			File:            "vendor.yaml",
			MIBs:            []string{"VENDOR-MIB"},
			MetricRuleNames: []string{"shared.rule"},
			TrapOIDs:        []string{"1.3.6.1.4.1.99998.28"},
		},
	}, false)
	writeProfileYAML(t, userDir, "site.yaml", `
charts:
  - id: site_chart
    title: Site chart
    context: snmp.trap.site_chart
    units: events/s
    algorithm: incremental
metrics:
  - name: shared.rule
    type: counter
    on_trap: VENDOR-MIB::event
    output:
      metric: snmp_trap_site_events
      dimension: events
      chart: site_chart
`)

	_, err := NewManager(Paths{UserDirs: []string{userDir}, StockDir: stockDir}).Acquire()
	require.ErrorContains(t, err, `duplicate metric rule "shared.rule"`)
}

func TestMetricRuleCannotReferenceChartFromAnotherProfile(t *testing.T) {
	idx := newEpoch()
	td := testIFMIBLinkDownTrapDef()
	require.NoError(t, idx.addTraps([]*TrapDef{td}))
	require.NoError(t, idx.addBundleAtomic(profileLoadBundle{charts: []profileMetricChart{{
		ID:        "other_chart",
		Title:     "Other chart",
		Context:   "snmp.trap.other_chart",
		Units:     "events/s",
		Algorithm: "incremental",
	}}}))

	err := idx.addBundleAtomic(profileLoadBundle{metrics: []profileMetricRule{{
		Name:   "site.cross_chart",
		Type:   profileMetricTypeCounter,
		OnTrap: td.OID,
		Output: MetricOutput{Metric: "snmp_trap_cross_chart_events", Dimension: "events", Chart: "other_chart"},
	}}})
	require.ErrorContains(t, err, `references unknown chart "other_chart"`)
}

func TestLoadProfileRejectsNonStringPredicateSelectors(t *testing.T) {
	tests := map[string]struct {
		predicate string
		want      string
	}{
		"varbind": {
			predicate: "        varbind: 42\n        field: category",
			want:      "varbind must be a string",
		},
		"field": {
			predicate: "        varbind: ifIndex\n        field: true",
			want:      "field must be a string",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			writeProfileYAML(t, dir, "invalid.yaml", fmt.Sprintf(`
metrics:
  - name: site.invalid_selector
    type: counter
    where:
      - equals: warning
%s
`, tc.predicate))

			_, err := loadProfileBundle(filepath.Join(dir, "invalid.yaml"))
			require.ErrorContains(t, err, tc.want)
		})
	}
}

func TestMetricPredicateRejectsMultipleSelectors(t *testing.T) {
	tests := map[string]string{
		"both_non_empty": `      - field: category
        varbind: ifIndex
        equals: state_change`,
		"empty_varbind": `      - field: category
        varbind: ""
        equals: state_change`,
		"empty_field": `      - field: ""
        varbind: ifIndex
        equals: state_change`,
	}

	for name, predicate := range tests {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			writeProfileYAML(t, dir, "ambiguous.yaml", fmt.Sprintf(`
metrics:
  - name: site.ambiguous_selector
    type: counter
    where:
%s
`, predicate))
			_, err := loadProfileBundle(filepath.Join(dir, "ambiguous.yaml"))
			require.ErrorContains(t, err, "predicate requires exactly one of varbind or field")
		})
	}

	idx := newEpoch()
	td := testIFMIBLinkDownTrapDef()
	require.NoError(t, idx.addTraps([]*TrapDef{td}))
	err := idx.addTestMetricDefinitions([]MetricRule{{
		Name:   "site.ambiguous_selector",
		Type:   profileMetricTypeCounter,
		OnTrap: td.OID,
		Where: MetricPredicates{{
			Field:   "category",
			Varbind: "ifIndex",
			Equals:  "state_change",
		}},
		Output: MetricOutput{Metric: "site_ambiguous_events", Dimension: "events", Chart: "site_ambiguous"},
	}}, []MetricChart{{
		ID:        "site_ambiguous",
		Title:     "Ambiguous selector",
		Context:   "snmp.trap.site.ambiguous",
		Units:     "events/s",
		Algorithm: "incremental",
	}})
	require.ErrorContains(t, err, "predicate requires exactly one of varbind or field")
}

func TestSeparateTrapStateRuleRejectsTransitionPredicates(t *testing.T) {
	tests := map[string]string{
		"set_when": `      set_when:
        field: severity
        equals: warning`,
		"clear_when": `      clear_when:
        field: severity
        equals: notice`,
	}

	for name, statePredicate := range tests {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			writeProfileYAML(t, dir, "invalid.yaml", fmt.Sprintf(`
traps:
  - oid: 1.3.6.1.4.1.99998.15
    name: STATE-MIB::problem
    category: state_change
    severity: warning
  - oid: 1.3.6.1.4.1.99998.16
    name: STATE-MIB::clear
    category: state_change
    severity: notice
metrics:
  - name: site.invalid_state
    type: state
    problem_trap: STATE-MIB::problem
    clear_trap: STATE-MIB::clear
    state:
%s
    output:
      metric: site_invalid_state
      dimension: state
      chart: site_invalid_state
charts:
  - id: site_invalid_state
    title: Invalid state
    context: snmp.trap.site.invalid_state
    units: state
    algorithm: absolute
`, statePredicate))

			bundle, err := loadProfileBundle(filepath.Join(dir, "invalid.yaml"))
			require.NoError(t, err)
			idx := newEpoch()
			require.NoError(t, idx.addTraps(bundle.traps))
			err = idx.addTestMetricDefinitions(bundle.metrics, bundle.charts)
			require.ErrorContains(t, err, "separate-OID state rule")
		})
	}
}

func TestDefinitionsReturnsOnlySelectedRulesAndCharts(t *testing.T) {
	idx := newEpoch()
	td := testIFMIBLinkDownTrapDef()
	require.NoError(t, idx.addTraps([]*TrapDef{td}))
	require.NoError(t, idx.addTestMetricDefinitions([]MetricRule{
		{
			Name:   "site.first",
			Type:   profileMetricTypeCounter,
			OnTrap: td.OID,
			Output: MetricOutput{Metric: "site_first_events", Dimension: "events", Chart: "first_chart"},
		},
		{
			Name:   "site.second",
			Type:   profileMetricTypeCounter,
			OnTrap: td.OID,
			Output: MetricOutput{Metric: "site_second_events", Dimension: "events", Chart: "second_chart"},
		},
	}, []MetricChart{
		{ID: "first_chart", Title: "First", Context: "snmp.trap.first", Units: "events/s", Algorithm: "incremental"},
		{ID: "second_chart", Title: "Second", Context: "snmp.trap.second", Units: "events/s", Algorithm: "incremental"},
	}))

	defs, err := idx.Definitions([]string{"site.first"})
	require.NoError(t, err)
	assert.Equal(t, []string{"site.first"}, slices.Sorted(maps.Keys(defs.RulesByName)))
	assert.Equal(t, []string{"first_chart"}, slices.Sorted(maps.Keys(defs.ChartsByID)))
}

func TestReadProfileFileRejectsOversizedDecompressedProfile(t *testing.T) {
	dir := t.TempDir()
	writeProfileYAMLZstd(t, dir, "oversized.yaml.zst", strings.Repeat("x", 32))

	_, err := readProfileFileLimited(filepath.Join(dir, "oversized.yaml.zst"), 16)
	require.ErrorContains(t, err, "exceeds maximum decompressed size")
}

func TestProfileIndexLookupTrapOIDTolerance(t *testing.T) {
	withZero := &TrapDef{OID: "1.3.6.1.2.1.33.2.0.1", Name: "UPS-MIB::upsTrapOnBattery"}
	withoutZero := &TrapDef{OID: "1.3.6.1.4.1.14179.2.6.3.24", Name: "AIRESPACE-WIRELESS-MIB::bsnAPFunctionalityDisabled"}

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
			idx := &Epoch{trapsByOID: tt.traps}

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
			assert.Equal(t, tt.want, model.AlternateTrapOID(tt.oid))
		})
	}
}

func TestProfileLoadEmptyUserDirKeepsStockProfiles(t *testing.T) {
	dir := t.TempDir()

	setTestDirs(t, dir)
	resetTestEpoch()

	idx, err := acquireTestEpoch()
	require.NoError(t, err)
	t.Cleanup(releaseTestEpoch)
	assert.NotNil(t, idx.Lookup("1.3.6.1.4.1.99999.999"))
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
	resetTestEpoch()

	_, err := acquireTestEpoch()
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
	resetTestEpoch()

	_, err := acquireTestEpoch()
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
	resetTestEpoch()

	_, err := acquireTestEpoch()
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
	resetTestEpoch()

	_, err := acquireTestEpoch()
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
	resetTestEpoch()

	_, err := acquireTestEpoch()
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
	resetTestEpoch()

	_, err := acquireTestEpoch()
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
	resetTestEpoch()

	_, err := acquireTestEpoch()
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
	resetTestEpoch()

	idx, err := acquireTestEpoch()
	require.NoError(t, err)
	defer releaseTestEpoch()

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
	resetTestEpoch()

	_, err := acquireTestEpoch()
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
	resetTestEpoch()

	_, err := acquireTestEpoch()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dedup_key_varbind")
}

func TestProfileLoadEmptyDedupKey(t *testing.T) {
	dir := t.TempDir()
	writeProfileYAML(t, dir, "bad.yaml", `
traps:
  - oid: 1.3.6.1.6.3.1.1.5.3
    name: IF-MIB::linkDown
    category: state_change
    severity: warning
    dedup_key_varbinds: [""]
`)

	setTestDirs(t, dir)
	resetTestEpoch()

	_, err := acquireTestEpoch()
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
	resetTestEpoch()

	_, err := acquireTestEpoch()
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
	resetTestEpoch()

	_, err := acquireTestEpoch()
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
	resetTestEpoch()

	_, err := acquireTestEpoch()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate trap name")
}

func TestProfileLoadRejectsDuplicateUserIdentity(t *testing.T) {
	firstDir := t.TempDir()
	secondDir := t.TempDir()

	writeProfileYAML(t, firstDir, "same.yaml", `
traps:
  - oid: 1.3.6.1.6.3.1.1.5.3
    name: IF-MIB::linkDown
    category: auth
    severity: crit
`)

	writeProfileYAML(t, secondDir, "same.yaml", `
traps:
  - oid: 1.3.6.1.6.3.1.1.5.3
    name: IF-MIB::linkDown
    category: state_change
    severity: warning
`)

	setTestDirs(t, firstDir, secondDir)
	resetTestEpoch()

	_, err := acquireTestEpoch()
	require.ErrorContains(t, err, "duplicate user profile name")
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

	msg := RenderMessage(entry, td)
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
    description: 'Interface {{value "ifDescr"}} (index {{value "ifIndex"}}) went down on {{hostname}}'
    varbinds: [ifIndex, ifDescr]
`)

	setTestDirs(t, dir)
	resetTestEpoch()

	idx, err := acquireTestEpoch()
	require.NoError(t, err)
	defer releaseTestEpoch()

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

	msg := RenderMessage(entry, td)
	assert.Contains(t, msg, "Gi0/1")
	assert.Contains(t, msg, "42")
	assert.Contains(t, msg, "10.0.0.1")
}

func TestRenderMessageResolvesTabularVarbindInstances(t *testing.T) {
	td := testIFMIBLinkDownTrapDef()
	entry := testIFMIBLinkDownEntry()

	msg := RenderMessage(entry, td)

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
    description: 'Test {{value "ifDescr"}}'
    varbinds: [ifDescr]
`)

	setTestDirs(t, dir)
	resetTestEpoch()

	idx, err := acquireTestEpoch()
	require.NoError(t, err)
	defer releaseTestEpoch()

	td := idx.Lookup("1.3.6.1.6.3.1.1.5.3")
	require.NotNil(t, td)

	entry := &TrapEntry{
		TrapName: "IF-MIB::linkDown",
		SourceIP: "10.0.0.1",
		Varbinds: []VarbindValue{},
	}

	msg := RenderMessage(entry, td)
	assert.Equal(t, "Test ", msg)
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
	resetTestEpoch()

	idx, err := acquireTestEpoch()
	require.NoError(t, err)
	defer releaseTestEpoch()

	td := idx.Lookup("1.3.6.1.6.3.1.1.5.3")
	require.NotNil(t, td)

	entry := &TrapEntry{
		TrapName: "IF-MIB::linkDown",
		SourceIP: "198.51.100.10",
		Varbinds: []VarbindValue{
			{OID: "1.3.6.1.2.1.2.2.1.1.7", Type: "INTEGER", Value: int64(7)},
		},
	}

	msg := RenderMessage(entry, td)
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
	resetTestEpoch()

	idx, err := acquireTestEpoch()
	require.NoError(t, err)
	defer releaseTestEpoch()

	td := idx.Lookup("1.3.6.1.6.3.1.1.5.4")
	require.NotNil(t, td)

	entry := &TrapEntry{TrapName: "IF-MIB::linkUp", SourceIP: "198.51.100.10"}
	assert.Equal(t, "Interface came up on 198.51.100.10.", RenderMessage(entry, td))

	entry.Varbinds = []VarbindValue{{OID: "1.3.6.1.2.1.2.2.1.2.7", Type: "OctetString", Value: "Gi0/7"}}
	assert.Equal(t, "Interface Gi0/7 came up on 198.51.100.10.", RenderMessage(entry, td))
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
	resetTestEpoch()

	idx, err := acquireTestEpoch()
	require.NoError(t, err)
	defer releaseTestEpoch()

	td := idx.Lookup("1.3.6.1.4.1.9.9.43.2.0.2")
	require.NotNil(t, td)

	entry := &TrapEntry{TrapName: "CISCO-CONFIG-MAN-MIB::ccmCLIRunningConfigChanged", SourceIP: "198.51.100.10"}
	assert.Equal(t, "Running configuration changed on 198.51.100.10.", RenderMessage(entry, td))

	entry.Varbinds = []VarbindValue{{OID: "1.3.6.1.4.1.9.9.43.1.1.6.1.8.1", Type: "DisplayString", Value: "admin"}}
	assert.Equal(t, "Running configuration changed by admin on 198.51.100.10.", RenderMessage(entry, td))
}

func TestRenderGoTemplateMessageRedactsSensitiveCommunityVarbind(t *testing.T) {
	entry := &TrapEntry{
		SourceIP: "198.51.100.10",
		Varbinds: []VarbindValue{
			{OID: model.SNMPTrapCommunityOID, Name: "snmpTrapCommunity.0", Type: "OctetString", Value: "private-community"},
		},
	}
	td := testSensitiveCommunityTrapDef(t, `Community {{value "snmpTrapCommunity"}} raw {{raw "snmpTrapCommunity"}}`)

	msg := RenderMessage(entry, td)

	assert.NotContains(t, msg, "private-community")
	assert.Contains(t, msg, model.RedactedVarbindValue)
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
	resetTestEpoch()

	_, err := acquireTestEpoch()
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
			resetTestEpoch()
			_, err := acquireTestEpoch()
			require.Error(t, err)
		})
	}
}

func TestLoadProfileRejectsLiteralBraces(t *testing.T) {
	for name, content := range map[string]string{
		"legacy description": "description: 'Interface {ifIndex}'",
		"mixed description":  "description: '{{trap_name}} on {hostname}'",
		"legacy label":       "labels:\n      state: '{ifIndex}'",
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			writeProfileYAML(t, dir, "test.yaml", `
varbinds:
  ifIndex:
    oid: 1.3.6.1.2.1.2.2.1.1
    type: INTEGER
    constraints: (1..4)
traps:
  - oid: 1.3.6.1.6.3.1.1.5.3
    name: IF-MIB::linkDown
    category: state_change
    severity: warning
    varbinds: [ifIndex]
    `+content+"\n")
			setTestDirs(t, dir)
			resetTestEpoch()
			_, err := acquireTestEpoch()
			require.ErrorContains(t, err, "literal braces are not allowed")
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
	resetTestEpoch()
	_, err := acquireTestEpoch()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unbounded varbind")
}

func testSensitiveCommunityTrapDef(t *testing.T, description string) *TrapDef {
	t.Helper()
	oid := strings.TrimSuffix(model.SNMPTrapCommunityOID, ".0")
	def := VarbindDef{OID: oid, Type: "OctetString", RawName: "snmpTrapCommunity"}
	td := &TrapDef{
		Description: description,
		SharedVarbinds: map[string]*VarbindDef{
			oid: &def,
		},
	}
	require.NoError(t, compileTrapTemplates(td, map[string]VarbindDef{"snmpTrapCommunity": def}))
	return td
}

func TestFindVarbindForProfileOIDExactMatchWins(t *testing.T) {
	entry := &TrapEntry{
		Varbinds: []VarbindValue{
			{OID: testIFMIBIfIndexOID + ".1", Type: "INTEGER", Value: int64(1)},
			{OID: testIFMIBIfIndexOID, Type: "INTEGER", Value: int64(99)},
		},
	}

	got, ok := model.FindVarbindForProfileOID(entry.Varbinds, testIFMIBIfIndexOID)

	require.True(t, ok)
	assert.Equal(t, testIFMIBIfIndexOID, got.OID)
	assert.Equal(t, int64(99), got.Value)
}

func TestOIDMatchesColumnRequiresArcBoundary(t *testing.T) {
	assert.True(t, model.OIDMatchesColumn(testIFMIBIfOperStatusOID, testIFMIBIfOperStatusOID+".1"))
	assert.False(t, model.OIDMatchesColumn(testIFMIBIfOperStatusOID, testIFMIBIfOperStatusOID))
	assert.False(t, model.OIDMatchesColumn(testIFMIBIfOperStatusOID, testIFMIBIfOperStatusOID+"0.1"))
}

func TestFindVarbindForProfileOIDFirstMatchingInstanceWins(t *testing.T) {
	entry := &TrapEntry{
		Varbinds: []VarbindValue{
			{OID: testIFMIBIfIndexOID + ".2", Type: "INTEGER", Value: int64(2)},
			{OID: testIFMIBIfIndexOID + ".1", Type: "INTEGER", Value: int64(1)},
		},
	}

	got, ok := model.FindVarbindForProfileOID(entry.Varbinds, testIFMIBIfIndexOID)

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

	got, ok := model.FindVarbindForProfileOID(entry.Varbinds, sysNameOID)

	require.True(t, ok)
	assert.Equal(t, sysNameOID+".0", got.OID)
	assert.Equal(t, "switch01", got.Value)
}

func TestFindVarbindDefForObservedOIDUsesLongestColumnPrefix(t *testing.T) {
	td := &TrapDef{
		SharedVarbinds: map[string]*VarbindDef{
			"1.3.6.1.4.1.999.1": {
				OID:     "1.3.6.1.4.1.999.1",
				Type:    "INTEGER",
				RawName: "shortColumn",
			},
			"1.3.6.1.4.1.999.1.1": {
				OID:     "1.3.6.1.4.1.999.1.1",
				Type:    "INTEGER",
				RawName: "longColumn",
			},
		},
	}

	got := FindVarbindDefForObservedOID(td, "1.3.6.1.4.1.999.1.1.7")

	require.NotNil(t, got)
	assert.Equal(t, "longColumn", got.RawName)
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
    description: 'Alias [{{value "ifAlias"}}]'
    varbinds: [ifAlias]
`)

	setTestDirs(t, dir)
	resetTestEpoch()

	idx, err := acquireTestEpoch()
	require.NoError(t, err)
	defer releaseTestEpoch()

	td := idx.Lookup("1.3.6.1.6.3.1.1.5.3")
	require.NotNil(t, td)

	entry := &TrapEntry{
		TrapName: "IF-MIB::linkDown",
		SourceIP: "10.0.0.1",
		Varbinds: []VarbindValue{
			{OID: "1.3.6.1.2.1.31.1.1.1.18", Type: "OctetString", Value: ""},
		},
	}

	msg := RenderMessage(entry, td)
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
    description: 'OperStatus is {{value "ifOperStatus"}}'
    varbinds: [ifOperStatus]
`)

	setTestDirs(t, dir)
	resetTestEpoch()

	idx, err := acquireTestEpoch()
	require.NoError(t, err)
	defer releaseTestEpoch()

	td := idx.Lookup("1.3.6.1.6.3.1.1.5.3")
	require.NotNil(t, td)

	entry := &TrapEntry{
		TrapName: "IF-MIB::linkDown",
		SourceIP: "10.0.0.1",
		Varbinds: []VarbindValue{
			{OID: "1.3.6.1.2.1.2.2.1.8", Type: "INTEGER", Value: int64(2)},
		},
	}

	msg := RenderMessage(entry, td)
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
    description: 'Raw: {{raw "ifOperStatus"}}'
    varbinds: [ifOperStatus]
`)

	setTestDirs(t, dir)
	resetTestEpoch()

	idx, err := acquireTestEpoch()
	require.NoError(t, err)
	defer releaseTestEpoch()

	td := idx.Lookup("1.3.6.1.6.3.1.1.5.3")
	require.NotNil(t, td)

	entry := &TrapEntry{
		TrapName: "IF-MIB::linkDown",
		SourceIP: "10.0.0.1",
		Varbinds: []VarbindValue{
			{OID: "1.3.6.1.2.1.2.2.1.8", Type: "INTEGER", Value: int64(2)},
		},
	}

	msg := RenderMessage(entry, td)
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

	msg := RenderMessage(entry, td)
	assert.LessOrEqual(t, len(msg), MaxMessageLen)
}

func TestRenderMessageTruncationKeepsValidUTF8(t *testing.T) {
	entry := &TrapEntry{TrapName: "X::Y", SourceIP: "1.1.1.1"}
	td := &TrapDef{Description: strings.Repeat("é", 300)}

	msg := RenderMessage(entry, td)
	assert.LessOrEqual(t, len(msg), MaxMessageLen)
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
      oper_status: '{{value "ifOperStatus"}}'
      severity: "warning"
    varbinds: [ifOperStatus]
`)

	setTestDirs(t, dir)
	resetTestEpoch()

	idx, err := acquireTestEpoch()
	require.NoError(t, err)
	defer releaseTestEpoch()

	td := idx.Lookup("1.3.6.1.6.3.1.1.5.3")
	require.NotNil(t, td)

	entry := &TrapEntry{
		TrapName: "IF-MIB::linkDown",
		SourceIP: "10.0.0.1",
		Varbinds: []VarbindValue{
			{OID: "1.3.6.1.2.1.2.2.1.8", Type: "INTEGER", Value: int64(2)},
		},
	}

	labels := RenderLabels(entry, td)
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
      interface: '{{value "ifDescr"}}'
    varbinds: [ifDescr]
`)

	setTestDirs(t, dir)
	resetTestEpoch()

	_, err := acquireTestEpoch()
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
		Description: "Host: {{hostname}}, IP: {{source_ip}}, Name: {{trap_name}}, Vendor: {{vendor}}, Interface: {{trap_interface}}, Neighbors: {{trap_neighbors}}",
	}
	require.NoError(t, compileTrapTemplates(td, nil))

	msg := RenderMessage(entry, td)
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
		Description: "Host: {{hostname}}",
	}
	require.NoError(t, compileTrapTemplates(td, nil))

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			msg := RenderMessage(tc.entry, td)
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
	resetTestEpoch()

	idx, err := acquireTestEpoch()
	require.NoError(t, err)
	defer releaseTestEpoch()

	td := idx.Lookup("1.3.6.1.6.3.1.1.5.3")
	require.NotNil(t, td)

	raw := VarbindValue{OID: "1.3.6.1.2.1.31.1.1.1.1", Type: "OctetString", Value: "Gi0/1"}
	resolved := ResolveVarbind("1.3.6.1.2.1.31.1.1.1.1", raw, td)
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

	resolved := ResolveVarbind("1.3.6.1.4.1.99999.1.1", raw, nil)
	assert.Equal(t, "customVarbind", resolved.Name)
	assert.Equal(t, ASN1Type("Counter32"), resolved.Type)
}

func TestResolve2TierRawFallbackNoName(t *testing.T) {
	raw := VarbindValue{
		OID:   "1.3.6.1.4.1.99999.1.1",
		Type:  "Counter32",
		Value: int64(12345),
	}

	resolved := ResolveVarbind("1.3.6.1.4.1.99999.1.1", raw, nil)
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
	resetTestEpoch()

	idx, err := acquireTestEpoch()
	require.NoError(t, err)
	defer releaseTestEpoch()

	td := idx.Lookup("1.3.6.1.6.3.1.1.5.3")
	require.NotNil(t, td)

	raw := VarbindValue{OID: "1.3.6.1.2.1.2.2.1.8", Type: "INTEGER", Value: int64(2)}
	resolved := ResolveVarbind("1.3.6.1.2.1.2.2.1.8", raw, td)
	assert.Equal(t, "down", resolved.Enum)
}

func TestResolve2TierResolvesTabularVarbindInstance(t *testing.T) {
	td := testIFMIBLinkDownTrapDef()
	raw := VarbindValue{OID: testIFMIBIfOperStatusOID + ".1", Type: "INTEGER", Value: int64(2)}

	resolved := ResolveVarbind(raw.OID, raw, td)

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
	stockDir := testStockProfilesDir(t)
	lease, err := NewManager(Paths{StockDir: stockDir}).Acquire()
	require.NoError(t, err)
	t.Cleanup(lease.Close)
	idx := lease.Epoch()

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
	stockDir := testStockProfilesDir(t)
	cataloguePath := filepath.Join(filepath.Dir(stockDir), "catalogue.json")
	data, err := os.ReadFile(cataloguePath)
	require.NoError(t, err)

	var catalogue map[string]struct {
		File            string   `json:"file"`
		MIBs            []string `json:"mibs"`
		MetricRuleNames []string `json:"metric_rule_names"`
		SHA256          string   `json:"sha256"`
		TrapCount       int      `json:"trap_count"`
		TrapOIDs        []string `json:"trap_oids"`
	}
	require.NoError(t, json.Unmarshal(data, &catalogue))
	require.NotEmpty(t, catalogue)

	entries, err := os.ReadDir(stockDir)
	require.NoError(t, err)
	require.NotEmpty(t, entries)

	routesByFile := make(map[string]profileTrapRoutes, len(entries))
	totalFiles := 0
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !isProfileFileName(name) || strings.HasPrefix(name, "_") {
			continue
		}
		require.False(t, strings.HasSuffix(name, ".zst"), "stock profiles must stay uncompressed in the repository")
		require.False(t, strings.HasSuffix(name, ".gz"), "stock profiles must stay uncompressed in the repository")
		routes := profileTrapRoutesFromFile(t, filepath.Join(stockDir, name))
		routesByFile[name] = routes
		totalFiles += len(routes.oids)
	}
	require.Len(t, routesByFile, len(catalogue), "catalogue and default profile file count must match")

	totalCatalogue := 0
	for vendor, entry := range catalogue {
		require.NotEmpty(t, entry.File, "catalogue entry %q has no file", vendor)
		routes, ok := routesByFile[entry.File]
		require.True(t, ok, "catalogue entry %q references missing profile file %q", vendor, entry.File)
		profileData, err := os.ReadFile(filepath.Join(stockDir, entry.File))
		require.NoError(t, err)
		assert.Equal(t, fmt.Sprintf("%x", sha256.Sum256(profileData)), entry.SHA256,
			"catalogue sha256 mismatch for %s", entry.File)
		assert.Equal(t, entry.TrapCount, len(routes.oids), "catalogue trap_count mismatch for %s", entry.File)
		assert.Equal(t, routes.oids, entry.TrapOIDs, "catalogue trap_oids mismatch for %s", entry.File)
		bundle, err := loadProfileBundle(filepath.Join(stockDir, entry.File))
		require.NoError(t, err, "stock profile %s", entry.File)
		expected := stockProfileRoutes{
			trapOIDs:        append([]string(nil), entry.TrapOIDs...),
			mibs:            append([]string(nil), entry.MIBs...),
			metricRuleNames: append([]string(nil), entry.MetricRuleNames...),
		}
		slices.Sort(expected.trapOIDs)
		slices.Sort(expected.mibs)
		slices.Sort(expected.metricRuleNames)
		require.NoError(t, validateStockProfileRoutes(vendor, expected, bundle), "stock profile %s", entry.File)
		totalCatalogue += entry.TrapCount
	}
	assert.Equal(t, totalFiles, totalCatalogue, "catalogue trap_count sum must match profile files")
}

func TestStockProfileDefaultFilesParse(t *testing.T) {
	stockDir := testStockProfilesDir(t)
	entries, err := os.ReadDir(stockDir)
	require.NoError(t, err)
	require.NotEmpty(t, entries)

	totalTraps := 0
	for _, entry := range entries {
		if entry.IsDir() || !isProfileFileName(entry.Name()) || strings.HasPrefix(entry.Name(), "_") {
			continue
		}
		bundle, err := loadProfileBundle(filepath.Join(stockDir, entry.Name()))
		require.NoError(t, err, "stock profile %s", entry.Name())
		require.NotEmpty(t, bundle.traps, "stock profile %s parsed without traps", entry.Name())
		totalTraps += len(bundle.traps)
	}
	require.Positive(t, totalTraps)
}

func testStockProfilesDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Clean("../../../../config/go.d/snmp.trap-profiles/default")
	info, err := os.Stat(dir)
	require.NoError(t, err)
	require.True(t, info.IsDir())
	return dir
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
	td := &TrapDef{
		OID:         testIFMIBLinkDownOID,
		Name:        "IF-MIB::linkDown",
		Category:    "state_change",
		Severity:    "warning",
		Description: `Link {{value "ifIndex"}} operational state changed to {{value "ifOperStatus"}} on {{hostname}}.`,
		VarbindRefs: []any{"ifIndex", "ifAdminStatus", "ifOperStatus"},
		SharedVarbinds: map[string]*VarbindDef{
			testIFMIBIfIndexOID: {
				OID:     testIFMIBIfIndexOID,
				Type:    "INTEGER",
				RawName: "ifIndex",
			},
			testIFMIBIfAdminStatusOID: {
				OID:     testIFMIBIfAdminStatusOID,
				Type:    "INTEGER",
				RawName: "ifAdminStatus",
				Enum: map[string]string{
					"1": "up",
					"2": "down",
					"3": "testing",
				},
			},
			testIFMIBIfOperStatusOID: {
				OID:     testIFMIBIfOperStatusOID,
				Type:    "INTEGER",
				RawName: "ifOperStatus",
				Enum: map[string]string{
					"1": "up",
					"2": "down",
					"3": "testing",
				},
			},
		},
	}
	fileVarbinds := make(map[string]VarbindDef, len(td.SharedVarbinds))
	for _, vb := range td.SharedVarbinds {
		fileVarbinds[vb.RawName] = *vb
	}
	if err := compileTrapTemplates(td, fileVarbinds); err != nil {
		panic(err)
	}
	return td
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

func writeProfileYAML(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	err := os.WriteFile(path, []byte(content), 0644)
	require.NoError(t, err)
}

func writeProfileYAMLZstd(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	file, err := os.Create(path)
	require.NoError(t, err)
	defer file.Close()

	zw, err := zstd.NewWriter(file)
	require.NoError(t, err)
	_, err = zw.Write([]byte(content))
	require.NoError(t, err)
	require.NoError(t, zw.Close())
}

func writeStockCatalogue(t *testing.T, stockDir string, catalogue stockProfileCatalogue, compressed bool) {
	t.Helper()
	for owner, entry := range catalogue {
		if entry.SHA256 != "" {
			continue
		}
		path := filepath.Join(stockDir, entry.File)
		if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
			path += ".zst"
		}
		content, err := readProfileFile(path)
		if errors.Is(err, os.ErrNotExist) {
			entry.SHA256 = strings.Repeat("0", sha256.Size*2)
		} else {
			require.NoError(t, err, "read stock profile for catalogue entry %q", owner)
			entry.SHA256 = fmt.Sprintf("%x", sha256.Sum256(content))
		}
		catalogue[owner] = entry
	}
	data, err := json.Marshal(catalogue)
	require.NoError(t, err)
	if compressed {
		writeProfileYAMLZstd(t, filepath.Dir(stockDir), "catalogue.json.zst", string(data))
		return
	}
	require.NoError(t, os.WriteFile(filepath.Join(filepath.Dir(stockDir), "catalogue.json"), data, 0o644))
}

func writeGzipFile(t *testing.T, dir, name, content string) {
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
