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

// A Sensor is a hardware sensor, used to retrieve device temperatures,
// fan speeds, voltages, etc.  Use type assertions to check for specific
// Sensor types and fetch their data.
type Sensor interface {
	parse(raw map[string]string) error
	name() string
	setName(name string)
}

// parseSensors parses all Sensors from an input raw data slice, produced
// during a filesystem walk.
func parseSensors(raw map[string]map[string]string) ([]Sensor, error) {
	sensors := make([]Sensor, 0, len(raw))
	for k, v := range raw {
		var s Sensor
		switch {
		case strings.HasPrefix(k, "curr"):
			s = new(CurrentSensor)
		case strings.HasPrefix(k, "intrusion"):
			s = new(IntrusionSensor)
		case strings.HasPrefix(k, "in"):
			s = new(VoltageSensor)
		case strings.HasPrefix(k, "fan"):
			s = new(FanSensor)
		case strings.HasPrefix(k, "power"):
			s = new(PowerSensor)
		case strings.HasPrefix(k, "temp"):
			s = new(TemperatureSensor)
		default:
			continue
		}

		s.setName(k)
		if err := s.parse(v); err != nil {
			return nil, err
		}

		sensors = append(sensors, s)
	}

	sort.Sort(byName(sensors))
	return sensors, nil
}

// byName implements sort.Interface for []Sensor.
type byName []Sensor

func (b byName) Len() int           { return len(b) }
func (b byName) Less(i, j int) bool { return b[i].name() < b[j].name() }
func (b byName) Swap(i, j int)      { b[i], b[j] = b[j], b[i] }
