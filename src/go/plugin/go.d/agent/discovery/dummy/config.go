// SPDX-License-Identifier: GPL-3.0-or-later

package dummy

import (
	"errors"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/confgroup"
)

type Config struct {
	Registry confgroup.Registry
	Names    []string
}

func validateConfig(cfg Config) error {
	if len(cfg.Registry) == 0 {
		return errors.New("empty config registry")
	}
	if len(cfg.Names) == 0 {
		return errors.New("names not set")
	}
	return nil
}
