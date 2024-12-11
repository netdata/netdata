// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package dnsmasq_dhcp

import "errors"

func (c *Collector) validateConfig() error {
	if c.LeasesPath == "" {
		return errors.New("empty 'leases_path'")
	}
	return nil
}

func (c *Collector) checkLeasesPath() error {
	f, err := openFile(c.LeasesPath)
	if err != nil {
		return err
	}
	_ = f.Close()
	return nil
}
