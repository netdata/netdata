// SPDX-License-Identifier: GPL-3.0-or-later

package cato_networks

const (
	warningKeyCollection      = "collection"
	warningKeyDiscoveryCache  = "discovery_cache"
	warningKeyMetrics         = "metrics"
	warningKeyBGP             = "bgp"
	warningKeyEvents          = "events"
	warningKeyEventMarker     = "event_marker"
	warningKeyEventAccountErr = "event_account_error"
)

func (c *Collector) warnRecoverable(key, class, format string, args ...any) {
	if c.warningStates == nil {
		c.warningStates = make(map[string]string)
	}
	if c.warningStates[key] == class {
		c.Debugf(format, args...)
		return
	}
	c.warningStates[key] = class
	c.Warningf(format, args...)
}

func (c *Collector) clearRecoverableWarning(key string) {
	if c.warningStates == nil {
		return
	}
	delete(c.warningStates, key)
}
