// SPDX-License-Identifier: GPL-3.0-or-later

package powervault

// collectPoolCapacity reports per-pool total and available capacity.
// Uses cached discovery data — no API calls.
// Values from the API are in 512-byte blocks; we convert to bytes.
func (c *Collector) collectPoolCapacity() {
	const blockSize = 512

	for _, p := range c.discovered.pools {
		name := p.Name
		c.mx.pool.totalBytes.WithLabelValues(name).Observe(float64(p.TotalSizeNumeric * blockSize))
		c.mx.pool.availableBytes.WithLabelValues(name).Observe(float64(p.TotalAvailNumeric * blockSize))
	}
}
