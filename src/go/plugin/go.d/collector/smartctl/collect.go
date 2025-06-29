// SPDX-License-Identifier: GPL-3.0-or-later

package smartctl

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/sourcegraph/conc/pool"
	"github.com/tidwall/gjson"
)

func (c *Collector) collect() (map[string]int64, error) {
	now := time.Now()

	if c.forceScan || c.isTimeToScan(now) {
		devices, err := c.scanDevices()
		if err != nil {
			return nil, err
		}

		for k, dev := range c.scannedDevices {
			if _, ok := devices[k]; !ok {
				delete(c.scannedDevices, k)
				delete(c.seenDevices, k)
				c.removeDeviceCharts(dev)
			}
		}

		c.forceDevicePoll = !maps.Equal(c.scannedDevices, devices)
		c.scannedDevices = devices
		c.lastScanTime = now
		c.forceScan = false
	}

	if c.forceDevicePoll || c.isTimeToPollDevices(now) {
		mx := make(map[string]int64)

		c.collectDevices(mx)

		c.forceDevicePoll = false
		c.lastDevicePollTime = now
		c.mx = mx
	}

	return c.mx, nil
}
func (c *Collector) collectDevices(mx map[string]int64) {
	if c.ConcurrentScans > 0 && len(c.scannedDevices) > 1 {
		if err := c.collectDevicesConcurrently(mx); err != nil {
			c.Warning(err)
		}
		return
	}

	for _, d := range c.scannedDevices {
		if err := c.collectScannedDevice(mx, d); err != nil {
			c.Warning(err)
			continue
		}
	}
}

type deviceInfoResult struct {
	scanDevice *scanDevice
	response   *gjson.Result
	err        error
}

func (c *Collector) collectDevicesConcurrently(mx map[string]int64) error {
	p := pool.New().WithMaxGoroutines(c.ConcurrentScans)
	resultsChan := make(chan deviceInfoResult, len(c.scannedDevices))

	for _, dev := range c.scannedDevices {
		dev := dev
		p.Go(func() {
			resp, err := c.exec.deviceInfo(dev.name, dev.typ, c.NoCheckPowerMode)
			resultsChan <- deviceInfoResult{
				scanDevice: dev,
				response:   resp,
				err:        err,
			}
		})
	}

	p.Wait()
	close(resultsChan)

	for r := range resultsChan {
		if err := c.processDeviceResult(mx, r); err != nil {
			c.Warning(err)
			continue
		}
	}

	return nil
}

func (c *Collector) processDeviceResult(mx map[string]int64, result deviceInfoResult) error {
	scanDev := result.scanDevice
	resp := result.response
	err := result.err

	if err != nil {
		if resp != nil && isDeviceOpenFailedNoSuchDevice(resp) && !scanDev.extra {
			c.Infof("smartctl reported that device '%s' type '%s' no longer exists", scanDev.name, scanDev.typ)
			c.forceScan = true
			return nil
		}
		return fmt.Errorf("failed to get device info for '%s' type '%s': %v", scanDev.name, scanDev.typ, err)
	}

	if isDeviceInLowerPowerMode(resp) {
		c.Debugf("device '%s' type '%s' is in a low-power mode, skipping", scanDev.name, scanDev.typ)
		return nil
	}

	dev := newSmartDevice(resp)
	if !isSmartDeviceValid(dev) {
		return nil
	}

	if !c.seenDevices[scanDev.key()] {
		c.seenDevices[scanDev.key()] = true
		c.addDeviceCharts(dev)
	}

	c.collectSmartDevice(mx, dev)

	return nil
}

func (c *Collector) collectScannedDevice(mx map[string]int64, scanDev *scanDevice) error {
	resp, err := c.exec.deviceInfo(scanDev.name, scanDev.typ, c.NoCheckPowerMode)
	return c.processDeviceResult(mx, deviceInfoResult{
		scanDevice: scanDev,
		response:   resp,
		err:        err,
	})
}

func (c *Collector) collectSmartDevice(mx map[string]int64, dev *smartDevice) {
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

	if dev.deviceType() == "scsi" {
		sel := dev.data.Get("scsi_error_counter_log")
		if !sel.Exists() {
			return
		}

		for _, v := range []string{"read", "write", "verify"} {
			for _, n := range []string{
				//"errors_corrected_by_eccdelayed",
				//"errors_corrected_by_eccfast",
				//"errors_corrected_by_rereads_rewrites",
				"total_errors_corrected",
				"total_uncorrected_errors",
			} {
				key := fmt.Sprintf("%sscsi_error_log_%s_%s", px, v, n)
				metric := fmt.Sprintf("%s.%s", v, n)

				if m := sel.Get(metric); m.Exists() {
					mx[key] = m.Int()
				}
			}
		}
	}
}

func (c *Collector) isTimeToScan(now time.Time) bool {
	return c.ScanEvery.Duration().Seconds() != 0 && now.After(c.lastScanTime.Add(c.ScanEvery.Duration()))
}

func (c *Collector) isTimeToPollDevices(now time.Time) bool {
	return now.After(c.lastDevicePollTime.Add(c.PollDevicesEvery.Duration()))

}

func isSmartDeviceValid(d *smartDevice) bool {
	return d.deviceName() != "" && d.deviceType() != ""
}

func isSmartAttrValid(a *smartAttribute) bool {
	return a.id() != "" && a.name() != ""
}

func isDeviceInLowerPowerMode(r *gjson.Result) bool {
	if !isExitStatusHasAnyBit(r, 1) {
		return false
	}

	messages := r.Get("smartctl.messages").Array()

	return slices.ContainsFunc(messages, func(msg gjson.Result) bool {
		text := msg.Get("string").String()
		return strings.HasPrefix(text, "Device is in") && strings.Contains(text, "mode")
	})
}

func isDeviceOpenFailedNoSuchDevice(r *gjson.Result) bool {
	if !isExitStatusHasAnyBit(r, 1) {
		return false
	}

	messages := r.Get("smartctl.messages").Array()

	return slices.ContainsFunc(messages, func(msg gjson.Result) bool {
		text := msg.Get("string").String()
		return strings.HasSuffix(text, "No such device")
	})
}

func isExitStatusHasAnyBit(r *gjson.Result, bit int, bits ...int) bool {
	// https://manpages.debian.org/bullseye/smartmontools/smartctl.8.en.html#EXIT_STATUS
	status := int(r.Get("smartctl.exit_status").Int())

	for _, b := range append([]int{bit}, bits...) {
		mask := 1 << b
		if (status & mask) != 0 {
			return true
		}
	}

	return false
}
