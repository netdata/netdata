// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || freebsd || openbsd || netbsd || dragonfly

package adaptecraid

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

type physicalDevice struct {
	number        string
	state         string
	location      string
	vendor        string
	model         string
	smart         string
	smartWarnings string
	powerState    string
	temperature   string
}

func (c *Collector) collectPhysicalDevices(mx map[string]int64) error {
	bs, err := c.exec.physicalDevicesInfo()
	if err != nil {
		return err
	}

	devices, err := parsePhysDevInfo(bs)
	if err != nil {
		return err
	}

	if len(devices) == 0 {
		return errors.New("no physical devices found")
	}

	for _, pd := range devices {
		if !c.pds[pd.number] {
			c.pds[pd.number] = true
			c.addPhysicalDeviceCharts(pd)
		}

		px := fmt.Sprintf("pd_%s_", pd.number)

		// Unfortunately, all available states are unknown.
		mx[px+"health_state_ok"] = 0
		mx[px+"health_state_critical"] = 0
		if isOkPDState(pd) {
			mx[px+"health_state_ok"] = 1
		} else {
			mx[px+"health_state_critical"] = 1
		}

		if v, err := strconv.ParseInt(pd.smartWarnings, 10, 64); err == nil {
			mx[px+"smart_warnings"] = v
		}
		if v, err := strconv.ParseInt(pd.temperature, 10, 64); err == nil {
			mx[px+"temperature"] = v
		}
	}

	return nil
}

func isOkPDState(pd *physicalDevice) bool {
	// https://github.com/thomas-krenn/check_adaptec_raid/blob/a104fd88deede87df4f07403b44394bffb30c5c3/check_adaptec_raid#L455
	switch pd.state {
	case "Online",
		"Global Hot-Spare",
		"Dedicated Hot-Spare",
		"Pooled Hot-Spare",
		"Hot Spare",
		"Ready",
		"Online (JBOD)",
		"Raw (Pass Through)":
		return true
	}
	return false
}

func parsePhysDevInfo(bs []byte) (map[string]*physicalDevice, error) {
	devices := make(map[string]*physicalDevice)

	var pd *physicalDevice

	sc := bufio.NewScanner(bytes.NewReader(bs))

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())

		if strings.HasPrefix(line, "Device #") {
			num := strings.TrimPrefix(line, "Device #")
			pd = &physicalDevice{number: num}
			devices[num] = pd
			continue
		}

		if pd == nil {
			continue
		}

		switch {
		case strings.HasPrefix(line, "State"):
			pd.state = getColonSepValue(line)
		case strings.HasPrefix(line, "Reported Location"):
			pd.location = getColonSepValue(line)
		case strings.HasPrefix(line, "Vendor"):
			pd.vendor = getColonSepValue(line)
		case strings.HasPrefix(line, "Model"):
			pd.model = getColonSepValue(line)
		case strings.HasPrefix(line, "S.M.A.R.T. warnings"):
			pd.smartWarnings = getColonSepValue(line)
		case strings.HasPrefix(line, "S.M.A.R.T."):
			pd.smart = getColonSepValue(line)
		case strings.HasPrefix(line, "Power State"):
			pd.powerState = getColonSepValue(line)
		case strings.HasPrefix(line, "Temperature"):
			v := getColonSepValue(line) // '42 C/ 107 F' or 'Not Supported'
			pd.temperature = strings.Fields(v)[0]
		}
	}

	return devices, nil
}
