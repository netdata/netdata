// SPDX-License-Identifier: GPL-3.0-or-later

package powerstore

func (c *Collector) collectAlerts() {
	c.sem <- struct{}{}
	defer func() { <-c.sem }()

	alerts, err := c.client.Alerts("ACTIVE")
	if err != nil {
		c.Warningf("error collecting alerts: %v", err)
		return
	}

	var crit, major, minor, info float64
	for _, a := range alerts {
		switch a.Severity {
		case "Critical":
			crit++
		case "Major":
			major++
		case "Minor":
			minor++
		case "Info":
			info++
		}
	}

	c.mx.alerts.critical.Observe(crit)
	c.mx.alerts.major.Observe(major)
	c.mx.alerts.minor.Observe(minor)
	c.mx.alerts.info.Observe(info)
}
