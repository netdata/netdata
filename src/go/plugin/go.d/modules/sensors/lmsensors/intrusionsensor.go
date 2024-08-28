package lmsensors

var _ Sensor = &IntrusionSensor{}

// An IntrusionSensor is a Sensor that detects when the machine's chassis
// has been opened.
type IntrusionSensor struct {
	// The name of the sensor.
	Name string

	// Whether or not the machine's chassis has been opened, and the alarm
	// has been triggered.
	Alarm bool
}

func (s *IntrusionSensor) name() string        { return s.Name }
func (s *IntrusionSensor) setName(name string) { s.Name = name }

func (s *IntrusionSensor) parse(raw map[string]string) error {
	for k, v := range raw {
		switch k {
		case "alarm":
			s.Alarm = v != "0"
		}
	}

	return nil
}
