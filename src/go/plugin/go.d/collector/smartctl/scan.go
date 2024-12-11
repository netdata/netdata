// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || freebsd || openbsd || netbsd || dragonfly

package smartctl

import (
	"errors"
	"fmt"
	"strings"
)

type scanDevice struct {
	name     string
	infoName string
	typ      string
	extra    bool // added via config "extra_devices"
}

func (s *scanDevice) key() string {
	return fmt.Sprintf("%s|%s", s.name, s.typ)
}

func (s *scanDevice) shortName() string {
	return strings.TrimPrefix(s.name, "/dev/")
}

func (c *Collector) scanDevices() (map[string]*scanDevice, error) {
	// Issue on Discord: https://discord.com/channels/847502280503590932/1261747175361347644/1261747175361347644
	// "sat" devices being identified as "scsi" with --scan, and then later
	// code attempts to validate the type by calling `smartctl` with the "scsi" type.
	// This validation can trigger unintended "Enabling discard_zeroes_data" messages in system logs (dmesg).
	// To address this specific issue we use `smartctl --scan-open` as a workaround.
	// This method reliably identifies device types.
	scanOpen := c.NoCheckPowerMode == "never"

	resp, err := c.exec.scan(scanOpen)
	if err != nil {
		return nil, fmt.Errorf("failed to scan devices: %v", err)
	}

	devices := make(map[string]*scanDevice)

	for _, d := range resp.Get("devices").Array() {
		dev := &scanDevice{
			name:     d.Get("name").String(),
			infoName: d.Get("info_name").String(),
			typ:      d.Get("type").String(),
		}

		if dev.name == "" || dev.typ == "" {
			c.Warningf("device info missing required fields (name: '%s', type: '%s'), skipping", dev.name, dev.typ)
			continue
		}

		if !c.deviceSr.MatchString(dev.infoName) {
			c.Debugf("device %s does not match selector, skipping it", dev.infoName)
			continue
		}

		if !scanOpen && dev.typ == "scsi" {
			// `smartctl --scan` attempts to guess the device type based on the path, but this can be unreliable.
			// Accurate device type information is crucial because we use the `--device` option to gather data.
			// Using the wrong type can lead to issues.
			// For example, using 'scsi' for 'sat' devices prevents `smartctl` from issuing the necessary ATA commands.

			c.handleGuessedScsiScannedDevice(dev)
		}

		c.Debugf("smartctl scan found device '%s' type '%s' info_name '%s'", dev.name, dev.typ, dev.infoName)

		devices[dev.key()] = dev
	}

	c.Debugf("smartctl scan found %d devices", len(devices))

	for _, v := range c.ExtraDevices {
		dev := &scanDevice{name: v.Name, typ: v.Type, extra: true}

		if _, ok := devices[dev.key()]; !ok {
			devices[dev.key()] = dev
		}
	}

	if len(devices) == 0 {
		return nil, errors.New("no devices found during scan")
	}

	return devices, nil
}

func (c *Collector) handleGuessedScsiScannedDevice(dev *scanDevice) {
	if dev.typ != "scsi" || c.hasScannedDevice(dev) {
		return
	}

	d := &scanDevice{name: dev.name, typ: "sat"}

	if c.hasScannedDevice(d) {
		dev.typ = d.typ
		return
	}

	resp, _ := c.exec.deviceInfo(dev.name, "sat", c.NoCheckPowerMode)
	if resp == nil || isExitStatusHasAnyBit(resp, 0, 1, 2) {
		return
	}

	attrs, ok := newSmartDevice(resp).ataSmartAttributeTable()
	if !ok || len(attrs) == 0 {
		return
	}

	c.Debugf("changing device '%s' type 'scsi' -> 'sat'", dev.name)
	dev.typ = "sat"
}

func (c *Collector) hasScannedDevice(d *scanDevice) bool {
	_, ok := c.scannedDevices[d.key()]
	return ok
}
