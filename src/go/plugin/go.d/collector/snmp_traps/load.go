// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"

	"gopkg.in/yaml.v2"

	"github.com/klauspost/compress/zstd"
	"github.com/netdata/netdata/go/plugins/pkg/executable"
	"github.com/netdata/netdata/go/plugins/pkg/multipath"
	"github.com/netdata/netdata/go/plugins/pkg/pluginconfig"
)

type profileLoadPaths struct {
	eagerDirs []string
	stockDir  string
	all       multipath.MultiPath
}

const maxProfileExtendsDepth = 32

var maxProfileFileBytes int64 = 128 * 1024 * 1024

// getProfileDirs returns the multipath of profile directories.
// Order: user dirs first, then stock dir last.
func getProfileDirs() multipath.MultiPath {
	return getProfileLoadPaths().all
}

func getProfileLoadPaths() profileLoadPaths {
	if testDirsOverride != nil {
		return profileLoadPaths{eagerDirs: append([]string(nil), testDirsOverride...), all: multipath.New(testDirsOverride...)}
	}

	if executable.Name == "test" {
		stockDir := trapProfilesDirFromThisFile()
		return profileLoadPaths{stockDir: stockDir, all: multipath.New(stockDir)}
	}

	if dir := filepath.Join(executable.Directory, "../config/go.d/snmp.trap-profiles/default"); isDir(dir) {
		return profileLoadPaths{stockDir: dir, all: multipath.New(dir)}
	}

	var dirs []string
	for _, dir := range pluginconfig.CollectorsUserDirs() {
		dirs = append(dirs, trapProfilesUserDir(dir))
	}
	stockDir := trapProfilesStockDir(pluginconfig.CollectorsStockDir())
	allDirs := append(append([]string(nil), dirs...), stockDir)

	return profileLoadPaths{eagerDirs: dirs, stockDir: stockDir, all: multipath.New(allDirs...)}
}

// testDirsOverride is set by tests to redirect profile loading to temp dirs.
var testDirsOverride []string

func trapProfilesUserDir(collectorsDir string) string {
	return filepath.Join(collectorsDir, "snmp.trap-profiles")
}

func trapProfilesStockDir(collectorsDir string) string {
	return filepath.Join(collectorsDir, "snmp.trap-profiles", "default")
}

// trapProfilesDirFromThisFile resolves the stock profile directory relative to
// this source file, used in test mode.
func trapProfilesDirFromThisFile() string {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return ""
	}
	base := filepath.Dir(thisFile)
	stockDir := filepath.Join(base, "..", "..", "config", "go.d", "snmp.trap-profiles", "default")
	if isDir(stockDir) {
		abs, _ := filepath.Abs(stockDir)
		return abs
	}
	return ""
}

func isDir(path string) bool {
	fi, err := os.Stat(path)
	if err != nil {
		return false
	}
	return fi.Mode().IsDir()
}

// loadProfileCache loads user profiles and builds a lazy route table for stock profiles.
func loadProfileCache() (*ProfileIndex, error) {
	paths := getProfileLoadPaths()
	index, seen, accepted, err := loadUserProfileTraps(paths)
	if err != nil {
		return nil, err
	}

	if paths.stockDir != "" {
		store, err := buildStockProfileStore(paths.stockDir, paths.all, seen, index)
		if err != nil {
			if pluginconfig.IsStock(paths.stockDir) || !errors.Is(err, os.ErrNotExist) {
				return nil, fmt.Errorf("failed to load trap profile index from '%s': %w", paths.stockDir, err)
			}
		} else {
			index.stock = store
		}
	}
	if loadedProfilesHaveMetrics(accepted) {
		if err := index.loadStockProfileMetrics(); err != nil {
			return nil, err
		}
	}
	if err := addLoadedProfileMetrics(index, accepted); err != nil {
		return nil, err
	}

	if len(index.trapsByOID) == 0 && (index.stock == nil || index.stock.empty()) {
		return nil, fmt.Errorf("no trap profiles found in %v", paths.all)
	}

	return index, nil
}

// loadUserProfileCache reloads only user profiles and carries over the existing
// stock store. This is used for live reloads so Netdata upgrades do not live-load
// changed stock profile metadata without the matching process/job restart.
func loadUserProfileCache(current *ProfileIndex) (*ProfileIndex, error) {
	if current == nil || current.stock == nil {
		return loadProfileCache()
	}

	paths := getProfileLoadPaths()
	index, seen, accepted, err := loadUserProfileTraps(paths)
	if err != nil {
		return nil, err
	}

	index.stock = current.stock.cloneFiltered(seen)
	if err := copyLoadedStockProfiles(index, current, seen); err != nil {
		return nil, err
	}
	if loadedProfilesHaveMetrics(accepted) {
		if err := index.loadStockProfileMetrics(); err != nil {
			return nil, err
		}
	}
	if err := addLoadedProfileMetrics(index, accepted); err != nil {
		return nil, err
	}

	if len(index.trapsByOID) == 0 && (index.stock == nil || index.stock.empty()) {
		return nil, fmt.Errorf("no trap profiles found in %v", paths.all)
	}

	return index, nil
}

func loadUserProfiles(paths profileLoadPaths) (*ProfileIndex, map[string]bool, error) {
	index, seen, accepted, err := loadUserProfileTraps(paths)
	if err != nil {
		return nil, nil, err
	}
	if err := addLoadedProfileMetrics(index, accepted); err != nil {
		return nil, nil, err
	}
	return index, seen, nil
}

func loadUserProfileTraps(paths profileLoadPaths) (*ProfileIndex, map[string]bool, []loadedProfileFile, error) {
	seen := make(map[string]bool)

	index := &ProfileIndex{
		trapsByOID:        make(map[string]*TrapDef),
		namesByTrapName:   make(map[string]*TrapDef),
		metricRulesByName: make(map[string]*profileMetricRule),
		metricChartsByID:  make(map[string]*profileMetricChart),
	}

	var accepted []loadedProfileFile
	for _, dir := range paths.eagerDirs {
		files, err := loadProfilesFromDir(dir, paths.all)
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				return nil, nil, nil, fmt.Errorf("failed to load trap profiles from '%s': %w", dir, err)
			}
			continue
		}

		for _, file := range files {
			if seen[file.name] {
				continue
			}
			seen[file.name] = true
			accepted = append(accepted, file)
			if err := index.addTraps(file.traps); err != nil {
				return nil, nil, nil, err
			}
		}
	}

	return index, seen, accepted, nil
}

func addLoadedProfileMetrics(index *ProfileIndex, accepted []loadedProfileFile) error {
	for _, file := range accepted {
		if err := index.addProfileMetrics(file.metrics, file.charts); err != nil {
			return err
		}
	}
	return nil
}

func loadedProfilesHaveMetrics(files []loadedProfileFile) bool {
	for _, file := range files {
		if len(file.metrics) > 0 {
			return true
		}
	}
	return false
}

func (idx *ProfileIndex) addTraps(traps []*TrapDef) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	return idx.addTrapsLocked(traps)
}

func (idx *ProfileIndex) addTrapsLocked(traps []*TrapDef) error {
	if idx.trapsByOID == nil {
		idx.trapsByOID = make(map[string]*TrapDef, len(traps))
	}
	if idx.namesByTrapName == nil {
		idx.namesByTrapName = make(map[string]*TrapDef, len(traps))
	}
	seenOIDs := make(map[string]string, len(traps))
	seenNames := make(map[string]string, len(traps))
	for _, td := range traps {
		if existing, ok := idx.trapsByOID[td.OID]; ok {
			return fmt.Errorf("%s: duplicate trap OID %s (already defined in %s)", td.sourceFile, td.OID, existing.sourceFile)
		}
		if existing, ok := idx.namesByTrapName[td.Name]; ok {
			return fmt.Errorf("%s: duplicate trap name %s (already defined in %s)", td.sourceFile, td.Name, existing.sourceFile)
		}
		if existing := seenOIDs[td.OID]; existing != "" {
			return fmt.Errorf("%s: duplicate trap OID %s (already defined in %s)", td.sourceFile, td.OID, existing)
		}
		if existing := seenNames[td.Name]; existing != "" {
			return fmt.Errorf("%s: duplicate trap name %s (already defined in %s)", td.sourceFile, td.Name, existing)
		}
		seenOIDs[td.OID] = td.sourceFile
		seenNames[td.Name] = td.sourceFile
	}
	for _, td := range traps {
		idx.trapsByOID[td.OID] = td
		idx.namesByTrapName[td.Name] = td
	}
	return nil
}

func (idx *ProfileIndex) addProfileMetrics(rules []profileMetricRule, charts []profileMetricChart) error {
	if len(rules) == 0 && len(charts) == 0 {
		return nil
	}
	if idx == nil {
		return fmt.Errorf("profile index not available")
	}

	idx.mu.RLock()
	knownCharts := make(map[string]*profileMetricChart, len(idx.metricChartsByID)+len(charts))
	maps.Copy(knownCharts, idx.metricChartsByID)
	knownRules := make(map[string]*profileMetricRule, len(idx.metricRulesByName)+len(rules))
	maps.Copy(knownRules, idx.metricRulesByName)
	idx.mu.RUnlock()
	knownOutputMetrics := make(map[string]string, len(knownRules)+len(rules))
	for name, rule := range knownRules {
		if rule == nil || rule.Output.Metric == "" {
			continue
		}
		knownOutputMetrics[rule.Output.Metric] = fmt.Sprintf("rule %q in %s", name, rule.sourceFile)
	}

	newCharts := make([]profileMetricChart, len(charts))
	for i := range charts {
		chart := charts[i]
		if err := normalizeProfileMetricChart(&chart); err != nil {
			return fmt.Errorf("%s: metric chart: %w", chart.sourceFile, err)
		}
		if existing := knownCharts[chart.ID]; existing != nil {
			return fmt.Errorf("%s: duplicate metric chart %q (already defined in %s)", chart.sourceFile, chart.ID, existing.sourceFile)
		}
		newCharts[i] = chart
		knownCharts[chart.ID] = &newCharts[i]
	}

	newRules := make([]profileMetricRule, len(rules))
	for i := range rules {
		rule := rules[i]
		if existing := knownRules[rule.Name]; existing != nil {
			return fmt.Errorf("%s: duplicate metric rule %q (already defined in %s)", rule.sourceFile, rule.Name, existing.sourceFile)
		}
		if err := validateProfileMetricRule(&rule, idx, knownCharts); err != nil {
			return err
		}
		if existing := knownOutputMetrics[rule.Output.Metric]; existing != "" {
			return fmt.Errorf("%s: metric rule %q output.metric %q already used by %s", rule.sourceFile, rule.Name, rule.Output.Metric, existing)
		}
		newRules[i] = rule
		knownRules[rule.Name] = &newRules[i]
		knownOutputMetrics[rule.Output.Metric] = fmt.Sprintf("rule %q in %s", rule.Name, rule.sourceFile)
	}
	if err := validateProfileMetricChartRuleShapes(knownRules); err != nil {
		return err
	}

	idx.mu.Lock()
	defer idx.mu.Unlock()
	if idx.metricRulesByName == nil {
		idx.metricRulesByName = make(map[string]*profileMetricRule, len(rules))
	}
	if idx.metricChartsByID == nil {
		idx.metricChartsByID = make(map[string]*profileMetricChart, len(charts))
	}
	for i := range newCharts {
		chart := newCharts[i]
		idx.metricChartsByID[chart.ID] = &newCharts[i]
	}
	for i := range newRules {
		rule := newRules[i]
		idx.metricRulesByName[rule.Name] = &newRules[i]
	}
	return nil
}

func validateProfileMetricChartRuleShapes(rules map[string]*profileMetricRule) error {
	type chartRuleShape struct {
		ruleName      string
		sourceFile    string
		usesResource  bool
		resourceClass string
	}
	names := make([]string, 0, len(rules))
	for name := range rules {
		names = append(names, name)
	}
	slices.Sort(names)

	shapes := make(map[string]chartRuleShape)
	for _, name := range names {
		rule := rules[name]
		if rule == nil {
			continue
		}
		shape := chartRuleShape{
			ruleName:     rule.Name,
			sourceFile:   rule.sourceFile,
			usesResource: rule.Identity.Resource != nil,
		}
		if rule.Identity.Resource != nil {
			shape.resourceClass = rule.Identity.Resource.Class
		}
		existing, ok := shapes[rule.Output.Chart]
		if !ok {
			shapes[rule.Output.Chart] = shape
		} else if existing.usesResource != shape.usesResource {
			return fmt.Errorf("%s: metric rule %q chart %q mixes resource and non-resource rules (already used by rule %q in %s)",
				rule.sourceFile, rule.Name, rule.Output.Chart, existing.ruleName, existing.sourceFile)
		} else if shape.usesResource && existing.resourceClass != shape.resourceClass {
			return fmt.Errorf("%s: metric rule %q chart %q mixes resource classes %q and %q (already used by rule %q in %s)",
				rule.sourceFile, rule.Name, rule.Output.Chart, existing.resourceClass, shape.resourceClass, existing.ruleName, existing.sourceFile)
		}
	}
	return nil
}

type loadedProfileFile struct {
	name    string
	traps   []*TrapDef
	metrics []profileMetricRule
	charts  []profileMetricChart
}

// loadProfilesFromDir walks a directory and loads all supported profile files.
func loadProfilesFromDir(dirpath string, extendsPaths multipath.MultiPath) ([]loadedProfileFile, error) {
	var files []loadedProfileFile
	if dirpath == "" {
		return nil, os.ErrNotExist
	}

	if err := filepath.WalkDir(dirpath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !isProfileFileName(d.Name()) {
			return nil
		}
		if strings.HasPrefix(d.Name(), "_") {
			return nil
		}

		bundle, ferr := loadProfileBundle(path, extendsPaths, nil)
		if ferr != nil {
			return fmt.Errorf("invalid profile '%s': %w", path, ferr)
		}

		files = append(files, loadedProfileFile{
			name:    profileLogicalName(d.Name()),
			traps:   bundle.traps,
			metrics: bundle.metrics,
			charts:  bundle.charts,
		})
		return nil
	}); err != nil {
		return nil, err
	}

	return files, nil
}

type profileLoadBundle struct {
	traps    []*TrapDef
	varbinds map[string]VarbindDef
	metrics  []profileMetricRule
	charts   []profileMetricChart
}

// loadProfile loads a single profile YAML and returns its validated trap definitions.
// Returns traps, loaded file-level varbinds, and error.
func loadProfile(filename string, extendsPaths multipath.MultiPath, stack []string) ([]*TrapDef, map[string]VarbindDef, error) {
	bundle, err := loadProfileBundle(filename, extendsPaths, stack)
	if err != nil {
		return nil, nil, err
	}
	return bundle.traps, bundle.varbinds, nil
}

func loadProfileBundle(filename string, extendsPaths multipath.MultiPath, stack []string) (profileLoadBundle, error) {
	if len(stack) > maxProfileExtendsDepth {
		return profileLoadBundle{}, fmt.Errorf("%s: profile extends depth exceeds maximum %d", filename, maxProfileExtendsDepth)
	}
	content, err := readProfileFile(filename)
	if err != nil {
		return profileLoadBundle{}, err
	}

	var def ProfileDefinition
	if err := unmarshalProfileYAML(content, &def); err != nil {
		return profileLoadBundle{}, err
	}

	// resolve extends chain
	var allTraps []*TrapDef
	var allMetrics []profileMetricRule
	var allCharts []profileMetricChart
	fileVarbinds := cloneVarbindMap(def.Varbinds)
	currentVarbinds := make(map[string]bool, len(def.Varbinds))
	for name := range def.Varbinds {
		currentVarbinds[name] = true
	}

	currentTrapOIDs := make(map[string]bool, len(def.Traps))
	for _, td := range def.Traps {
		if td.OID == "" {
			continue
		}
		if currentTrapOIDs[td.OID] {
			return profileLoadBundle{}, fmt.Errorf("%s: duplicate trap OID %s in profile", filename, td.OID)
		}
		currentTrapOIDs[td.OID] = true
	}
	baseByOID := make(map[string]*TrapDef)
	allTrapPos := make(map[string]int)
	allMetricPos := make(map[string]int)
	allChartPos := make(map[string]int)

	currentMetricNames := make(map[string]bool, len(def.Metrics))
	for _, metric := range def.Metrics {
		if metric.Name == "" {
			continue
		}
		if currentMetricNames[metric.Name] {
			return profileLoadBundle{}, fmt.Errorf("%s: duplicate metric rule %s in profile", filename, metric.Name)
		}
		currentMetricNames[metric.Name] = true
	}
	currentChartIDs := make(map[string]bool, len(def.Charts))
	for _, chart := range def.Charts {
		if chart.ID == "" {
			continue
		}
		if currentChartIDs[chart.ID] {
			return profileLoadBundle{}, fmt.Errorf("%s: duplicate metric chart %s in profile", filename, chart.ID)
		}
		currentChartIDs[chart.ID] = true
	}

	if len(def.Extends) > 0 {
		for _, extName := range def.Extends {
			if err := validateExtendsName(extName); err != nil {
				return profileLoadBundle{}, fmt.Errorf("%s: invalid extends entry %q: %w", filename, extName, err)
			}
			if stacksContains(stack, extName) {
				return profileLoadBundle{}, fmt.Errorf("%s: circular extends detected: %q", filename, extName)
			}
			extPath, err := findProfileExtends(extendsPaths, extName)
			if err != nil {
				return profileLoadBundle{}, fmt.Errorf("%s: cannot find extension %q: %v", filename, extName, err)
			}
			base, err := loadProfileBundle(extPath, extendsPaths, append(stack, extName))
			if err != nil {
				return profileLoadBundle{}, err
			}
			for _, bt := range base.traps {
				baseByOID[bt.OID] = bt
				if currentTrapOIDs[bt.OID] {
					continue
				}
				if pos, ok := allTrapPos[bt.OID]; ok {
					allTraps[pos] = bt
				} else {
					allTrapPos[bt.OID] = len(allTraps)
					allTraps = append(allTraps, bt)
				}
			}
			mergeProfileMetricRules(&allMetrics, allMetricPos, base.metrics, currentMetricNames)
			mergeProfileMetricCharts(&allCharts, allChartPos, base.charts, currentChartIDs)
			mergeBaseVarbinds(fileVarbinds, base.varbinds, currentVarbinds)
		}
	}

	absFile, _ := filepath.Abs(filename)
	if err := validateFileVarbinds(fileVarbinds, absFile); err != nil {
		return profileLoadBundle{}, err
	}

	for i := range def.Traps {
		td := &def.Traps[i]
		td.sourceFile = absFile
		if base := baseByOID[td.OID]; base != nil {
			mergeTrapDef(td, base)
		}

		if err := validateTrapDef(td, fileVarbinds); err != nil {
			return profileLoadBundle{}, err
		}

		td.sharedVarbinds = buildSharedVarbinds(td, fileVarbinds)
	}

	for i := range def.Traps {
		allTraps = append(allTraps, &def.Traps[i])
	}

	for i := range def.Metrics {
		rule := &def.Metrics[i]
		rule.sourceFile = absFile
		if err := normalizeProfileMetricRule(rule); err != nil {
			return profileLoadBundle{}, fmt.Errorf("%s: metric rule: %w", absFile, err)
		}
		if chart := chartFromMetricRule(rule); chart != nil {
			mergeProfileMetricCharts(&allCharts, allChartPos, []profileMetricChart{*chart}, nil)
		}
		allMetrics = append(allMetrics, *rule)
	}
	for i := range def.Charts {
		chart := &def.Charts[i]
		chart.sourceFile = absFile
		mergeProfileMetricCharts(&allCharts, allChartPos, []profileMetricChart{*chart}, nil)
	}

	return profileLoadBundle{
		traps:    allTraps,
		varbinds: fileVarbinds,
		metrics:  allMetrics,
		charts:   allCharts,
	}, nil
}

func readProfileFile(filename string) ([]byte, error) {
	return readMaybeCompressedFile(filename)
}

func readMaybeCompressedFile(filename string) ([]byte, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var r io.Reader = file
	var gz *gzip.Reader
	var zr *zstd.Decoder
	switch {
	case strings.HasSuffix(filename, ".zst"):
		zr, err = zstd.NewReader(file)
		if err != nil {
			return nil, err
		}
		defer zr.Close()
		r = zr
	case strings.HasSuffix(filename, ".gz"):
		gz, err = gzip.NewReader(file)
		if err != nil {
			return nil, err
		}
		defer gz.Close()
		r = gz
	}

	lr := io.LimitReader(r, maxProfileFileBytes+1)
	data, err := io.ReadAll(lr)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxProfileFileBytes {
		return nil, fmt.Errorf("profile file %q exceeds maximum decompressed size %d bytes", filename, maxProfileFileBytes)
	}
	return data, nil
}

func findProfileExtends(paths multipath.MultiPath, name string) (string, error) {
	var lastErr error
	for _, candidate := range profilePathCandidates(name) {
		path, err := paths.Find(candidate)
		if err == nil {
			return path, nil
		}
		lastErr = err
	}
	return "", lastErr
}

func isProfileFileName(name string) bool {
	return strings.HasSuffix(name, ".yaml") ||
		strings.HasSuffix(name, ".yml") ||
		strings.HasSuffix(name, ".yaml.zst") ||
		strings.HasSuffix(name, ".yml.zst") ||
		strings.HasSuffix(name, ".yaml.gz") ||
		strings.HasSuffix(name, ".yml.gz")
}

func profileLogicalName(name string) string {
	name = strings.TrimSuffix(name, ".zst")
	return strings.TrimSuffix(name, ".gz")
}

func profilePathCandidates(name string) []string {
	if strings.HasSuffix(name, ".zst") || strings.HasSuffix(name, ".gz") {
		return []string{name}
	}
	return []string{name, name + ".zst", name + ".gz"}
}

type stockProfileStore struct {
	dir              string
	extendsPaths     multipath.MultiPath
	files            map[string]string
	exactRoutes      map[string]string
	enterpriseRoutes map[string]string
	loaded           map[string]bool
	failed           map[string]error
	metricsLoaded    bool
	mu               sync.Mutex
}

func (s *stockProfileStore) cloneFiltered(replaced map[string]bool) *stockProfileStore {
	if s == nil {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	clone := &stockProfileStore{
		dir:              s.dir,
		extendsPaths:     s.extendsPaths,
		files:            make(map[string]string, len(s.files)),
		exactRoutes:      make(map[string]string, len(s.exactRoutes)),
		enterpriseRoutes: make(map[string]string, len(s.enterpriseRoutes)),
		loaded:           make(map[string]bool, len(s.loaded)),
		failed:           make(map[string]error, len(s.failed)),
		metricsLoaded:    s.metricsLoaded,
	}
	for name, path := range s.files {
		if replaced[name] {
			continue
		}
		clone.files[name] = path
	}
	for oid, name := range s.exactRoutes {
		if replaced[name] {
			continue
		}
		clone.exactRoutes[oid] = name
	}
	for prefix, name := range s.enterpriseRoutes {
		if replaced[name] {
			continue
		}
		clone.enterpriseRoutes[prefix] = name
	}
	for name, loaded := range s.loaded {
		if replaced[name] {
			continue
		}
		clone.loaded[name] = loaded
	}
	for name, err := range s.failed {
		if replaced[name] {
			continue
		}
		clone.failed[name] = err
	}

	return clone
}

func (s *stockProfileStore) loadedProfilePaths(replaced map[string]bool) map[string]string {
	if s == nil {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	paths := make(map[string]string)
	for name, loaded := range s.loaded {
		if !loaded || replaced[name] {
			continue
		}
		if path := s.files[name]; path != "" {
			paths[filepath.Clean(path)] = name
		}
	}
	return paths
}

func copyLoadedStockProfiles(dst, current *ProfileIndex, replaced map[string]bool) error {
	if dst == nil || current == nil || current.stock == nil {
		return nil
	}

	loadedPaths := current.stock.loadedProfilePaths(replaced)
	if len(loadedPaths) == 0 {
		return nil
	}

	var traps []*TrapDef
	var metrics []profileMetricRule
	var charts []profileMetricChart
	current.mu.RLock()
	for _, td := range current.trapsByOID {
		if td == nil {
			continue
		}
		if _, ok := loadedPaths[filepath.Clean(td.sourceFile)]; ok {
			traps = append(traps, td)
		}
	}
	for _, rule := range current.metricRulesByName {
		if rule == nil {
			continue
		}
		if _, ok := loadedPaths[filepath.Clean(rule.sourceFile)]; ok {
			metrics = append(metrics, *rule)
		}
	}
	for _, chart := range current.metricChartsByID {
		if chart == nil {
			continue
		}
		if _, ok := loadedPaths[filepath.Clean(chart.sourceFile)]; ok {
			charts = append(charts, *chart)
		}
	}
	current.mu.RUnlock()

	if err := dst.addTraps(traps); err != nil {
		return err
	}
	return dst.addProfileMetrics(metrics, charts)
}

func (s *stockProfileStore) empty() bool {
	return s == nil || (len(s.exactRoutes) == 0 && len(s.enterpriseRoutes) == 0)
}

func buildStockProfileStore(dir string, extendsPaths multipath.MultiPath, replaced map[string]bool, idx *ProfileIndex) (*stockProfileStore, error) {
	if store, ok, err := buildStockProfileStoreFromCatalogue(dir, extendsPaths, replaced, idx); ok || err != nil {
		return store, err
	}
	return buildStockProfileStoreFromProfiles(dir, extendsPaths, replaced, idx)
}

func buildStockProfileStoreFromProfiles(dir string, extendsPaths multipath.MultiPath, replaced map[string]bool, idx *ProfileIndex) (*stockProfileStore, error) {
	files, err := profileFilesInDir(dir)
	if err != nil {
		return nil, err
	}

	store := &stockProfileStore{
		dir:              dir,
		extendsPaths:     extendsPaths,
		files:            make(map[string]string),
		exactRoutes:      make(map[string]string),
		enterpriseRoutes: make(map[string]string),
		loaded:           make(map[string]bool),
		failed:           make(map[string]error),
	}

	stockOIDs := make(map[string]string)
	stockOIDFiles := make(map[string]string)
	stockNames := make(map[string]string)
	prefixFiles := make(map[string]map[string]bool)

	for _, path := range files {
		name := profileLogicalName(filepath.Base(path))
		if replaced[name] {
			continue
		}
		traps, _, err := loadProfile(path, extendsPaths, nil)
		if err != nil {
			return nil, fmt.Errorf("invalid profile '%s': %w", path, err)
		}
		if len(traps) == 0 {
			continue
		}
		store.files[name] = path

		for _, td := range traps {
			if existing := idx.lookupLoaded(td.OID); existing != nil {
				return nil, fmt.Errorf("%s: duplicate trap OID %s (already defined in %s)", td.sourceFile, td.OID, existing.sourceFile)
			}
			if existing := idx.loadedTrapNameSource(td.Name); existing != "" {
				return nil, fmt.Errorf("%s: duplicate trap name %s (already defined in %s)", td.sourceFile, td.Name, existing)
			}
			if existing := stockOIDs[td.OID]; existing != "" {
				return nil, fmt.Errorf("%s: duplicate trap OID %s (already defined in %s)", td.sourceFile, td.OID, existing)
			}
			if existing := stockNames[td.Name]; existing != "" {
				return nil, fmt.Errorf("%s: duplicate trap name %s (already defined in %s)", td.sourceFile, td.Name, existing)
			}
			stockOIDs[td.OID] = td.sourceFile
			stockOIDFiles[td.OID] = name
			stockNames[td.Name] = td.sourceFile

			if prefix, ok := enterpriseTrapPrefix(td.OID); ok {
				if prefixFiles[prefix] == nil {
					prefixFiles[prefix] = make(map[string]bool)
				}
				prefixFiles[prefix][name] = true
				continue
			}
			store.exactRoutes[td.OID] = name
		}
	}

	for oid := range stockOIDs {
		prefix, ok := enterpriseTrapPrefix(oid)
		if !ok {
			continue
		}
		filesForPrefix := prefixFiles[prefix]
		if len(filesForPrefix) == 1 {
			for file := range filesForPrefix {
				store.enterpriseRoutes[prefix] = file
			}
			continue
		}
		store.exactRoutes[oid] = stockOIDFiles[oid]
	}

	return store, nil
}

type stockProfileCatalogue map[string]stockProfileCatalogueEntry

type stockProfileCatalogueEntry struct {
	File     string   `json:"file"`
	TrapOIDs []string `json:"trap_oids"`
}

func buildStockProfileStoreFromCatalogue(
	dir string,
	extendsPaths multipath.MultiPath,
	replaced map[string]bool,
	idx *ProfileIndex,
) (*stockProfileStore, bool, error) {
	catalogue, err := loadStockProfileCatalogue(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, false, nil
		}
		return nil, true, err
	}

	store := &stockProfileStore{
		dir:              dir,
		extendsPaths:     extendsPaths,
		files:            make(map[string]string),
		exactRoutes:      make(map[string]string),
		enterpriseRoutes: make(map[string]string),
		loaded:           make(map[string]bool),
		failed:           make(map[string]error),
	}

	stockOIDs := make(map[string]string)
	stockOIDFiles := make(map[string]string)
	prefixFiles := make(map[string]map[string]bool)

	for _, vendor := range sortedCatalogueVendors(catalogue) {
		entry := catalogue[vendor]
		if entry.File == "" {
			return nil, true, fmt.Errorf("stock trap profile catalogue entry %q has empty file", vendor)
		}
		if len(entry.TrapOIDs) == 0 {
			return nil, true, fmt.Errorf("stock trap profile catalogue entry %q for file %q has no trap_oids", vendor, entry.File)
		}

		path, name, err := catalogueProfilePath(dir, entry.File)
		if err != nil {
			return nil, true, fmt.Errorf("stock trap profile catalogue entry %q references file %q: %w", vendor, entry.File, err)
		}
		if replaced[name] {
			continue
		}
		store.files[name] = path

		for _, oid := range entry.TrapOIDs {
			source := path
			if existing := idx.lookupLoaded(oid); existing != nil {
				return nil, true, fmt.Errorf("%s: duplicate trap OID %s (already defined in %s)", source, oid, existing.sourceFile)
			}
			if existing := stockOIDs[oid]; existing != "" {
				return nil, true, fmt.Errorf("%s: duplicate trap OID %s (already defined in %s)", source, oid, existing)
			}
			stockOIDs[oid] = source
			stockOIDFiles[oid] = name

			if prefix, ok := enterpriseTrapPrefix(oid); ok {
				if prefixFiles[prefix] == nil {
					prefixFiles[prefix] = make(map[string]bool)
				}
				prefixFiles[prefix][name] = true
				continue
			}
			store.exactRoutes[oid] = name
		}
	}

	for oid := range stockOIDs {
		prefix, ok := enterpriseTrapPrefix(oid)
		if !ok {
			continue
		}
		filesForPrefix := prefixFiles[prefix]
		if len(filesForPrefix) == 1 {
			for file := range filesForPrefix {
				store.enterpriseRoutes[prefix] = file
			}
			continue
		}
		store.exactRoutes[oid] = stockOIDFiles[oid]
	}

	return store, true, nil
}

func loadStockProfileCatalogue(stockDir string) (stockProfileCatalogue, error) {
	if stockDir == "" {
		return nil, os.ErrNotExist
	}
	data, err := readStockProfileCatalogueFile(filepath.Dir(stockDir))
	if err != nil {
		return nil, err
	}
	var catalogue stockProfileCatalogue
	if err := json.Unmarshal(data, &catalogue); err != nil {
		return nil, fmt.Errorf("invalid stock trap profile catalogue: %w", err)
	}
	if len(catalogue) == 0 {
		return nil, fmt.Errorf("stock trap profile catalogue is empty")
	}
	return catalogue, nil
}

func readStockProfileCatalogueFile(dir string) ([]byte, error) {
	var lastErr error
	for _, name := range []string{"catalogue.json", "catalogue.json.zst", "catalogue.json.gz"} {
		data, err := readMaybeCompressedFile(filepath.Join(dir, name))
		if err == nil {
			return data, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		lastErr = err
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, os.ErrNotExist
}

func sortedCatalogueVendors(catalogue stockProfileCatalogue) []string {
	vendors := make([]string, 0, len(catalogue))
	for vendor := range catalogue {
		vendors = append(vendors, vendor)
	}
	slices.Sort(vendors)
	return vendors
}

func catalogueProfilePath(dir, name string) (string, string, error) {
	if filepath.Base(name) != name || strings.ContainsRune(name, '\\') {
		return "", "", fmt.Errorf("must be a profile filename, not a path")
	}
	if !profilePathIsSupported(name) {
		return "", "", fmt.Errorf("unsupported profile filename")
	}

	for _, candidate := range profilePathCandidates(name) {
		if !isProfileFileName(candidate) {
			continue
		}
		path := filepath.Join(dir, candidate)
		fi, err := os.Stat(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return "", "", err
		}
		if fi.IsDir() {
			return "", "", fmt.Errorf("%s is a directory", path)
		}
		return path, profileLogicalName(candidate), nil
	}
	return "", "", os.ErrNotExist
}

func profilePathIsSupported(name string) bool {
	return slices.ContainsFunc(profilePathCandidates(name), isProfileFileName)
}

func (idx *ProfileIndex) loadedTrapNameSource(name string) string {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	if idx.namesByTrapName == nil {
		return ""
	}
	if td := idx.namesByTrapName[name]; td != nil {
		return td.sourceFile
	}
	return ""
}

func profileFilesInDir(dirpath string) ([]string, error) {
	if dirpath == "" {
		return nil, os.ErrNotExist
	}
	var files []string
	err := filepath.WalkDir(dirpath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !isProfileFileName(d.Name()) || strings.HasPrefix(d.Name(), "_") {
			return nil
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		return nil, err
	}
	slices.Sort(files)
	return files, nil
}

func enterpriseTrapPrefix(oid string) (string, bool) {
	const prefix = "1.3.6.1.4.1."
	if !strings.HasPrefix(oid, prefix) {
		return "", false
	}
	rest := strings.TrimPrefix(oid, prefix)
	nextDot := strings.IndexByte(rest, '.')
	if nextDot <= 0 {
		return "", false
	}
	pen := rest[:nextDot]
	for i := 0; i < len(pen); i++ {
		if pen[i] < '0' || pen[i] > '9' {
			return "", false
		}
	}
	return prefix + pen, true
}

func (idx *ProfileIndex) loadStockForOID(oid string) error {
	if idx == nil || idx.stock == nil {
		return nil
	}
	return idx.stock.loadForOID(idx, oid)
}

func (idx *ProfileIndex) loadStockProfileMetrics() error {
	if idx == nil || idx.stock == nil {
		return nil
	}
	return idx.stock.loadProfileMetrics(idx)
}

func (s *stockProfileStore) loadProfileMetrics(idx *ProfileIndex) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.metricsLoaded {
		return nil
	}
	names := make([]string, 0, len(s.files))
	for name := range s.files {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		if s.loaded[name] {
			continue
		}
		path := s.files[name]
		if path == "" {
			continue
		}
		bundle, err := loadProfileBundle(path, s.extendsPaths, nil)
		if err != nil {
			if s.failed == nil {
				s.failed = make(map[string]error)
			}
			s.failed[name] = fmt.Errorf("failed to load stock trap profile for metrics from %q: %w", path, err)
			return s.failed[name]
		}
		if err := idx.addTraps(bundle.traps); err != nil {
			if s.failed == nil {
				s.failed = make(map[string]error)
			}
			s.failed[name] = err
			return err
		}
		if err := idx.addProfileMetrics(bundle.metrics, bundle.charts); err != nil {
			if s.failed == nil {
				s.failed = make(map[string]error)
			}
			s.failed[name] = err
			return err
		}
		s.loaded[name] = true
	}
	s.metricsLoaded = true
	return nil
}

func (s *stockProfileStore) loadForOID(idx *ProfileIndex, oid string) error {
	name := s.route(oid)
	if name == "" {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.loaded[name] {
		return nil
	}
	if s.failed == nil {
		s.failed = make(map[string]error)
	}
	if err := s.failed[name]; err != nil {
		return err
	}
	path := s.files[name]
	if path == "" {
		err := fmt.Errorf("stock trap profile %q not found in %s", name, s.dir)
		s.failed[name] = err
		return err
	}
	bundle, err := loadProfileBundle(path, s.extendsPaths, nil)
	if err != nil {
		err = fmt.Errorf("failed to lazy-load stock trap profile %q: %w", path, err)
		s.failed[name] = err
		return err
	}
	if err := idx.addTraps(bundle.traps); err != nil {
		s.failed[name] = err
		return err
	}
	if err := idx.addProfileMetrics(bundle.metrics, bundle.charts); err != nil {
		s.failed[name] = err
		return err
	}
	s.loaded[name] = true
	return nil
}

func (s *stockProfileStore) route(oid string) string {
	for _, candidate := range []string{oid, alternateTrapOID(oid)} {
		if file := s.exactRoutes[candidate]; file != "" {
			return file
		}
		if prefix, ok := enterpriseTrapPrefix(candidate); ok {
			if file := s.enterpriseRoutes[prefix]; file != "" {
				return file
			}
		}
	}
	return ""
}

func validateExtendsName(name string) error {
	if name == "" {
		return fmt.Errorf("empty filename")
	}
	if filepath.Base(name) != name || strings.ContainsRune(name, '\\') {
		return fmt.Errorf("must be a profile filename, not a path")
	}
	if name == "." || name == ".." {
		return fmt.Errorf("must be a profile filename, not a path")
	}
	if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
		return fmt.Errorf("must end with .yaml or .yml")
	}
	return nil
}

func mergeTrapDef(target *TrapDef, base *TrapDef) {
	if target.Name == "" {
		target.Name = base.Name
	}
	if target.Category == "" {
		target.Category = base.Category
	}
	if target.Severity == "" {
		target.Severity = base.Severity
	}
	if target.Description == "" {
		target.Description = base.Description
	}
	if target.Status == "" {
		target.Status = base.Status
	}
	if target.VarbindRefs == nil {
		target.VarbindRefs = cloneAnySlice(base.VarbindRefs)
	}
	if target.Labels == nil {
		target.Labels = cloneStringMap(base.Labels)
	} else {
		target.Labels = mergeStringMaps(base.Labels, target.Labels)
	}
	if target.DedupKeyVarbinds == nil {
		target.DedupKeyVarbinds = append([]string(nil), base.DedupKeyVarbinds...)
	}
}

func mergeBaseVarbinds(target, base map[string]VarbindDef, protected map[string]bool) {
	for name, vb := range base {
		if !protected[name] {
			target[name] = vb
		}
	}
}

func mergeProfileMetricRules(target *[]profileMetricRule, positions map[string]int, base []profileMetricRule, protected map[string]bool) {
	for _, rule := range base {
		if protected != nil && protected[rule.Name] {
			continue
		}
		if pos, ok := positions[rule.Name]; ok {
			(*target)[pos] = rule
			continue
		}
		positions[rule.Name] = len(*target)
		*target = append(*target, rule)
	}
}

func mergeProfileMetricCharts(target *[]profileMetricChart, positions map[string]int, base []profileMetricChart, protected map[string]bool) {
	for _, chart := range base {
		if protected != nil && protected[chart.ID] {
			continue
		}
		if pos, ok := positions[chart.ID]; ok {
			(*target)[pos] = chart
			continue
		}
		positions[chart.ID] = len(*target)
		*target = append(*target, chart)
	}
}

func cloneVarbindMap(src map[string]VarbindDef) map[string]VarbindDef {
	dst := make(map[string]VarbindDef, len(src))
	maps.Copy(dst, src)
	return dst
}

func cloneAnySlice(src []any) []any {
	if src == nil {
		return nil
	}
	return append([]any(nil), src...)
}

func cloneStringMap(src map[string]string) map[string]string {
	if src == nil {
		return nil
	}
	dst := make(map[string]string, len(src))
	maps.Copy(dst, src)
	return dst
}

func mergeStringMaps(base, override map[string]string) map[string]string {
	dst := cloneStringMap(base)
	maps.Copy(dst, override)
	return dst
}

func stacksContains(stack []string, name string) bool {
	return slices.Contains(stack, name)
}

var (
	profileMetricResourceYAMLSpec = yamlKeySpec{children: map[string]yamlKeySpec{
		"class":            {},
		"key":              {},
		"key_from_varbind": {},
		"max":              {},
		"max_per_source":   {},
	}}

	charttplDimensionLifecycleYAMLSpec = yamlKeySpec{children: map[string]yamlKeySpec{
		"max_dims":            {},
		"expire_after_cycles": {},
	}}

	charttplLifecycleYAMLSpec = yamlKeySpec{children: map[string]yamlKeySpec{
		"max_instances":       {},
		"expire_after_cycles": {},
		"dimensions":          charttplDimensionLifecycleYAMLSpec,
	}}

	profileMetricRuleYAMLSpec = yamlKeySpec{children: map[string]yamlKeySpec{
		"name":               {},
		"type":               {},
		"auto_safe":          {},
		"enabled":            {},
		"on_trap":            {},
		"problem_trap":       {},
		"clear_trap":         {},
		"where":              {allowAny: true},
		"identity":           {children: map[string]yamlKeySpec{"device": {}, "resource": profileMetricResourceYAMLSpec}},
		"resource":           profileMetricResourceYAMLSpec,
		"output":             {children: map[string]yamlKeySpec{"metric": {}, "dimension": {}, "chart": {}}},
		"state":              {children: map[string]yamlKeySpec{"set_when": {allowAny: true}, "clear_when": {allowAny: true}, "problem_value": {}, "clear_value": {}, "ttl": {}, "ttl_behavior": {}, "varbind": {}, "set": {}, "clear": {}}},
		"scale":              {children: map[string]yamlKeySpec{"multiplier": {}, "divisor": {}}},
		"missing":            {},
		"value_from_varbind": {},
		"chart_meta":         {children: map[string]yamlKeySpec{"title": {}, "family": {}, "context": {}, "units": {}, "algorithm": {}, "type": {}, "description": {}, "lifecycle": charttplLifecycleYAMLSpec}},
		"metric":             {},
		"dimension":          {},
		"chart_id":           {},
		"value":              {},
	}}

	profileMetricChartYAMLSpec = yamlKeySpec{children: map[string]yamlKeySpec{
		"id":          {},
		"title":       {},
		"family":      {},
		"context":     {},
		"units":       {},
		"algorithm":   {},
		"type":        {},
		"description": {},
		"lifecycle":   charttplLifecycleYAMLSpec,
	}}

	profileYAMLSpec = yamlKeySpec{children: map[string]yamlKeySpec{
		"vendor":     {},
		"mib_count":  {},
		"trap_count": {},
		"extends":    {},
		"varbinds":   {allowAny: true},
		"traps":      {allowAny: true},
		"metrics":    {elem: &profileMetricRuleYAMLSpec},
		"charts":     {elem: &profileMetricChartYAMLSpec},
	}}
)

func unmarshalProfileYAML(content []byte, def *ProfileDefinition) (err error) {
	defer func() {
		if v := recover(); v != nil {
			err = fmt.Errorf("panic parsing profile YAML: %v", v)
		}
	}()
	var raw any
	if err := yaml.Unmarshal(content, &raw); err != nil {
		return err
	}
	if err := rejectUnknownYAMLKeys(raw, profileYAMLSpec, "profile"); err != nil {
		return err
	}
	return yaml.Unmarshal(content, def)
}
