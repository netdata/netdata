// SPDX-License-Identifier: GPL-3.0-or-later

package samba

func (c *Collector) initSmbStatusBinary() (smbStatusBinary, error) {
	smbStatus := newSmbStatusBinary(c.Timeout.Duration(), c.Logger)

	return smbStatus, nil
}
