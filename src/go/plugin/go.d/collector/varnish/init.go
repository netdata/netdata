// SPDX-License-Identifier: GPL-3.0-or-later

package varnish

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/netdata/netdata/go/plugins/pkg/executable"
)

func (c *Collector) initVarnishstatBinary() (varnishstatBinary, error) {
	if c.Config.DockerContainer != "" {
		return newVarnishstatDockerExecBinary(c.Config, c.Logger), nil
	}

	ndsudoPath := filepath.Join(executable.Directory, "ndsudo")

	if _, err := os.Stat(ndsudoPath); err != nil {
		return nil, fmt.Errorf("ndsudo executable not found: %v", err)

	}

	varnishstat := newVarnishstatExecBinary(ndsudoPath, c.Config, c.Logger)

	return varnishstat, nil
}
