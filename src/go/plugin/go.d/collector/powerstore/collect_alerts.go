// SPDX-License-Identifier: GPL-3.0-or-later

package powerstore

func (c *Collector) collectAlerts(mx *metrics) {
	c.sem <- struct{}{}
	defer func() { <-c.sem }()

	alerts, err := c.client.Alerts("ACTIVE")
	if err != nil {
		c.Warningf("error collecting alerts: %v", err)
		return
	}

	for _, a := range alerts {
		switch a.Severity {
		case "Critical":
			mx.Alerts.Critical++
		case "Major":
			mx.Alerts.Major++
		case "Minor":
			mx.Alerts.Minor++
		case "Info":
			mx.Alerts.Info++
		}
	}
}
