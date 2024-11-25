// SPDX-License-Identifier: GPL-3.0-or-later

package whoisquery

import "fmt"

func (w *WhoisQuery) collect() (map[string]int64, error) {
	remainingTime, err := w.prov.remainingTime()
	if err != nil {
		return nil, fmt.Errorf("%v (source: %s)", err, w.Source)
	}

	mx := make(map[string]int64)
	w.collectExpiration(mx, remainingTime)

	return mx, nil
}

func (w *WhoisQuery) collectExpiration(mx map[string]int64, remainingTime float64) {
	mx["expiry"] = int64(remainingTime)
	mx["days_until_expiration_warning"] = w.DaysUntilWarn
	mx["days_until_expiration_critical"] = w.DaysUntilCrit
}
