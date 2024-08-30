package lmsensors

import (
	"strconv"
)

var _ Sensor = &FanSensor{}

// A FanSensor is a Sensor that detects fan speeds in rotations per minute.
type FanSensor struct {
	// The name of the sensor.
	Name string

	// A label that describes what the sensor is monitoring.  Label may be empty.
	Label string

	// Whether the fan speed is below the minimum threshold.
	Alarm bool

	// Whether the fan will sound an audible alarm when fan speed is below the minimum threshold.
	Beep bool

	// The input fan speed, in rotations per minute, indicated by the sensor.
	Input float64

	// The low threshold fan speed, in rotations per minute, indicated by the sensor.
	Minimum float64

	// The high threshold fan speed, in rotations per minute, indicated by the sensor.
	Maximum float64
}

func (s *FanSensor) Type() SensorType { return SensorTypeFan }

func (s *FanSensor) parse(raw map[string]string) error {
	for k, v := range raw {
		switch k {
		case "input", "min", "max":
			f, err := strconv.ParseFloat(v, 64)
			if err != nil {
				return err
			}

			switch k {
			case "input":
				s.Input = f
			case "min":
				s.Minimum = f
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

func (s *FanSensor) name() string { return s.Name }
