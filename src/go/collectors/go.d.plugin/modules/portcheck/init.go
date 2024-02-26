// SPDX-License-Identifier: GPL-3.0-or-later

package portcheck

import (
	"errors"

	"github.com/netdata/netdata/go/go.d.plugin/agent/module"
)

func (pc *PortCheck) validateConfig() error {
	if pc.Host == "" {
		return errors.New("'host' parameter not set")
	}
	if len(pc.Ports) == 0 {
		return errors.New("'ports' parameter not set")
	}
	return nil
}

func (pc *PortCheck) initCharts() (*module.Charts, error) {
	var charts module.Charts

	for _, port := range pc.Ports {
		if err := charts.Add(*newPortCharts(pc.Host, port)...); err != nil {
			return nil, err
		}
	}

	return &charts, nil
}
