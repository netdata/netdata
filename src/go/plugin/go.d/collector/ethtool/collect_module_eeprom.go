// SPDX-License-Identifier: GPL-3.0-or-later

package ethtool

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const precision = 1000

type (
	moduleEeprom struct {
		ddm *moduleDdm
	}
	moduleDdm struct {
		laserBiasMA      *float64
		laserPowerMW     *float64
		laserPowerDBM    *float64
		rxSignalPowerMW  *float64
		rxSignalPowerDBM *float64
		tempC            *float64
		tempF            *float64
		voltageV         *float64
	}
)

func (c *Collector) collectModuleEeprom(mx map[string]int64, iface string) error {
	bs, err := c.exec.moduleEeprom(iface)
	if err != nil {
		return fmt.Errorf("failed to get eeprom: %w", err)
	}

	eeprom, err := parseEeprom(bs)
	if err != nil {
		return fmt.Errorf("failed to parse eeprom: %w", err)
	}
	if eeprom == nil || eeprom.ddm == nil {
		return errors.New("module doesn't have ddm")
	}

	if !c.seenOpticIfaces[iface] {
		c.seenOpticIfaces[iface] = true
		c.addModuleEepromCharts(iface, eeprom)
	}

	px := fmt.Sprintf("iface_%s_", iface)

	writeDdmValue(mx, px+"laser_bias_current_ma", eeprom.ddm.laserBiasMA)
	writeDdmValue(mx, px+"laser_output_power_mw", eeprom.ddm.laserPowerMW)
	writeDdmValue(mx, px+"laser_output_power_dbm", eeprom.ddm.laserPowerDBM)
	writeDdmValue(mx, px+"receiver_signal_average_optical_power_mw", eeprom.ddm.rxSignalPowerMW)
	writeDdmValue(mx, px+"receiver_signal_average_optical_power_dbm", eeprom.ddm.rxSignalPowerDBM)
	writeDdmValue(mx, px+"module_temperature_c", eeprom.ddm.tempC)
	writeDdmValue(mx, px+"module_temperature_f", eeprom.ddm.tempF)
	writeDdmValue(mx, px+"module_voltage_v", eeprom.ddm.voltageV)

	return nil
}

func parseEeprom(bs []byte) (*moduleEeprom, error) {
	var ddm moduleDdm
	var foundDdm bool

	sc := bufio.NewScanner(bytes.NewReader(bs))

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}

		metric, value, ok := cutTrimSpace(line, ":")
		if !ok {
			continue
		}

		var err error

		switch metric {
		case "Laser bias current":
			err = parseDdmValue(&ddm.laserBiasMA, value)
		case "Laser output power":
			err = parseCompoundDdmValues(&ddm.laserPowerMW, &ddm.laserPowerDBM, value)
		case "Receiver signal average optical power":
			err = parseCompoundDdmValues(&ddm.rxSignalPowerMW, &ddm.rxSignalPowerDBM, value)
		case "Module temperature":
			err = parseCompoundDdmValues(&ddm.tempC, &ddm.tempF, value)
		case "Module voltage":
			err = parseDdmValue(&ddm.voltageV, value)
		default:
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("failed to parse '%s': %v", line, err)
		}
		foundDdm = true
	}

	if !foundDdm {
		return nil, nil
	}
	return &moduleEeprom{ddm: &ddm}, nil
}

func writeDdmValue(mx map[string]int64, key string, v *float64) {
	if v == nil {
		return
	}
	mx[key] = int64(*v * precision)
}

func parseDdmValue(v **float64, s string) error {
	val, _, ok := cutTrimSpace(s, " ")
	if !ok {
		return errors.New("missing value")
	}
	f, err := strconv.ParseFloat(val, 64)
	if err != nil {
		return fmt.Errorf("invalid number '%s': %v", val, err)
	}
	*v = &f
	return nil
}

func parseCompoundDdmValues(v1, v2 **float64, s string) error {
	val1, val2, ok := cutTrimSpace(s, "/")
	if !ok {
		return errors.New("missing compound values")
	}
	if err := parseDdmValue(v1, val1); err != nil {
		return err
	}
	if err := parseDdmValue(v2, val2); err != nil {
		return err
	}
	return nil
}

func cutTrimSpace(s string, sep string) (string, string, bool) {
	b, a, ok := strings.Cut(s, sep)
	return strings.TrimSpace(b), strings.TrimSpace(a), ok
}
