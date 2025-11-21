// SPDX-License-Identifier: GPL-3.0-or-later

package pipeline

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"text/template"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/discovery/sd/model"

	"gopkg.in/yaml.v2"
)

func newServiceEngine(cfg []ServiceRuleConfig) (*serviceEngine, error) {
	rules, err := newServiceRules(cfg)
	if err != nil {
		return nil, err
	}
	return &serviceEngine{rules: rules}, nil
}

type serviceEngine struct {
	*logger.Logger
	rules []*serviceRule
	buf   bytes.Buffer
}

type serviceRule struct {
	id    string
	match *template.Template
	tmpl  *template.Template // optional
}

func newServiceRules(cfg []ServiceRuleConfig) ([]*serviceRule, error) {
	fmap := newFuncMap()
	var rules []*serviceRule

	for i, rc := range cfg {
		i++

		m, err := parseTemplate(rc.Match, fmap)
		if err != nil {
			return nil, fmt.Errorf("service '%s'[%d]: match: %v", rc.ID, i, err)
		}

		var tmpl *template.Template
		if strings.TrimSpace(rc.ConfigTemplate) != "" {
			tmpl, err = parseTemplate(rc.ConfigTemplate, fmap)
			if err != nil {
				return nil, fmt.Errorf("service '%s'[%d]: config_template: %v", rc.ID, i, err)
			}
		}

		rules = append(rules, &serviceRule{
			id: rc.ID, match: m, tmpl: tmpl,
		})
	}
	return rules, nil
}

func (s *serviceEngine) compose(tgt model.Target) []confgroup.Config {
	var out []confgroup.Config

	for i, r := range s.rules {
		s.buf.Reset()

		if err := r.match.Execute(&s.buf, tgt); err != nil {
			s.Warningf("failed to execute services[%d]->match on target '%s'", i+1, tgt.TUID())
			continue
		}
		if strings.TrimSpace(s.buf.String()) != "true" {
			continue
		}

		// No config_template => drop
		if r.tmpl == nil {
			break
		}

		s.buf.Reset()
		if err := r.tmpl.Execute(&s.buf, tgt); err != nil {
			s.Warningf("failed to execute services[%d]->config_template on target '%s': %v", i+1, tgt.TUID(), err)
			continue
		}
		if s.buf.Len() == 0 {
			continue
		}

		cfgs, err := parseConfigTemplateData(s.buf.Bytes())
		if err != nil {
			s.Warningf("failed to parse services[%d] template data: %v", i+1, err)
			continue
		}

		for _, cfg := range cfgs {
			if cfg.Module() == "" {
				cfg.SetModule(r.id)
			}
		}

		out = append(out, cfgs...)
	}

	if len(out) > 0 {
		s.Debugf("created %d config(s) for target '%s'", len(out), tgt.TUID())
	}

	return out
}

func parseConfigTemplateData(bs []byte) ([]confgroup.Config, error) {
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
