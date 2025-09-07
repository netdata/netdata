// SPDX-License-Identifier: GPL-3.0-or-later

package pipeline

import (
	"errors"
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/discovery/sd/discoverer/dockersd"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/discovery/sd/discoverer/k8ssd"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/discovery/sd/discoverer/netlistensd"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/discovery/sd/discoverer/snmpsd"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"
)

type Config struct {
	Source         string             `yaml:"-"`
	ConfigDefaults confgroup.Registry `yaml:"-"`

	Disabled confopt.FlexBool     `yaml:"disabled"`
	Name     string               `yaml:"name"`
	Discover []DiscoveryConfig    `yaml:"discover"`
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
	if err := validateClassifyConfig(cfg.Classify); err != nil {
		return fmt.Errorf("classify rules: %v", err)
	}
	if err := validateComposeConfig(cfg.Compose); err != nil {
		return fmt.Errorf("compose rules: %v", err)
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
