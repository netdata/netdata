package lmsensors

import (
	"strconv"
)

var _ Sensor = &FanSensor{}

// A FanSensor is a Sensor that detects fan speeds in rotations per minute.
type FanSensor struct {
	// The name of the sensor.
	Name string

	// Whether or not the fan speed is below the minimum threshold.
	Alarm bool

	// Whether or not the fan will sound an audible alarm when fan speed is
	// below the minimum threshold.
	Beep bool

	// The input fan speed, in rotations per minute, indicated by the sensor.
	Input int

	// The low threshold fan speed, in rotations per minute, indicated by the
	// sensor.
	Minimum int
}

func (s *FanSensor) name() string        { return s.Name }
func (s *FanSensor) setName(name string) { s.Name = name }

func (s *FanSensor) parse(raw map[string]string) error {
	for k, v := range raw {
		switch k {
		case "input", "min":
			i, err := strconv.Atoi(v)
			if err != nil {
				return err
			}

			switch k {
			case "input":
				s.Input = i
			case "min":
				s.Minimum = i
			}
		case "alarm":
			s.Alarm = v != "0"
		case "beep":
			s.Beep = v != "0"
		}
	}

	return nil
}
