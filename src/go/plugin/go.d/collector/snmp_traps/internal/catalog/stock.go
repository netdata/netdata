// SPDX-License-Identifier: GPL-3.0-or-later

package catalog

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/model"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/profilecatalog"
)

type stockProfileStore struct {
	loadBundle   func(string, [sha256.Size]byte) (profileLoadBundle, error)
	files        map[string]stockProfileFile
	routes       map[string]stockProfileRoutes
	exactRoutes  map[string]string
	mibRoutes    map[string][]string
	metricRoutes map[string]string
	hydration    map[string]*profileHydration
}

type stockProfileFile struct {
	path   string
	sha256 [sha256.Size]byte
}

type stockProfileRoutes struct {
	trapOIDs        []string
	mibs            []string
	metricRuleNames []string
}

type profileHydration struct {
	parseOnce   sync.Once
	publishOnce sync.Once
	bundle      profileLoadBundle
	parseErr    error
	publishErr  error
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
	SHA256          string   `json:"sha256"`
}

func buildStockProfileStore(
	dir string,
	sources profilecatalog.Catalog[profileSource],
	idx *Epoch,
) (*stockProfileStore, error) {
	catalogue, err := loadStockProfileCatalogue(dir)
	if err != nil {
		return nil, err
	}

	store := &stockProfileStore{
		loadBundle:   loadStockProfileBundle,
		files:        make(map[string]stockProfileFile),
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
		digest, err := parseStockProfileSHA256(entry.SHA256)
		if err != nil {
			return nil, fmt.Errorf("stock trap profile catalogue entry %q has invalid sha256: %w", owner, err)
		}
		if !sources.HasStock(identity) {
			return nil, fmt.Errorf("stock trap profile catalogue entry %q references missing profile %q", owner, entry.File)
		}

		source, _ := sources.Get(identity)
		// A user profile with the same extensionless identity fully replaces
		// the stock profile and therefore owns the effective routes.
		if !sources.EffectiveIsStock(identity) {
			continue
		}
		store.files[identity] = stockProfileFile{path: source.path, sha256: digest}
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

func parseStockProfileSHA256(value string) ([sha256.Size]byte, error) {
	var digest [sha256.Size]byte
	if len(value) != sha256.Size*2 || value != strings.ToLower(value) {
		return digest, fmt.Errorf("must be %d lowercase hexadecimal characters", sha256.Size*2)
	}
	decoded, err := hex.DecodeString(value)
	if err != nil {
		return digest, fmt.Errorf("must be %d lowercase hexadecimal characters", sha256.Size*2)
	}
	copy(digest[:], decoded)
	return digest, nil
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
	state.publishOnce.Do(func() {
		bundle, err := s.parse(name)
		if err != nil {
			state.publishErr = err
			return
		}
		state.publishErr = idx.addBundleAtomic(bundle, name)
	})
	return state.publishErr
}

func (s *stockProfileStore) parse(name string) (profileLoadBundle, error) {
	state := s.hydration[name]
	if state == nil {
		return profileLoadBundle{}, fmt.Errorf("stock trap profile %q has no hydration state", name)
	}
	state.parseOnce.Do(func() {
		file := s.files[name]
		if file.path == "" {
			state.parseErr = fmt.Errorf("stock trap profile %q has no file", name)
			return
		}
		loadBundle := s.loadBundle
		if loadBundle == nil {
			loadBundle = loadStockProfileBundle
		}
		bundle, err := loadBundle(file.path, file.sha256)
		if err != nil {
			state.parseErr = fmt.Errorf("failed to hydrate stock trap profile %q: %w", file.path, err)
			return
		}
		if err := validateStockProfileRoutes(name, s.routes[name], bundle); err != nil {
			state.parseErr = err
			return
		}
		state.bundle = bundle
	})
	return state.bundle, state.parseErr
}

func loadStockProfileBundle(filename string, expected [sha256.Size]byte) (profileLoadBundle, error) {
	content, err := readProfileFile(filename)
	if err != nil {
		return profileLoadBundle{}, err
	}
	actual := sha256.Sum256(content)
	if actual != expected {
		return profileLoadBundle{}, fmt.Errorf("content sha256 mismatch: expected %x, got %x", expected, actual)
	}
	return parseProfileBundle(filename, content)
}

func (s *stockProfileStore) validationBundleForRules(current string, rules []profileMetricRule) (profileLoadBundle, error) {
	needed := make(map[string]struct{})
	for _, rule := range rules {
		for _, ref := range []string{rule.OnTrap, rule.ProblemTrap, rule.ClearTrap} {
			ref = strings.TrimSpace(ref)
			if ref == "" {
				continue
			}
			if model.IsNumericOID(ref) {
				for _, oid := range []string{ref, model.AlternateTrapOID(ref)} {
					if name := s.exactRoutes[oid]; name != "" && name != current {
						needed[name] = struct{}{}
						break
					}
				}
				continue
			}
			mib, _, ok := strings.Cut(ref, "::")
			if !ok || mib == "" {
				continue
			}
			for _, name := range s.mibRoutes[mib] {
				if name != current {
					needed[name] = struct{}{}
				}
			}
		}
	}

	names := make([]string, 0, len(needed))
	for name := range needed {
		names = append(names, name)
	}
	slices.Sort(names)
	var combined profileLoadBundle
	for _, name := range names {
		bundle, err := s.parse(name)
		if err != nil {
			return profileLoadBundle{}, err
		}
		combined.traps = append(combined.traps, bundle.traps...)
		combined.metrics = append(combined.metrics, bundle.metrics...)
		combined.charts = append(combined.charts, bundle.charts...)
	}
	return combined, nil
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
