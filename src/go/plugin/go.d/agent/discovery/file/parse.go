// SPDX-License-Identifier: GPL-3.0-or-later

package file

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/confgroup"

	"github.com/goccy/go-yaml"
)

type format int

const (
	unknownFormat format = iota
	unknownEmptyFormat
	staticFormat
	sdFormat
)

func parse(req confgroup.Registry, path string) (*confgroup.Group, error) {
	bs, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(bs) == 0 {
		return nil, nil
	}

	switch cfgFormat(bs) {
	case staticFormat:
		return parseStaticFormat(req, path, bs)
	case sdFormat:
		return parseSDFormat(req, path, bs)
	case unknownEmptyFormat:
		return nil, nil
	default:
		return nil, fmt.Errorf("unknown file format: '%s'", path)
	}
}

func parseStaticFormat(reg confgroup.Registry, path string, bs []byte) (*confgroup.Group, error) {
	name := fileName(path)
	// TODO: properly handle module renaming
	// See agent/setup.go buildDiscoveryConf() for details
	if name == "wmi" {
		name = "windows"
	}
	modDef, ok := reg.Lookup(name)
	if !ok {
		return nil, nil
	}

	var modCfg staticConfig
	if err := yaml.Unmarshal(bs, &modCfg); err != nil {
		return nil, err
	}

	for _, cfg := range modCfg.Jobs {
		cfg.SetModule(name)
		def := mergeDef(modCfg.Default, modDef)
		cfg.ApplyDefaults(def)
	}

	group := &confgroup.Group{
		Configs: modCfg.Jobs,
		Source:  path,
	}

	return group, nil
}

func parseSDFormat(reg confgroup.Registry, path string, bs []byte) (*confgroup.Group, error) {
	var cfgs sdConfig
	if err := yaml.Unmarshal(bs, &cfgs); err != nil {
		return nil, err
	}

	var i int
	for _, cfg := range cfgs {
		if def, ok := reg.Lookup(cfg.Module()); ok && cfg.Module() != "" {
			cfg.ApplyDefaults(def)
			cfgs[i] = cfg
			i++
		}
	}

	group := &confgroup.Group{
		Configs: cfgs[:i],
		Source:  path,
	}

	return group, nil
}

func cfgFormat(bs []byte) format {
	var data any
	if err := yaml.Unmarshal(bs, &data); err != nil {
		return unknownFormat
	}
	if data == nil {
		return unknownEmptyFormat
	}

	type (
		static = map[string]any
		sd     = []any
	)
	switch data.(type) {
	case static:
		return staticFormat
	case sd:
		return sdFormat
	default:
		return unknownFormat
	}
}

func mergeDef(a, b confgroup.Default) confgroup.Default {
	return confgroup.Default{
		MinUpdateEvery:     firstPositive(a.MinUpdateEvery, b.MinUpdateEvery),
		UpdateEvery:        firstPositive(a.UpdateEvery, b.UpdateEvery),
		AutoDetectionRetry: firstPositive(a.AutoDetectionRetry, b.AutoDetectionRetry),
		Priority:           firstPositive(a.Priority, b.Priority),
	}
}

func firstPositive(value int, others ...int) int {
	if value > 0 || len(others) == 0 {
		return value
	}
	return firstPositive(others[0], others[1:]...)
}

func fileName(path string) string {
	_, file := filepath.Split(path)
	ext := filepath.Ext(path)
	return file[:len(file)-len(ext)]
}
