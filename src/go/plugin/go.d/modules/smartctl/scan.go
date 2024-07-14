// SPDX-License-Identifier: GPL-3.0-or-later

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

func (s *Smartctl) scanDevices() (map[string]*scanDevice, error) {
	resp, err := s.exec.scan()
	if err != nil {
		return nil, fmt.Errorf("failed to scan devices: %v", err)
	}

	devices := make(map[string]*scanDevice)

	for _, d := range resp.Get("devices").Array() {
		dev := &scanDevice{
			name:     d.Get("name").String(),
			infoName: d.Get("info_name").String(),
			typ:      d.Get("type").String(), // guessed type (we do '--scan' not '--scan-open')
		}

		if dev.name == "" || dev.typ == "" {
			s.Warningf("device info missing required fields (name: '%s', type: '%s'), skipping", dev.name, dev.typ)
			continue
		}

		if !s.deviceSr.MatchString(dev.infoName) {
			s.Debugf("device %s does not match selector, skipping it", dev.infoName)
			continue
		}

		if dev.typ == "scsi" {
			// `smartctl --scan` attempts to guess the device type based on the path, but this can be unreliable.
			// Accurate device type information is crucial because we use the `--device` option to gather data.
			// Using the wrong type can lead to issues.
			// For example, using 'scsi' for 'sat' devices prevents `smartctl` from issuing the necessary ATA commands.
			d := scanDevice{name: dev.name, typ: "sat"}
			if _, ok := s.scannedDevices[d.key()]; ok {
				dev.typ = "sat"
			} else {
				resp, _ := s.exec.deviceInfo(dev.name, dev.typ, s.NoCheckPowerMode)
				if resp != nil && isExitStatusHasBit(resp, 2) {
					s.Debugf("changing device '%s' type 'scsi' -> 'sat'", dev.name)
					dev.typ = "sat"
				}
			}
		}

		s.Debugf("smartctl scan found device '%s' type '%s' info_name '%s'", dev.name, dev.typ, dev.infoName)

		devices[dev.key()] = dev
	}

	s.Debugf("smartctl scan found %d devices", len(devices))

	for _, v := range s.ExtraDevices {
		if v.Name == "" || v.Type == "" {
			continue
		}
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
