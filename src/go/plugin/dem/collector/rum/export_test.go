// SPDX-License-Identifier: GPL-3.0-or-later

package rum

// TransportCounter lets external tests read the job's transport accounting.
func (c *Collector) TransportCounter(name string) uint64 { return c.transport.get(name) }
