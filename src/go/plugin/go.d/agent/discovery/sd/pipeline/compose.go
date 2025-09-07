// SPDX-License-Identifier: GPL-3.0-or-later

package pipeline

import (
	"bytes"
	"errors"
	"fmt"
	"text/template"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/discovery/sd/model"

	"github.com/goccy/go-yaml"
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
				c.Warningf("failed to execute rule[%d]->config[%d]->template on target '%s': %v",
					i+1, j+1, tgt.TUID(), err)
				continue
			}
			if c.buf.Len() == 0 {
				continue
			}

			cfgs, err := c.parseTemplateData(c.buf.Bytes())
			if err != nil {
				c.Warningf("failed to parse template data: %v", err)
				continue
			}

			configs = append(configs, cfgs...)
		}
	}

	if len(configs) > 0 {
		c.Debugf("created %d config(s) for target '%s'", len(configs), tgt.TUID())
	}
	return configs
}

func (c *configComposer) parseTemplateData(bs []byte) ([]confgroup.Config, error) {
	var data any
	if err := yaml.Unmarshal(bs, &data); err != nil {
		return nil, err
	}

	type (
		single = map[any]any
		multi  = []any
	)

	switch data.(type) {
	case single:
		var cfg confgroup.Config
		if err := yaml.Unmarshal(bs, &cfg); err != nil {
			return nil, err
		}
		return []confgroup.Config{cfg}, nil
	case multi:
		var cfgs []confgroup.Config
		if err := yaml.Unmarshal(bs, &cfgs); err != nil {
			return nil, err
		}
		return cfgs, nil
	default:
		return nil, errors.New("unknown config format")
	}
}

func newComposeRules(cfg []ComposeRuleConfig) ([]*composeRule, error) {
	var rules []*composeRule

	fmap := newFuncMap()

	for i, ruleCfg := range cfg {
		i++
		rule := composeRule{name: ruleCfg.Name}

		sr, err := parseSelector(ruleCfg.Selector)
		if err != nil {
			return nil, fmt.Errorf("rule '%d': %v", i, err)
		}
		rule.sr = sr

		for j, confCfg := range ruleCfg.Config {
			j++
			var conf composeRuleConf

			sr, err := parseSelector(confCfg.Selector)
			if err != nil {
				return nil, fmt.Errorf("rule '%d/%d': %v", i, j, err)
			}
			conf.sr = sr

			tmpl, err := parseTemplate(confCfg.Template, fmap)
			if err != nil {
				return nil, fmt.Errorf("rule '%d/%d': %v", i, j, err)
			}
			conf.tmpl = tmpl

			rule.conf = append(rule.conf, &conf)
		}

		rules = append(rules, &rule)
	}

	return rules, nil
}
