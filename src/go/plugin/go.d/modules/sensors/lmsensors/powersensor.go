package lmsensors

import (
	"strconv"
	"time"
)

var _ Sensor = &PowerSensor{}

// A PowerSensor is a Sensor that detects average electrical power consumption in watts.
type PowerSensor struct {
	// The name of the sensor.
	Name string

	// A label that describes what the sensor is monitoring.  Label may be empty.
	Label string

	// The average electrical power consumption, in watts, indicated by the sensor.
	Average float64

	// The interval of time over which the average electrical power consumption is collected.
	AverageInterval time.Duration

	// Whether this sensor has a battery.
	Battery bool

	// The model number of the sensor.
	ModelNumber string

	// Miscellaneous OEM information about the sensor.
	OEMInfo string

	// The serial number of the sensor.
	SerialNumber string
}

func (s *PowerSensor) Type() SensorType { return SensorTypePower }

func (s *PowerSensor) parse(raw map[string]string) error {
	for k, v := range raw {
		switch k {
		case "average":
			f, err := strconv.ParseFloat(v, 64)
			if err != nil {
				return err
			}

			// Raw temperature values are scaled by one million
			f /= 1000000
			s.Average = f
		case "average_interval":
			// Time values in milliseconds
			d, err := time.ParseDuration(v + "ms")
			if err != nil {
				return err
			}

			s.AverageInterval = d
		case "is_battery":
			s.Battery = v != "0"
		case "model_number":
			s.ModelNumber = v
		case "oem_info":
			s.OEMInfo = v
		case "serial_number":
			s.SerialNumber = v
		case "label":
			s.Label = v
		}
	}

	return nil
}

func (s *PowerSensor) name() string { return s.Name }
