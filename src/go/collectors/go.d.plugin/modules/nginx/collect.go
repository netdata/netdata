// SPDX-License-Identifier: GPL-3.0-or-later

package nginx

import (
	"github.com/netdata/go.d.plugin/pkg/stm"
)

func (n *Nginx) collect() (map[string]int64, error) {
	status, err := n.apiClient.getStubStatus()

	if err != nil {
		return nil, err
	}

	return stm.ToMap(status), nil
}
