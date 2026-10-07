// SPDX-License-Identifier: GPL-3.0-or-later

package bmc

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/bougou/go-ipmi/pkg/types"
)

// SDR ID string type codes.
const (
	idStringUnicode = 0
	idStringLatin1  = 3 // 8-bit ASCII + Latin-1
)

const unnamedSensor = "UNNAMED"

// reservedSensorNumber is not a valid sensor number.
const reservedSensorNumber = 0xff

// Compact record ID string instance modifier types.
const (
	modifierNumeric    = 0
	modifierAlphabetic = 1
)

// libipmimonitoring reading type and units enum values that the C collector
// embeds in sensor keys.
const (
	legacyReadingDouble  = 0x02
	legacyReadingUnknown = 0xff

	legacyUnitNone       = 0x00
	legacyUnitCelsius    = 0x01
	legacyUnitFahrenheit = 0x02
	legacyUnitVolts      = 0x03
	legacyUnitAmps       = 0x04
	legacyUnitRPM        = 0x05
	legacyUnitWatts      = 0x06
	legacyUnitPercent    = 0x07
	legacyUnitUnknown    = 0xff
)

// descriptor is one inventory sensor: its published identity and what reading it requires.
type descriptor struct {
	sensor Sensor

	number        uint8
	owner         types.GeneratorID
	sensorType    uint8
	eventType     uint8 // event/reading type code
	unit          types.SensorUnit
	factors       types.ReadingFactors
	linearization types.LinearizationFunc
	analog        bool
	// supported is false for records that cannot be addressed or named correctly.
	// They are published with unknown state and never queried.
	supported bool
}

// sharing describes a compact record that defines count sensors with
// consecutive numbers and instance-suffixed names.
type sharing struct {
	count    int
	offset   int   // instance number of the first sensor
	modifier uint8 // instance modifier type
}

// describeSDR returns the sensors of a full or compact sensor record.
func describeSDR(sdr *types.SDR) []descriptor {
	var (
		d          descriptor
		idBytes    []byte
		idEncoding types.TypeLength
		share      = sharing{
			count: 1,
		}
	)
	switch {
	case sdr.Full != nil:
		f := sdr.Full
		d = descriptor{
			number:        uint8(f.SensorNumber),
			owner:         f.GeneratorID,
			sensorType:    uint8(f.SensorType),
			eventType:     uint8(f.SensorEventReadingType),
			unit:          f.SensorUnit,
			factors:       f.ReadingFactors,
			linearization: f.LinearizationFunc,
			analog:        f.SensorUnit.IsAnalog(),
		}
		idBytes, idEncoding = f.IDStringBytes, f.IDStringTypeLength
	case sdr.Compact != nil:
		// Compact records carry no conversion factors, so they have no numeric readings.
		c := sdr.Compact
		d = descriptor{
			number:     uint8(c.SensorNumber),
			owner:      c.GeneratorID,
			sensorType: uint8(c.SensorType),
			eventType:  uint8(c.SensorEventReadingType),
			unit:       c.SensorUnit,
		}
		idBytes, idEncoding = c.IDStringBytes, c.IDStringTypeLength
		share = sharing{
			count:    max(1, int(c.ShareCount)),
			offset:   int(c.IDStringInstanceModifierOffset),
			modifier: c.IDStringInstanceModifierType,
		}
	default:
		return nil
	}

	name, nameOK := decodeIDString(idEncoding, idBytes)
	d.supported = nameOK && bmcOwned(d.owner) && share.valid(d.number)

	unit, legacyUnit := unitOf(d.unit)
	legacyReading := legacyReadingDouble
	if !d.analog {
		unit, legacyUnit, legacyReading = "", legacyUnitNone, legacyReadingUnknown
	}
	sensorType, component := sensorTypeAndComponent(d.sensorType, name)

	sensors := make([]descriptor, 0, share.count)
	for n := range share.count {
		sd := d
		sd.number = uint8(int(d.number) + n)
		sensorName := name
		if share.count > 1 {
			sensorName += " " + share.suffix(n)
		}
		sd.sensor = Sensor{
			Key:       sensorKey(sdr.RecordHeader.RecordID, sd.number, legacyReading, legacyUnit, sensorName),
			Name:      sensorName,
			Type:      sensorType,
			Component: component,
			Unit:      unit,
			State:     StateUnknown,
		}
		sensors = append(sensors, sd)
	}
	return sensors
}

// decodeIDString decodes an SDR ID string into a printable name. It reports
// false for malformed strings and Unicode, which is not decoded.
func decodeIDString(encoding types.TypeLength, raw []byte) (string, bool) {
	chars, err := encoding.Chars(raw)
	name := string(chars)
	if encoding.TypeCode() == idStringLatin1 {
		// The SDK returns Latin-1 bytes unconverted; each byte is its own code point.
		runes := make([]rune, len(chars))
		for i, b := range chars {
			runes[i] = rune(b)
		}
		name = string(runes)
	}
	name = strings.TrimRight(name, "\x00")
	name = strings.Map(func(c rune) rune {
		if unicode.IsControl(c) {
			return '_'
		}
		return c
	}, name)
	if name == "" {
		name = unnamedSensor
	}
	return name, err == nil && (encoding.TypeCode() != idStringUnicode || len(raw) == 0)
}

// bmcOwned reports whether the BMC itself owns the sensor. Satellite and
// bridged owners are not queried as if the BMC owned them; BMC LUNs are
// addressed per command.
func bmcOwned(owner types.GeneratorID) bool {
	return owner.OwnerID() == types.BMC_SA && owner.ChannelNumber() == 0
}

// valid reports whether the modifier type is known and the shared sensor
// numbers starting at first stay below the reserved sensor number.
func (s sharing) valid(first uint8) bool {
	knownModifier := s.modifier == modifierNumeric || s.modifier == modifierAlphabetic
	return knownModifier && int(first)+s.count <= reservedSensorNumber
}

// suffix returns the name suffix of the n-th shared sensor: its instance number,
// or letters (A..Z, AA..) for the alphabetic modifier.
func (s sharing) suffix(n int) string {
	v := s.offset + n
	if s.modifier != modifierAlphabetic {
		return strconv.Itoa(v)
	}
	suffix := string(rune('A' + v%26))
	if v >= 26 {
		suffix = string(rune('A'+v/26-1)) + suffix
	}
	return suffix
}

// sensorKey is the C collector's sensor identity for a healthy reading.
func sensorKey(recordID uint16, number uint8, legacyReading, legacyUnit int, name string) string {
	return fmt.Sprintf("i%d_n%d_t%d_u%d_%s", recordID, number, legacyReading, legacyUnit, name)
}

// unitOf maps an SDR unit to the reported unit and its libipmimonitoring code.
// Unsupported units, rates and modifiers map to no unit.
func unitOf(u types.SensorUnit) (string, int) {
	if u.Percentage && u.ModifierRelation == types.SensorModifierRelation_None &&
		u.RateUnit == types.SensorRateUnit_None && u.BaseUnit == types.SensorUnitType_Unspecified {
		return UnitPercent, legacyUnitPercent
	}
	if u.Percentage || u.ModifierRelation != types.SensorModifierRelation_None ||
		u.ModifierUnit != types.SensorUnitType_Unspecified {
		return "", legacyUnitUnknown
	}
	isRPM := u.BaseUnit == types.SensorUnitType_RPM && u.RateUnit == types.SensorRateUnit_PerMin
	if u.RateUnit != types.SensorRateUnit_None && !isRPM {
		return "", legacyUnitUnknown
	}

	switch u.BaseUnit {
	case types.SensorUnitType_DegreesC:
		return UnitCelsius, legacyUnitCelsius
	case types.SensorUnitType_DegreesF:
		return UnitFahrenheit, legacyUnitFahrenheit
	case types.SensorUnitType_Volts:
		return UnitVolts, legacyUnitVolts
	case types.SensorUnitType_Amps:
		return UnitAmps, legacyUnitAmps
	case types.SensorUnitType_RPM:
		return UnitRPM, legacyUnitRPM
	case types.SensorUnitType_Watts:
		return UnitWatts, legacyUnitWatts
	default:
		return "", legacyUnitUnknown
	}
}
