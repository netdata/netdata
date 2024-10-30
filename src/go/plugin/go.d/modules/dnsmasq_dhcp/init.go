// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package dnsmasq_dhcp

import "errors"

func (d *DnsmasqDHCP) validateConfig() error {
	if d.LeasesPath == "" {
		return errors.New("empty 'leases_path'")
	}
	return nil
}

func (d *DnsmasqDHCP) checkLeasesPath() error {
	f, err := openFile(d.LeasesPath)
	if err != nil {
		return err
	}
	_ = f.Close()
	return nil
}
