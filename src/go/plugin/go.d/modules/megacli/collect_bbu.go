// SPDX-License-Identifier: GPL-3.0-or-later

package megacli

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"
)

type megaBBU struct {
	adapterNumber         string
	batteryType           string
	temperature           string
	relativeStateOfCharge string
	absoluteStateOfCharge string // apparently can be 0 while relative > 0 (e.g. relative 91%, absolute 0%)
	cycleCount            string
}

func (m *MegaCli) collectBBU(mx map[string]int64) error {
	bs, err := m.exec.bbuInfo()
	if err != nil {
		return err
	}

	bbus, err := parseBBUInfo(bs)
	if err != nil {
		return err
	}

	if len(bbus) == 0 {
		m.Debugf("no BBUs found")
		return nil
	}

	for _, bbu := range bbus {
		if !m.bbu[bbu.adapterNumber] {
			m.bbu[bbu.adapterNumber] = true
			m.addBBUCharts(bbu)
		}

		px := fmt.Sprintf("bbu_adapter_%s_", bbu.adapterNumber)

		writeInt(mx, px+"temperature", bbu.temperature)
		writeInt(mx, px+"relative_state_of_charge", bbu.relativeStateOfCharge)
		writeInt(mx, px+"absolute_state_of_charge", bbu.absoluteStateOfCharge)
		writeInt(mx, px+"cycle_count", bbu.cycleCount)
	}

	m.Debugf("found %d BBUs", len(m.bbu))

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
		case strings.HasPrefix(line, "BBU Firmware Status"),
			strings.HasPrefix(line, "BBU GasGauge Status"),
			strings.HasPrefix(line, "BBU Design Info for Adapter"),
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
				bbu.relativeStateOfCharge = getColonSepNumValue(line)
			case strings.HasPrefix(line, "Absolute State of charge:"):
				bbu.absoluteStateOfCharge = getColonSepNumValue(line)
			case strings.HasPrefix(line, "Cycle Count:"):
				bbu.cycleCount = getColonSepNumValue(line)
			}
		}
	}

	return bbus, nil
}
