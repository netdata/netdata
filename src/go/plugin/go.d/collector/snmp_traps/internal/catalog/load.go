// SPDX-License-Identifier: GPL-3.0-or-later

package catalog

import (
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"gopkg.in/yaml.v2"

	"github.com/klauspost/compress/zstd"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/model"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/profilecatalog"
)

var maxProfileFileBytes int64 = 128 * 1024 * 1024

var profileIdentityRE = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

type profileSource struct {
	path   string
	stock  bool
	bundle *profileLoadBundle
}

func loadEpoch(paths Paths) (*Epoch, error) {
	if strings.TrimSpace(paths.StockDir) == "" {
		return nil, errors.New("stock trap profiles directory is not configured")
	}

	allDirs := append([]string(nil), paths.UserDirs...)
	allDirs = append(allDirs, paths.StockDir)
	specs := make([]profilecatalog.DirSpec, 0, len(allDirs))
	for _, dir := range paths.UserDirs {
		specs = append(specs, profilecatalog.DirSpec{Path: dir})
	}
	specs = append(specs, profilecatalog.DirSpec{Path: paths.StockDir, IsStock: true})

	sources, err := profilecatalog.Load(specs, profilecatalog.Options[profileSource]{
		LoadFile: func(ctx profilecatalog.FileContext) (profileSource, error) {
			source := profileSource{path: ctx.Path, stock: ctx.IsStock}
			if ctx.IsStock {
				return source, nil
			}
			bundle, err := loadProfileBundle(ctx.Path)
			if err != nil {
				return profileSource{}, fmt.Errorf("invalid profile %q: %w", ctx.Path, err)
			}
			source.bundle = &bundle
			return source, nil
		},
		ParseFileName: parseProfileFileName,
		ValidName:     profileIdentityRE.MatchString,
		UserErrors:    profilecatalog.FailInvalidUser,
	})
	if err != nil {
		return nil, fmt.Errorf("load trap profile files: %w", err)
	}

	index := NewEpoch()
	var userBundles []profileLoadBundle
	for _, named := range sources.InOrder() {
		source := named.Profile
		index.profiles = append(index.profiles, ProfileInfo{Name: named.Name, Path: source.path, IsStock: source.stock})
		if source.stock {
			continue
		}
		if source.bundle == nil {
			return nil, fmt.Errorf("user trap profile %q was not validated", source.path)
		}
		if err := index.addTraps(source.bundle.traps); err != nil {
			return nil, err
		}
		userBundles = append(userBundles, *source.bundle)
	}
	slices.SortFunc(index.profiles, func(a, b ProfileInfo) int { return strings.Compare(a.Name, b.Name) })

	store, err := buildStockProfileStore(paths.StockDir, sources, index)
	if err != nil {
		return nil, err
	}
	index.stock = store
	for _, bundle := range userBundles {
		if err := index.addBundleAtomic(profileLoadBundle{metrics: bundle.metrics, charts: bundle.charts}); err != nil {
			return nil, err
		}
	}
	if len(index.trapsByOID) == 0 && store.empty() {
		return nil, fmt.Errorf("no trap profiles found in %v", allDirs)
	}
	return index, nil
}

func (idx *Epoch) addTraps(traps []*TrapDef) error {
	for i, td := range traps {
		if td == nil {
			return fmt.Errorf("trap definition at index %d is nil", i)
		}
		if existing := idx.lookupLoaded(td.OID); existing != nil {
			return fmt.Errorf("%s: duplicate trap OID %s (already defined in %s)", td.SourceFile, td.OID, existing.SourceFile)
		}
		if existing := idx.lookupTrapName(td.Name); existing != nil {
			return fmt.Errorf("%s: duplicate trap name %s (already defined in %s)", td.SourceFile, td.Name, existing.SourceFile)
		}
	}
	idx.mu.Lock()
	defer idx.mu.Unlock()
	return idx.addTrapsLocked(traps)
}

func (idx *Epoch) addBundleAtomic(bundle profileLoadBundle, stockProfile ...string) error {
	currentStockProfile := ""
	if len(stockProfile) > 0 {
		currentStockProfile = stockProfile[0]
	}
	var dependencies profileLoadBundle
	if idx.stock != nil && len(bundle.metrics) > 0 {
		var err error
		dependencies, err = idx.stock.validationBundleForRules(currentStockProfile, bundle.metrics)
		if err != nil {
			return err
		}
	}

	idx.publishMu.Lock()
	defer idx.publishMu.Unlock()

	staged := NewEpoch()
	staged.base = idx
	staged.validationStockProfile = currentStockProfile
	if len(dependencies.traps) > 0 || len(dependencies.metrics) > 0 || len(dependencies.charts) > 0 {
		if err := staged.addValidationBundle(dependencies); err != nil {
			return err
		}
	}

	if err := staged.addTraps(bundle.traps); err != nil {
		return err
	}
	if err := staged.addProfileMetrics(bundle.metrics, bundle.charts, false); err != nil {
		return err
	}

	idx.mu.Lock()
	maps.Copy(idx.trapsByOID, staged.trapsByOID)
	maps.Copy(idx.namesByTrapName, staged.namesByTrapName)
	maps.Copy(idx.metricRulesByName, staged.metricRulesByName)
	maps.Copy(idx.metricRulesByOut, staged.metricRulesByOut)
	maps.Copy(idx.metricChartsByID, staged.metricChartsByID)
	idx.mu.Unlock()
	return nil
}

func (idx *Epoch) addValidationBundle(bundle profileLoadBundle) error {
	if idx.validationTrapsByOID == nil {
		idx.validationTrapsByOID = make(map[string]*TrapDef)
		idx.validationNamesByTrap = make(map[string]*TrapDef)
		idx.validationRulesByName = make(map[string]*profileMetricRule)
		idx.validationRulesByOut = make(map[string]*profileMetricRule)
		idx.validationChartsByID = make(map[string]*profileMetricChart)
	}
	for _, td := range bundle.traps {
		if existing := idx.lookupLoaded(td.OID); existing != nil {
			if existing == td {
				continue
			}
			return fmt.Errorf("%s: duplicate trap OID %s (already defined in %s)", td.SourceFile, td.OID, existing.SourceFile)
		}
		if existing := idx.lookupTrapName(td.Name); existing != nil {
			if existing == td {
				continue
			}
			return fmt.Errorf("%s: duplicate trap name %s (already defined in %s)", td.SourceFile, td.Name, existing.SourceFile)
		}
		idx.validationTrapsByOID[td.OID] = td
		idx.validationNamesByTrap[td.Name] = td
	}
	for i := range bundle.charts {
		chart := &bundle.charts[i]
		if existing := idx.lookupMetricChart(chart.ID); existing != nil {
			if existing.SourceFile == chart.SourceFile {
				continue
			}
			return fmt.Errorf("%s: duplicate metric chart %q (already defined in %s)", chart.SourceFile, chart.ID, existing.SourceFile)
		}
		idx.validationChartsByID[chart.ID] = chart
	}
	for i := range bundle.metrics {
		rule := &bundle.metrics[i]
		if existing := idx.lookupMetricRule(rule.Name); existing != nil {
			if existing.SourceFile == rule.SourceFile {
				continue
			}
			return fmt.Errorf("%s: duplicate metric rule %q (already defined in %s)", rule.SourceFile, rule.Name, existing.SourceFile)
		}
		if existing := idx.lookupMetricOutput(rule.Output.Metric); existing != nil && existing.Name != rule.Name {
			return fmt.Errorf("%s: metric rule %q output.metric %q already used by rule %q in %s", rule.SourceFile, rule.Name, rule.Output.Metric, existing.Name, existing.SourceFile)
		}
		idx.validationRulesByName[rule.Name] = rule
		idx.validationRulesByOut[rule.Output.Metric] = rule
	}
	return nil
}

func (idx *Epoch) addTrapsLocked(traps []*TrapDef) error {
	if idx.trapsByOID == nil {
		idx.trapsByOID = make(map[string]*TrapDef, len(traps))
	}
	if idx.namesByTrapName == nil {
		idx.namesByTrapName = make(map[string]*TrapDef, len(traps))
	}
	seenOIDs := make(map[string]string, len(traps))
	seenNames := make(map[string]string, len(traps))
	for _, td := range traps {
		if existing := idx.trapsByOID[td.OID]; existing != nil {
			return fmt.Errorf("%s: duplicate trap OID %s (already defined in %s)", td.SourceFile, td.OID, existing.SourceFile)
		}
		if alt := model.AlternateTrapOID(td.OID); alt != td.OID {
			if existing := idx.trapsByOID[alt]; existing != nil {
				return fmt.Errorf("%s: duplicate trap OID %s (alternate form already defined in %s)", td.SourceFile, td.OID, existing.SourceFile)
			}
		}
		if existing, ok := idx.namesByTrapName[td.Name]; ok {
			return fmt.Errorf("%s: duplicate trap name %s (already defined in %s)", td.SourceFile, td.Name, existing.SourceFile)
		}
		oidKey := trapOIDCollisionKey(td.OID)
		if existing := seenOIDs[oidKey]; existing != "" {
			return fmt.Errorf("%s: duplicate trap OID %s (already defined in %s)", td.SourceFile, td.OID, existing)
		}
		if existing := seenNames[td.Name]; existing != "" {
			return fmt.Errorf("%s: duplicate trap name %s (already defined in %s)", td.SourceFile, td.Name, existing)
		}
		seenOIDs[oidKey] = td.SourceFile
		seenNames[td.Name] = td.SourceFile
	}
	for _, td := range traps {
		idx.trapsByOID[td.OID] = td
		idx.namesByTrapName[td.Name] = td
	}
	return nil
}

func trapOIDCollisionKey(oid string) string {
	alt := model.AlternateTrapOID(oid)
	if alt != oid && alt < oid {
		return alt
	}
	return oid
}

func (idx *Epoch) addProfileMetrics(rules []profileMetricRule, charts []profileMetricChart, includeExisting bool) error {
	if len(rules) == 0 && len(charts) == 0 {
		return nil
	}
	if idx == nil {
		return fmt.Errorf("profile index not available")
	}

	newCharts := make([]profileMetricChart, len(charts))
	newChartsByID := make(map[string]*profileMetricChart, len(charts))
	for i := range charts {
		chart := charts[i]
		if err := normalizeProfileMetricChart(&chart); err != nil {
			return fmt.Errorf("%s: metric chart: %w", chart.SourceFile, err)
		}
		if existing := idx.lookupMetricChart(chart.ID); existing != nil {
			return fmt.Errorf("%s: duplicate metric chart %q (already defined in %s)", chart.SourceFile, chart.ID, existing.SourceFile)
		}
		if existing := newChartsByID[chart.ID]; existing != nil {
			return fmt.Errorf("%s: duplicate metric chart %q (already defined in %s)", chart.SourceFile, chart.ID, existing.SourceFile)
		}
		newCharts[i] = chart
		newChartsByID[chart.ID] = &newCharts[i]
	}
	knownCharts := newChartsByID
	if includeExisting {
		idx.mu.RLock()
		knownCharts = maps.Clone(idx.metricChartsByID)
		idx.mu.RUnlock()
		maps.Copy(knownCharts, newChartsByID)
	}

	newRules := make([]profileMetricRule, len(rules))
	newRulesByName := make(map[string]*profileMetricRule, len(rules))
	newRulesByOut := make(map[string]*profileMetricRule, len(rules))
	for i := range rules {
		rule := rules[i]
		if existing := idx.lookupMetricRule(rule.Name); existing != nil {
			return fmt.Errorf("%s: duplicate metric rule %q (already defined in %s)", rule.SourceFile, rule.Name, existing.SourceFile)
		}
		if existing := newRulesByName[rule.Name]; existing != nil {
			return fmt.Errorf("%s: duplicate metric rule %q (already defined in %s)", rule.SourceFile, rule.Name, existing.SourceFile)
		}
		if owner := idx.lazyStockMetricOwner(rule.Name); owner != "" {
			return fmt.Errorf("%s: duplicate metric rule %q (also routed by stock profile %q)", rule.SourceFile, rule.Name, owner)
		}
		if err := validateProfileMetricRule(&rule, idx, knownCharts); err != nil {
			return err
		}
		if existing := idx.lookupMetricOutput(rule.Output.Metric); existing != nil {
			return fmt.Errorf("%s: metric rule %q output.metric %q already used by rule %q in %s", rule.SourceFile, rule.Name, rule.Output.Metric, existing.Name, existing.SourceFile)
		}
		if existing := newRulesByOut[rule.Output.Metric]; existing != nil {
			return fmt.Errorf("%s: metric rule %q output.metric %q already used by rule %q in %s", rule.SourceFile, rule.Name, rule.Output.Metric, existing.Name, existing.SourceFile)
		}
		newRules[i] = rule
		newRulesByName[rule.Name] = &newRules[i]
		newRulesByOut[rule.Output.Metric] = &newRules[i]
	}
	rulesForShapeValidation := newRulesByName
	if includeExisting {
		idx.mu.RLock()
		rulesForShapeValidation = maps.Clone(idx.metricRulesByName)
		idx.mu.RUnlock()
		maps.Copy(rulesForShapeValidation, newRulesByName)
	}
	if err := validateProfileMetricChartRuleShapes(rulesForShapeValidation); err != nil {
		return err
	}

	idx.mu.Lock()
	defer idx.mu.Unlock()
	if idx.metricRulesByName == nil {
		idx.metricRulesByName = make(map[string]*profileMetricRule, len(rules))
	}
	if idx.metricRulesByOut == nil {
		idx.metricRulesByOut = make(map[string]*profileMetricRule, len(rules))
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
		idx.metricRulesByOut[rule.Output.Metric] = &newRules[i]
	}
	return nil
}

func (idx *Epoch) lazyStockMetricOwner(name string) string {
	if idx == nil {
		return ""
	}
	current := idx.validationStockProfile
	for currentEpoch := idx; currentEpoch != nil; currentEpoch = currentEpoch.base {
		if currentEpoch.stock == nil {
			continue
		}
		owner := currentEpoch.stock.metricRoutes[name]
		if owner == current {
			return ""
		}
		return owner
	}
	return ""
}

func validateProfileMetricChartRuleShapes(rules map[string]*profileMetricRule) error {
	type chartRuleShape struct {
		ruleName      string
		SourceFile    string
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
			SourceFile:   rule.SourceFile,
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
				rule.SourceFile, rule.Name, rule.Output.Chart, existing.ruleName, existing.SourceFile)
		} else if shape.usesResource && existing.resourceClass != shape.resourceClass {
			return fmt.Errorf("%s: metric rule %q chart %q mixes resource classes %q and %q (already used by rule %q in %s)",
				rule.SourceFile, rule.Name, rule.Output.Chart, existing.resourceClass, shape.resourceClass, existing.ruleName, existing.SourceFile)
		}
	}
	return nil
}

type profileLoadBundle struct {
	traps   []*TrapDef
	metrics []profileMetricRule
	charts  []profileMetricChart
}

// Bundle is one fully resolved and statically validated profile file. It is a
// temporary bridge for the root metric runtime until that runtime moves into
// this package.
type Bundle struct {
	Traps   []*TrapDef
	Metrics []MetricRule
	Charts  []MetricChart
}

// LoadProfileFile loads and validates one profile file.
func LoadProfileFile(filename string) (Bundle, error) {
	bundle, err := loadProfileBundle(filename)
	if err != nil {
		return Bundle{}, err
	}
	return Bundle{Traps: bundle.traps, Metrics: bundle.metrics, Charts: bundle.charts}, nil
}

func loadProfileBundle(filename string) (profileLoadBundle, error) {
	content, err := readProfileFile(filename)
	if err != nil {
		return profileLoadBundle{}, err
	}
	return parseProfileBundle(filename, content)
}

func parseProfileBundle(filename string, content []byte) (profileLoadBundle, error) {
	var def ProfileDefinition
	if err := unmarshalProfileYAML(content, &def); err != nil {
		return profileLoadBundle{}, err
	}

	trapOIDs := make(map[string]bool, len(def.Traps))
	for _, td := range def.Traps {
		if td.OID == "" {
			continue
		}
		if trapOIDs[td.OID] {
			return profileLoadBundle{}, fmt.Errorf("%s: duplicate trap OID %s in profile", filename, td.OID)
		}
		trapOIDs[td.OID] = true
	}

	metricNames := make(map[string]bool, len(def.Metrics))
	for _, metric := range def.Metrics {
		if metric.Name == "" {
			continue
		}
		if metricNames[metric.Name] {
			return profileLoadBundle{}, fmt.Errorf("%s: duplicate metric rule %s in profile", filename, metric.Name)
		}
		metricNames[metric.Name] = true
	}
	chartIDs := make(map[string]bool, len(def.Charts))
	for _, chart := range def.Charts {
		if chart.ID == "" {
			continue
		}
		if chartIDs[chart.ID] {
			return profileLoadBundle{}, fmt.Errorf("%s: duplicate metric chart %s in profile", filename, chart.ID)
		}
		chartIDs[chart.ID] = true
	}

	absFile, _ := filepath.Abs(filename)
	if err := validateFileVarbinds(def.Varbinds, absFile); err != nil {
		return profileLoadBundle{}, err
	}

	traps := make([]*TrapDef, 0, len(def.Traps))
	for i := range def.Traps {
		td := &def.Traps[i]
		td.SourceFile = absFile
		if err := validateTrapDef(td, def.Varbinds); err != nil {
			return profileLoadBundle{}, err
		}
		td.SharedVarbinds = buildSharedVarbinds(td, def.Varbinds)
		traps = append(traps, td)
	}

	metrics := make([]profileMetricRule, 0, len(def.Metrics))
	for i := range def.Metrics {
		rule := &def.Metrics[i]
		rule.SourceFile = absFile
		if err := normalizeProfileMetricRule(rule); err != nil {
			return profileLoadBundle{}, fmt.Errorf("%s: metric rule: %w", absFile, err)
		}
		metrics = append(metrics, *rule)
	}
	charts := make([]profileMetricChart, 0, len(def.Charts))
	for i := range def.Charts {
		chart := &def.Charts[i]
		chart.SourceFile = absFile
		charts = append(charts, *chart)
	}

	return profileLoadBundle{traps: traps, metrics: metrics, charts: charts}, nil
}

func readProfileFile(filename string) ([]byte, error) {
	return readCompressedFile(filename)
}

func readCatalogueFile(filename string) ([]byte, error) {
	return readCompressedFile(filename)
}

func readCompressedFile(filename string) ([]byte, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var r io.Reader = file
	var zr *zstd.Decoder
	if strings.HasSuffix(filename, ".zst") {
		zr, err = zstd.NewReader(file)
		if err != nil {
			return nil, err
		}
		defer zr.Close()
		r = zr
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

func isProfileFileName(name string) bool {
	_, ok := parseProfileFileName(name)
	return ok
}

func parseProfileFileName(name string) (string, bool) {
	for _, suffix := range []string{".yaml.zst", ".yml.zst", ".yaml", ".yml"} {
		if before, ok := strings.CutSuffix(name, suffix); ok {
			return before, true
		}
	}
	return "", false
}

type yamlKeySpec struct {
	children map[string]yamlKeySpec
	elem     *yamlKeySpec
	allowAny bool
}

func rejectUnknownYAMLKeys(node any, spec yamlKeySpec, path string) error {
	if spec.allowAny || node == nil {
		return nil
	}

	switch v := node.(type) {
	case map[any]any:
		if spec.children == nil {
			return nil
		}
		for rawKey, rawValue := range v {
			key, ok := rawKey.(string)
			if !ok {
				return fmt.Errorf("%s: config key %v is not a string", path, rawKey)
			}
			child, ok := spec.children[key]
			if !ok {
				return fmt.Errorf("%s: unknown config key %q", path, key)
			}
			childPath := path + "." + key
			if err := rejectUnknownYAMLKeys(rawValue, child, childPath); err != nil {
				return err
			}
		}
	case []any:
		if spec.elem == nil {
			return nil
		}
		for i, item := range v {
			itemPath := fmt.Sprintf("%s[%d]", path, i)
			if err := rejectUnknownYAMLKeys(item, *spec.elem, itemPath); err != nil {
				return err
			}
		}
	}

	return nil
}

var (
	profileMetricResourceYAMLSpec = yamlKeySpec{children: map[string]yamlKeySpec{
		"class":            {},
		"key_from_varbind": {},
		"max_per_source":   {},
	}}

	profileMetricPredicateYAMLSpec = yamlKeySpec{children: map[string]yamlKeySpec{
		"varbind": {}, "field": {}, "equals": {}, "in": {}, "exists": {}, "absent": {}, "greater_than": {},
		"less_than": {}, "range": {}, "not": {},
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
		"enabled":            {},
		"on_trap":            {},
		"problem_trap":       {},
		"clear_trap":         {},
		"where":              {elem: &profileMetricPredicateYAMLSpec},
		"identity":           {children: map[string]yamlKeySpec{"device": {}, "resource": profileMetricResourceYAMLSpec}},
		"output":             {children: map[string]yamlKeySpec{"metric": {}, "dimension": {}, "chart": {}}},
		"state":              {children: map[string]yamlKeySpec{"set_when": profileMetricPredicateYAMLSpec, "clear_when": profileMetricPredicateYAMLSpec, "problem_value": {}, "clear_value": {}, "ttl": {}}},
		"scale":              {children: map[string]yamlKeySpec{"multiplier": {}, "divisor": {}}},
		"missing":            {},
		"value_from_varbind": {},
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
