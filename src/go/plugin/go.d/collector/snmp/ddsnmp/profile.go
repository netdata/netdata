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

// FindProfiles returns profiles matching the given sysObjectID.
// Profiles are sorted by match specificity: most specific first.
func FindProfiles(sysObjID, sysDescr string, manualProfiles []string) []*Profile {
	loadProfiles()

	finalize := func(profiles []*Profile) []*Profile {
		if len(profiles) == 0 {
			return nil
		}
		enrichProfiles(profiles)
		deduplicateMetricsAcrossProfiles(profiles)
		return profiles
	}

	// Fallback/manual path (no sysObjectID)
	if sysObjID == "" {
		if len(manualProfiles) == 0 {
			log.Warning("No sysObjectID found and no manual_profiles configured. Either ensure the device provides sysObjectID or configure manual_profiles option.")
			return nil
		}

		var selected []*Profile
		for _, prof := range ddProfiles {
			name := stripFileNameExt(prof.SourceFile)
			if slices.ContainsFunc(manualProfiles, func(p string) bool { return stripFileNameExt(p) == name }) {
				selected = append(selected, prof.clone())
			}
		}

		return finalize(selected)
	}

	// Auto-detect path
	matchedOIDs := make(map[*Profile]string)
	var selected []*Profile

	for _, prof := range ddProfiles {
		if ok, matchedOid := prof.Definition.Selector.Matches(sysObjID, sysDescr); ok {
			cloned := prof.clone()
			selected = append(selected, cloned)
			matchedOIDs[cloned] = matchedOid
		}
	}

	sortProfilesBySpecificity(selected, matchedOIDs)
	return finalize(selected)
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

func (p *Profile) merge(base *Profile) {
	p.mergeMetadata(base)
	p.mergeMetrics(base)
	// Append other fields as before (these likely don't need deduplication)
	p.Definition.MetricTags = append(p.Definition.MetricTags, base.Definition.MetricTags...)
	p.Definition.StaticTags = append(p.Definition.StaticTags, base.Definition.StaticTags...)
}

func (p *Profile) mergeMetrics(base *Profile) {
	seen := make(map[string]bool)

	for _, m := range p.Definition.Metrics {
		switch {
		case m.IsScalar():
			seen[m.Symbol.Name+"|"+m.Symbol.OID] = true
		case m.IsColumn():
			for _, sym := range m.Symbols {
				seen[sym.Name] = true
			}
		}
	}

	for _, bm := range base.Definition.Metrics {
		switch {
		case bm.IsScalar():
			key := bm.Symbol.Name + "|" + bm.Symbol.OID
			if !seen[key] {
				p.Definition.Metrics = append(p.Definition.Metrics, bm)
				seen[key] = true
			}
		case bm.IsColumn():
			bm.Symbols = slices.DeleteFunc(bm.Symbols, func(sym ddprofiledefinition.SymbolConfig) bool {
				v := seen[sym.Name]
				seen[sym.Name] = true
				return v
			})
			if len(bm.Symbols) > 0 {
				p.Definition.Metrics = append(p.Definition.Metrics, bm)
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

func (p *Profile) mergeMetadata(base *Profile) {
	if p.Definition.Metadata == nil {
		p.Definition.Metadata = make(ddprofiledefinition.MetadataConfig)
	}

	for resName, baseRes := range base.Definition.Metadata {
		targetRes, exists := p.Definition.Metadata[resName]
		if !exists {
			targetRes = ddprofiledefinition.NewMetadataResourceConfig()
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

		// 1. Longer OIDs first (more specific)
		if diff := len(bOID) - len(aOID); diff != 0 {
			return diff
		}

		// 2. Same length: exact OIDs before patterns
		aIsExact := ddprofiledefinition.IsPlainOid(aOID)
		bIsExact := ddprofiledefinition.IsPlainOid(bOID)
		if aIsExact != bIsExact {
			if aIsExact {
				return -1
			}
			return 1
		}

		// 3. Same type: lexicographic order for stability
		return strings.Compare(aOID, bOID)
	})
}

func enrichProfiles(profiles []*Profile) {
	for _, prof := range profiles {
		if prof.Definition == nil {
			continue
		}

		for i := range prof.Definition.Metrics {
			metric := &prof.Definition.Metrics[i]

			for j := range metric.MetricTags {
				tagCfg := &metric.MetricTags[j]

				if tagCfg.Mapping != nil {
					continue
				}

				if tagCfg.MappingRef == "ifType" {
					tagCfg.Mapping = sharedMappings.ifType
				}
			}
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
	}
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
