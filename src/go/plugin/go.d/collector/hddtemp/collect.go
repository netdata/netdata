// SPDX-License-Identifier: GPL-3.0-or-later

package hddtemp

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

type diskStats struct {
	devPath     string
	model       string
	temperature string
	unit        string
}

func (h *HddTemp) collect() (map[string]int64, error) {
	msg, err := h.conn.queryHddTemp()
	if err != nil {
		return nil, err
	}

	h.Debugf("hddtemp daemon response: %s", msg)

	disks, err := parseHddTempMessage(msg)
	if err != nil {
		return nil, err
	}

	mx := make(map[string]int64)

	for _, disk := range disks {
		id := getDiskID(disk)
		if id == "" {
			h.Debugf("can not extract disk id from '%s'", disk.devPath)
			continue
		}

		if !h.disks[id] {
			h.disks[id] = true
			h.addDiskTempSensorStatusChart(id, disk)
		}

		px := fmt.Sprintf("disk_%s_", id)

		for _, st := range []string{"ok", "na", "unk", "nos", "slp", "err"} {
			mx[px+"temp_sensor_status_"+st] = 0
		}
		switch disk.temperature {
		case "NA":
			mx[px+"temp_sensor_status_na"] = 1
		case "UNK":
			mx[px+"temp_sensor_status_unk"] = 1
		case "NOS":
			mx[px+"temp_sensor_status_nos"] = 1
		case "SLP":
			mx[px+"temp_sensor_status_slp"] = 1
		case "ERR":
			mx[px+"temp_sensor_status_err"] = 1
		default:
			if v, ok := getTemperature(disk); ok {
				if !h.disksTemp[id] {
					h.disksTemp[id] = true
					h.addDiskTempChart(id, disk)
				}
				mx[px+"temp_sensor_status_ok"] = 1
				mx[px+"temperature"] = v
			} else {
				mx[px+"temp_sensor_status_unk"] = 1
			}
		}
	}

	return mx, nil
}

func getDiskID(d diskStats) string {
	i := strings.LastIndexByte(d.devPath, '/')
	if i == -1 {
		return ""
	}
	return d.devPath[i+1:]
}

func getTemperature(d diskStats) (int64, bool) {
	v, err := strconv.ParseInt(d.temperature, 10, 64)
	if err != nil {
		return 0, false
	}
	if d.unit == "F" {
		v = (v - 32) * 5 / 9
	}
	return v, true
}

func parseHddTempMessage(msg string) ([]diskStats, error) {
	if msg == "" {
		return nil, errors.New("empty hddtemp message")
	}

	// https://github.com/guzu/hddtemp/blob/e16aed6d0145d7ad8b3308dd0b9199fc701c0417/src/daemon.c#L165
	parts := strings.Split(msg, "|")

	var i int
	// remove empty values
	for _, v := range parts {
		if v = strings.TrimSpace(v); v != "" {
			parts[i] = v
			i++
		}
	}
	parts = parts[:i]

	if len(parts) == 0 || len(parts)%4 != 0 {
		return nil, errors.New("invalid hddtemp output format")
	}

	var disks []diskStats

	for i := 0; i < len(parts); i += 4 {
		disks = append(disks, diskStats{
			devPath:     parts[i],
			model:       parts[i+1],
			temperature: parts[i+2],
			unit:        parts[i+3],
		})
	}

	return disks, nil
}
