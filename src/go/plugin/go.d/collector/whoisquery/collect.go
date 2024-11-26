// SPDX-License-Identifier: GPL-3.0-or-later

package whoisquery

import "fmt"

func (c *Collector) collect() (map[string]int64, error) {
	remainingTime, err := c.prov.remainingTime()
	if err != nil {
		return nil, fmt.Errorf("%v (source: %s)", err, c.Source)
	}

	mx := make(map[string]int64)
	c.collectExpiration(mx, remainingTime)

	return mx, nil
}

func (c *Collector) collectExpiration(mx map[string]int64, remainingTime float64) {
	mx["expiry"] = int64(remainingTime)
	mx["days_until_expiration_warning"] = c.DaysUntilWarn
	mx["days_until_expiration_critical"] = c.DaysUntilCrit
}
