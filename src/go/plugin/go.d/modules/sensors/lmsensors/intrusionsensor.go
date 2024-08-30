package lmsensors

var _ Sensor = &IntrusionSensor{}

// An IntrusionSensor is a Sensor that detects when the machine's chassis has been opened.
type IntrusionSensor struct {
	// The name of the sensor.
	Name string

	// A label that describes what the sensor is monitoring.  Label may be empty.
	Label string

	// Whether the machine's chassis has been opened, and the alarm has been triggered.
	Alarm bool
}

func (s *IntrusionSensor) Type() SensorType { return SensorTypeIntrusion }

func (s *IntrusionSensor) parse(raw map[string]string) error {
	for k, v := range raw {
		switch k {
		case "alarm":
			s.Alarm = v != "0"
		case "label":
			s.Label = v
		}
	}

	return nil
}

func (s *IntrusionSensor) name() string { return s.Name }
