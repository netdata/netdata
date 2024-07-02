// SPDX-License-Identifier: GPL-3.0-or-later

package phpdaemon

import "github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/stm"

func (p *PHPDaemon) collect() (map[string]int64, error) {
	s, err := p.client.queryFullStatus()

	if err != nil {
		return nil, err
	}

	// https://github.com/kakserpom/phpdaemon/blob/master/PHPDaemon/Core/Daemon.php
	// see getStateOfWorkers()
	s.Initialized = s.Idle - (s.Init + s.Preinit)

	return stm.ToMap(s), nil
}
