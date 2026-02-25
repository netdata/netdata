// SPDX-License-Identifier: GPL-3.0-or-later

package pipeline

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery/sd/model"
	"github.com/netdata/netdata/go/plugins/plugin/agent/internal/naming"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
)

type Config struct {
	Source         string             `yaml:"-" json:"-"`
	ConfigDefaults confgroup.Registry `yaml:"-" json:"-"`

	Disabled bool   `yaml:"disabled,omitempty" json:"disabled,omitempty"`
	Name     string `yaml:"name" json:"name"`

	// Canonical format: discoverer: { <type>: <config> }
	Discoverer DiscovererPayload `yaml:"discoverer,omitempty" json:"discoverer,omitempty"`

	// New single-step format for service rules:
	Services []ServiceRuleConfig `yaml:"services,omitempty" json:"services,omitempty"`

	// Legacy formats (converted during unmarshal, excluded from JSON):
	LegacyDiscover []LegacyDiscoveryConfig `yaml:"discover,omitempty" json:"-"`
	LegacyClassify []ClassifyRuleConfig    `yaml:"classify,omitempty" json:"-"`
	LegacyCompose  []ComposeRuleConfig     `yaml:"compose,omitempty" json:"-"`
}

// DiscovererPayload stores discoverer type/config internally in a generic form
// while preserving canonical external shape discoverer: { <type>: <config> }.
type DiscovererPayload struct {
	Kind   string          `yaml:"-" json:"-"`
	Config json.RawMessage `yaml:"-" json:"-"`
}

func (d DiscovererPayload) TypeName() string {
	return strings.TrimSpace(d.Kind)
}

func (d DiscovererPayload) Type() string {
	return d.TypeName()
}

func (d DiscovererPayload) Empty() bool {
	return d.TypeName() == ""
}

func (d *DiscovererPayload) UnmarshalJSON(data []byte) error {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}

	if len(m) == 0 {
		*d = DiscovererPayload{}
		return nil
	}
	if len(m) > 1 {
		return errors.New("multiple discoverers configured, only one is allowed")
	}

	for typ, cfg := range m {
		*d = DiscovererPayload{Kind: typ, Config: cloneRaw(cfg)}
		return nil
	}

	*d = DiscovererPayload{}
	return nil
}

func (d DiscovererPayload) MarshalJSON() ([]byte, error) {
	if d.Empty() {
		return []byte("{}"), nil
	}

	cfg := cloneRaw(d.Config)
	if len(cfg) == 0 {
		cfg = []byte("{}")
	}

	return json.Marshal(map[string]json.RawMessage{d.TypeName(): cfg})
}

func (d *DiscovererPayload) UnmarshalYAML(unmarshal func(any) error) error {
	var raw any
	if err := unmarshal(&raw); err != nil {
		return err
	}
	if raw == nil {
		*d = DiscovererPayload{}
		return nil
	}

	norm, err := normalizeYAMLValue(raw)
	if err != nil {
		return err
	}

	m, ok := norm.(map[string]any)
	if !ok {
		return fmt.Errorf("invalid discoverer format: expected map, got %T", norm)
	}

	if len(m) == 0 {
		*d = DiscovererPayload{}
		return nil
	}
	if len(m) > 1 {
		return errors.New("multiple discoverers configured, only one is allowed")
	}

	for typ, cfg := range m {
		bs, err := json.Marshal(cfg)
		if err != nil {
			return fmt.Errorf("marshal discoverer %q config: %w", typ, err)
		}
		*d = DiscovererPayload{Kind: typ, Config: bs}
		return nil
	}

	*d = DiscovererPayload{}
	return nil
}

func (d DiscovererPayload) MarshalYAML() (any, error) {
	if d.Empty() {
		return map[string]any{}, nil
	}

	cfg := any(map[string]any{})
	if len(d.Config) != 0 {
		if err := json.Unmarshal(d.Config, &cfg); err != nil {
			return nil, fmt.Errorf("unmarshal discoverer %q config json: %w", d.TypeName(), err)
		}
	}

	return map[string]any{d.TypeName(): cfg}, nil
}

func cloneRaw(raw json.RawMessage) json.RawMessage {
	if raw == nil {
		return nil
	}
	out := make([]byte, len(raw))
	copy(out, raw)
	return out
}

func normalizeYAMLValue(v any) (any, error) {
	switch vv := v.(type) {
	case map[any]any:
		m := make(map[string]any, len(vv))
		for k, iv := range vv {
			ks, ok := k.(string)
			if !ok {
				return nil, fmt.Errorf("yaml map key must be string, got %T", k)
			}
			norm, err := normalizeYAMLValue(iv)
			if err != nil {
				return nil, err
			}
			m[ks] = norm
		}
		return m, nil
	case map[string]any:
		m := make(map[string]any, len(vv))
		for k, iv := range vv {
			norm, err := normalizeYAMLValue(iv)
			if err != nil {
				return nil, err
			}
			m[k] = norm
		}
		return m, nil
	case []any:
		arr := make([]any, 0, len(vv))
		for _, iv := range vv {
			norm, err := normalizeYAMLValue(iv)
			if err != nil {
				return nil, err
			}
			arr = append(arr, norm)
		}
		return arr, nil
	default:
		return v, nil
	}
}

// CleanName returns the name sanitized for use in dyncfg IDs.
// Sanitizes for safe use in IDs and paths.
func (c Config) CleanName() string {
	return naming.Sanitize(c.Name)
}

// UnmarshalYAML implements yaml.Unmarshaler.
// It converts legacy formats to the canonical format:
// - discover[] -> discoverer{}
// - classify/compose -> services[]
func (c *Config) UnmarshalYAML(unmarshal func(any) error) error {
	type plain Config // avoid recursion
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}

	// Convert legacy discover[] to new discoverer{} format
	if len(c.LegacyDiscover) > 0 && c.Discoverer.Empty() {
		if err := c.convertLegacyDiscover(); err != nil {
			return fmt.Errorf("failed to convert legacy discover config: %w", err)
		}
	}

	// Convert legacy classify/compose to canonical services format
	if len(c.Services) == 0 && (len(c.LegacyClassify) > 0 || len(c.LegacyCompose) > 0) {
		services, err := ConvertOldToServices(c.LegacyClassify, c.LegacyCompose)
		if err != nil {
			return fmt.Errorf("failed to convert legacy config: %w", err)
		}
		c.Services = services
	}

	// Clear legacy fields - config is now in canonical form
	c.LegacyDiscover = nil
	c.LegacyClassify = nil
	c.LegacyCompose = nil

	return nil
}

// convertLegacyDiscover converts legacy discover[] array to new discoverer{} struct.
// For non-k8s discoverers, first value wins; for k8s, arrays are merged.
func (c *Config) convertLegacyDiscover() error {
	var converted bool

	for i, d := range c.LegacyDiscover {
		typ := strings.TrimSpace(d.Discoverer)
		if typ == "" {
			continue
		}

		rawCfg, ok := d.Config[typ]
		if !ok {
			return fmt.Errorf("legacy discover[%d]: missing config for discoverer %q", i, typ)
		}

		norm, err := normalizeYAMLValue(rawCfg)
		if err != nil {
			return fmt.Errorf("legacy discover[%d]: normalize %q config: %w", i, typ, err)
		}

		cfgJSON, err := json.Marshal(norm)
		if err != nil {
			return fmt.Errorf("legacy discover[%d]: marshal %q config: %w", i, typ, err)
		}

		if c.Discoverer.Empty() {
			c.Discoverer = DiscovererPayload{Kind: typ, Config: cfgJSON}
			converted = true
			continue
		}

		if c.Discoverer.Type() != typ || typ != "k8s" {
			continue
		}

		merged, err := mergeJSONArrays(c.Discoverer.Config, cfgJSON)
		if err != nil {
			return fmt.Errorf("legacy discover[%d]: merge %q configs: %w", i, typ, err)
		}
		c.Discoverer.Config = merged
		converted = true
	}

	if !converted {
		return errors.New("legacy discover[] did not provide a usable discoverer config")
	}

	return nil
}

func mergeJSONArrays(aRaw, bRaw json.RawMessage) (json.RawMessage, error) {
	var a []any
	if len(aRaw) != 0 {
		if err := json.Unmarshal(aRaw, &a); err != nil {
			return nil, err
		}
	}

	var b []any
	if len(bRaw) != 0 {
		if err := json.Unmarshal(bRaw, &b); err != nil {
			return nil, err
		}
	}

	out := append(a, b...)
	bs, err := json.Marshal(out)
	if err != nil {
		return nil, err
	}
	return bs, nil
}

func NewDiscovererPayload(typ string, cfg any) (DiscovererPayload, error) {
	bs, err := json.Marshal(cfg)
	if err != nil {
		return DiscovererPayload{}, err
	}
	return DiscovererPayload{Kind: typ, Config: bs}, nil
}

// MarshalYAML implements yaml.Marshaler.
// It only marshals the canonical format, not legacy fields.
func (c Config) MarshalYAML() (any, error) {
	type output struct {
		Disabled   bool                `yaml:"disabled,omitempty"`
		Name       string              `yaml:"name"`
		Discoverer DiscovererPayload   `yaml:"discoverer,omitempty"`
		Services   []ServiceRuleConfig `yaml:"services,omitempty"`
	}
	return output{
		Disabled:   c.Disabled,
		Name:       c.Name,
		Discoverer: c.Discoverer,
		Services:   c.Services,
	}, nil
}

// LegacyDiscoveryConfig is the old discover[] array item format.
// Kept for backwards compatibility during unmarshal.
type LegacyDiscoveryConfig struct {
	Discoverer string         `yaml:"discoverer"`
	Config     map[string]any `yaml:",inline"`
}

type ServiceRuleConfig struct {
	ID             string `yaml:"id" json:"id"`                                               // mandatory (for logging/diagnostics)
	Match          string `yaml:"match" json:"match"`                                         // mandatory
	ConfigTemplate string `yaml:"config_template,omitempty" json:"config_template,omitempty"` // optional (drop if empty)
}

type ClassifyRuleConfig struct {
	Name     string `yaml:"name"`
	Selector string `yaml:"selector"` // mandatory
	Tags     string `yaml:"tags"`     // mandatory
	Match    []struct {
		Tags string `yaml:"tags"` // mandatory
		Expr string `yaml:"expr"` // mandatory
	} `yaml:"match"` // mandatory, at least 1
}

type ComposeRuleConfig struct {
	Name     string `yaml:"name"`     // optional
	Selector string `yaml:"selector"` // mandatory
	Config   []struct {
		Selector string `yaml:"selector"` // mandatory
		Template string `yaml:"template"` // mandatory
	} `yaml:"config"` // mandatory, at least 1
}

// ValidateConfig validates a pipeline configuration.
// Exported for use by dyncfg validation.
func ValidateConfig(cfg Config) error {
	if cfg.Name == "" {
		return errors.New("'name' not set")
	}
	if cfg.Discoverer.Empty() {
		return errors.New("no discoverer configured")
	}
	if err := validateServicesConfig(cfg.Services); err != nil {
		return fmt.Errorf("services rules: %v", err)
	}
	return nil
}

func validateServicesConfig(rules []ServiceRuleConfig) error {
	if len(rules) == 0 {
		return errors.New("empty config, need at least 1 service rule")
	}
	for i, r := range rules {
		i++
		if r.ID == "" {
			return fmt.Errorf("'service[%d]->id' not set", i)
		}
		if r.Match == "" {
			return fmt.Errorf("'service[%s][%d]->match' not set", r.ID, i)
		}
		// config_template is optional
	}
	return nil
}

func ConvertOldToServices(cls []ClassifyRuleConfig, cmp []ComposeRuleConfig) ([]ServiceRuleConfig, error) {
	var out []ServiceRuleConfig

	// Build quick lookups for tag -> list of match exprs that add this tag.
	tagToExprs := map[string][]string{}
	for _, r := range cls {
		for _, m := range r.Match {
			// split tags line into tokens:
			tags, _ := model.ParseTags(m.Tags) // reuse existing parser if accessible
			for tag := range tags {
				if strings.HasPrefix(tag, "-") { // ignore deletions
					continue
				}
				tagToExprs[tag] = append(tagToExprs[tag], m.Expr)
			}
		}
		// also include rule-level tags
		rtags, _ := model.ParseTags(r.Tags)
		for tag := range rtags {
			if strings.HasPrefix(tag, "-") {
				continue
			}
			// no expr here; this is too generic to build a service rule from.
		}
	}

	// For each compose rule config entry, create services for its selector tags.
	for _, r := range cmp {
		for _, c := range r.Config {
			sel := strings.TrimSpace(c.Selector)
			exprs := tagToExprs[sel]
			for i, expr := range exprs {
				id := sel
				if i > 0 {
					id = fmt.Sprintf("%s_%d", sel, i+1)
				}
				out = append(out, ServiceRuleConfig{
					ID:             id,
					Match:          expr,
					ConfigTemplate: c.Template,
				})
			}
		}
	}

	return out, nil
}
