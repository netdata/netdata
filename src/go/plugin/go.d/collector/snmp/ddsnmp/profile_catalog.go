// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmp

import (
	"slices"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

type ProfileConsumer = ddprofiledefinition.ProfileConsumer

const (
	ConsumerMetrics  = ddprofiledefinition.ConsumerMetrics
	ConsumerTopology = ddprofiledefinition.ConsumerTopology
)

type ManualProfilePolicy int

const (
	ManualProfileFallback ManualProfilePolicy = iota
	ManualProfileAugment
	ManualProfileOverride
)

type Catalog struct {
	profiles []*Profile
}

type ResolveRequest struct {
	SysObjectID    string
	SysDescr       string
	ManualProfiles []string
	ManualPolicy   ManualProfilePolicy
}

type ResolvedProfileSet struct {
	profiles []*Profile
}

type ProjectedView struct {
	profiles []*Profile
}

func DefaultCatalog() *Catalog {
	return &Catalog{}
}

func (c *Catalog) Resolve(req ResolveRequest) *ResolvedProfileSet {
	available := c.catalogProfiles()

	switch {
	case req.ManualPolicy == ManualProfileOverride:
		return &ResolvedProfileSet{profiles: finalizeResolvedProfiles(selectManualProfiles(available, req.ManualProfiles))}
	case req.SysObjectID == "":
		if len(req.ManualProfiles) == 0 {
			log.Warning("No sysObjectID found and no manual_profiles configured. Either ensure the device provides sysObjectID or configure manual_profiles option.")
			return &ResolvedProfileSet{}
		}
		return &ResolvedProfileSet{profiles: finalizeResolvedProfiles(selectManualProfiles(available, req.ManualProfiles))}
	default:
		profiles := selectMatchedProfiles(available, req.SysObjectID, req.SysDescr)
		if req.ManualPolicy == ManualProfileAugment {
			profiles = appendMissingManualProfiles(profiles, req.ManualProfiles, available)
		}
		return &ResolvedProfileSet{profiles: finalizeResolvedProfiles(profiles)}
	}
}

func (c *Catalog) catalogProfiles() []*Profile {
	if c != nil && c.profiles != nil {
		return c.profiles
	}
	loadProfiles()
	return ddProfiles
}

func (r *ResolvedProfileSet) Profiles() []*Profile {
	if r == nil {
		return nil
	}
	return r.profiles
}

func (r *ResolvedProfileSet) Project(consumer ProfileConsumer) ProjectedView {
	if r == nil || len(r.profiles) == 0 {
		return ProjectedView{}
	}

	profiles := make([]*Profile, 0, len(r.profiles))
	for _, prof := range r.profiles {
		projected := prof.clone()
		projectProfile(projected, consumer)
		if profileHasProjectedData(projected.Definition, consumer) {
			profiles = append(profiles, projected)
		}
	}
	return ProjectedView{profiles: profiles}
}

func (v ProjectedView) Profiles() []*Profile {
	return v.profiles
}

func (v ProjectedView) FilterByKind(kinds map[ddprofiledefinition.TopologyKind]bool) ProjectedView {
	for _, prof := range v.profiles {
		if prof == nil || prof.Definition == nil {
			continue
		}
		prof.Definition.Metrics = nil
		prof.Definition.Topology = slices.DeleteFunc(prof.Definition.Topology, func(topo ddprofiledefinition.TopologyConfig) bool {
			return !kinds[topo.Kind]
		})
	}
	return ProjectedView{profiles: slices.DeleteFunc(v.profiles, func(prof *Profile) bool {
		return prof == nil || prof.Definition == nil || (len(prof.Definition.Topology) == 0 && len(prof.Definition.Metrics) == 0)
	})}
}

func selectMatchedProfiles(available []*Profile, sysObjID, sysDescr string) []*Profile {
	matchedOIDs := make(map[*Profile]string)
	var selected []*Profile
	for _, prof := range available {
		if ok, matchedOid := prof.Definition.Selector.Matches(sysObjID, sysDescr); ok {
			cloned := prof.clone()
			selected = append(selected, cloned)
			matchedOIDs[cloned] = matchedOid
		}
	}
	sortProfilesBySpecificity(selected, matchedOIDs)
	return selected
}

func selectManualProfiles(available []*Profile, manualProfiles []string) []*Profile {
	var selected []*Profile
	for _, prof := range available {
		name := stripFileNameExt(prof.SourceFile)
		if slices.ContainsFunc(manualProfiles, func(p string) bool { return stripFileNameExt(p) == name }) {
			selected = append(selected, prof.clone())
		}
	}
	return selected
}

func appendMissingManualProfiles(profiles []*Profile, manualProfiles []string, available []*Profile) []*Profile {
	seen := make(map[string]bool)
	for _, prof := range profiles {
		seen[stripFileNameExt(prof.SourceFile)] = true
	}
	for _, prof := range selectManualProfiles(available, manualProfiles) {
		name := stripFileNameExt(prof.SourceFile)
		if !seen[name] {
			profiles = append(profiles, prof)
			seen[name] = true
		}
	}
	return profiles
}

func finalizeResolvedProfiles(profiles []*Profile) []*Profile {
	if len(profiles) == 0 {
		return nil
	}
	deduplicateMetricsAcrossProfiles(profiles)
	return profiles
}

func projectProfile(prof *Profile, consumer ProfileConsumer) {
	if prof == nil || prof.Definition == nil {
		return
	}

	def := prof.Definition
	def.Metadata = projectMetadata(def.Metadata, consumer)
	def.SysobjectIDMetadata = projectSysobjectIDMetadata(def.SysobjectIDMetadata, consumer)
	def.MetricTags = projectGlobalMetricTags(def.MetricTags, consumer)

	switch consumer {
	case ConsumerMetrics:
		def.Topology = nil
	case ConsumerTopology:
		def.Metrics = nil
		def.VirtualMetrics = nil
	default:
		def.Metrics = nil
		def.Topology = nil
		def.VirtualMetrics = nil
		def.Metadata = nil
		def.SysobjectIDMetadata = nil
		def.MetricTags = nil
	}
}

func projectMetadata(meta ddprofiledefinition.MetadataConfig, consumer ProfileConsumer) ddprofiledefinition.MetadataConfig {
	if len(meta) == 0 {
		return nil
	}
	projected := make(ddprofiledefinition.MetadataConfig)
	for resName, res := range meta {
		fields := make(map[string]ddprofiledefinition.MetadataField)
		for name, field := range res.Fields {
			if consumersInclude(field.Consumers, consumer) {
				fields[name] = field
			}
		}
		idTags := projectMetricTagList(res.IDTags, consumer)
		if len(fields) == 0 && len(idTags) == 0 {
			continue
		}
		projected[resName] = ddprofiledefinition.MetadataResourceConfig{
			Fields: fields,
			IDTags: idTags,
		}
	}
	if len(projected) == 0 {
		return nil
	}
	return projected
}

func projectSysobjectIDMetadata(entries []ddprofiledefinition.SysobjectIDMetadataEntryConfig, consumer ProfileConsumer) []ddprofiledefinition.SysobjectIDMetadataEntryConfig {
	if len(entries) == 0 {
		return nil
	}
	projected := make([]ddprofiledefinition.SysobjectIDMetadataEntryConfig, 0, len(entries))
	for _, entry := range entries {
		fields := make(map[string]ddprofiledefinition.MetadataField)
		for name, field := range entry.Metadata {
			if consumersInclude(field.Consumers, consumer) {
				fields[name] = field
			}
		}
		if len(fields) == 0 {
			continue
		}
		projected = append(projected, ddprofiledefinition.SysobjectIDMetadataEntryConfig{
			SysobjectID: entry.SysobjectID,
			Metadata:    fields,
		})
	}
	if len(projected) == 0 {
		return nil
	}
	return projected
}

func projectMetricTagList(tags []ddprofiledefinition.MetricTagConfig, consumer ProfileConsumer) []ddprofiledefinition.MetricTagConfig {
	// Metadata id_tags do not carry Consumers today. They inherit metadata defaults.
	if consumer == ConsumerMetrics || consumer == ConsumerTopology {
		return tags
	}
	return nil
}

func projectGlobalMetricTags(tags []ddprofiledefinition.GlobalMetricTagConfig, consumer ProfileConsumer) []ddprofiledefinition.GlobalMetricTagConfig {
	filtered := tags[:0]
	for _, tag := range tags {
		if consumersInclude(tag.Consumers, consumer) {
			filtered = append(filtered, tag)
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	return filtered
}

func consumersInclude(consumers ddprofiledefinition.ConsumerSet, consumer ProfileConsumer) bool {
	return len(consumers) == 0 || consumers.Contains(consumer)
}

func profileHasProjectedData(def *ddprofiledefinition.ProfileDefinition, consumer ProfileConsumer) bool {
	if def == nil {
		return false
	}
	switch consumer {
	case ConsumerMetrics:
		return len(def.Metrics) > 0 ||
			len(def.VirtualMetrics) > 0 ||
			len(def.MetricTags) > 0 ||
			len(def.Metadata) > 0 ||
			len(def.SysobjectIDMetadata) > 0
	case ConsumerTopology:
		return len(def.Topology) > 0 ||
			len(def.MetricTags) > 0 ||
			len(def.Metadata) > 0 ||
			len(def.SysobjectIDMetadata) > 0
	default:
		return false
	}
}
