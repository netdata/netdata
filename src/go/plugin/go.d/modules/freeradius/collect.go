// SPDX-License-Identifier: GPL-3.0-or-later

package freeradius

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/stm"
)

func (f *FreeRADIUS) collect() (map[string]int64, error) {
	status, err := f.client.Status()
	if err != nil {
		return nil, err
	}

	return stm.ToMap(status), nil
}
