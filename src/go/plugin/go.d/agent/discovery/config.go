// SPDX-License-Identifier: GPL-3.0-or-later

package discovery

import (
	"errors"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/discovery/dummy"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/discovery/file"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/discovery/sd"
)

type Config struct {
	Registry confgroup.Registry
	File     file.Config
	Dummy    dummy.Config
	SD       sd.Config
}

func validateConfig(cfg Config) error {
	if len(cfg.Registry) == 0 {
		return errors.New("empty config registry")
	}
	if len(cfg.File.Read)+len(cfg.File.Watch) == 0 && len(cfg.Dummy.Names) == 0 {
		return errors.New("discoverers not set")
	}
	return nil
}
