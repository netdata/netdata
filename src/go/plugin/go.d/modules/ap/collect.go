// SPDX-License-Identifier: GPL-3.0-or-later

package ap

import (
	"bufio"
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

type interfaceStats struct {
	name             string
	clients          int64
	ssid             string
	bw_received      int64
	bw_sent          int64
	packets_received int64
	packets_sent     int64
	issues_retries   int64
	issues_failures  int64
	average_signal   int64
	bitrate_receive  int64
	bitrate_transmit int64
	// bitrate_expected int64
}

func (a *AP) collect() (map[string]int64, error) {
	bs, err := a.execIWDev.list()
	if err != nil {
		return nil, err
	}

	apInterfaces := processIWDevResponse(bs)

	if len(apInterfaces) == 0 {
		return nil, fmt.Errorf("no interfaces found in AP mode'")
	}

	mx := make(map[string]int64)

	a.handleCharts(apInterfaces)

	for iface := range apInterfaces {
		bs, err = a.execStationDump.list(iface)
		if err != nil {
			return nil, err
		}

		mx["a"] = 1

		ifaceStats := processIWStationDump(bs)
		// fmt.Print(ifaceStats)
		mx[fmt.Sprintf("ap_%s_clients", iface)] = ifaceStats.clients
		mx[fmt.Sprintf("ap_%s_bw_received", iface)] = ifaceStats.bw_received
		mx[fmt.Sprintf("ap_%s_bw_sent", iface)] = ifaceStats.bw_sent
		mx[fmt.Sprintf("ap_%s_packets_received", iface)] = ifaceStats.packets_received
		mx[fmt.Sprintf("ap_%s_packets_sent", iface)] = ifaceStats.packets_sent
		mx[fmt.Sprintf("ap_%s_issues_retries", iface)] = ifaceStats.issues_retries
		mx[fmt.Sprintf("ap_%s_issues_failures", iface)] = ifaceStats.issues_failures
		mx[fmt.Sprintf("ap_%s_average_signal", iface)] = int64(ifaceStats.average_signal / ifaceStats.clients)
		mx[fmt.Sprintf("ap_%s_bitrate_receive", iface)] = int64(ifaceStats.bitrate_receive / ifaceStats.clients)
		mx[fmt.Sprintf("ap_%s_bitrate_transmit", iface)] = int64(ifaceStats.bitrate_transmit / ifaceStats.clients)
	}

	return mx, nil
}

func processIWDevResponse(response []byte) map[string]interfaceStats {
	scanner := bufio.NewScanner(bytes.NewReader(response))

	devices := make(map[string]interfaceStats)
	var iface, ssid string
	ap := false

	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "	Interface ") {
			if ap && iface != "" {
				if _, exists := devices[iface]; !exists {
					devices[iface] = interfaceStats{}
				}
				device := devices[iface]
				device.name = iface
				device.ssid = ssid
				devices[iface] = device
			}
			fields := strings.Fields(line)
			if len(fields) > 1 {
				iface = fields[1]
			}
			ssid = ""
			ap = false
		} else if strings.HasSuffix(line, "type AP") {
			ap = true
		}
	}

	if ap && iface != "" {
		if _, exists := devices[iface]; !exists {
			devices[iface] = interfaceStats{}
		}
		device := devices[iface]
		device.name = iface
		device.ssid = ssid
		devices[iface] = device
	}

	return devices
}

func processIWStationDump(response []byte) interfaceStats {
	scanner := bufio.NewScanner(bytes.NewReader(response))

	var stats interfaceStats

	clients := int64(0)

	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "Station ") {
			clients++
			stats.clients = clients
		} else if strings.HasPrefix(line, "	rx bytes:") {
			fields := strings.Fields(line)
			if v, ok := parseInt(fields[2]); ok {
				stats.bw_received += v
			}
		} else if strings.HasPrefix(line, "	rx packets:") {
			fields := strings.Fields(line)
			if v, ok := parseInt(fields[2]); ok {
				stats.packets_received += v
			}
		} else if strings.HasPrefix(line, "	tx bytes:") {
			fields := strings.Fields(line)
			if v, ok := parseInt(fields[2]); ok {
				stats.bw_sent += v
			}
		} else if strings.HasPrefix(line, "	tx packets:") {
			fields := strings.Fields(line)
			if v, ok := parseInt(fields[2]); ok {
				stats.packets_sent += v
			}
		} else if strings.HasPrefix(line, "	tx retries:") {
			fields := strings.Fields(line)
			if v, ok := parseInt(fields[2]); ok {
				stats.issues_retries += v
			}
		} else if strings.HasPrefix(line, "	tx failed:") {
			fields := strings.Fields(line)
			if v, ok := parseInt(fields[2]); ok {
				stats.issues_failures += v
			}
		} else if strings.HasPrefix(line, "	signal avg:") {
			fields := strings.Fields(line)
			if v, ok := parseInt(fields[2]); ok {
				stats.average_signal += v * 1000
			}
		} else if strings.HasPrefix(line, "	rx bitrate:") {
			fields := strings.Fields(line)
			if v, ok := parseFloat(fields[2]); ok {
				stats.bitrate_receive += int64(v * 1000)
			}
		} else if strings.HasPrefix(line, "	tx bitrate:") {
			fields := strings.Fields(line)
			if v, ok := parseFloat(fields[2]); ok {
				stats.bitrate_transmit += int64(v * 1000)
			}
		}
	}

	return stats
}

func (a *AP) handleCharts(apInterfaces map[string]interfaceStats) {
	seen := make(map[string]bool)
	for iface := range apInterfaces {
		seen[iface] = true

		if !a.interfaces[iface] {
			a.addInterfaceCharts(iface, apInterfaces[iface].ssid)
			a.interfaces[iface] = true
		}

	}

	for iface := range apInterfaces {
		if !seen[iface] {
			a.removeInterfaceCharts(iface)
			delete(a.interfaces, iface)
		}
	}
}

func parseInt(s string) (int64, bool) {
	if s == "-" {
		return 0, false
	}
	v, err := strconv.ParseInt(s, 10, 64)
	return v, err == nil
}

func parseFloat(s string) (float64, bool) {
	if s == "-" {
		return 0, false
	}
	v, err := strconv.ParseFloat(s, 64)
	return v, err == nil
}
