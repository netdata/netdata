// SPDX-License-Identifier: GPL-3.0-or-later

package varnish

func (c *Collector) initVarnishstatBinary() (varnishstatBinary, error) {
	if c.Config.DockerContainer != "" {
		return newVarnishstatDockerExecBinary(c.Config, c.Logger), nil
	}

	varnishstat := newVarnishstatExecBinary(c.Config, c.Logger)

	return varnishstat, nil
}
