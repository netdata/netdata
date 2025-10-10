// SPDX-License-Identifier: GPL-3.0-or-later

package freeradius

import (
	"github.com/netdata/netdata/go/plugins/pkg/stm"
)

func (c *Collector) collect() (map[string]int64, error) {
	status, err := c.client.Status()
	if err != nil {
		return nil, err
	}

	return stm.ToMap(status), nil
}
