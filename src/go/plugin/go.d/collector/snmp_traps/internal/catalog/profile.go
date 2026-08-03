// SPDX-License-Identifier: GPL-3.0-or-later

package catalog

import (
	"errors"
	"fmt"
	"maps"
	"strconv"
	"strings"
	"sync"
	"text/template"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/model"
)

type TrapEntry = model.TrapEntry
type VarbindValue = model.VarbindValue
type ASN1Type = model.ASN1Type

var validCategories = map[string]bool{
	"state_change":  true,
	"config_change": true,
	"security":      true,
	"auth":          true,
	"license":       true,
	"mobility":      true,
	"diagnostic":    true,
	"unknown":       true,
}

var validSeverities = map[string]bool{
	"emerg":   true,
	"alert":   true,
	"crit":    true,
	"err":     true,
	"warning": true,
	"notice":  true,
	"info":    true,
	"debug":   true,
}

var validStatuses = map[string]bool{
	"current":    true,
	"deprecated": true,
	"mandatory":  true,
	"obsolete":   true,
	"optional":   true,
}

const maxBoundedVarbindValues = 64

// validateLabelKey checks if s matches ^[a-z][a-z0-9_]*$
func validateLabelKey(s string) bool {
	if len(s) == 0 {
		return false
	}
	if s[0] < 'a' || s[0] > 'z' {
		return false
	}
	for i := 1; i < len(s); i++ {
		c := s[i]
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '_' {
			return false
		}
	}
	return true
}

// VarbindDef is a varbind metadata entry from the file-scoped varbinds table.
type VarbindDef struct {
	OID         string            `yaml:"oid"`
	Type        string            `yaml:"type"`
	Enum        map[string]string `yaml:"enum,omitempty"`
	Constraints string            `yaml:"constraints,omitempty"`

	// RawName is the symbolic varbind name (key from the file-scoped table).
	// Set during profile loading; not a YAML field.
	RawName string `yaml:"-"`
}

// TrapDef is a single trap entry from a profile YAML.
type TrapDef struct {
	OID              string            `yaml:"oid"`
	Name             string            `yaml:"name"`
	Category         string            `yaml:"category"`
	Severity         string            `yaml:"severity"`
	Description      string            `yaml:"description,omitempty"`
	Status           string            `yaml:"status,omitempty"`
	VarbindRefs      []any             `yaml:"varbinds,omitempty"`
	Labels           map[string]string `yaml:"labels,omitempty"`
	DedupKeyVarbinds []string          `yaml:"dedup_key_varbinds,omitempty"`

	// SharedVarbinds maps varbind OID → VarbindDef for runtime resolution.
	// Merged from file table references + per-trap inline definitions.
	SharedVarbinds map[string]*VarbindDef `yaml:"-"`

	// SourceFile is the profile file this trap came from.
	SourceFile string `yaml:"-"`

	descriptionTemplate *template.Template
	labelTemplates      map[string]*template.Template
}

// WithOverrides returns an immutable copy with job-level category, severity,
// and label overrides applied.
func (t *TrapDef) WithOverrides(category, severity string, labels map[string]string) *TrapDef {
	if t == nil {
		return nil
	}
	cp := *t
	if t.Labels != nil {
		cp.Labels = make(map[string]string, len(t.Labels)+len(labels))
		maps.Copy(cp.Labels, t.Labels)
	}
	if t.labelTemplates != nil && len(labels) > 0 {
		cp.labelTemplates = make(map[string]*template.Template, len(t.labelTemplates))
		maps.Copy(cp.labelTemplates, t.labelTemplates)
	}
	if category != "" {
		cp.Category = category
	}
	if severity != "" {
		cp.Severity = severity
	}
	if labels != nil {
		if cp.Labels == nil {
			cp.Labels = make(map[string]string, len(labels))
		}
		maps.Copy(cp.Labels, labels)
		for key := range labels {
			delete(cp.labelTemplates, key)
		}
	}
	return &cp
}

// varbindResolvedNameRefs returns symbolic varbind names this trap references from the file table.
func (t *TrapDef) varbindResolvedNameRefs() map[string]bool {
	names := make(map[string]bool)
	for _, v := range t.VarbindRefs {
		if name, ok := v.(string); ok {
			names[name] = true
		}
	}
	return names
}

// varbindByOID returns the VarbindDef for a given OID from the shared varbinds map.
func (t *TrapDef) varbindByOID(oid string) *VarbindDef {
	if t.SharedVarbinds == nil {
		return nil
	}
	return t.SharedVarbinds[oid]
}

// varbindByName returns the VarbindDef for a given symbolic name.
func (t *TrapDef) varbindByName(name string) *VarbindDef {
	if t.SharedVarbinds == nil {
		return nil
	}
	for _, def := range t.SharedVarbinds {
		if def != nil && def.RawName == name {
			return def
		}
	}
	return nil
}

// VarbindByName returns a file-scoped or inline varbind definition by symbolic name.
func (t *TrapDef) VarbindByName(name string) *VarbindDef {
	if t == nil {
		return nil
	}
	if vb := t.varbindByName(name); vb != nil {
		return vb
	}
	return t.inlineVarbindByName(name)
}

func (t *TrapDef) inlineVarbindByName(name string) *VarbindDef {
	for _, ref := range t.VarbindRefs {
		imap, ok := ref.(map[any]any)
		if !ok {
			continue
		}
		vb, err := inlineVarbindDef(imap)
		if err == nil && vb.RawName == name {
			return vb
		}
	}
	return nil
}

// ProfileDefinition is the deserialized form of a profile YAML file.
type ProfileDefinition struct {
	Vendor    string                `yaml:"vendor,omitempty"`
	MibCount  int                   `yaml:"mib_count,omitempty"`
	TrapCount int                   `yaml:"trap_count,omitempty"`
	Varbinds  map[string]VarbindDef `yaml:"varbinds,omitempty"`
	Traps     []TrapDef             `yaml:"traps,omitempty"`
	Metrics   []profileMetricRule   `yaml:"metrics,omitempty"`
	Charts    []profileMetricChart  `yaml:"charts,omitempty"`
}

// Epoch is a loaded, validated OID index ready for trap lookup.
type Epoch struct {
	publishMu         sync.Mutex
	mu                sync.RWMutex
	trapsByOID        map[string]*TrapDef
	namesByTrapName   map[string]*TrapDef
	metricRulesByName map[string]*profileMetricRule
	metricRulesByOut  map[string]*profileMetricRule
	metricChartsByID  map[string]*profileMetricChart
	stock             *stockProfileStore
	profiles          []ProfileInfo

	// Validation overlays exist only on a staged epoch. They let one bundle
	// validate against already-published definitions and parsed lazy
	// dependencies without cloning or publishing either set.
	base                   *Epoch
	validationTrapsByOID   map[string]*TrapDef
	validationNamesByTrap  map[string]*TrapDef
	validationRulesByName  map[string]*profileMetricRule
	validationRulesByOut   map[string]*profileMetricRule
	validationChartsByID   map[string]*profileMetricChart
	validationStockProfile string
}

// NewEpoch returns an empty epoch for callers that assemble static trap definitions.
func NewEpoch() *Epoch {
	return &Epoch{
		trapsByOID:        make(map[string]*TrapDef),
		namesByTrapName:   make(map[string]*TrapDef),
		metricRulesByName: make(map[string]*profileMetricRule),
		metricRulesByOut:  make(map[string]*profileMetricRule),
		metricChartsByID:  make(map[string]*profileMetricChart),
	}
}

// AddTraps validates and adds static trap definitions to an unpublished epoch.
func (idx *Epoch) AddTraps(traps []*TrapDef) error { return idx.addTraps(traps) }

// PrepareTrap compiles one manually assembled trap definition.
func PrepareTrap(td *TrapDef) error {
	if td == nil {
		return errors.New("trap definition is nil")
	}
	fileVarbinds := make(map[string]VarbindDef, len(td.SharedVarbinds))
	for _, vb := range td.SharedVarbinds {
		if vb != nil && vb.RawName != "" {
			fileVarbinds[vb.RawName] = *vb
		}
	}
	if td.SharedVarbinds == nil {
		td.SharedVarbinds = buildSharedVarbinds(td, fileVarbinds)
	}
	return compileTrapTemplates(td, fileVarbinds)
}

// ProfileInfo describes one effective extensionless profile identity.
type ProfileInfo struct {
	Name    string
	Path    string
	IsStock bool
}

// Profiles returns the effective profile inventory sorted by identity.
func (idx *Epoch) Profiles() []ProfileInfo {
	if idx == nil {
		return nil
	}
	return append([]ProfileInfo(nil), idx.profiles...)
}

// Lookup returns the TrapDef for a given numeric OID, or nil if not found.
func (idx *Epoch) Lookup(oid string) *TrapDef {
	td, _ := idx.LookupWithError(oid)
	return td
}

// LookupWithError returns the TrapDef for a given numeric OID and reports
// stock lazy-load failures separately from genuine unknown OIDs.
func (idx *Epoch) LookupWithError(oid string) (*TrapDef, error) {
	if idx == nil {
		return nil, nil
	}
	if td := idx.lookupLoaded(oid); td != nil {
		return td, nil
	}
	if err := idx.loadStockForOID(oid); err != nil {
		return nil, err
	}
	return idx.lookupLoaded(oid), nil
}

func (idx *Epoch) lookupLoaded(oid string) *TrapDef {
	if td := idx.lookupExactOID(oid); td != nil {
		return td
	}
	if alt := model.AlternateTrapOID(oid); alt != oid {
		return idx.lookupExactOID(alt)
	}
	return nil
}

func (idx *Epoch) lookupExactOID(oid string) *TrapDef {
	if idx == nil {
		return nil
	}
	idx.mu.RLock()
	td := idx.trapsByOID[oid]
	idx.mu.RUnlock()
	if td != nil {
		return td
	}
	if td = idx.validationTrapsByOID[oid]; td != nil {
		return td
	}
	return idx.base.lookupExactOID(oid)
}

func (idx *Epoch) lookupTrapName(name string) *TrapDef {
	if idx == nil {
		return nil
	}
	idx.mu.RLock()
	td := idx.namesByTrapName[name]
	idx.mu.RUnlock()
	if td != nil {
		return td
	}
	if td = idx.validationNamesByTrap[name]; td != nil {
		return td
	}
	return idx.base.lookupTrapName(name)
}

func (idx *Epoch) lookupMetricRule(name string) *profileMetricRule {
	if idx == nil {
		return nil
	}
	idx.mu.RLock()
	rule := idx.metricRulesByName[name]
	idx.mu.RUnlock()
	if rule != nil {
		return rule
	}
	if rule = idx.validationRulesByName[name]; rule != nil {
		return rule
	}
	return idx.base.lookupMetricRule(name)
}

func (idx *Epoch) lookupMetricOutput(name string) *profileMetricRule {
	if idx == nil {
		return nil
	}
	idx.mu.RLock()
	rule := idx.metricRulesByOut[name]
	idx.mu.RUnlock()
	if rule != nil {
		return rule
	}
	if rule = idx.validationRulesByOut[name]; rule != nil {
		return rule
	}
	return idx.base.lookupMetricOutput(name)
}

func (idx *Epoch) lookupMetricChart(id string) *profileMetricChart {
	if idx == nil {
		return nil
	}
	idx.mu.RLock()
	chart := idx.metricChartsByID[id]
	idx.mu.RUnlock()
	if chart != nil {
		return chart
	}
	if chart = idx.validationChartsByID[id]; chart != nil {
		return chart
	}
	return idx.base.lookupMetricChart(id)
}

// MetricDefinitions is an immutable snapshot of static metric rules and charts.
type MetricDefinitions struct {
	RulesByName map[string]*MetricRule
	ChartsByID  map[string]*MetricChart
}

// Definitions returns the selected static metric definitions and their charts.
func (idx *Epoch) Definitions(ruleNames []string) (MetricDefinitions, error) {
	if idx == nil {
		return MetricDefinitions{}, errors.New("profile index not available")
	}
	if err := idx.loadStockMetricRules(ruleNames); err != nil {
		return MetricDefinitions{}, err
	}
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	defs := MetricDefinitions{
		RulesByName: make(map[string]*MetricRule, len(ruleNames)),
		ChartsByID:  make(map[string]*MetricChart, len(ruleNames)),
	}
	for _, name := range ruleNames {
		rule := idx.metricRulesByName[name]
		if rule == nil {
			continue
		}
		defs.RulesByName[name] = rule
		if chart := idx.metricChartsByID[rule.Output.Chart]; chart != nil {
			defs.ChartsByID[chart.ID] = chart
		}
	}
	return defs, nil
}

// ResolveTrap resolves a numeric OID or exact MIB-qualified trap name.
func (idx *Epoch) ResolveTrap(ref string) (*TrapDef, error) {
	return resolveProfileMetricTrap(idx, ref)
}

// ValidCategory reports whether category belongs to the profile taxonomy.
func ValidCategory(category string) bool { return validCategories[category] }

// ValidSeverity reports whether severity belongs to the profile taxonomy.
func ValidSeverity(severity string) bool { return validSeverities[severity] }

func validateFileVarbinds(fileVarbinds map[string]VarbindDef, src string) error {
	for name, vb := range fileVarbinds {
		if vb.OID == "" {
			return fmt.Errorf("%s: varbind %q missing required field 'oid'", src, name)
		}
		if !model.IsNumericOID(vb.OID) {
			return fmt.Errorf("%s: varbind %q has invalid oid %q", src, name, vb.OID)
		}
		if vb.Type == "" {
			return fmt.Errorf("%s: varbind %q missing required field 'type'", src, name)
		}
	}
	return nil
}

// validateTrapDef checks required fields, closed-set enums, and varbind consistency.
func validateTrapDef(td *TrapDef, fileVarbinds map[string]VarbindDef) error {
	src := td.SourceFile
	if src == "" {
		src = "<unknown>"
	}

	if td.OID == "" {
		return fmt.Errorf("%s: trap entry missing required field 'oid'", src)
	}
	if !model.IsNumericOID(td.OID) {
		return fmt.Errorf("%s: trap entry has invalid oid %q", src, td.OID)
	}
	if td.Name == "" {
		return fmt.Errorf("%s: trap entry %s missing required field 'name'", src, td.OID)
	}
	if !strings.Contains(td.Name, "::") {
		return fmt.Errorf("%s: trap entry %s: name %q is not MIB-qualified (must be MIB::symbol)", src, td.OID, td.Name)
	}
	if !validCategories[td.Category] {
		return fmt.Errorf("%s: trap entry %s: invalid category %q (must be one of: %v)", src, td.OID, td.Category, categoryList())
	}
	if !validSeverities[td.Severity] {
		return fmt.Errorf("%s: trap entry %s: invalid severity %q (must be one of: %v)", src, td.OID, td.Severity, severityList())
	}
	if td.Status != "" && !validStatuses[td.Status] {
		return fmt.Errorf("%s: trap entry %s: invalid status %q (must be current, deprecated, mandatory, obsolete, or optional)", src, td.OID, td.Status)
	}

	for _, name := range td.DedupKeyVarbinds {
		if name == "" {
			continue
		}
		if _, ok := fileVarbinds[name]; !ok {
			return fmt.Errorf("%s: trap entry %s: dedup_key_varbind %q not found in file-scoped varbinds table", src, td.OID, name)
		}
	}

	for _, ref := range td.VarbindRefs {
		switch v := ref.(type) {
		case string:
			name := v
			if _, exists := fileVarbinds[name]; !exists {
				return fmt.Errorf("%s: trap entry %s: varbind %q not found in file-scoped varbinds table", src, td.OID, name)
			}
		case map[any]any:
			if _, err := inlineVarbindDef(v); err != nil {
				return fmt.Errorf("%s: trap entry %s: invalid inline varbind: %w", src, td.OID, err)
			}
		default:
			return fmt.Errorf("%s: trap entry %s: invalid varbind reference type %T", src, td.OID, ref)
		}
	}

	for key := range td.Labels {
		if !validateLabelKey(key) {
			return fmt.Errorf("%s: trap entry %s: label key %q does not match ^[a-z][a-z0-9_]*$", src, td.OID, key)
		}
	}

	if err := compileTrapTemplates(td, fileVarbinds); err != nil {
		return err
	}

	return nil
}

func isBoundedLabelVarbind(vb VarbindDef) bool {
	if len(vb.Enum) > 0 {
		return len(vb.Enum) <= maxBoundedVarbindValues
	}
	switch strings.ToLower(vb.Type) {
	case "boolean", "truthvalue":
		return true
	}
	if n, ok := numericRangeCardinality(vb.Constraints); ok {
		return n > 0 && n <= maxBoundedVarbindValues
	}
	return false
}

func numericRangeCardinality(constraints string) (int64, bool) {
	minVal, maxVal, ok := numericRangeBounds(constraints)
	if !ok {
		return 0, false
	}
	return maxVal - minVal + 1, true
}

func numericRangeBounds(constraints string) (int64, int64, bool) {
	s := strings.TrimSpace(constraints)
	if !strings.HasPrefix(s, "(") || !strings.HasSuffix(s, ")") || !strings.Contains(s, "..") {
		return 0, 0, false
	}
	parts := strings.Split(strings.TrimSuffix(strings.TrimPrefix(s, "("), ")"), "..")
	if len(parts) != 2 {
		return 0, 0, false
	}
	minVal, err := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
	if err != nil {
		return 0, 0, false
	}
	maxVal, err := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
	if err != nil || maxVal < minVal {
		return 0, 0, false
	}
	return minVal, maxVal, true
}

// buildSharedVarbinds merges file-scoped varbinds with per-trap inline definitions.
func buildSharedVarbinds(td *TrapDef, fileVarbinds map[string]VarbindDef) map[string]*VarbindDef {
	shared := make(map[string]*VarbindDef)

	for _, ref := range td.VarbindRefs {
		name, ok := ref.(string)
		if !ok {
			continue
		}
		if vb, exists := fileVarbinds[name]; exists {
			copyVb := vb
			copyVb.RawName = name
			shared[copyVb.OID] = &copyVb
		}
	}

	for _, ref := range td.VarbindRefs {
		imap, ok := ref.(map[any]any)
		if !ok {
			continue
		}
		vb, err := inlineVarbindDef(imap)
		if err == nil {
			shared[vb.OID] = vb
		}
	}

	return shared
}

func inlineVarbindDef(v map[any]any) (*VarbindDef, error) {
	name, _ := v["name"].(string)
	oid, _ := v["oid"].(string)
	typ, _ := v["type"].(string)

	if name == "" {
		return nil, fmt.Errorf("missing required field 'name'")
	}
	if oid == "" {
		return nil, fmt.Errorf("missing required field 'oid'")
	}
	if !model.IsNumericOID(oid) {
		return nil, fmt.Errorf("invalid oid %q", oid)
	}
	if typ == "" {
		return nil, fmt.Errorf("missing required field 'type'")
	}

	vb := &VarbindDef{OID: oid, Type: typ, RawName: name}
	if enumMap, ok := v["enum"].(map[any]any); ok {
		vb.Enum = make(map[string]string, len(enumMap))
		for k, val := range enumMap {
			vb.Enum[fmt.Sprintf("%v", k)] = fmt.Sprintf("%v", val)
		}
	}
	return vb, nil
}

func categoryList() []string {
	return []string{"state_change", "config_change", "security", "auth", "license", "mobility", "diagnostic", "unknown"}
}

func severityList() []string {
	return []string{"emerg", "alert", "crit", "err", "warning", "notice", "info", "debug"}
}
