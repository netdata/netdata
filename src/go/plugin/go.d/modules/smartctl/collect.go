// SPDX-License-Identifier: GPL-3.0-or-later

package smartctl

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/tidwall/gjson"
)

func (s *Smartctl) collect() (map[string]int64, error) {
	now := time.Now()

	if s.forceScan || s.isTimeToScan(now) {
		devices, err := s.scanDevices()
		if err != nil {
			return nil, err
		}

		for k, dev := range s.scannedDevices {
			if _, ok := devices[k]; !ok {
				delete(s.scannedDevices, k)
				delete(s.seenDevices, k)
				s.removeDeviceCharts(dev)
			}
		}

		s.forceDevicePoll = !maps.Equal(s.scannedDevices, devices)
		s.scannedDevices = devices
		s.lastScanTime = now
		s.forceScan = false
	}

	if s.forceDevicePoll || s.isTimeToPollDevices(now) {
		mx := make(map[string]int64)

		// TODO: make it concurrent
		for _, d := range s.scannedDevices {
			if err := s.collectScannedDevice(mx, d); err != nil {
				return nil, err
			}
		}

		s.forceDevicePoll = false
		s.lastDevicePollTime = now
		s.mx = mx
	}

	return s.mx, nil
}

func (s *Smartctl) collectScannedDevice(mx map[string]int64, scanDev *scanDevice) error {
	resp, err := s.exec.deviceInfo(scanDev.name, scanDev.typ, s.NoCheckPowerMode)
	if err != nil {
		if resp != nil && isDeviceOpenFailedNoSuchDevice(resp) {
			s.Infof("smartctl reported that device '%s' type '%s' no longer exists", scanDev.name, scanDev.typ)
			s.forceScan = true
			return nil
		}
		return fmt.Errorf("failed to get device info for '%s' type '%s': %v", scanDev.name, scanDev.typ, err)
	}

	if isDeviceInLowerPowerMode(resp) {
		s.Debugf("device '%s' type '%s' is in a low-power mode, skipping", scanDev.name, scanDev.typ)
		return nil
	}

	dev := newSmartDevice(resp)
	if !isSmartDeviceValid(dev) {
		return nil
	}

	if !s.seenDevices[scanDev.key()] {
		s.seenDevices[scanDev.key()] = true
		s.addDeviceCharts(dev)
	}

	s.collectSmartDevice(mx, dev)

	return nil
}

func (s *Smartctl) collectSmartDevice(mx map[string]int64, dev *smartDevice) {
	px := fmt.Sprintf("device_%s_type_%s_", dev.deviceName(), dev.deviceType())

	if v, ok := dev.powerOnTime(); ok {
		mx[px+"power_on_time"] = v
	}
	if v, ok := dev.temperature(); ok {
		mx[px+"temperature"] = v
	}
	if v, ok := dev.powerCycleCount(); ok {
		mx[px+"power_cycle_count"] = v
	}
	if v, ok := dev.smartStatusPassed(); ok {
		mx[px+"smart_status_passed"] = 0
		mx[px+"smart_status_failed"] = 0
		if v {
			mx[px+"smart_status_passed"] = 1
		} else {
			mx[px+"smart_status_failed"] = 1
		}
	}
	if v, ok := dev.ataSmartErrorLogCount(); ok {
		mx[px+"ata_smart_error_log_summary_count"] = v
	}

	if attrs, ok := dev.ataSmartAttributeTable(); ok {
		for _, attr := range attrs {
			if !isSmartAttrValid(attr) {
				continue
			}
			n := strings.ToLower(attr.name())
			n = strings.ReplaceAll(n, " ", "_")
			px := fmt.Sprintf("%sattr_%s_", px, n)

			if v, err := strconv.ParseInt(attr.value(), 10, 64); err == nil {
				mx[px+"normalized"] = v
			}

			if v, err := strconv.ParseInt(attr.rawValue(), 10, 64); err == nil {
				mx[px+"raw"] = v
			}

			rs := strings.TrimSpace(attr.rawString())
			if i := strings.IndexByte(rs, ' '); i != -1 {
				rs = rs[:i]
			}
			if v, err := strconv.ParseInt(rs, 10, 64); err == nil {
				mx[px+"decoded"] = v
			}
		}
	}
}

func (s *Smartctl) isTimeToScan(now time.Time) bool {
	return now.After(s.lastScanTime.Add(s.ScanEvery.Duration()))
}

func (s *Smartctl) isTimeToPollDevices(now time.Time) bool {
	return now.After(s.lastDevicePollTime.Add(s.PollDevicesEvery.Duration()))

}

func isSmartDeviceValid(d *smartDevice) bool {
	return d.deviceName() != "" && d.deviceType() != ""
}

func isSmartAttrValid(a *smartAttribute) bool {
	return a.id() != "" && a.name() != ""
}

func isDeviceInLowerPowerMode(r *gjson.Result) bool {
	if !isExitStatusHasBit(r, 1) {
		return false
	}

	messages := r.Get("smartctl.messages").Array()

	return slices.ContainsFunc(messages, func(msg gjson.Result) bool {
		text := msg.Get("string").String()
		return strings.HasPrefix(text, "Device is in") && strings.Contains(text, "mode")
	})
}

func isDeviceOpenFailedNoSuchDevice(r *gjson.Result) bool {
	if !isExitStatusHasBit(r, 1) {
		return false
	}

	messages := r.Get("smartctl.messages").Array()

	return slices.ContainsFunc(messages, func(msg gjson.Result) bool {
		text := msg.Get("string").String()
		return strings.HasSuffix(text, "No such device")
	})
}

func isExitStatusHasBit(r *gjson.Result, bit int) bool {
	// https://manpages.debian.org/bullseye/smartmontools/smartctl.8.en.html#EXIT_STATUS
	status := int(r.Get("smartctl.exit_status").Int())
	mask := 1 << bit
	return (status & mask) != 0
}
