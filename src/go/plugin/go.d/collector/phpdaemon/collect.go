// SPDX-License-Identifier: GPL-3.0-or-later

package phpdaemon

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/stm"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

// https://github.com/kakserpom/phpdaemon/blob/master/PHPDaemon/Core/Daemon.php
// see getStateOfWorkers()

type fullStatus struct {
	// Alive is sum of Idle, Busy and Reloading
	Alive    int64 `json:"alive" stm:"alive"`
	Shutdown int64 `json:"shutdown" stm:"shutdown"`

	// Idle that the worker is not in the middle of execution valuable callback (e.g. request) at this moment of time.
	// It does not mean that worker not have any pending operations.
	// Idle is sum of Preinit, Init and Initialized.
	Idle int64 `json:"idle" stm:"idle"`
	// Busy means that the worker is in the middle of execution valuable callback.
	Busy      int64 `json:"busy" stm:"busy"`
	Reloading int64 `json:"reloading" stm:"reloading"`

	Preinit int64 `json:"preinit" stm:"preinit"`
	// Init means that worker is starting right now.
	Init int64 `json:"init" stm:"init"`
	// Initialized means that the worker is in Idle state.
	Initialized int64 `json:"initialized" stm:"initialized"`

	Uptime *int64 `json:"uptime" stm:"uptime"`
}

func (c *Collector) collect() (map[string]int64, error) {
	req, err := web.NewHTTPRequest(c.RequestConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request to '%s': %w", c.URL, err)
	}

	var st fullStatus

	if err := web.DoHTTP(c.httpClient).RequestJSON(req, &st); err != nil {
		return nil, err
	}

	// https://github.com/kakserpom/phpdaemon/blob/master/PHPDaemon/Core/Daemon.php
	// see getStateOfWorkers()
	st.Initialized = st.Idle - (st.Init + st.Preinit)

	mx := stm.ToMap(st)

	c.once.Do(func() {
		if _, ok := mx["uptime"]; ok {
			_ = c.charts.Add(uptimeChart.Copy())
		}
	})

	return mx, nil
}
