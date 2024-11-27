// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package ap

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const precision = 1000

type iwInterface struct {
	name string
	ssid string
	typ  string
}

type stationStats struct {
	clients   int64
	rxBytes   int64
	rxPackets int64
	txBytes   int64
	txPackets int64
	txRetries int64
	txFailed  int64
	signalAvg int64
	txBitrate float64
	rxBitrate float64
}

func (c *Collector) collect() (map[string]int64, error) {
	bs, err := c.exec.devices()
	if err != nil {
		return nil, err
	}

	// TODO: call this periodically, not on every data collection
	apInterfaces, err := parseIwDevices(bs)
	if err != nil {
		return nil, fmt.Errorf("parsing AP interfaces: %v", err)
	}

	if len(apInterfaces) == 0 {
		return nil, errors.New("no type AP interfaces found")
	}

	mx := make(map[string]int64)
	seen := make(map[string]bool)

	for _, iface := range apInterfaces {
		bs, err = c.exec.stationStatistics(iface.name)
		if err != nil {
			return nil, fmt.Errorf("getting station statistics for %s: %v", iface, err)
		}

		stats, err := parseIwStationStatistics(bs)
		if err != nil {
			return nil, fmt.Errorf("parsing station statistics for %s: %v", iface, err)
		}

		key := fmt.Sprintf("%s-%s", iface.name, iface.ssid)

		seen[key] = true

		if _, ok := c.seenIfaces[key]; !ok {
			c.seenIfaces[key] = iface
			c.addInterfaceCharts(iface)
		}

		px := fmt.Sprintf("ap_%s_%s_", iface.name, iface.ssid)

		mx[px+"clients"] = stats.clients
		mx[px+"bw_received"] = stats.rxBytes
		mx[px+"bw_sent"] = stats.txBytes
		mx[px+"packets_received"] = stats.rxPackets
		mx[px+"packets_sent"] = stats.txPackets
		mx[px+"issues_retries"] = stats.txRetries
		mx[px+"issues_failures"] = stats.txFailed
		mx[px+"average_signal"], mx[px+"bitrate_receive"], mx[px+"bitrate_transmit"] = 0, 0, 0
		if clients := float64(stats.clients); clients > 0 {
			mx[px+"average_signal"] = int64(float64(stats.signalAvg) / clients * precision)
			mx[px+"bitrate_receive"] = int64(stats.rxBitrate / clients * precision)
			mx[px+"bitrate_transmit"] = int64(stats.txBitrate / clients * precision)
		}
	}

	for key, iface := range c.seenIfaces {
		if !seen[key] {
			delete(c.seenIfaces, key)
			c.removeInterfaceCharts(iface)
		}
	}

	return mx, nil
}

func parseIwDevices(resp []byte) ([]*iwInterface, error) {
	ifaces := make(map[string]*iwInterface)
	var iface *iwInterface

	sc := bufio.NewScanner(bytes.NewReader(resp))

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())

		switch {
		case strings.HasPrefix(line, "Interface"):
			parts := strings.Fields(line)
			if len(parts) != 2 {
				return nil, fmt.Errorf("invalid interface line: '%s'", line)
			}
			name := parts[1]
			if _, ok := ifaces[name]; !ok {
				iface = &iwInterface{name: name}
				ifaces[name] = iface
			}
		case strings.HasPrefix(line, "ssid") && iface != nil:
			parts := strings.Fields(line)
			if len(parts) != 2 {
				return nil, fmt.Errorf("invalid ssid line: '%s'", line)
			}
			iface.ssid = parts[1]
		case strings.HasPrefix(line, "type") && iface != nil:
			parts := strings.Fields(line)
			if len(parts) != 2 {
				return nil, fmt.Errorf("invalid type line: '%s'", line)
			}
			iface.typ = parts[1]
		}
	}

	var apIfaces []*iwInterface

	for _, iface := range ifaces {
		if strings.ToLower(iface.typ) == "ap" {
			apIfaces = append(apIfaces, iface)
		}
	}

	return apIfaces, nil
}

func parseIwStationStatistics(resp []byte) (*stationStats, error) {
	var stats stationStats

	sc := bufio.NewScanner(bytes.NewReader(resp))

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())

		var v float64
		var err error

		switch {
		case strings.HasPrefix(line, "Station"):
			stats.clients++
		case strings.HasPrefix(line, "rx bytes:"):
			if v, err = get3rdValue(line); err == nil {
				stats.rxBytes += int64(v)
			}
		case strings.HasPrefix(line, "rx packets:"):
			if v, err = get3rdValue(line); err == nil {
				stats.rxPackets += int64(v)
			}
		case strings.HasPrefix(line, "tx bytes:"):
			if v, err = get3rdValue(line); err == nil {
				stats.txBytes += int64(v)
			}
		case strings.HasPrefix(line, "tx packets:"):
			if v, err = get3rdValue(line); err == nil {
				stats.txPackets += int64(v)
			}
		case strings.HasPrefix(line, "tx retries:"):
			if v, err = get3rdValue(line); err == nil {
				stats.txRetries += int64(v)
			}
		case strings.HasPrefix(line, "tx failed:"):
			if v, err = get3rdValue(line); err == nil {
				stats.txFailed += int64(v)
			}
		case strings.HasPrefix(line, "signal avg:"):
			if v, err = get3rdValue(line); err == nil {
				stats.signalAvg += int64(v)
			}
		case strings.HasPrefix(line, "tx bitrate:"):
			if v, err = get3rdValue(line); err == nil {
				stats.txBitrate += v
			}
		case strings.HasPrefix(line, "rx bitrate:"):
			if v, err = get3rdValue(line); err == nil {
				stats.rxBitrate += v
			}
		default:
			continue
		}

		if err != nil {
			return nil, fmt.Errorf("parsing line '%s': %v", line, err)
		}
	}

	return &stats, nil
}

func get3rdValue(line string) (float64, error) {
	parts := strings.Fields(line)
	if len(parts) < 3 {
		return 0.0, errors.New("invalid format")
	}

	v := parts[2]

	if v == "-" {
		return 0.0, nil
	}
	return strconv.ParseFloat(v, 64)
}
