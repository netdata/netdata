package lmsensors

import (
	"sort"
	"strings"
)

// A Device is a physical or virtual device which may have zero or
// more Sensors.
type Device struct {
	// The name of the device.
	Name string

	// Any Sensors that belong to this Device.  Use type assertions to
	// check for specific Sensor types and fetch their data.
	Sensors []Sensor
}

type SensorType string

const (
	SensorTypeCurrent     SensorType = "current"
	SensorTypeFan         SensorType = "fan"
	SensorTypeIntrusion   SensorType = "intrusion"
	SensorTypePower       SensorType = "power"
	SensorTypeTemperature SensorType = "temperature"
	SensorTypeVoltage     SensorType = "voltage"
)

// A Sensor is a hardware sensor, used to retrieve device temperatures, fan speeds, voltages, etc.
// Use type assertions to check for specific
// Sensor types and fetch their data.
type Sensor interface {
	Type() SensorType
}

// parseSensors parses all Sensors from an input raw data slice, produced during a filesystem walk.
func parseSensors(raw map[string]map[string]string) ([]Sensor, error) {
	sensors := make([]Sensor, 0, len(raw))
	for k, v := range raw {
		var sn Sensor
		var err error

		switch {
		case strings.HasPrefix(k, "curr"):
			s := &CurrentSensor{Name: k}
			sn = s
			err = s.parse(v)
		case strings.HasPrefix(k, "intrusion"):
			s := &IntrusionSensor{Name: k}
			sn = s
			err = s.parse(v)
		case strings.HasPrefix(k, "in"):
			s := &VoltageSensor{Name: k}
			sn = s
			err = s.parse(v)
		case strings.HasPrefix(k, "fan"):
			s := &FanSensor{Name: k}
			sn = s
			err = s.parse(v)
		case strings.HasPrefix(k, "power"):
			s := &PowerSensor{Name: k}
			sn = s
			err = s.parse(v)
		case strings.HasPrefix(k, "temp"):
			s := &TemperatureSensor{Name: k}
			sn = s
			err = s.parse(v)
		default:
			continue
		}
		if err != nil {
			return nil, err
		}

		if sn == nil {
			continue
		}

		sensors = append(sensors, sn)
	}

	type namer interface{ name() string }

	sort.Slice(sensors, func(i, j int) bool {
		v1, ok1 := sensors[i].(namer)
		v2, ok2 := sensors[j].(namer)
		return ok1 && ok2 && v1.name() < v2.name()
	})

	return sensors, nil
}
