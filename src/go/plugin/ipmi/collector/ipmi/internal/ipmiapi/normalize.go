// SPDX-License-Identifier: GPL-3.0-or-later

package ipmiapi

import (
	"fmt"
	"math"
	"strings"
	"unicode"

	"github.com/bougou/go-ipmi/pkg/types"
)

type descriptor struct {
	sensor    Sensor
	number    uint8
	owner     types.GeneratorID
	kind      uint8
	event     uint8
	unit      types.SensorUnit
	factors   types.ReadingFactors
	linear    types.LinearizationFunc
	analog    bool
	supported bool
}

func describe(sdr *types.SDR) []descriptor {
	var d descriptor
	var id []byte
	var encoding types.TypeLength
	count, offset, modifier := 1, 0, uint8(0)
	if f := sdr.Full; f != nil {
		d.owner = f.GeneratorID
		d.number = uint8(f.SensorNumber)
		d.kind = uint8(f.SensorType)
		d.event = uint8(f.SensorEventReadingType)
		d.unit = f.SensorUnit
		d.factors = f.ReadingFactors
		d.linear = f.LinearizationFunc
		d.analog = f.SensorUnit.IsAnalog()
		id = f.IDStringBytes
		encoding = f.IDStringTypeLength
	} else if c := sdr.Compact; c != nil {
		d.owner = c.GeneratorID
		d.number = uint8(c.SensorNumber)
		d.kind = uint8(c.SensorType)
		d.event = uint8(c.SensorEventReadingType)
		d.unit = c.SensorUnit
		id = c.IDStringBytes
		encoding = c.IDStringTypeLength
		count = max(1, int(c.ShareCount))
		offset = int(c.IDStringInstanceModifierOffset)
		modifier = c.IDStringInstanceModifierType
	} else {
		return nil
	}
	chars, err := encoding.Chars(id)
	name := string(chars)
	if encoding.TypeCode() == 3 {
		latin1 := make([]rune, len(chars))
		for i, b := range chars {
			latin1[i] = rune(b)
		}
		name = string(latin1)
	}
	name = strings.TrimRight(name, "\x00")
	name = strings.Map(func(c rune) rune {
		if unicode.IsControl(c) {
			return '_'
		}
		return c
	}, name)
	if name == "" {
		name = "UNNAMED"
	}
	// Bridged/channel-specific ownership is intentionally not guessed. BMC LUNs
	// use the library's per-command responder context for local I/O.
	d.supported = err == nil && (encoding.TypeCode() != 0 || len(id) == 0) && d.owner.OwnerID() == 0x20 && d.owner.ChannelNumber() == 0 && modifier <= 1 && int(d.number)+count <= 255
	unit, metric, legacyUnit := metricUnit(d.unit)
	legacyReading := 255
	if d.analog {
		legacyReading = 2
	} else {
		unit = ""
		metric = ""
		legacyUnit = 0
	}
	sensorType, component := sensorLabels(d.kind, name)
	var result []descriptor
	for n := 0; n < count; n++ {
		current := d
		current.number = uint8(int(d.number) + n)
		sensorName := name
		if count > 1 {
			suffix := fmt.Sprint(offset + n)
			if modifier == 1 {
				v := offset + n
				suffix = string(rune('A' + v%26))
				if v >= 26 {
					suffix = string(rune('A'+v/26-1)) + suffix
				}
			}
			sensorName += " " + suffix
		}
		current.sensor = Sensor{Key: fmt.Sprintf("i%d_n%d_t%d_u%d_%s", sdr.RecordHeader.RecordID, current.number, legacyReading, legacyUnit, sensorName), Name: sensorName, Type: sensorType, Component: component, Unit: unit, Metric: metric, State: "unknown"}
		result = append(result, current)
	}
	return result
}

func metricUnit(unit types.SensorUnit) (string, string, int) {
	if unit.Percentage && unit.ModifierRelation == 0 && unit.RateUnit == 0 && unit.BaseUnit == 0 {
		return "%", "reading_percent", 7
	}
	if unit.Percentage || unit.ModifierRelation != 0 || unit.ModifierUnit != 0 {
		return "", "", 255
	}
	if unit.RateUnit != 0 && !(unit.BaseUnit == types.SensorUnitType_RPM && unit.RateUnit == types.SensorRateUnit_PerMin) {
		return "", "", 255
	}
	switch unit.BaseUnit {
	case types.SensorUnitType_DegreesC:
		return "Celsius", "temperature_c", 1
	case types.SensorUnitType_DegreesF:
		return "Fahrenheit", "temperature_f", 2
	case types.SensorUnitType_Volts:
		return "Volts", "voltage", 3
	case types.SensorUnitType_Amps:
		return "Amps", "ampere", 4
	case types.SensorUnitType_RPM:
		return "RPM", "fan_speed", 5
	case types.SensorUnitType_Watts:
		return "Watts", "power", 6
	default:
		return "", "", 255
	}
}

func convertValue(raw uint8, unit types.SensorUnit, factors types.ReadingFactors, linear types.LinearizationFunc) *float64 {
	if !unit.IsAnalog() || linear > types.LinearizationFunc_CBRT {
		return nil
	}
	value := types.ConvertReading(raw, unit.AnalogDataFormat, factors, linear)
	if linear == types.LinearizationFunc_EXP10 {
		// Upstream 1462645 truncates EXP10's fractional input. Remove this workaround
		// after upgrading to the published fix: math.Pow10(int(x)) -> math.Pow(10,x).
		value = math.Pow(10, types.ConvertReading(raw, unit.AnalogDataFormat, factors, types.LinearizationFunc_Linear))
	}
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return nil
	}
	return &value
}
