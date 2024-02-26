// SPDX-License-Identifier: GPL-3.0-or-later

package discovery

import (
	"errors"

	"github.com/netdata/netdata/go/go.d.plugin/agent/confgroup"
	"github.com/netdata/netdata/go/go.d.plugin/agent/discovery/dummy"
	"github.com/netdata/netdata/go/go.d.plugin/agent/discovery/file"
)

type Config struct {
	Registry confgroup.Registry
	File     file.Config
	Dummy    dummy.Config
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
