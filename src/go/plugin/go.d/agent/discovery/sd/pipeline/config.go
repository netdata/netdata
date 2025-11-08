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
)

type Config struct {
	Source         string             `yaml:"-"`
	ConfigDefaults confgroup.Registry `yaml:"-"`

	Disabled bool   `yaml:"disabled"`
	Name     string `yaml:"name"`

	Discover []DiscoveryConfig `yaml:"discover"`

	// New single-step format:
	Services []ServiceRuleConfig `yaml:"services"`

	// Legacy two-step:
	Classify []ClassifyRuleConfig `yaml:"classify"`
	Compose  []ComposeRuleConfig  `yaml:"compose"`
}

type DiscoveryConfig struct {
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
	if err := validateDiscoveryConfig(cfg.Discover); err != nil {
		return fmt.Errorf("discover config: %v", err)
	}

	switch {
	case len(cfg.Services) > 0:
		if err := validateServicesConfig(cfg.Services); err != nil {
			return fmt.Errorf("services rules: %v", err)
		}
	default:
		// Legacy path
		if err := validateClassifyConfig(cfg.Classify); err != nil {
			return fmt.Errorf("classify rules: %v", err)
		}
		if err := validateComposeConfig(cfg.Compose); err != nil {
			return fmt.Errorf("compose rules: %v", err)
		}
	}
	return nil
}

func validateDiscoveryConfig(config []DiscoveryConfig) error {
	if len(config) == 0 {
		return errors.New("no discoverers, must be at least one")
	}
	for _, cfg := range config {
		switch cfg.Discoverer {
		case "net_listeners", "docker", "k8s", "snmp":
		default:
			return fmt.Errorf("unknown discoverer: '%s'", cfg.Discoverer)
		}
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

func validateClassifyConfig(rules []ClassifyRuleConfig) error {
	if len(rules) == 0 {
		return errors.New("empty config, need least 1 rule")
	}
	for i, rule := range rules {
		i++
		if rule.Selector == "" {
			return fmt.Errorf("'rule[%s][%d]->selector' not set", rule.Name, i)
		}
		if rule.Tags == "" {
			return fmt.Errorf("'rule[%s][%d]->tags' not set", rule.Name, i)
		}
		if len(rule.Match) == 0 {
			return fmt.Errorf("'rule[%s][%d]->match' not set, need at least 1 rule match", rule.Name, i)
		}

		for j, match := range rule.Match {
			j++
			if match.Tags == "" {
				return fmt.Errorf("'rule[%s][%d]->match[%d]->tags' not set", rule.Name, i, j)
			}
			if match.Expr == "" {
				return fmt.Errorf("'rule[%s][%d]->match[%d]->expr' not set", rule.Name, i, j)
			}
		}
	}
	return nil
}

func validateComposeConfig(rules []ComposeRuleConfig) error {
	if len(rules) == 0 {
		return errors.New("empty config, need least 1 rule")
	}
	for i, rule := range rules {
		i++
		if rule.Selector == "" {
			return fmt.Errorf("'rule[%s][%d]->selector' not set", rule.Name, i)
		}

		if len(rule.Config) == 0 {
			return fmt.Errorf("'rule[%s][%d]->config' not set", rule.Name, i)
		}

		for j, conf := range rule.Config {
			j++
			if conf.Selector == "" {
				return fmt.Errorf("'rule[%s][%d]->config[%d]->selector' not set", rule.Name, i, j)
			}
			if conf.Template == "" {
				return fmt.Errorf("'rule[%s][%d]->config[%d]->template' not set", rule.Name, i, j)
			}
		}
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
