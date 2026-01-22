// SPDX-License-Identifier: GPL-3.0-or-later

package funcapi

import "encoding/json"

// ParamSelection defines whether a required param allows single or multiple selections.
type ParamSelection uint8

const (
	// ParamSelect allows a single choice.
	ParamSelect ParamSelection = iota
	// ParamMultiSelect allows multiple choices.
	ParamMultiSelect
)

// String returns the UI keyword used for this selection mode.
func (p ParamSelection) String() string {
	switch p {
	case ParamMultiSelect:
		return "multiselect"
	default:
		return "select"
	}
}

// MarshalJSON encodes the selection mode as a UI keyword.
func (p ParamSelection) MarshalJSON() ([]byte, error) {
	return json.Marshal(p.String())
}

// ParamOption defines a single option for a required param.
// Column is not serialized and can be used for safe SQL mapping (e.g., __sort).
type ParamOption struct {
	// ID is the stable identifier returned in selected values.
	ID       string
	// Name is the label shown in the UI.
	Name     string
	// Default marks the option as the default selection (if none are set, the first option is used).
	Default  bool
	// Disabled prevents selection in the UI.
	Disabled bool
	// Sort includes a sort directive with the option.
	Sort     *FieldSort
	// Column is not serialized and can be used for safe SQL mapping.
	Column   string
}

// ParamConfig defines a required param and its available options.
type ParamConfig struct {
	// ID identifies the required param in requests.
	ID         string
	// Name is the label shown in the UI.
	Name       string
	// Help provides UI help text for the param.
	Help       string
	// Selection sets single or multi-select behavior.
	Selection  ParamSelection
	// Options supplies the available choices.
	Options    []ParamOption
	// UniqueView requests unique view behavior for the param in the UI.
	UniqueView bool
}

// RequiredParam converts ParamConfig to the wire format used by required_params.
func (p ParamConfig) RequiredParam() map[string]any {
	out := map[string]any{
		"id":      p.ID,
		"name":    p.Name,
		"type":    p.Selection.String(),
		"options": buildParamOptions(p.Options),
	}
	if p.Help != "" {
		out["help"] = p.Help
	}
	if p.UniqueView {
		out["unique_view"] = true
	}
	return out
}

func buildParamOptions(opts []ParamOption) []map[string]any {
	if len(opts) == 0 {
		return []map[string]any{}
	}

	hasDefault := false
	for _, opt := range opts {
		if opt.Default {
			hasDefault = true
			break
		}
	}

	options := make([]map[string]any, 0, len(opts))
	for i, opt := range opts {
		o := map[string]any{
			"id":   opt.ID,
			"name": opt.Name,
		}
		if opt.Disabled {
			o["disabled"] = true
		}
		if opt.Sort != nil {
			o["sort"] = opt.Sort.String()
		}
		if opt.Default || (!hasDefault && i == 0) {
			o["defaultSelected"] = true
		}
		options = append(options, o)
	}
	return options
}

// ResolvedParam holds resolved values for a required param.
type ResolvedParam struct {
	// IDs contains selected option IDs in order.
	IDs     []string
	// Options contains selected option metadata in order.
	Options []ParamOption
}

// GetOne returns the first selected ID.
func (p ResolvedParam) GetOne() string {
	if len(p.IDs) > 0 {
		return p.IDs[0]
	}
	return ""
}

// ResolvedParams maps param ID to resolved values.
type ResolvedParams map[string]ResolvedParam

// Get returns all selected IDs for the param.
func (p ResolvedParams) Get(id string) []string {
	if p == nil {
		return nil
	}
	return p[id].IDs
}

// GetOne returns the first selected ID for the param.
func (p ResolvedParams) GetOne(id string) string {
	if p == nil {
		return ""
	}
	if v, ok := p[id]; ok && len(v.IDs) > 0 {
		return v.IDs[0]
	}
	return ""
}

// Option returns the first selected option for the param.
func (p ResolvedParams) Option(id string) (ParamOption, bool) {
	if p == nil {
		return ParamOption{}, false
	}
	if v, ok := p[id]; ok && len(v.Options) > 0 {
		return v.Options[0], true
	}
	return ParamOption{}, false
}

// Column returns the column mapping for the selected option (used by __sort).
// If the option has no Column mapping, the selected ID is returned.
func (p ResolvedParams) Column(id string) string {
	opt, ok := p.Option(id)
	if !ok {
		return ""
	}
	if opt.Column != "" {
		return opt.Column
	}
	return opt.ID
}

// ResolveParam resolves user values against a ParamConfig, applying defaults or the first option when needed.
func ResolveParam(cfg ParamConfig, values []string) ResolvedParam {
	byID := make(map[string]ParamOption, len(cfg.Options))
	for _, opt := range cfg.Options {
		byID[opt.ID] = opt
	}

	var selected []ParamOption

	switch cfg.Selection {
	case ParamMultiSelect:
		for _, val := range values {
			if opt, ok := byID[val]; ok {
				selected = append(selected, opt)
			}
		}
	default:
		if len(values) > 0 {
			if opt, ok := byID[values[0]]; ok {
				selected = []ParamOption{opt}
			}
		}
	}

	if len(selected) == 0 {
		selected = defaultOptions(cfg)
	}

	resolved := ResolvedParam{}
	if len(selected) > 0 {
		resolved.Options = selected
		resolved.IDs = make([]string, 0, len(selected))
		for _, opt := range selected {
			resolved.IDs = append(resolved.IDs, opt.ID)
		}
	}
	return resolved
}

// ResolveParams resolves multiple ParamConfig entries.
func ResolveParams(cfgs []ParamConfig, values map[string][]string) ResolvedParams {
	resolved := ResolvedParams{}
	for _, cfg := range cfgs {
		resolved[cfg.ID] = ResolveParam(cfg, values[cfg.ID])
	}
	return resolved
}

func defaultOptions(cfg ParamConfig) []ParamOption {
	var defaults []ParamOption
	for _, opt := range cfg.Options {
		if opt.Default {
			defaults = append(defaults, opt)
		}
	}
	if len(defaults) > 0 {
		if cfg.Selection == ParamMultiSelect {
			return defaults
		}
		return []ParamOption{defaults[0]}
	}
	if len(cfg.Options) == 0 {
		return nil
	}
	return []ParamOption{cfg.Options[0]}
}

// MergeParamConfigs replaces base configs with overrides by ID, preserving base order.
func MergeParamConfigs(base, overrides []ParamConfig) []ParamConfig {
	if len(overrides) == 0 {
		return base
	}

	overrideByID := make(map[string]ParamConfig, len(overrides))
	for _, cfg := range overrides {
		overrideByID[cfg.ID] = cfg
	}

	seen := make(map[string]bool, len(base)+len(overrides))
	merged := make([]ParamConfig, 0, len(base)+len(overrides))
	for _, cfg := range base {
		if override, ok := overrideByID[cfg.ID]; ok {
			merged = append(merged, override)
			seen[cfg.ID] = true
			continue
		}
		merged = append(merged, cfg)
		seen[cfg.ID] = true
	}
	for _, cfg := range overrides {
		if !seen[cfg.ID] {
			merged = append(merged, cfg)
		}
	}
	return merged
}
