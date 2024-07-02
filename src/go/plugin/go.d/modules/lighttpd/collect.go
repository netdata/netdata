// SPDX-License-Identifier: GPL-3.0-or-later

package lighttpd

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/stm"
)

func (l *Lighttpd) collect() (map[string]int64, error) {
	status, err := l.apiClient.getServerStatus()

	if err != nil {
		return nil, err
	}

	mx := stm.ToMap(status)

	if len(mx) == 0 {
		return nil, fmt.Errorf("nothing was collected from %s", l.URL)
	}

	return mx, nil
}
