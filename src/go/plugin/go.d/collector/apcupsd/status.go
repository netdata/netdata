// SPDX-License-Identifier: GPL-3.0-or-later

package apcupsd

import (
	"bufio"
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

var upsStatuses = []string{
	"CAL",
	"TRIM",
	"BOOST",
	"ONLINE",
	"ONBATT",
	"OVERLOAD",
	"LOWBATT",
	"REPLACEBATT",
	"NOBATT",
	"SLAVE",
	"SLAVEDOWN",
	"COMMLOST",
	"SHUTTING_DOWN",
}

var upsSelftestStatuses = []string{
	"NO",
	"NG",
	"WN",
	"IP",
	"OK",
	"BT",
	"UNK",
}

// examples: https://github.com/therealbstern/apcupsd/tree/master/examples/status
type apcupsdStatus struct {
	bcharge  *float64 // battery charge level (percentage)
	battv    *float64 // battery voltage (Volts)
	nombattv *float64 // nominal battery voltage (Volts)
	linev    *float64 // line voltage (Volts)
	minlinev *float64 // min line voltage (Volts)
	maxlinev *float64 // max line voltage (Volts)
	linefreq *float64 // line frequency (Hz)
	outputv  *float64 // output voltage (Volts)
	nomoutv  *float64 // nominal output voltage (Volts)
	loadpct  *float64 // UPS Load (Percent Load Capacity)
	itemp    *float64 // internal UPS temperature (Celsius)
	nompower *float64 // nominal power (Watts)
	timeleft *float64 // estimated runtime left (minutes)
	battdate string   // Last battery change date (MM/DD/YY or YYYY-MM-DD)
	status   string
	selftest string
}

func parseStatus(resp []byte) (*apcupsdStatus, error) {
	var st apcupsdStatus
	sc := bufio.NewScanner(bytes.NewBuffer(resp))

	for sc.Scan() {
		line := sc.Text()

		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}

		key, value = strings.TrimSpace(key), strings.TrimSpace(value)

		if value == "N/A" {
			continue
		}

		var err error

		// https://github.com/therealbstern/apcupsd/blob/224d19d5faa508d04267f6135fe53d50800550de/src/lib/apcstatus.c#L30
		switch key {
		case "BCHARGE":
			st.bcharge, err = parseFloat(value)
		case "BATTV":
			st.battv, err = parseFloat(value)
		case "NOMBATTV":
			st.nombattv, err = parseFloat(value)
		case "LINEV":
			st.linev, err = parseFloat(value)
		case "MINLINEV":
			st.minlinev, err = parseFloat(value)
		case "MAXLINEV":
			st.maxlinev, err = parseFloat(value)
		case "LINEFREQ":
			st.linefreq, err = parseFloat(value)
		case "OUTPUTV":
			st.outputv, err = parseFloat(value)
		case "NOMOUTV":
			st.nomoutv, err = parseFloat(value)
		case "LOADPCT":
			st.loadpct, err = parseFloat(value)
		case "ITEMP":
			st.itemp, err = parseFloat(value)
		case "NOMPOWER":
			st.nompower, err = parseFloat(value)
		case "TIMELEFT":
			st.timeleft, err = parseFloat(value)
		case "BATTDATE":
			st.battdate = value
		case "STATUS":
			if value == "SHUTTING DOWN" {
				value = "SHUTTING_DOWN"
			}
			st.status = value
		case "SELFTEST":
			if value == "??" {
				value = "UNK"
			}
			st.selftest = value
		default:
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("line '%s': %v", line, err)
		}
	}

	return &st, nil
}

func parseFloat(s string) (*float64, error) {
	val, _, _ := strings.Cut(s, " ")
	f, err := strconv.ParseFloat(val, 64)
	if err != nil {
		return nil, err
	}
	return &f, nil
}
