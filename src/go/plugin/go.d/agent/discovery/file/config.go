// SPDX-License-Identifier: GPL-3.0-or-later

package file

import (
	"errors"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/confgroup"
)

type Config struct {
	Registry confgroup.Registry
	Read     []string
	Watch    []string
}

func validateConfig(cfg Config) error {
	if len(cfg.Registry) == 0 {
		return errors.New("empty config registry")
	}
	if len(cfg.Read)+len(cfg.Watch) == 0 {
		return errors.New("discoverers not set")
	}
	return nil
}
