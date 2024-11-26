// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || freebsd || openbsd || netbsd || dragonfly

package adaptecraid

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"strings"
)

type logicalDevice struct {
	number        string
	name          string
	raidLevel     string
	status        string
	failedStripes string
}

func (c *Collector) collectLogicalDevices(mx map[string]int64) error {
	bs, err := c.exec.logicalDevicesInfo()
	if err != nil {
		return err
	}

	devices, err := parseLogicDevInfo(bs)
	if err != nil {
		return err
	}

	if len(devices) == 0 {
		return errors.New("no logical devices found")
	}

	for _, ld := range devices {
		if !c.lds[ld.number] {
			c.lds[ld.number] = true
			c.addLogicalDeviceCharts(ld)
		}

		px := fmt.Sprintf("ld_%s_", ld.number)

		// Unfortunately, all available states are unknown.
		mx[px+"health_state_ok"] = 0
		mx[px+"health_state_critical"] = 0
		if isOkLDStatus(ld) {
			mx[px+"health_state_ok"] = 1
		} else {
			mx[px+"health_state_critical"] = 1
		}
	}

	return nil
}

func isOkLDStatus(ld *logicalDevice) bool {
	// https://github.com/thomas-krenn/check_adaptec_raid/blob/a104fd88deede87df4f07403b44394bffb30c5c3/check_adaptec_raid#L340
	return ld.status == "Optimal"
}

func parseLogicDevInfo(bs []byte) (map[string]*logicalDevice, error) {
	devices := make(map[string]*logicalDevice)

	var ld *logicalDevice

	sc := bufio.NewScanner(bytes.NewReader(bs))

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())

		if strings.HasPrefix(line, "Logical device number") ||
			strings.HasPrefix(line, "Logical Device number") {
			parts := strings.Fields(line)
			num := parts[len(parts)-1]
			ld = &logicalDevice{number: num}
			devices[num] = ld
			continue
		}

		if ld == nil {
			continue
		}

		switch {
		case strings.HasPrefix(line, "Logical device name"),
			strings.HasPrefix(line, "Logical Device name"):
			ld.name = getColonSepValue(line)
		case strings.HasPrefix(line, "RAID level"):
			ld.raidLevel = getColonSepValue(line)
		case strings.HasPrefix(line, "Status of logical device"),
			strings.HasPrefix(line, "Status of Logical Device"):
			ld.status = getColonSepValue(line)
		case strings.HasPrefix(line, "Failed stripes"):
			ld.failedStripes = getColonSepValue(line)
		}
	}

	return devices, nil
}
