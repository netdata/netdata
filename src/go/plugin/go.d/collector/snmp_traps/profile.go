// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
)

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

var knownSpecialVars = map[string]bool{
	"_HOSTNAME":          true,
	"TRAP_SOURCE_IP":     true,
	"TRAP_NAME":          true,
	"TRAP_DEVICE_VENDOR": true,
	"TRAP_INTERFACE":     true,
	"TRAP_NEIGHBORS":     true,
}

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

	// rawName is the symbolic varbind name (key from the file-scoped table).
	// Set during profile loading; not a YAML field.
	rawName string
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

	// sharedVarbinds maps varbind OID → VarbindDef for runtime resolution.
	// Merged from file table references + per-trap inline definitions.
	sharedVarbinds map[string]*VarbindDef

	// sourceFile is the profile file this trap came from.
	sourceFile string
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
	if t.sharedVarbinds == nil {
		return nil
	}
	return t.sharedVarbinds[oid]
}

// varbindByName returns the VarbindDef for a given symbolic name.
func (t *TrapDef) varbindByName(name string) *VarbindDef {
	if t.sharedVarbinds == nil {
		return nil
	}
	for _, def := range t.sharedVarbinds {
		if def != nil && def.rawName == name {
			return def
		}
	}
	return nil
}

func (t *TrapDef) inlineVarbindByName(name string) *VarbindDef {
	for _, ref := range t.VarbindRefs {
		imap, ok := ref.(map[any]any)
		if !ok {
			continue
		}
		vb, err := inlineVarbindDef(imap)
		if err == nil && vb.rawName == name {
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
	Extends   []string              `yaml:"extends,omitempty"`
	Varbinds  map[string]VarbindDef `yaml:"varbinds,omitempty"`
	Traps     []TrapDef             `yaml:"traps,omitempty"`
}

// ProfileIndex is a loaded, validated OID index ready for trap lookup.
type ProfileIndex struct {
	trapsByOID map[string]*TrapDef
}

// Lookup returns the TrapDef for a given numeric OID, or nil if not found.
func (idx *ProfileIndex) Lookup(oid string) *TrapDef {
	if idx == nil {
		return nil
	}
	if td := idx.trapsByOID[oid]; td != nil {
		return td
	}
	if alt := alternateTrapOID(oid); alt != oid {
		return idx.trapsByOID[alt]
	}
	return nil
}

func alternateTrapOID(oid string) string {
	if len(oid) == 0 || oid[0] == '.' || oid[len(oid)-1] == '.' {
		return oid
	}

	dots := 0
	prevDot := -1
	lastDot := -1
	segmentStart := 0

	for i := 0; i < len(oid); i++ {
		c := oid[i]
		switch {
		case c == '.':
			if i == segmentStart {
				return oid
			}
			dots++
			prevDot = lastDot
			lastDot = i
			segmentStart = i + 1
		case c < '0' || c > '9':
			return oid
		}
	}

	if dots < 3 || prevDot < 0 || lastDot <= 0 || lastDot >= len(oid)-1 {
		return oid
	}

	if oid[prevDot+1:lastDot] == "0" {
		return oid[:prevDot] + oid[lastDot:]
	}
	return oid[:lastDot] + ".0" + oid[lastDot:]
}

// profileCache holds the plugin-wide shared profile state.
// SOW-0035 has no hot reload, so a single active generation is sufficient.
// SOW-0037 hot reload must replace this with per-generation holder accounting.
type profileCache struct {
	mu         sync.Mutex
	loaded     bool
	index      *ProfileIndex
	activeRefs int
	generation uint64
	loadErr    error
}

var globalProfileCache profileCache

// AcquireProfileCache loads profiles on first call and increments the refcount.
// Returns the profile index, its cache generation, and any load error.
func AcquireProfileCache() (*ProfileIndex, uint64, error) {
	globalProfileCache.mu.Lock()
	defer globalProfileCache.mu.Unlock()

	if !globalProfileCache.loaded {
		idx, err := loadProfileCache()
		if err != nil {
			globalProfileCache.loadErr = err
			return nil, 0, err
		}
		globalProfileCache.index = idx
		globalProfileCache.loaded = true
		globalProfileCache.generation++
		globalProfileCache.loadErr = nil
	}

	if globalProfileCache.index == nil {
		return nil, 0, globalProfileCache.loadErr
	}

	globalProfileCache.activeRefs++
	return globalProfileCache.index, globalProfileCache.generation, nil
}

// ReleaseProfileCache decrements the refcount. When refs reach 0, the index is released.
func ReleaseProfileCache(generation uint64) {
	globalProfileCache.mu.Lock()
	defer globalProfileCache.mu.Unlock()

	if globalProfileCache.activeRefs <= 0 || generation != globalProfileCache.generation {
		return
	}
	globalProfileCache.activeRefs--
	if globalProfileCache.activeRefs == 0 {
		globalProfileCache.index = nil
		globalProfileCache.loaded = false
		globalProfileCache.loadErr = nil
	}
}

// resetProfileCacheForTest clears the shared cache for test isolation.
func resetProfileCacheForTest() {
	globalProfileCache.mu.Lock()
	defer globalProfileCache.mu.Unlock()
	globalProfileCache.loaded = false
	globalProfileCache.index = nil
	globalProfileCache.activeRefs = 0
	globalProfileCache.generation = 0
	globalProfileCache.loadErr = nil
}

func validateFileVarbinds(fileVarbinds map[string]VarbindDef, src string) error {
	for name, vb := range fileVarbinds {
		if vb.OID == "" {
			return fmt.Errorf("%s: varbind %q missing required field 'oid'", src, name)
		}
		if !isNumericOID(vb.OID) {
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
	src := td.sourceFile
	if src == "" {
		src = "<unknown>"
	}

	if td.OID == "" {
		return fmt.Errorf("%s: trap entry missing required field 'oid'", src)
	}
	if !isNumericOID(td.OID) {
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

	if err := validateLabelTemplates(td, fileVarbinds); err != nil {
		return err
	}

	return nil
}

func validateLabelTemplates(td *TrapDef, fileVarbinds map[string]VarbindDef) error {
	src := td.sourceFile
	for key, tmpl := range td.Labels {
		for _, ref := range templateRefs(tmpl) {
			name := strings.TrimSuffix(ref, ".raw")
			if name == "" {
				continue
			}
			if knownSpecialVars[name] {
				if name == "TRAP_NAME" || name == "TRAP_DEVICE_VENDOR" {
					continue
				}
				return fmt.Errorf("%s: trap entry %s: label %q references unbounded field %q", src, td.OID, key, name)
			}
			if isNumericOID(name) {
				return fmt.Errorf("%s: trap entry %s: label %q references raw OID %q without cardinality metadata", src, td.OID, key, name)
			}
			vb := fileVarbinds[name]
			if vb.OID == "" {
				if inline := td.inlineVarbindByName(name); inline != nil {
					vb = *inline
				}
			}
			if vb.OID == "" {
				return fmt.Errorf("%s: trap entry %s: label %q references unknown varbind %q", src, td.OID, key, name)
			}
			if !isBoundedLabelVarbind(vb) {
				return fmt.Errorf("%s: trap entry %s: label %q references unbounded varbind %q", src, td.OID, key, name)
			}
		}
	}
	return nil
}

func templateRefs(tmpl string) []string {
	var refs []string
	i := 0
	for i < len(tmpl) {
		if tmpl[i] != '{' {
			i++
			continue
		}
		end := strings.IndexByte(tmpl[i:], '}')
		if end == -1 {
			i++
			continue
		}
		refs = append(refs, tmpl[i+1:i+end])
		i += end + 1
	}
	return refs
}

func isBoundedLabelVarbind(vb VarbindDef) bool {
	if len(vb.Enum) > 0 {
		return true
	}
	switch strings.ToLower(vb.Type) {
	case "boolean", "truthvalue":
		return true
	}
	if n, ok := numericRangeCardinality(vb.Constraints); ok {
		return n > 0 && n <= 64
	}
	return false
}

func numericRangeCardinality(constraints string) (int64, bool) {
	s := strings.TrimSpace(constraints)
	if !strings.HasPrefix(s, "(") || !strings.HasSuffix(s, ")") || !strings.Contains(s, "..") {
		return 0, false
	}
	parts := strings.Split(strings.TrimSuffix(strings.TrimPrefix(s, "("), ")"), "..")
	if len(parts) != 2 {
		return 0, false
	}
	minVal, err := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
	if err != nil {
		return 0, false
	}
	maxVal, err := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
	if err != nil || maxVal < minVal {
		return 0, false
	}
	return maxVal - minVal + 1, true
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
			copyVb.rawName = name
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
	if !isNumericOID(oid) {
		return nil, fmt.Errorf("invalid oid %q", oid)
	}
	if typ == "" {
		return nil, fmt.Errorf("missing required field 'type'")
	}

	vb := &VarbindDef{OID: oid, Type: typ, rawName: name}
	if enumMap, ok := v["enum"].(map[any]any); ok {
		vb.Enum = make(map[string]string, len(enumMap))
		for k, val := range enumMap {
			vb.Enum[fmt.Sprintf("%v", k)] = fmt.Sprintf("%v", val)
		}
	}
	return vb, nil
}

func isNumericOID(oid string) bool {
	if oid == "" || oid[0] == '.' || oid[len(oid)-1] == '.' {
		return false
	}
	for _, part := range strings.Split(oid, ".") {
		if part == "" {
			return false
		}
		for _, ch := range part {
			if ch < '0' || ch > '9' {
				return false
			}
		}
	}
	return true
}

func categoryList() []string {
	return []string{"state_change", "config_change", "security", "auth", "license", "mobility", "diagnostic", "unknown"}
}

func severityList() []string {
	return []string{"emerg", "alert", "crit", "err", "warning", "notice", "info", "debug"}
}
