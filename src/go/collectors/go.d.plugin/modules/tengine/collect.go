// SPDX-License-Identifier: GPL-3.0-or-later

package tengine

import (
	"github.com/netdata/go.d.plugin/pkg/stm"
)

func (t *Tengine) collect() (map[string]int64, error) {
	status, err := t.apiClient.getStatus()
	if err != nil {
		return nil, err
	}

	mx := make(map[string]int64)
	for _, m := range *status {
		for k, v := range stm.ToMap(m) {
			mx[k] += v
		}
	}
	return mx, nil
}
