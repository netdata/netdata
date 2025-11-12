package sql

func (c *Collector) collectMetricsModeColumns(mx map[string]int64, m ConfigMetricBlock, rows []map[string]string) error {
	for _, ch := range m.Charts {
		for _, row := range rows {
			chartID := c.chartInstanceID(m, ch, row)
			if chartID == "" {
				continue // missing a required label source in this row
			}
			c.ensureChart(chartID, m, ch, row)

			for _, d := range ch.Dims {
				raw, ok := row[d.Source]
				if !ok {
					continue
				}
				id := dimID(chartID, d.Name)

				// --- status_when overrides numeric parsing ---
				if d.StatusWhen != nil {
					ok, err := c.evalStatusWhen(d.StatusWhen, raw)
					if err != nil {
						// bad regex or similar; log and skip this dim for this row
						c.Warningf("status_when eval failed for %s/%s: %v", chartID, d.Name, err)
						continue
					}
					mx[id] += btoi(ok)
					continue
				}

				// --- numeric path ---
				mx[id] += toInt64(raw)
			}
		}
	}
	return nil
}
