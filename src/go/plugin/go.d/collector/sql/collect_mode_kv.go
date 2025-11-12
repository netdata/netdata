// SPDX-License-Identifier: GPL-3.0-or-later

package sql

// KV mode: build chart(s) and aggregate dim values into mx
func (c *Collector) collectMetricsModeKV(mx map[string]int64, m ConfigMetricBlock, rows []map[string]string) error {
	if m.KVMode == nil {
		// config validator should prevent this; guard anyway
		return nil
	}
	nameCol := m.KVMode.NameCol
	valCol := m.KVMode.ValueCol

	for _, ch := range m.Charts {
		for _, row := range rows {
			// chart instance for this row
			chartID := c.chartInstanceID(m, ch, row)
			if chartID == "" {
				// missing a required label source for this row
				continue
			}
			// create chart (once) with static + row labels and all dims
			c.ensureChart(chartID, m, ch, row)

			// get KV pair from row
			k, ok1 := row[nameCol]
			vraw, ok2 := row[valCol]
			if !ok1 || !ok2 {
				continue
			}

			// route to dims whose Source matches this key
			for _, d := range ch.Dims {
				if d.Source != k {
					continue
				}
				id := dimID(chartID, d.Name)

				// status_when overrides numeric parsing
				if d.StatusWhen != nil {
					ok, err := c.evalStatusWhen(d.StatusWhen, vraw)
					if err != nil {
						c.Warningf("status_when eval failed for %s/%s: %v", chartID, d.Name, err)
						continue
					}
					mx[id] += btoi(ok)
					continue
				}

				// numeric path
				mx[id] += toInt64(vraw)
			}
		}
	}
	return nil
}
