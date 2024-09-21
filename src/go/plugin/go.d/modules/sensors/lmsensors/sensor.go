package lmsensors

import (
	"time"
)

// A Chip is a physical or virtual device which may have zero or more Sensors.
type Chip struct {
	Name       string
	UniqueName string
	SysDevice  string
	Sensors    Sensors
}

type Sensors struct {
	// https://www.kernel.org/doc/Documentation/hwmon/sysfs-interface
	Voltage     []*VoltageSensor
	Fan         []*FanSensor
	Temperature []*TemperatureSensor
	Current     []*CurrentSensor
	Power       []*PowerSensor
	Energy      []*EnergySensor
	Humidity    []*HumiditySensor
	Intrusion   []*IntrusionSensor
}

// A VoltageSensor is a Sensor that detects voltage.
type VoltageSensor struct {
	ReadTime time.Duration

	Name  string
	Label string

	Alarm *bool

	Input   *float64
	Average *float64

	Lowest  *float64
	Highest *float64

	Min     *float64
	Max     *float64
	CritMin *float64
	CritMax *float64
}

// A FanSensor is a Sensor that detects fan speeds in rotations per minute.
type FanSensor struct {
	ReadTime time.Duration

	Name  string
	Label string

	Alarm *bool

	Input  *float64
	Min    *float64
	Max    *float64
	Target *float64
}

// A TemperatureSensor is a Sensor that detects temperatures in degrees Celsius.
type TemperatureSensor struct {
	ReadTime time.Duration

	Name        string
	Label       string
	TempTypeRaw int

	Alarm *bool

	Input     *float64
	Lowest    *float64
	Highest   *float64
	Min       *float64
	Max       *float64
	CritMin   *float64
	CritMax   *float64
	Emergency *float64
}

func (s TemperatureSensor) TempType() string {
	switch s.TempTypeRaw {
	case 1:
		return "CPU embedded diode"
	case 2:
		return "3904 transistor"
	case 3:
		return "thermal diode"
	case 4:
		return "thermistor"
	case 5:
		return "AMD AMDSI"
	case 6:
		return "Intel PECI"
	default:
		return ""
	}
}

// A CurrentSensor is a Sensor that detects current in Amperes.
type CurrentSensor struct {
	ReadTime time.Duration

	Name  string
	Label string

	Alarm *bool

	Input   *float64
	Lowest  *float64
	Highest *float64
	Min     *float64
	Max     *float64
	CritMin *float64
	CritMax *float64

	Average *float64
}

// A PowerSensor is a Sensor that detects average electrical power consumption in watts.
type PowerSensor struct {
	ReadTime time.Duration

	Name  string
	Label string

	Alarm *bool

	Input        *float64
	InputLowest  *float64
	InputHighest *float64
	Cap          *float64
	CapMin       *float64
	CapMax       *float64
	Max          *float64
	CritMax      *float64

	Average        *float64
	AverageMin     *float64
	AverageMax     *float64
	AverageLowest  *float64
	AverageHighest *float64

	Accuracy *float64
}

// A EnergySensor is a Sensor that detects energy in microJoule.
type EnergySensor struct {
	ReadTime time.Duration

	Name  string
	Label string

	Input *float64
}

// A HumiditySensor is a Sensor that detects humidity in milli-percent.
type HumiditySensor struct {
	ReadTime time.Duration

	Name  string
	Label string

	Input *float64
}

// An IntrusionSensor is a Sensor that detects when the machine's chassis has been opened.
type IntrusionSensor struct {
	ReadTime time.Duration

	Name  string
	Label string

	Alarm *bool
}
