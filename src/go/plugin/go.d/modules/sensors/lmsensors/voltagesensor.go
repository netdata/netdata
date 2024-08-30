package lmsensors

import (
	"strconv"
)

var _ Sensor = &VoltageSensor{}

// A VoltageSensor is a Sensor that detects voltage.
type VoltageSensor struct {
	// The name of the sensor.
	Name string

	// A label that describes what the sensor is monitoring.  Label may be empty.
	Label string

	// Whether the sensor has an alarm triggered.
	Alarm bool

	// Whether the sensor will sound an audible alarm when an alarm
	// is triggered.
	Beep bool

	// The input voltage indicated by the sensor.
	Input float64

	// The minimum voltage threshold indicated by the sensor.
	Min float64

	// The maximum voltage threshold indicated by the sensor.
	Maximum float64
}

func (s *VoltageSensor) Type() SensorType { return SensorTypeVoltage }

func (s *VoltageSensor) parse(raw map[string]string) error {
	for k, v := range raw {
		switch k {
		case "input", "min", "max":
			f, err := strconv.ParseFloat(v, 64)
			if err != nil {
				return err
			}

			// Raw temperature values are scaled by 1000
			f /= 1000

			switch k {
			case "input":
				s.Input = f
			case "min":
				s.Min = f
			case "max":
				s.Maximum = f
			}
		case "alarm":
			s.Alarm = v != "0"
		case "beep":
			s.Beep = v != "0"
		case "label":
			s.Label = v
		}
	}

	return nil
}

func (s *VoltageSensor) name() string { return s.Name }
