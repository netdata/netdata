// SPDX-License-Identifier: GPL-3.0-or-later

package pipeline

import (
	"errors"
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/discovery/sd/discoverer/dockersd"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/discovery/sd/discoverer/k8ssd"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/discovery/sd/discoverer/netlistensd"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/discovery/sd/discoverer/snmpsd"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/discovery/sd/model"

	"gopkg.in/yaml.v2"
)

type Config struct {
	Source         string             `yaml:"-"`
	ConfigDefaults confgroup.Registry `yaml:"-"`

	Disabled bool   `yaml:"disabled"`
	Name     string `yaml:"name"`

	// New format: single discoverer struct
	Discoverer DiscovererConfig `yaml:"discoverer,omitempty" json:"discoverer,omitempty"`

	// New single-step format for service rules:
	Services []ServiceRuleConfig `yaml:"services,omitempty" json:"services,omitempty"`

	// Legacy formats (converted during unmarshal):
	LegacyDiscover []LegacyDiscoveryConfig `yaml:"discover,omitempty" json:"discover,omitempty"`
	LegacyClassify []ClassifyRuleConfig    `yaml:"classify,omitempty" json:"classify,omitempty"`
	LegacyCompose  []ComposeRuleConfig     `yaml:"compose,omitempty" json:"compose,omitempty"`
}

// DiscovererConfig holds the configuration for a single discoverer type.
// Only one of the fields should be set.
type DiscovererConfig struct {
	K8s          []k8ssd.Config      `yaml:"k8s,omitempty" json:"k8s,omitempty"`
	Docker       *dockersd.Config    `yaml:"docker,omitempty" json:"docker,omitempty"`
	NetListeners *netlistensd.Config `yaml:"net_listeners,omitempty" json:"net_listeners,omitempty"`
	SNMP         *snmpsd.Config      `yaml:"snmp,omitempty" json:"snmp,omitempty"`
}

// Type returns the discoverer type name, or empty string if none set.
func (d DiscovererConfig) Type() string {
	switch {
	case d.NetListeners != nil:
		return "net_listeners"
	case d.Docker != nil:
		return "docker"
	case len(d.K8s) > 0:
		return "k8s"
	case d.SNMP != nil:
		return "snmp"
	default:
		return ""
	}
}

// Empty returns true if no discoverer is configured.
func (d DiscovererConfig) Empty() bool {
	return d.Type() == ""
}

// CleanName returns the name sanitized for use in dyncfg IDs.
// Replaces spaces and colons to avoid parsing issues.
func (c Config) CleanName() string {
	name := strings.ReplaceAll(c.Name, " ", "_")
	name = strings.ReplaceAll(name, ":", "_")
	return name
}

// UnmarshalYAML implements yaml.Unmarshaler.
// It converts legacy formats to the canonical format:
// - discover[] → discoverer{}
// - classify/compose → services[]
func (c *Config) UnmarshalYAML(unmarshal func(any) error) error {
	type plain Config // avoid recursion
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}

	// Convert legacy discover[] to new discoverer{} format
	if len(c.LegacyDiscover) > 0 && c.Discoverer.Empty() {
		c.convertLegacyDiscover()
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
// Only the first discoverer of each type is used.
func (c *Config) convertLegacyDiscover() {
	for _, d := range c.LegacyDiscover {
		switch d.Discoverer {
		case "net_listeners":
			if c.Discoverer.NetListeners == nil {
				c.Discoverer.NetListeners = &d.NetListeners
			}
		case "docker":
			if c.Discoverer.Docker == nil {
				c.Discoverer.Docker = &d.Docker
			}
		case "k8s":
			c.Discoverer.K8s = append(c.Discoverer.K8s, d.K8s...)
		case "snmp":
			if c.Discoverer.SNMP == nil {
				c.Discoverer.SNMP = &d.SNMP
			}
		}
	}
}

// MarshalYAML implements yaml.Marshaler.
// It only marshals the canonical format, not legacy fields.
func (c Config) MarshalYAML() (any, error) {
	type output struct {
		Disabled   bool                `yaml:"disabled,omitempty"`
		Name       string              `yaml:"name"`
		Discoverer DiscovererConfig    `yaml:"discoverer,omitempty"`
		Services   []ServiceRuleConfig `yaml:"services,omitempty"`
	}
	return output{
		Disabled:   c.Disabled,
		Name:       c.Name,
		Discoverer: c.Discoverer,
		Services:   c.Services,
	}, nil
}

// UnmarshalJSON implements json.Unmarshaler for dyncfg payloads.
// Note: Dyncfg JSON uses a flattened format per discoverer type,
// so this is only used for generic JSON that matches the YAML structure.
func (c *Config) UnmarshalJSON(data []byte) error {
	type plain Config // avoid recursion

	// Use yaml.Unmarshal since it handles JSON too and our tags are compatible
	if err := yaml.Unmarshal(data, (*plain)(c)); err != nil {
		return err
	}

	// Convert legacy discover[] to new discoverer{} format
	if len(c.LegacyDiscover) > 0 && c.Discoverer.Empty() {
		c.convertLegacyDiscover()
	}

	// Convert legacy classify/compose to canonical services format
	if len(c.Services) == 0 && (len(c.LegacyClassify) > 0 || len(c.LegacyCompose) > 0) {
		services, err := ConvertOldToServices(c.LegacyClassify, c.LegacyCompose)
		if err != nil {
			return fmt.Errorf("failed to convert legacy config: %w", err)
		}
		c.Services = services
	}

	// Clear legacy fields
	c.LegacyDiscover = nil
	c.LegacyClassify = nil
	c.LegacyCompose = nil

	return nil
}

// LegacyDiscoveryConfig is the old discover[] array item format.
// Kept for backwards compatibility during unmarshal.
type LegacyDiscoveryConfig struct {
	Discoverer   string             `yaml:"discoverer"`
	NetListeners netlistensd.Config `yaml:"net_listeners"`
	Docker       dockersd.Config    `yaml:"docker"`
	K8s          []k8ssd.Config     `yaml:"k8s"`
	SNMP         snmpsd.Config      `yaml:"snmp"`
}

type ServiceRuleConfig struct {
	ID             string `yaml:"id"`              // mandatory (for logging/diagnostics)
	Match          string `yaml:"match"`           // mandatory
	ConfigTemplate string `yaml:"config_template"` // optional (drop if empty)
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

func validateConfig(cfg Config) error {
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
