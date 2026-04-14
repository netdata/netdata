// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"encoding/json"
	"slices"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"gopkg.in/yaml.v2"
)

func (m *Manager) dyncfgSetConfigMeta(cfg confgroup.Config, module, name string, fn dyncfg.Function) {
	cfg.SetProvider("dyncfg")
	cfg.SetSource(fn.Source())
	cfg.SetSourceType("dyncfg")
	cfg.SetModule(module)
	cfg.SetName(name)
	if def, ok := m.configDefaults.Lookup(module); ok {
		cfg.ApplyDefaults(def)
	}
}

func userConfigFromPayload(cfg any, jobName string, fn dyncfg.Function) ([]byte, error) {
	if err := fn.UnmarshalPayload(cfg); err != nil {
		return nil, err
	}

	bs, err := yaml.Marshal(cfg)
	if err != nil {
		return nil, err
	}

	var yms yaml.MapSlice
	if err := yaml.Unmarshal(bs, &yms); err != nil {
		return nil, err
	}

	yms = slices.DeleteFunc(yms, func(item yaml.MapItem) bool { return item.Key == "name" })

	yms = append([]yaml.MapItem{{Key: "name", Value: jobName}}, yms...)

	v := map[string]any{
		"jobs": []any{yms},
	}

	return yaml.Marshal(v)
}

func configFromPayload(fn dyncfg.Function) (confgroup.Config, error) {
	var cfg confgroup.Config

	if fn.IsContentTypeJSON() {
		if err := json.Unmarshal(fn.Payload(), &cfg); err != nil {
			return nil, err
		}

		return cfg.Clone()
	}

	if err := yaml.Unmarshal(fn.Payload(), &cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (m *Manager) extractModuleJobName(id string) (mn string, jn string, ok bool) {
	if mn, ok = m.extractModuleName(id); !ok {
		return "", "", false
	}
	if jn, ok = extractJobName(id); !ok {
		return "", "", false
	}
	return mn, jn, true
}

func (m *Manager) extractModuleName(id string) (string, bool) {
	id = strings.TrimPrefix(id, m.dyncfgCollectorPrefixValue())
	before, _, ok := strings.Cut(id, ":")
	if !ok {
		return id, id != ""
	}
	return before, true
}

func extractJobName(id string) (string, bool) {
	i := strings.LastIndexByte(id, ':')
	if i == -1 {
		return "", false
	}
	return id[i+1:], true
}
