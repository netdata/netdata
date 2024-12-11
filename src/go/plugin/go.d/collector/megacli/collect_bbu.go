// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || freebsd || openbsd || netbsd || dragonfly

package megacli

import (
	"bufio"
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

type megaBBU struct {
	adapterNumber string
	batteryType   string
	temperature   string
	rsoc          string
	asoc          string // apparently can be 0 while relative > 0 (e.g. relative 91%, absolute 0%)
	cycleCount    string
	fullChargeCap string
	designCap     string
}

func (c *Collector) collectBBU(mx map[string]int64) error {
	bs, err := c.exec.bbuInfo()
	if err != nil {
		return err
	}

	bbus, err := parseBBUInfo(bs)
	if err != nil {
		return err
	}

	if len(bbus) == 0 {
		c.Debugf("no BBUs found")
		return nil
	}

	for _, bbu := range bbus {
		if !c.bbu[bbu.adapterNumber] {
			c.bbu[bbu.adapterNumber] = true
			c.addBBUCharts(bbu)
		}

		px := fmt.Sprintf("bbu_adapter_%s_", bbu.adapterNumber)

		writeInt(mx, px+"temperature", bbu.temperature)
		writeInt(mx, px+"relative_state_of_charge", bbu.rsoc)
		writeInt(mx, px+"absolute_state_of_charge", bbu.asoc)
		writeInt(mx, px+"cycle_count", bbu.cycleCount)
		if v, ok := calcCapDegradationPerc(bbu); ok {
			mx[px+"capacity_degradation_perc"] = v
		}
	}

	c.Debugf("found %d BBUs", len(c.bbu))

	return nil
}

func parseBBUInfo(bs []byte) (map[string]*megaBBU, error) {
	bbus := make(map[string]*megaBBU)

	var section string
	var bbu *megaBBU

	sc := bufio.NewScanner(bytes.NewReader(bs))

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())

		switch {
		case strings.HasPrefix(line, "BBU status for Adapter"):
			section = "status"
			ad := getColonSepValue(line)
			if _, ok := bbus[ad]; !ok {
				bbu = &megaBBU{adapterNumber: ad}
				bbus[ad] = bbu
			}
			continue
		case strings.HasPrefix(line, "BBU Capacity Info for Adapter"):
			section = "capacity"
			continue
		case strings.HasPrefix(line, "BBU Design Info for Adapter"):
			section = "design"
			continue
		case strings.HasPrefix(line, "BBU Firmware Status"),
			strings.HasPrefix(line, "BBU GasGauge Status"),
			strings.HasPrefix(line, "BBU Properties for Adapter"):
			section = ""
			continue
		}

		if bbu == nil {
			continue
		}

		switch section {
		case "status":
			switch {
			case strings.HasPrefix(line, "BatteryType:"):
				bbu.batteryType = getColonSepValue(line)
			case strings.HasPrefix(line, "Temperature:"):
				bbu.temperature = getColonSepNumValue(line)
			}
		case "capacity":
			switch {
			case strings.HasPrefix(line, "Relative State of Charge:"):
				bbu.rsoc = getColonSepNumValue(line)
			case strings.HasPrefix(line, "Absolute State of charge:"):
				bbu.asoc = getColonSepNumValue(line)
			case strings.HasPrefix(line, "Full Charge Capacity:"):
				bbu.fullChargeCap = getColonSepNumValue(line)
			case strings.HasPrefix(line, "Cycle Count:"):
				bbu.cycleCount = getColonSepNumValue(line)
			}
		case "design":
			if strings.HasPrefix(line, "Design Capacity:") {
				bbu.designCap = getColonSepNumValue(line)
			}
		}
	}

	return bbus, nil
}

func calcCapDegradationPerc(bbu *megaBBU) (int64, bool) {
	full, err := strconv.ParseInt(bbu.fullChargeCap, 10, 64)
	if err != nil || full == 0 {
		return 0, false
	}
	design, err := strconv.ParseInt(bbu.designCap, 10, 64)
	if err != nil || design == 0 {
		return 0, false
	}

	v := 100 - float64(full)/float64(design)*100

	return int64(v), true
}
