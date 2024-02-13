// SPDX-License-Identifier: GPL-3.0-or-later

package upsd

import (
	"errors"
	"strconv"
	"strings"
)

func (u *Upsd) collect() (map[string]int64, error) {
	if u.conn == nil {
		conn, err := u.establishConnection()
		if err != nil {
			return nil, err
		}
		u.conn = conn
	}

	upsUnits, err := u.conn.upsUnits()
	if err != nil {
		if !errors.Is(err, errUpsdCommand) {
			_ = u.conn.disconnect()
			u.conn = nil
		}
		return nil, err
	}

	u.Debugf("found %d UPS units", len(upsUnits))

	mx := make(map[string]int64)

	u.collectUPSUnits(mx, upsUnits)

	return mx, nil
}

func (u *Upsd) establishConnection() (upsdConn, error) {
	conn := u.newUpsdConn(u.Config)

	if err := conn.connect(); err != nil {
		return nil, err
	}

	if u.Username != "" && u.Password != "" {
		if err := conn.authenticate(u.Username, u.Password); err != nil {
			_ = conn.disconnect()
			return nil, err
		}
	}

	return conn, nil
}

func (u *Upsd) collectUPSUnits(mx map[string]int64, upsUnits []upsUnit) {
	seen := make(map[string]bool)

	for _, ups := range upsUnits {
		seen[ups.name] = true
		u.Debugf("collecting metrics UPS '%s'", ups.name)

		if !u.upsUnits[ups.name] {
			u.upsUnits[ups.name] = true
			u.addUPSCharts(ups)
		}

		writeVar(mx, ups, varBatteryCharge)
		writeVar(mx, ups, varBatteryRuntime)
		writeVar(mx, ups, varBatteryVoltage)
		writeVar(mx, ups, varBatteryVoltageNominal)

		writeVar(mx, ups, varInputVoltage)
		writeVar(mx, ups, varInputVoltageNominal)
		writeVar(mx, ups, varInputCurrent)
		writeVar(mx, ups, varInputCurrentNominal)
		writeVar(mx, ups, varInputFrequency)
		writeVar(mx, ups, varInputFrequencyNominal)

		writeVar(mx, ups, varOutputVoltage)
		writeVar(mx, ups, varOutputVoltageNominal)
		writeVar(mx, ups, varOutputCurrent)
		writeVar(mx, ups, varOutputCurrentNominal)
		writeVar(mx, ups, varOutputFrequency)
		writeVar(mx, ups, varOutputFrequencyNominal)

		writeVar(mx, ups, varUpsLoad)
		writeVar(mx, ups, varUpsRealPowerNominal)
		writeVar(mx, ups, varUpsTemperature)
		writeUpsLoadUsage(mx, ups)
		writeUpsStatus(mx, ups)
	}

	for name := range u.upsUnits {
		if !seen[name] {
			delete(u.upsUnits, name)
			u.removeUPSCharts(name)
		}
	}
}

func writeVar(mx map[string]int64, ups upsUnit, v string) {
	s, ok := ups.vars[v]
	if !ok {
		return
	}
	n, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return
	}
	mx[prefix(ups)+v] = int64(n * varPrecision)
}

func writeUpsLoadUsage(mx map[string]int64, ups upsUnit) {
	if hasVar(ups.vars, varUpsRealPower) {
		pow, _ := strconv.ParseFloat(ups.vars[varUpsRealPower], 64)
		mx[prefix(ups)+"ups.load.usage"] = int64(pow * varPrecision)
		return
	}

	if !hasVar(ups.vars, varUpsLoad) || !hasVar(ups.vars, varUpsRealPowerNominal) {
		return
	}
	load, err := strconv.ParseFloat(ups.vars[varUpsLoad], 64)
	if err != nil {
		return
	}
	nomPower, err := strconv.ParseFloat(ups.vars[varUpsRealPowerNominal], 64)
	if err != nil || nomPower == 0 {
		return
	}
	mx[prefix(ups)+"ups.load.usage"] = int64((load / 100 * nomPower) * varPrecision)
}

// https://networkupstools.org/docs/developer-guide.chunked/ar01s04.html#_status_data
var upsStatuses = map[string]bool{
	"OL":      true,
	"OB":      true,
	"LB":      true,
	"HB":      true,
	"RB":      true,
	"CHRG":    true,
	"DISCHRG": true,
	"BYPASS":  true,
	"CAL":     true,
	"OFF":     true,
	"OVER":    true,
	"TRIM":    true,
	"BOOST":   true,
	"FSD":     true,
}

func writeUpsStatus(mx map[string]int64, ups upsUnit) {
	if !hasVar(ups.vars, varUpsStatus) {
		return
	}

	px := prefix(ups) + "ups.status."

	for st := range upsStatuses {
		mx[px+st] = 0
	}
	mx[px+"other"] = 0

	for _, st := range strings.Split(ups.vars[varUpsStatus], " ") {
		if _, ok := upsStatuses[st]; ok {
			mx[px+st] = 1
		} else {
			mx[px+"other"] = 1
		}
	}
}

func hasVar(vars map[string]string, v string) bool {
	_, ok := vars[v]
	return ok
}

func prefix(ups upsUnit) string {
	return "ups_" + ups.name + "_"
}
