// SPDX-License-Identifier: GPL-3.0-or-later

package statsd

import (
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

// initTemplates loads the configured profiles and publishes the initial native
// set: diagnostics only, since no profile is active before admitted input.
func (c *Collector) initTemplates() error {
	var err error
	if c.profiles, err = loadProfiles(c.profileDirs, c.Profiles, c.Logger); err != nil {
		return err
	}
	c.templates, err = c.templateSet(nil)
	return err
}

// initReceiver sizes metadata retirement by the longest prepared chart or
// dimension lifetime, so a declaration outlives every chart that shows it.
func (c *Collector) initReceiver() {
	lifetime := uint64(defaultChartExpiry)
	for _, p := range c.profiles {
		lifetime = max(lifetime, p.lifetime)
	}
	c.diagnostics = newDiagnostics(c.store, c.Config)
	c.receiver = newReceiver(
		c.MaxSeries,
		time.Duration(c.MetricIdleTimeout),
		c.store.(metrix.DescriptorRetention),
		lifetime,
		c.profiles,
	)
}
