// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmp

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/multipath"
	"github.com/netdata/netdata/go/plugins/pkg/pluginconfig"
)

// ProfileSource identifies a loaded source without disclosing its absolute path.
// Priority is one-based search-root order; zero means the root is unknown.
type ProfileSource struct {
	ID       string `json:"id"`
	Class    string `json:"class"`
	Priority int    `json:"priority"`
}

type ProfileProjectionCounts struct {
	Metrics           int `json:"metrics"`
	VirtualMetrics    int `json:"virtual_metrics"`
	Topology          int `json:"topology"`
	BGP               int `json:"bgp"`
	Licensing         int `json:"licensing"`
	Metadata          int `json:"metadata_resources"`
	SysObjectMetadata int `json:"sys_object_metadata_rules"`
	Tags              int `json:"global_tags"`
}

type ProfileExtensionContext struct {
	Source ProfileSource `json:"source"`
	Parent int           `json:"parent"`
}

type SelectedProfileContext struct {
	Source          ProfileSource             `json:"source"`
	Origin          string                    `json:"origin"`
	MatchedSelector string                    `json:"matched_selector,omitempty"`
	Extensions      []ProfileExtensionContext `json:"extensions,omitempty"`
	// Zero means excluded. Otherwise this is the one-based final projection order.
	ProjectedOrdinal int                     `json:"projected_ordinal"`
	AcquisitionIndex *uint32                 `json:"acquisition_index,omitempty"`
	Projection       ProfileProjectionCounts `json:"projection"`
}

type ProfileContextData struct {
	SysObjectID           string                   `json:"sys_object_id,omitempty"`
	SysDescr              string                   `json:"sys_descr,omitempty"`
	State                 string                   `json:"capture_state"`
	ManualPolicy          string                   `json:"manual_policy,omitempty"`
	ManualApplied         bool                     `json:"manual_applied"`
	ManualProfiles        []string                 `json:"manual_profiles,omitempty"`
	MissingManualProfiles []string                 `json:"missing_manual_profiles,omitempty"`
	Consumers             []ProfileConsumer        `json:"consumers,omitempty"`
	BGPMode               string                   `json:"bgp_mode,omitempty"`
	TopologyKinds         []string                 `json:"topology_kinds,omitempty"`
	Selected              []SelectedProfileContext `json:"selected,omitempty"`
}

// ProfileContext is detached and immutable. Store cuts can share it without
// copying profile evidence on each poll or retaining profile definitions.
type ProfileContext struct {
	data           ProfileContextData
	records, bytes uint64
}

func (c *ProfileContext) Shape() (uint64, uint64) {
	if c == nil || c.data.State != "available" {
		return 0, 0
	}
	return c.records, c.bytes
}

func (c *ProfileContext) Snapshot() ProfileContextData {
	if c == nil {
		return ProfileContextData{State: "not_attempted"}
	}
	d := c.data
	d.ManualProfiles = slices.Clone(d.ManualProfiles)
	d.MissingManualProfiles = slices.Clone(d.MissingManualProfiles)
	d.Consumers = slices.Clone(d.Consumers)
	d.TopologyKinds = slices.Clone(d.TopologyKinds)
	d.Selected = slices.Clone(d.Selected)
	for i := range d.Selected {
		d.Selected[i].Extensions = slices.Clone(d.Selected[i].Extensions)
		if d.Selected[i].AcquisitionIndex != nil {
			index := *d.Selected[i].AcquisitionIndex
			d.Selected[i].AcquisitionIndex = &index
		}
	}
	return d
}

func profileSource(filename string, roots multipath.MultiPath) ProfileSource {
	s := ProfileSource{ID: profileOriginID(filename, roots), Class: "unknown"}
	abs, err := filepath.Abs(filename)
	if err != nil {
		return s
	}
	for i, root := range roots {
		rootAbs, err := filepath.Abs(root)
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(rootAbs, abs)
		if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		s.Priority = i + 1
		if pluginconfig.StockConfigDir() != "" {
			s.Class = "user"
			if pluginconfig.IsStock(root) {
				s.Class = "stock"
			}
		}
		break
	}
	return s
}

func contextSource(p *Profile) ProfileSource {
	if p.sourceOrigin.ID != "" {
		return p.sourceOrigin
	}
	return ProfileSource{ID: filepath.Base(p.SourceFile), Class: "unknown"}
}

// Context observes the completed projection; it never runs selectors again.
// Admission precedes materialization of the detached report.
func (v ProjectedView) Context(maxRecords, maxBytes uint64) *ProfileContext {
	d := ProfileContextData{State: "available", Consumers: v.consumers, BGPMode: v.bgpMode, TopologyKinds: v.topologyKinds}
	if v.resolved == nil {
		return nil
	}
	r := v.resolved
	d.SysObjectID, d.SysDescr = r.sysObjectID, r.sysDescr
	switch r.manualPolicy {
	case ManualProfileFallback:
		d.ManualPolicy = "fallback"
	case ManualProfileAugment:
		d.ManualPolicy = "augment"
	case ManualProfileOverride:
		d.ManualPolicy = "override"
	}
	d.ManualApplied = r.manualApplied
	d.ManualProfiles = r.manualProfiles
	d.MissingManualProfiles = r.missingManualProfiles
	records, size := uint64(1), uint64(64+len(d.SysObjectID)+len(d.SysDescr))
	add := func(s string) { records++; size += uint64(16 + len(s)) }
	for _, s := range d.ManualProfiles {
		add(s)
	}
	for _, s := range d.MissingManualProfiles {
		add(s)
	}
	for _, s := range d.Consumers {
		add(string(s))
	}
	for _, s := range d.TopologyKinds {
		add(s)
	}
	var shapeExtensions func([]*extensionInfo)
	shapeExtensions = func(es []*extensionInfo) {
		for _, e := range es {
			source := e.origin
			if source.ID == "" {
				source = ProfileSource{ID: filepath.Base(e.sourceFile), Class: "unknown"}
			}
			records++
			size += uint64(64 + len(source.ID) + len(source.Class))
			shapeExtensions(e.extensions)
		}
	}
	for _, p := range r.profiles {
		source := contextSource(p)
		records++
		size += uint64(128 + len(source.ID) + len(source.Class) + len(p.selectionOrigin) + len(p.matchedSelector))
		shapeExtensions(p.extensionHierarchy)
	}
	if records > maxRecords || size > maxBytes {
		return &ProfileContext{data: ProfileContextData{State: "limit_exceeded"}, records: 1, bytes: 32}
	}
	acquisitionSources := make([]string, 0, len(v.profiles))
	for _, p := range v.profiles {
		acquisitionSources = append(acquisitionSources, p.SourceFile)
	}
	slices.Sort(acquisitionSources)
	acquisitionIndexes := make(map[string]uint32, len(acquisitionSources))
	for i, source := range acquisitionSources {
		acquisitionIndexes[source] = uint32(i)
	}
	ordinals := make(map[string]int, len(v.profiles))
	for i, p := range v.profiles {
		ordinals[p.SourceFile] = i + 1
	}
	for _, p := range r.profiles {
		row := SelectedProfileContext{
			Source:           contextSource(p),
			Origin:           p.selectionOrigin,
			MatchedSelector:  p.matchedSelector,
			ProjectedOrdinal: ordinals[p.SourceFile],
		}
		var extensions func([]*extensionInfo, int)
		extensions = func(es []*extensionInfo, parent int) {
			for _, e := range es {
				source := e.origin
				if source.ID == "" {
					source = ProfileSource{ID: filepath.Base(e.sourceFile), Class: "unknown"}
				}
				row.Extensions = append(row.Extensions, ProfileExtensionContext{Source: source, Parent: parent})
				extensions(e.extensions, len(row.Extensions))
			}
		}
		extensions(p.extensionHierarchy, 0)
		if row.ProjectedOrdinal > 0 {
			index := acquisitionIndexes[p.SourceFile]
			row.AcquisitionIndex = &index
			def := v.profiles[row.ProjectedOrdinal-1].Definition
			row.Projection = ProfileProjectionCounts{
				Metrics:           len(def.Metrics),
				VirtualMetrics:    len(def.VirtualMetrics),
				Topology:          len(def.Topology),
				BGP:               len(def.BGP),
				Licensing:         len(def.Licensing),
				Metadata:          len(def.Metadata),
				SysObjectMetadata: len(def.SysobjectIDMetadata),
				Tags:              len(def.MetricTags),
			}
		}
		d.Selected = append(d.Selected, row)
	}
	c := &ProfileContext{data: d, records: records, bytes: size}
	if _, err := validateProfileContext(d, maxRecords, maxBytes); err != nil {
		return &ProfileContext{data: ProfileContextData{State: "unavailable"}}
	}
	c.data = c.Snapshot()
	return c
}

// RestoreProfileContext validates portable evidence before making it immutable.
func RestoreProfileContext(d ProfileContextData, maxRecords, maxBytes uint64) (*ProfileContext, error) {
	c, err := validateProfileContext(d, maxRecords, maxBytes)
	if err != nil || c == nil {
		return c, err
	}
	c.data = c.Snapshot()
	return c, nil
}

func validateProfileContext(d ProfileContextData, maxRecords, maxBytes uint64) (*ProfileContext, error) {
	if d.State == "not_attempted" || d.State == "limit_exceeded" || d.State == "unavailable" {
		if d.ManualPolicy != "" || d.ManualApplied || d.BGPMode != "" || d.SysObjectID != "" || d.SysDescr != "" ||
			len(d.ManualProfiles)+len(d.MissingManualProfiles)+len(d.Consumers)+len(d.TopologyKinds)+len(d.Selected) != 0 {
			return nil, fmt.Errorf("unavailable profile context contains evidence")
		}
		if d.State == "not_attempted" {
			return nil, nil
		}
		return &ProfileContext{data: d}, nil
	}
	if d.State != "available" {
		return nil, fmt.Errorf("invalid profile context state")
	}
	if d.ManualPolicy != "fallback" && d.ManualPolicy != "augment" && d.ManualPolicy != "override" {
		return nil, fmt.Errorf("invalid manual policy")
	}
	c := &ProfileContext{data: d, records: 1, bytes: uint64(64 + len(d.SysObjectID) + len(d.SysDescr))}
	add := func(s string) { c.records++; c.bytes += uint64(16 + len(s)) }
	for _, s := range d.ManualProfiles {
		add(s)
	}
	for _, s := range d.MissingManualProfiles {
		add(s)
	}
	for _, s := range d.Consumers {
		if s != ConsumerMetrics && s != ConsumerTopology && s != ConsumerBGP && s != ConsumerLicensing {
			return nil, fmt.Errorf("invalid profile consumer")
		}
		add(string(s))
	}
	for _, s := range d.TopologyKinds {
		add(s)
	}
	validSource := func(s ProfileSource) bool {
		return s.ID != "" && !strings.Contains(s.ID, "\\") && !strings.HasPrefix(s.ID, "/") && !strings.Contains(s.ID, ":") &&
			!slices.Contains(strings.Split(s.ID, "/"), "..") &&
			s.Priority >= 0 &&
			(s.Class == "stock" || s.Class == "user" || s.Class == "unknown")
	}
	if d.BGPMode != "full" && d.BGPMode != "absent" && d.BGPMode != "topology_peers" {
		return nil, fmt.Errorf("invalid BGP projection")
	}
	if uint64(len(d.Selected)) > maxRecords || c.records > maxRecords || c.bytes > maxBytes {
		return nil, fmt.Errorf("profile context exceeds limits")
	}
	projected := make(map[int]bool)
	acquisition := make(map[uint32]bool)
	for _, p := range d.Selected {
		if !validSource(p.Source) || p.ProjectedOrdinal < 0 || p.ProjectedOrdinal > len(d.Selected) {
			return nil, fmt.Errorf("invalid selected profile source or ordinal")
		}
		if p.Origin != "selector" && p.Origin != "manual" {
			return nil, fmt.Errorf("invalid profile selection origin")
		}
		counts := p.Projection
		if counts.Metrics < 0 || counts.VirtualMetrics < 0 || counts.Topology < 0 || counts.BGP < 0 || counts.Licensing < 0 ||
			counts.Metadata < 0 ||
			counts.SysObjectMetadata < 0 ||
			counts.Tags < 0 {
			return nil, fmt.Errorf("invalid projection counts")
		}
		if p.ProjectedOrdinal == 0 {
			if p.AcquisitionIndex != nil || counts != (ProfileProjectionCounts{}) {
				return nil, fmt.Errorf("excluded profile contains projected evidence")
			}
		} else {
			if projected[p.ProjectedOrdinal] || p.AcquisitionIndex == nil || acquisition[*p.AcquisitionIndex] || *p.AcquisitionIndex >= uint32(len(d.Selected)) {
				return nil, fmt.Errorf("invalid acquisition order")
			}
			projected[p.ProjectedOrdinal] = true
			acquisition[*p.AcquisitionIndex] = true
		}
		c.records++
		c.bytes += uint64(128 + len(p.Source.ID) + len(p.Source.Class) + len(p.Origin) + len(p.MatchedSelector))
		if c.records > maxRecords || c.bytes > maxBytes || uint64(len(p.Extensions)) > maxRecords-c.records {
			return nil, fmt.Errorf("profile context exceeds limits")
		}
		for i, e := range p.Extensions {
			if !validSource(e.Source) || e.Parent < 0 || e.Parent > i {
				return nil, fmt.Errorf("invalid profile extension")
			}
			c.records++
			c.bytes += uint64(64 + len(e.Source.ID) + len(e.Source.Class))
		}
	}
	for i := 0; i < len(projected); i++ {
		if !projected[i+1] || !acquisition[uint32(i)] {
			return nil, fmt.Errorf("noncontiguous profile order")
		}
	}
	if c.records > maxRecords || c.bytes > maxBytes {
		return nil, fmt.Errorf("profile context exceeds limits")
	}
	return c, nil
}
