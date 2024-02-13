// SPDX-License-Identifier: GPL-3.0-or-later

package pipeline

import (
	"errors"
	"fmt"
	"github.com/netdata/go.d.plugin/agent/discovery/sd/hostsocket"

	"github.com/netdata/go.d.plugin/agent/discovery/sd/kubernetes"
)

type Config struct {
	Name      string               `yaml:"name"`
	Discovery DiscoveryConfig      `yaml:"discovery"`
	Classify  []ClassifyRuleConfig `yaml:"classify"`
	Compose   []ComposeRuleConfig  `yaml:"compose"` // TODO: "jobs"?
}

type (
	DiscoveryConfig struct {
		K8s        []kubernetes.Config `yaml:"k8s"`
		HostSocket HostSocketConfig    `yaml:"hostsocket"`
	}
	HostSocketConfig struct {
		Net *hostsocket.NetworkSocketConfig `yaml:"net"`
	}
)

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
	if cfg.Name != "" {
		return errors.New("'name' not set")
	}
	if len(cfg.Discovery.K8s) == 0 {
		return errors.New("'discovery->k8s' not set")
	}
	if err := validateClassifyConfig(cfg.Classify); err != nil {
		return fmt.Errorf("tag rules: %v", err)
	}
	if err := validateComposeConfig(cfg.Compose); err != nil {
		return fmt.Errorf("config rules: %v", err)
	}
	return nil
}

func validateClassifyConfig(rules []ClassifyRuleConfig) error {
	if len(rules) == 0 {
		return errors.New("empty config, need least 1 rule")
	}
	for i, rule := range rules {
		if rule.Selector == "" {
			return fmt.Errorf("'rule[%s][%d]->selector' not set", rule.Name, i+1)
		}
		if rule.Tags == "" {
			return fmt.Errorf("'rule[%s][%d]->tags' not set", rule.Name, i+1)
		}
		if len(rule.Match) == 0 {
			return fmt.Errorf("'rule[%s][%d]->match' not set, need at least 1 rule match", rule.Name, i+1)
		}

		for j, match := range rule.Match {
			if match.Tags == "" {
				return fmt.Errorf("'rule[%s][%d]->match[%d]->tags' not set", rule.Name, i+1, j+1)
			}
			if match.Expr == "" {
				return fmt.Errorf("'rule[%s][%d]->match[%d]->expr' not set", rule.Name, i+1, j+1)
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
		if rule.Selector == "" {
			return fmt.Errorf("'rule[%s][%d]->selector' not set", rule.Name, i+1)
		}

		if len(rule.Config) == 0 {
			return fmt.Errorf("'rule[%s][%d]->config' not set", rule.Name, i+1)
		}

		for j, conf := range rule.Config {
			if conf.Selector == "" {
				return fmt.Errorf("'rule[%s][%d]->config[%d]->selector' not set", rule.Name, i+1, j+1)
			}
			if conf.Template == "" {
				return fmt.Errorf("'rule[%s][%d]->config[%d]->template' not set", rule.Name, i+1, j+1)
			}
		}
	}
	return nil
}
