// SPDX-License-Identifier: GPL-3.0-or-later

package receiver

import (
	"context"
	"time"
)

const geoIPRefreshInterval = time.Minute

func (c *Collector) runGeoIP(ctx context.Context, ticks <-chan time.Time) {
	for {
		if ctx.Err() != nil {
			return
		}
		if err := c.geo.Refresh(); err != nil {
			c.Limit("geoip-refresh", 1, time.Hour).
				Warningf("GeoIP refresh: %v; provision a supported database using atomic file replacement", err)
		}
		c.publishGeoIP()
		select {
		case <-ctx.Done():
			return
		case <-ticks:
		case <-c.geo.Faults():
		}
	}
}

// Serialize snapshotting and publication so a collection cannot overwrite a
// newer worker observation with an older source's diagnostics.
func (c *Collector) publishGeoIP() {
	c.publicationMu.Lock()
	defer c.publicationMu.Unlock()
	if c.publication != nil {
		c.publication.SetGeoIP(c.geo.Status())
	}
}
