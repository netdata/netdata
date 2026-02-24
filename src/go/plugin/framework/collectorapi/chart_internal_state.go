// SPDX-License-Identifier: GPL-3.0-or-later

package collectorapi

func (c *Chart) CachedType() string {
	return c.typ
}

func (c *Chart) SetCachedType(v string) {
	c.typ = v
}

func (c *Chart) CachedID() string {
	return c.id
}

func (c *Chart) SetCachedID(v string) {
	c.id = v
}

func (c *Chart) IsCreated() bool {
	return c.created
}

func (c *Chart) SetCreated(v bool) {
	c.created = v
}

func (c *Chart) IsUpdated() bool {
	return c.updated
}

func (c *Chart) SetUpdated(v bool) {
	c.updated = v
}

func (c *Chart) IsIgnored() bool {
	return c.ignore
}

func (c *Chart) SetIgnored(v bool) {
	c.ignore = v
}

func (c *Chart) IsRemoved() bool {
	return c.remove
}

func (c *Chart) SetRemoved(v bool) {
	c.remove = v
}

func (d *Dim) IsRemoved() bool {
	return d.remove
}

func (d *Dim) SetRemoved(v bool) {
	d.remove = v
}
