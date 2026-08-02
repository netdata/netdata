// SPDX-License-Identifier: GPL-3.0-or-later

package catalog

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/netdata/netdata/go/plugins/pkg/multipath"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/model"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/profilecatalog"
)

type stockProfileStore struct {
	extendsPaths multipath.MultiPath
	loadBundle   func(string, multipath.MultiPath, []string) (profileLoadBundle, error)
	files        map[string]string
	routes       map[string]stockProfileRoutes
	exactRoutes  map[string]string
	mibRoutes    map[string][]string
	metricRoutes map[string]string
	hydration    map[string]*profileHydration
}

type stockProfileRoutes struct {
	trapOIDs        []string
	mibs            []string
	metricRuleNames []string
}

type profileHydration struct {
	once sync.Once
	err  error
}

func (s *stockProfileStore) empty() bool {
	return s == nil || len(s.files) == 0
}

type stockProfileCatalogue map[string]stockProfileCatalogueEntry

type stockProfileCatalogueEntry struct {
	File            string   `json:"file"`
	MIBs            []string `json:"mibs"`
	MetricRuleNames []string `json:"metric_rule_names,omitempty"`
	TrapOIDs        []string `json:"trap_oids"`
}

func buildStockProfileStore(
	dir string,
	extendsPaths multipath.MultiPath,
	sources profilecatalog.Catalog[profileSource],
	idx *Epoch,
) (*stockProfileStore, error) {
	catalogue, err := loadStockProfileCatalogue(dir)
	if err != nil {
		return nil, err
	}

	store := &stockProfileStore{
		extendsPaths: extendsPaths,
		loadBundle:   loadProfileBundle,
		files:        make(map[string]string),
		routes:       make(map[string]stockProfileRoutes),
		exactRoutes:  make(map[string]string),
		mibRoutes:    make(map[string][]string),
		metricRoutes: make(map[string]string),
		hydration:    make(map[string]*profileHydration),
	}
	manifestFiles := make(map[string]string, len(catalogue))
	for _, owner := range sortedCatalogueOwners(catalogue) {
		if !profileIdentityRE.MatchString(owner) {
			return nil, fmt.Errorf("stock trap profile catalogue has invalid identity %q", owner)
		}
		entry := catalogue[owner]
		identity, ok := parseManifestProfileFile(entry.File)
		if !ok {
			return nil, fmt.Errorf("stock trap profile catalogue entry %q has invalid file %q", owner, entry.File)
		}
		if previous := manifestFiles[identity]; previous != "" {
			return nil, fmt.Errorf("stock trap profile catalogue entries %q and %q reference profile %q", previous, owner, identity)
		}
		manifestFiles[identity] = owner
		if !sources.HasStock(identity) {
			return nil, fmt.Errorf("stock trap profile catalogue entry %q references missing profile %q", owner, entry.File)
		}

		// A user profile with the same extensionless identity replaces the stock
		// profile completely, including its manifest routes.
		if !sources.EffectiveIsStock(identity) {
			continue
		}
		source, _ := sources.Get(identity)
		store.files[identity] = source.path
		store.hydration[identity] = &profileHydration{}
		routes := stockProfileRoutes{}

		for _, oid := range entry.TrapOIDs {
			if !model.IsNumericOID(oid) {
				return nil, fmt.Errorf("stock trap profile catalogue entry %q contains invalid trap OID %q", owner, oid)
			}
			if td := idx.lookupLoaded(oid); td != nil {
				return nil, fmt.Errorf("%s: duplicate trap OID %s (already defined in %s)", source.path, oid, td.SourceFile)
			}
			previous := store.exactRoutes[oid]
			if previous == "" {
				previous = store.exactRoutes[model.AlternateTrapOID(oid)]
			}
			if previous != "" {
				return nil, fmt.Errorf("stock trap profile catalogue routes OID %s to both %q and %q", oid, previous, identity)
			}
			store.exactRoutes[oid] = identity
			routes.trapOIDs = append(routes.trapOIDs, oid)
		}
		seenMIBs := make(map[string]bool, len(entry.MIBs))
		for _, mib := range entry.MIBs {
			mib = strings.TrimSpace(mib)
			if mib == "" {
				return nil, fmt.Errorf("stock trap profile catalogue entry %q contains an empty MIB name", owner)
			}
			if seenMIBs[mib] {
				return nil, fmt.Errorf("stock trap profile catalogue entry %q contains duplicate MIB name %q", owner, mib)
			}
			seenMIBs[mib] = true
			store.mibRoutes[mib] = append(store.mibRoutes[mib], identity)
			routes.mibs = append(routes.mibs, mib)
		}
		for _, rule := range entry.MetricRuleNames {
			rule = strings.TrimSpace(rule)
			if rule == "" {
				return nil, fmt.Errorf("stock trap profile catalogue entry %q contains an empty metric rule name", owner)
			}
			if previous := store.metricRoutes[rule]; previous != "" {
				return nil, fmt.Errorf("stock trap profile catalogue routes metric rule %q to both %q and %q", rule, previous, identity)
			}
			store.metricRoutes[rule] = identity
			routes.metricRuleNames = append(routes.metricRuleNames, rule)
		}
		slices.Sort(routes.trapOIDs)
		slices.Sort(routes.mibs)
		slices.Sort(routes.metricRuleNames)
		store.routes[identity] = routes
	}

	for _, name := range sources.StockNames() {
		if manifestFiles[name] == "" {
			return nil, fmt.Errorf("stock trap profile %q is missing from the stock catalogue", name)
		}
	}
	for mib := range store.mibRoutes {
		slices.Sort(store.mibRoutes[mib])
	}
	return store, nil
}

func loadStockProfileCatalogue(stockDir string) (stockProfileCatalogue, error) {
	data, path, err := readStockProfileCatalogueFile(filepath.Dir(stockDir))
	if err != nil {
		return nil, err
	}
	var catalogue stockProfileCatalogue
	if err := json.Unmarshal(data, &catalogue); err != nil {
		return nil, fmt.Errorf("invalid stock trap profile catalogue %q: %w", path, err)
	}
	if len(catalogue) == 0 {
		return nil, fmt.Errorf("stock trap profile catalogue %q is empty", path)
	}
	return catalogue, nil
}

func readStockProfileCatalogueFile(dir string) ([]byte, string, error) {
	if _, err := os.Stat(filepath.Join(dir, "catalogue.json.gz")); err == nil {
		return nil, "", fmt.Errorf("unsupported gzip stock trap profile catalogue in %q", dir)
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, "", err
	}

	var found []string
	for _, name := range []string{"catalogue.json", "catalogue.json.zst"} {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err == nil {
			found = append(found, path)
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, "", err
		}
	}
	if len(found) == 0 {
		return nil, "", fmt.Errorf("stock trap profile catalogue is missing from %q", dir)
	}
	if len(found) != 1 {
		return nil, "", fmt.Errorf("stock trap profile catalogue is ambiguous in %q: found both raw and zstd files", dir)
	}
	data, err := readCatalogueFile(found[0])
	if err != nil {
		return nil, "", err
	}
	return data, found[0], nil
}

func sortedCatalogueOwners(catalogue stockProfileCatalogue) []string {
	owners := make([]string, 0, len(catalogue))
	for owner := range catalogue {
		owners = append(owners, owner)
	}
	slices.Sort(owners)
	return owners
}

func parseManifestProfileFile(name string) (string, bool) {
	if filepath.Base(name) != name || strings.ContainsRune(name, '\\') {
		return "", false
	}
	identity := strings.TrimSuffix(strings.TrimSuffix(name, ".yaml"), ".yml")
	ok := identity != name
	return identity, ok && profileIdentityRE.MatchString(identity)
}

func (idx *Epoch) loadStockForOID(oid string) error {
	if idx == nil || idx.stock == nil {
		return nil
	}
	for _, candidate := range []string{oid, model.AlternateTrapOID(oid)} {
		if name := idx.stock.exactRoutes[candidate]; name != "" {
			return idx.stock.hydrate(idx, name)
		}
	}
	return nil
}

func (idx *Epoch) loadStockMetricRules(names []string) error {
	if idx == nil || idx.stock == nil {
		return nil
	}
	for _, name := range names {
		if profile := idx.stock.metricRoutes[name]; profile != "" {
			if err := idx.stock.hydrate(idx, profile); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *stockProfileStore) loadForTrapName(idx *Epoch, ref string) error {
	mib, _, ok := strings.Cut(ref, "::")
	if !ok || mib == "" {
		return nil
	}
	for _, name := range s.mibRoutes[mib] {
		if err := s.hydrate(idx, name); err != nil {
			return err
		}
	}
	return nil
}

func (s *stockProfileStore) hydrate(idx *Epoch, name string) error {
	state := s.hydration[name]
	if state == nil {
		return fmt.Errorf("stock trap profile %q has no hydration state", name)
	}
	state.once.Do(func() {
		path := s.files[name]
		if path == "" {
			state.err = fmt.Errorf("stock trap profile %q has no file", name)
			return
		}
		loadBundle := s.loadBundle
		if loadBundle == nil {
			loadBundle = loadProfileBundle
		}
		bundle, err := loadBundle(path, s.extendsPaths, nil)
		if err != nil {
			state.err = fmt.Errorf("failed to hydrate stock trap profile %q: %w", path, err)
			return
		}
		if err := validateStockProfileRoutes(name, s.routes[name], bundle); err != nil {
			state.err = err
			return
		}
		if err := idx.addBundleAtomic(bundle); err != nil {
			state.err = err
		}
	})
	return state.err
}

func validateStockProfileRoutes(name string, expected stockProfileRoutes, bundle profileLoadBundle) error {
	actual := stockProfileRoutes{}
	mibs := make(map[string]struct{})
	for _, trap := range bundle.traps {
		if trap == nil {
			continue
		}
		actual.trapOIDs = append(actual.trapOIDs, trap.OID)
		if mib, _, ok := strings.Cut(trap.Name, "::"); ok {
			mibs[mib] = struct{}{}
		}
	}
	for mib := range mibs {
		actual.mibs = append(actual.mibs, mib)
	}
	for _, rule := range bundle.metrics {
		actual.metricRuleNames = append(actual.metricRuleNames, rule.Name)
	}
	slices.Sort(actual.trapOIDs)
	slices.Sort(actual.mibs)
	slices.Sort(actual.metricRuleNames)

	if !slices.Equal(expected.trapOIDs, actual.trapOIDs) {
		return fmt.Errorf("stock trap profile %q manifest does not match hydrated profile for trap_oids: manifest=%v profile=%v", name, expected.trapOIDs, actual.trapOIDs)
	}
	if !slices.Equal(expected.mibs, actual.mibs) {
		return fmt.Errorf("stock trap profile %q manifest does not match hydrated profile for mibs: manifest=%v profile=%v", name, expected.mibs, actual.mibs)
	}
	if !slices.Equal(expected.metricRuleNames, actual.metricRuleNames) {
		return fmt.Errorf("stock trap profile %q manifest does not match hydrated profile for metric_rule_names: manifest=%v profile=%v", name, expected.metricRuleNames, actual.metricRuleNames)
	}
	return nil
}
