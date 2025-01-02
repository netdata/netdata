// SPDX-License-Identifier: GPL-3.0-or-later

package pipeline

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/discovery/sd/model"
)

func newTargetClassificator(cfg []ClassifyRuleConfig) (*targetClassificator, error) {
	rules, err := newClassifyRules(cfg)
	if err != nil {
		return nil, err
	}

	c := &targetClassificator{
		rules: rules,
		buf:   bytes.Buffer{},
	}

	return c, nil
}

type (
	targetClassificator struct {
		*logger.Logger
		rules []*classifyRule
		buf   bytes.Buffer
	}

	classifyRule struct {
		name  string
		sr    selector
		tags  model.Tags
		match []*classifyRuleMatch
	}
	classifyRuleMatch struct {
		tags model.Tags
		expr *template.Template
	}
)

func (c *targetClassificator) classify(tgt model.Target) model.Tags {
	tgtTags := tgt.Tags().Clone()
	var tags model.Tags

	for i, rule := range c.rules {
		if !rule.sr.matches(tgtTags) {
			continue
		}

		for j, match := range rule.match {
			c.buf.Reset()

			if err := match.expr.Execute(&c.buf, tgt); err != nil {
				c.Warningf("failed to execute classify rule[%d]->match[%d]->expr on target '%s'", i+1, j+1, tgt.TUID())
				continue
			}
			if strings.TrimSpace(c.buf.String()) != "true" {
				continue
			}

			if tags == nil {
				tags = model.NewTags()
			}

			tags.Add(rule.tags)
			tags.Add(match.tags)
			tgtTags.Merge(tags)
		}
	}

	return tags
}

func newClassifyRules(cfg []ClassifyRuleConfig) ([]*classifyRule, error) {
	var rules []*classifyRule

	fmap := newFuncMap()

	for i, ruleCfg := range cfg {
		i++
		rule := classifyRule{name: ruleCfg.Name}

		sr, err := parseSelector(ruleCfg.Selector)
		if err != nil {
			return nil, fmt.Errorf("rule '%d': %v", i, err)
		}
		rule.sr = sr

		tags, err := model.ParseTags(ruleCfg.Tags)
		if err != nil {
			return nil, fmt.Errorf("rule '%d': %v", i, err)
		}
		rule.tags = tags

		for j, matchCfg := range ruleCfg.Match {
			j++
			var match classifyRuleMatch

			tags, err := model.ParseTags(matchCfg.Tags)
			if err != nil {
				return nil, fmt.Errorf("rule '%d/%d': %v", i, j, err)
			}
			match.tags = tags

			tmpl, err := parseTemplate(matchCfg.Expr, fmap)
			if err != nil {
				return nil, fmt.Errorf("rule '%d/%d': %v", i, j, err)
			}
			match.expr = tmpl

			rule.match = append(rule.match, &match)
		}

		rules = append(rules, &rule)
	}

	return rules, nil
}

func parseTemplate(s string, fmap template.FuncMap) (*template.Template, error) {
	return template.New("root").
		Option("missingkey=error").
		Funcs(fmap).
		Parse(s)
}
