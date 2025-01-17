// SPDX-License-Identifier: GPL-3.0-or-later

package ethtool

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/netdata/netdata/go/plugins/pkg/executable"
)

func (c *Collector) validateConfig() error {
	if c.OpticInterfaces == "" {
		return errors.New("no optic interfaces specified")
	}
	return nil
}

func (c *Collector) initEthtoolCli() (ethtoolCli, error) {
	ndsudoPath := filepath.Join(executable.Directory, "ndsudo")
	if _, err := os.Stat(ndsudoPath); err != nil {
		return nil, fmt.Errorf("ndsudo executable not found: %v", err)

	}

	et := newEthtoolExec(ndsudoPath, c.Timeout.Duration(), c.Logger)

	return et, nil
}
