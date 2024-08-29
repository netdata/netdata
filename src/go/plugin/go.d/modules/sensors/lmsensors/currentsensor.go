package lmsensors

import (
	"strconv"
)

var _ Sensor = &CurrentSensor{}

// A CurrentSensor is a Sensor that detects current in Amperes.
type CurrentSensor struct {
	// The name of the sensor.
	Name string

	// A label that describes what the sensor is monitoring.  Label may be empty.
	Label string

	// Whether the sensor has an alarm triggered.
	Alarm bool

	// The input current, in Amperes, indicated by the sensor.
	Input float64

	// The maximum current threshold, in Amperes, indicated by the sensor.
	Maximum float64

	// The critical current threshold, in Amperes, indicated by the sensor.
	Critical float64
}

func (s *CurrentSensor) Type() SensorType { return SensorTypeCurrent }

func (s *CurrentSensor) parse(raw map[string]string) error {
	for k, v := range raw {
		switch k {
		case "crit", "input", "max":
			f, err := strconv.ParseFloat(v, 64)
			if err != nil {
				return err
			}

			// Raw current values are scaled by 1000
			f /= 1000

			switch k {
			case "crit":
				s.Critical = f
			case "input":
				s.Input = f
			case "max":
				s.Maximum = f
			}
		case "alarm":
			s.Alarm = v != "0"
		case "label":
			s.Label = v
		}
	}

	return nil
}

func (s *CurrentSensor) name() string { return s.Name }
