// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmp

import (
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

type scalarMetricKey struct {
	name string
	oid  string
}

type columnMetricKey struct {
	table      string
	symbolName string
}

type topologyScalarMetricKey struct {
	kind ddprofiledefinition.TopologyKind
	name string
	oid  string
}

type topologyColumnMetricKey struct {
	kind       ddprofiledefinition.TopologyKind
	table      string
	symbolName string
}

type topologyScalarConflictKey struct {
	name string
	oid  string
}

type topologyColumnConflictKey struct {
	table      string
	symbolName string
}

// FindProfiles returns profiles matching the given sysObjectID.
// Profiles are sorted by match specificity: most specific first.
func FindProfiles(sysObjID, sysDescr string, manualProfiles []string) []*Profile {
	return DefaultCatalog().Resolve(ResolveRequest{
		SysObjectID:    sysObjID,
		SysDescr:       sysDescr,
		ManualProfiles: manualProfiles,
		ManualPolicy:   ManualProfileFallback,
	}).Profiles()
}

// FinalizeProfiles applies load-time profile preparation and deduplicates metrics for a
// given profile list. This mirrors the post-processing performed by FindProfiles.
func FinalizeProfiles(profiles []*Profile) []*Profile {
	if len(profiles) == 0 {
		return nil
	}
	for _, prof := range profiles {
		enrichProfile(prof)
		handleCrossTableTagsWithoutMetrics(prof)
	}
	deduplicateMetricsAcrossProfiles(profiles)
	return profiles
}

type (
	Profile struct {
		SourceFile         string                                 `yaml:"-"`
		Definition         *ddprofiledefinition.ProfileDefinition `yaml:",inline"`
		extensionHierarchy []*extensionInfo
	}
	// extensionInfo represents a single extension in the hierarchy
	extensionInfo struct {
		name       string           // Extension name (e.g., "_base.yaml")
		sourceFile string           // Full path to the extension file
		extensions []*extensionInfo // Nested extensions
	}
)

// SourceTree returns a string representation of the profile source and its extension hierarchy
// Format: "root: [intermediate1: [base], intermediate2]"
func (p *Profile) SourceTree() string {
	rootName := stripFileNameExt(p.SourceFile)

	if len(p.extensionHierarchy) == 0 {
		return rootName
	}

	extensions := formatExtensions(p.extensionHierarchy)
	return fmt.Sprintf("%s: %s", rootName, extensions)
}

// HasExtension returns true if the profile extends the given profile name
// (matches either full filename or filename without extension).
func (p *Profile) HasExtension(name string) bool {
	if p == nil {
		return false
	}
	target := stripFileNameExt(name)
	for _, ext := range p.extensionHierarchy {
		if extensionHas(ext, target) {
			return true
		}
	}
	return false
}

func extensionHas(ext *extensionInfo, target string) bool {
	if ext == nil {
		return false
	}
	if stripFileNameExt(ext.name) == target || stripFileNameExt(ext.sourceFile) == target {
		return true
	}
	for _, child := range ext.extensions {
		if extensionHas(child, target) {
			return true
		}
	}
	return false
}

func formatExtensions(extensions []*extensionInfo) string {
	if len(extensions) == 0 {
		return "[]"
	}

	var items []string
	for _, ext := range extensions {
		name := stripFileNameExt(ext.sourceFile)
		if len(ext.extensions) > 0 {
			items = append(items, fmt.Sprintf("%s: %s", name, formatExtensions(ext.extensions)))
		} else {
			items = append(items, name)
		}
	}

	return fmt.Sprintf("[%s]", strings.Join(items, ", "))
}

func (p *Profile) clone() *Profile {
	cloned := &Profile{
		SourceFile: p.SourceFile,
		Definition: p.Definition.Clone(),
	}
	if p.extensionHierarchy != nil {
		cloned.extensionHierarchy = cloneExtensionHierarchy(p.extensionHierarchy)
	}
	return cloned
}

func cloneExtensionHierarchy(extensions []*extensionInfo) []*extensionInfo {
	if extensions == nil {
		return nil
	}

	cloned := make([]*extensionInfo, len(extensions))
	for i, ext := range extensions {
		cloned[i] = &extensionInfo{
			name:       ext.name,
			sourceFile: ext.sourceFile,
			extensions: cloneExtensionHierarchy(ext.extensions),
		}
	}
	return cloned
}

func (p *Profile) merge(base *Profile) error {
	p.mergeMetadata(base)
	p.mergeMetrics(base)
	if err := p.mergeTopology(base); err != nil {
		return err
	}
	// Append other fields as before (these likely don't need deduplication)
	p.Definition.MetricTags = append(p.Definition.MetricTags, base.Definition.MetricTags...)
	p.Definition.StaticTags = append(slices.Clone(base.Definition.StaticTags), p.Definition.StaticTags...)
	return nil
}

func (p *Profile) mergeMetrics(base *Profile) {
	seenScalars := make(map[scalarMetricKey]bool)
	seenColumns := make(map[columnMetricKey]bool)
	seenTableOIDs := make(map[string]string)

	for _, m := range p.Definition.Metrics {
		switch {
		case m.IsScalar():
			seenScalars[scalarMetricKey{name: m.Symbol.Name, oid: m.Symbol.OID}] = true
		case m.IsColumn():
			seenTableOIDs[columnMetricTableIdentity(m.Table)] = m.Table.OID
			for _, sym := range m.Symbols {
				seenColumns[columnMetricSymbolKey(m.Table, sym)] = true
			}
		}
	}

	for _, bm := range base.Definition.Metrics {
		switch {
		case bm.IsScalar():
			key := scalarMetricKey{name: bm.Symbol.Name, oid: bm.Symbol.OID}
			if !seenScalars[key] {
				p.Definition.Metrics = append(p.Definition.Metrics, bm)
				seenScalars[key] = true
			}
		case bm.IsColumn():
			tableID := columnMetricTableIdentity(bm.Table)
			if tableOID, ok := seenTableOIDs[tableID]; ok && tableOID != bm.Table.OID {
				continue
			}

			symbols := make([]ddprofiledefinition.SymbolConfig, 0, len(bm.Symbols))
			for _, sym := range bm.Symbols {
				key := columnMetricSymbolKey(bm.Table, sym)
				if seenColumns[key] {
					continue
				}
				symbols = append(symbols, sym)
			}
			bm.Symbols = symbols
			if len(bm.Symbols) > 0 {
				p.Definition.Metrics = append(p.Definition.Metrics, bm)
				seenTableOIDs[tableID] = bm.Table.OID
			}
		}
	}

	seenVmetrics := make(map[string]bool)

	for _, m := range p.Definition.VirtualMetrics {
		seenVmetrics[m.Name] = true
	}
	for _, bm := range base.Definition.VirtualMetrics {
		if !seenVmetrics[bm.Name] {
			p.Definition.VirtualMetrics = append(p.Definition.VirtualMetrics, bm)
			seenVmetrics[bm.Name] = true
		}
	}
}

func columnMetricSymbolKey(table ddprofiledefinition.SymbolConfig, sym ddprofiledefinition.SymbolConfig) columnMetricKey {
	return columnMetricKey{
		table:      columnMetricTableIdentity(table),
		symbolName: sym.Name,
	}
}

func columnMetricTableIdentity(table ddprofiledefinition.SymbolConfig) string {
	if table.Name != "" {
		return table.Name
	}
	return table.OID
}

func (p *Profile) mergeTopology(base *Profile) error {
	seenScalars := make(map[topologyScalarMetricKey]bool)
	seenColumns := make(map[topologyColumnMetricKey]bool)
	seenTableOIDs := make(map[string]string)
	scalarKinds := make(map[topologyScalarConflictKey]ddprofiledefinition.TopologyKind)
	columnKinds := make(map[topologyColumnConflictKey]ddprofiledefinition.TopologyKind)

	for _, topo := range p.Definition.Topology {
		if err := indexTopologyMergeConflicts(topo, scalarKinds, columnKinds); err != nil {
			return err
		}
		switch {
		case topo.IsScalar():
			seenScalars[topologyScalarMetricKey{kind: topo.Kind, name: topo.Symbol.Name, oid: topo.Symbol.OID}] = true
		case topo.IsColumn():
			seenTableOIDs[topologyColumnTableIdentity(topo.Kind, topo.Table)] = topo.Table.OID
			for _, sym := range topo.Symbols {
				seenColumns[topologyColumnSymbolKey(topo.Kind, topo.Table, sym)] = true
			}
		}
	}

	for _, baseTopo := range base.Definition.Topology {
		if err := indexTopologyMergeConflicts(baseTopo, scalarKinds, columnKinds); err != nil {
			return err
		}
		switch {
		case baseTopo.IsScalar():
			key := topologyScalarMetricKey{kind: baseTopo.Kind, name: baseTopo.Symbol.Name, oid: baseTopo.Symbol.OID}
			if !seenScalars[key] {
				p.Definition.Topology = append(p.Definition.Topology, baseTopo)
				seenScalars[key] = true
			}
		case baseTopo.IsColumn():
			tableID := topologyColumnTableIdentity(baseTopo.Kind, baseTopo.Table)
			if tableOID, ok := seenTableOIDs[tableID]; ok && tableOID != baseTopo.Table.OID {
				continue
			}

			symbols := make([]ddprofiledefinition.SymbolConfig, 0, len(baseTopo.Symbols))
			for _, sym := range baseTopo.Symbols {
				key := topologyColumnSymbolKey(baseTopo.Kind, baseTopo.Table, sym)
				if seenColumns[key] {
					continue
				}
				symbols = append(symbols, sym)
			}
			baseTopo.Symbols = symbols
			if len(baseTopo.Symbols) > 0 {
				p.Definition.Topology = append(p.Definition.Topology, baseTopo)
				seenTableOIDs[tableID] = baseTopo.Table.OID
			}
		}
	}

	return nil
}

func indexTopologyMergeConflicts(
	topo ddprofiledefinition.TopologyConfig,
	scalarKinds map[topologyScalarConflictKey]ddprofiledefinition.TopologyKind,
	columnKinds map[topologyColumnConflictKey]ddprofiledefinition.TopologyKind,
) error {
	switch {
	case topo.IsScalar():
		key := topologyScalarConflictKey{name: topo.Symbol.Name, oid: topo.Symbol.OID}
		return indexTopologyKindConflict(fmt.Sprintf("scalar %q/%q", topo.Symbol.Name, topo.Symbol.OID), key, topo.Kind, scalarKinds)
	case topo.IsColumn():
		for _, sym := range topo.Symbols {
			key := topologyColumnConflictKey{table: columnMetricTableIdentity(topo.Table), symbolName: sym.Name}
			if err := indexTopologyKindConflict(fmt.Sprintf("table %q symbol %q", columnMetricTableIdentity(topo.Table), sym.Name), key, topo.Kind, columnKinds); err != nil {
				return err
			}
		}
	}
	return nil
}

func indexTopologyKindConflict[K comparable](
	label string,
	key K,
	kind ddprofiledefinition.TopologyKind,
	seen map[K]ddprofiledefinition.TopologyKind,
) error {
	if existingKind, ok := seen[key]; ok && existingKind != kind {
		return fmt.Errorf("conflicting topology kinds for %s: %q and %q", label, existingKind, kind)
	}
	seen[key] = kind
	return nil
}

func topologyColumnSymbolKey(kind ddprofiledefinition.TopologyKind, table ddprofiledefinition.SymbolConfig, sym ddprofiledefinition.SymbolConfig) topologyColumnMetricKey {
	return topologyColumnMetricKey{
		kind:       kind,
		table:      columnMetricTableIdentity(table),
		symbolName: sym.Name,
	}
}

func topologyColumnTableIdentity(kind ddprofiledefinition.TopologyKind, table ddprofiledefinition.SymbolConfig) string {
	return string(kind) + "|" + columnMetricTableIdentity(table)
}

func (p *Profile) mergeMetadata(base *Profile) {
	if p.Definition.Metadata == nil {
		p.Definition.Metadata = make(ddprofiledefinition.MetadataConfig)
	}

	for resName, baseRes := range base.Definition.Metadata {
		targetRes, exists := p.Definition.Metadata[resName]
		if !exists {
			targetRes = ddprofiledefinition.MetadataResourceConfig{}
		}

		targetRes.IDTags = append(targetRes.IDTags, baseRes.IDTags...)

		if targetRes.Fields == nil && len(baseRes.Fields) > 0 {
			targetRes.Fields = make(map[string]ddprofiledefinition.MetadataField, len(baseRes.Fields))
		}

		for field, symbol := range baseRes.Fields {
			if _, ok := targetRes.Fields[field]; !ok {
				targetRes.Fields[field] = symbol
			}
		}

		p.Definition.Metadata[resName] = targetRes
	}

	if len(base.Definition.SysobjectIDMetadata) > 0 {
		existingOIDs := make(map[string]bool)
		for _, entry := range p.Definition.SysobjectIDMetadata {
			existingOIDs[entry.SysobjectID] = true
		}

		for _, baseEntry := range base.Definition.SysobjectIDMetadata {
			if !existingOIDs[baseEntry.SysobjectID] {
				p.Definition.SysobjectIDMetadata = append(p.Definition.SysobjectIDMetadata, baseEntry)
			}
		}
	}
}

func (p *Profile) validate() error {
	return ddprofiledefinition.ValidateEnrichProfile(p.Definition)
}

func (p *Profile) removeConstantMetrics() {
	if p.Definition == nil {
		return
	}

	p.Definition.Metrics = slices.DeleteFunc(p.Definition.Metrics, func(m ddprofiledefinition.MetricsConfig) bool {
		if m.IsScalar() && m.Symbol.ConstantValueOne {
			return true
		}

		if m.IsColumn() {
			m.Symbols = slices.DeleteFunc(m.Symbols, func(s ddprofiledefinition.SymbolConfig) bool {
				return s.ConstantValueOne
			})
		}

		return m.IsColumn() && len(m.Symbols) == 0
	})
}

// sortProfilesBySpecificity sorts profiles by their match specificity.
// More specific profiles (longer OIDs, exact matches) come first.
// The matchedOIDs map contains the OID that matched for each profile.
func sortProfilesBySpecificity(profiles []*Profile, matchedOIDs map[*Profile]string) {
	slices.SortStableFunc(profiles, func(a, b *Profile) int {
		aOID := matchedOIDs[a]
		bOID := matchedOIDs[b]

		// 0) Profiles with an OID match (non-empty) come before descr-only matches (empty)
		aHasOID := aOID != ""
		bHasOID := bOID != ""
		if aHasOID != bHasOID {
			if aHasOID {
				return -1
			}
			return 1
		}

		// If both are descr-only (both empty), keep stable order.
		if !aHasOID && !bHasOID {
			return 0
		}

		// 1) Longer OIDs first (more specific)
		if diff := len(bOID) - len(aOID); diff != 0 {
			return diff
		}

		// 2) Same length: exact OIDs before patterns
		aIsExact := ddprofiledefinition.IsPlainOid(aOID)
		bIsExact := ddprofiledefinition.IsPlainOid(bOID)
		if aIsExact != bIsExact {
			if aIsExact {
				return -1
			}
			return 1
		}

		// 3) Same type: lexicographic order for stability
		return strings.Compare(aOID, bOID)
	})
}

func enrichProfile(prof *Profile) {
	if prof.Definition == nil {
		return
	}

	for i := range prof.Definition.Metrics {
		enrichMetricTagMappingRefs(prof.Definition.Metrics[i].MetricTags)
	}
	for i := range prof.Definition.Topology {
		enrichMetricTagMappingRefs(prof.Definition.Topology[i].MetricTags)
	}
}

func enrichMetricTagMappingRefs(tags ddprofiledefinition.MetricTagConfigList) {
	for j := range tags {
		tagCfg := &tags[j]

		if tagCfg.Mapping.HasItems() {
			continue
		}

		switch tagCfg.MappingRef {
		case "ifType":
			tagCfg.Mapping = ddprofiledefinition.NewExactMapping(sharedMappings.ifType)
		case "ifTypeGroup":
			tagCfg.Mapping = ddprofiledefinition.NewExactMapping(sharedMappings.ifTypeGroup)
		}
	}
}

func deduplicateMetricsAcrossProfiles(profiles []*Profile) {
	if len(profiles) < 2 {
		return
	}

	// Profiles are already sorted by specificity from FindProfiles
	// Just deduplicate metrics, keeping the first occurrence (most specific)
	seenMetrics := make(map[string]bool)
	seenVmetrics := make(map[string]bool)

	for _, prof := range profiles {
		if prof.Definition == nil {
			continue
		}

		prof.Definition.Metrics = slices.DeleteFunc(
			prof.Definition.Metrics,
			func(metric ddprofiledefinition.MetricsConfig) bool {
				key := generateMetricKey(metric)
				if seenMetrics[key] {
					return true
				}
				seenMetrics[key] = true
				return false
			},
		)

		prof.Definition.VirtualMetrics = slices.DeleteFunc(
			prof.Definition.VirtualMetrics,
			func(vm ddprofiledefinition.VirtualMetricConfig) bool {
				if seenVmetrics[vm.Name] {
					return true
				}
				seenVmetrics[vm.Name] = true
				return false
			},
		)

		deduplicateTopologyInProfile(prof, seenMetrics)
	}
}

func deduplicateTopologyInProfile(prof *Profile, seenMetrics map[string]bool) {
	filtered := prof.Definition.Topology[:0]
	for _, topo := range prof.Definition.Topology {
		if topo.IsScalar() {
			key := generateTopologyScalarMetricKey(topo)
			if seenMetrics[key] {
				continue
			}
			seenMetrics[key] = true
			filtered = append(filtered, topo)
			continue
		}

		if topo.IsColumn() {
			symbols := topo.Symbols[:0]
			for _, sym := range topo.Symbols {
				key := generateTopologyColumnMetricKey(topo, sym)
				if seenMetrics[key] {
					continue
				}
				seenMetrics[key] = true
				symbols = append(symbols, sym)
			}
			topo.Symbols = symbols
			if len(topo.Symbols) == 0 {
				continue
			}
		}

		filtered = append(filtered, topo)
	}
	if len(filtered) == 0 {
		prof.Definition.Topology = nil
		return
	}
	prof.Definition.Topology = filtered
}

func generateTopologyScalarMetricKey(topo ddprofiledefinition.TopologyConfig) string {
	return strings.Join([]string{
		"topology-scalar",
		string(topo.Kind),
		topo.Symbol.OID,
		topo.Symbol.Name,
	}, "|")
}

func generateTopologyColumnMetricKey(topo ddprofiledefinition.TopologyConfig, sym ddprofiledefinition.SymbolConfig) string {
	return strings.Join([]string{
		"topology-table",
		string(topo.Kind),
		topo.Table.OID,
		columnMetricTableIdentity(topo.Table),
		sym.Name,
	}, "|")
}

func generateMetricKey(metric ddprofiledefinition.MetricsConfig) string {
	var parts []string

	if metric.IsScalar() {
		parts = append(parts, "scalar")
		parts = append(parts, metric.Symbol.OID)
		parts = append(parts, metric.Symbol.Name)
		return strings.Join(parts, "|")
	}

	parts = append(parts, "table")
	parts = append(parts, metric.Table.OID)
	parts = append(parts, metric.Table.Name)

	symbolKeys := make([]string, 0, len(metric.Symbols))
	for _, sym := range metric.Symbols {
		symbolKey := fmt.Sprintf("%s:%s", sym.OID, sym.Name)
		symbolKeys = append(symbolKeys, symbolKey)
	}
	sort.Strings(symbolKeys)
	parts = append(parts, symbolKeys...)

	return strings.Join(parts, "|")
}

func stripFileNameExt(path string) string {
	return strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
}
