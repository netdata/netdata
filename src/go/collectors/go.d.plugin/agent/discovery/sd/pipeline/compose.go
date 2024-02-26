// SPDX-License-Identifier: GPL-3.0-or-later

package pipeline

import (
	"bytes"
	"text/template"

	"github.com/netdata/netdata/go/go.d.plugin/agent/confgroup"
	"github.com/netdata/netdata/go/go.d.plugin/agent/discovery/sd/model"
	"github.com/netdata/netdata/go/go.d.plugin/logger"

	"gopkg.in/yaml.v2"
)

func newConfigComposer(cfg []ComposeRuleConfig) (*configComposer, error) {
	rules, err := newComposeRules(cfg)
	if err != nil {
		return nil, err
	}

	c := &configComposer{
		rules: rules,
		buf:   bytes.Buffer{},
	}

	return c, nil
}

type (
	configComposer struct {
		*logger.Logger
		rules []*composeRule
		buf   bytes.Buffer
	}

	composeRule struct {
		name string
		sr   selector
		conf []*composeRuleConf
	}
	composeRuleConf struct {
		sr   selector
		tmpl *template.Template
	}
)

func (c *configComposer) compose(tgt model.Target) []confgroup.Config {
	var configs []confgroup.Config

	for i, rule := range c.rules {
		if !rule.sr.matches(tgt.Tags()) {
			continue
		}

		for j, conf := range rule.conf {
			if !conf.sr.matches(tgt.Tags()) {
				continue
			}

			c.buf.Reset()

			if err := conf.tmpl.Execute(&c.buf, tgt); err != nil {
				c.Warningf("failed to execute rule[%d]->config[%d]->template on target '%s'", i+1, j+1, tgt.TUID())
				continue
			}
			if c.buf.Len() == 0 {
				continue
			}

			var cfg confgroup.Config

			if err := yaml.Unmarshal(c.buf.Bytes(), &cfg); err != nil {
				c.Warningf("failed on yaml unmarshalling: %v", err)
				continue
			}

			configs = append(configs, cfg)
		}
	}

	if len(configs) > 0 {
		c.Infof("created %d config(s) for target '%s'", len(configs), tgt.TUID())
	}
	return configs
}

func newComposeRules(cfg []ComposeRuleConfig) ([]*composeRule, error) {
	var rules []*composeRule

	fmap := newFuncMap()

	for _, ruleCfg := range cfg {
		rule := composeRule{name: ruleCfg.Name}

		sr, err := parseSelector(ruleCfg.Selector)
		if err != nil {
			return nil, err
		}
		rule.sr = sr

		for _, confCfg := range ruleCfg.Config {
			var conf composeRuleConf

			sr, err := parseSelector(confCfg.Selector)
			if err != nil {
				return nil, err
			}
			conf.sr = sr

			tmpl, err := parseTemplate(confCfg.Template, fmap)
			if err != nil {
				return nil, err
			}
			conf.tmpl = tmpl

			rule.conf = append(rule.conf, &conf)
		}

		rules = append(rules, &rule)
	}

	return rules, nil
}
