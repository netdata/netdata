package lmsensors

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"
)

type (
	rawSensors map[string]map[string]rawValue // e.g. [temp1][input]

	rawValue struct {
		value    string
		readTime time.Duration
	}
)

// parseSensors parses all Sensors from an input raw data slice, produced during a filesystem walk.
func parseSensors(rawSns rawSensors) (*Sensors, error) {
	var sensors Sensors

	for name, values := range rawSns {
		typ := name
		if i := strings.IndexFunc(name, unicode.IsDigit); i > 0 {
			typ = name[:i]
		}

		switch typ {
		case "in":
			sn := &VoltageSensor{Name: name}
			if err := parseVoltage(sn, values); err != nil {
				return nil, fmt.Errorf("voltage sensor '%s': %w", name, err)
			}
			sensors.Voltage = append(sensors.Voltage, sn)
		case "fan":
			sn := &FanSensor{Name: name}
			if err := parseFan(sn, values); err != nil {
				return nil, fmt.Errorf("fan sensor '%s': %w", name, err)
			}
			sensors.Fan = append(sensors.Fan, sn)
		case "temp":
			sn := &TemperatureSensor{Name: name}
			if err := parseTemperature(sn, values); err != nil {
				return nil, fmt.Errorf("temperature sensor '%s': %w", name, err)
			}
			sensors.Temperature = append(sensors.Temperature, sn)
		case "curr":
			sn := &CurrentSensor{Name: name}
			if err := parseCurrent(sn, values); err != nil {
				return nil, fmt.Errorf("current sensor '%s': %w", name, err)
			}
			sensors.Current = append(sensors.Current, sn)
		case "power":
			sn := &PowerSensor{Name: name}
			if err := parsePower(sn, values); err != nil {
				return nil, fmt.Errorf("power sensor '%s': %w", name, err)
			}
			sensors.Power = append(sensors.Power, sn)
		case "energy":
			sn := &EnergySensor{Name: name}
			if err := parseEnergy(sn, values); err != nil {
				return nil, fmt.Errorf("energy sensor '%s': %w", name, err)
			}
			sensors.Energy = append(sensors.Energy, sn)
		case "humidity":
			sn := &HumiditySensor{Name: name}
			if err := parseHumidity(sn, values); err != nil {
				return nil, fmt.Errorf("humidity sensor '%s': %w", name, err)
			}
			sensors.Humidity = append(sensors.Humidity, sn)
		case "intrusion":
			sn := &IntrusionSensor{Name: name}
			if err := parseIntrusion(sn, values); err != nil {
				return nil, fmt.Errorf("intrusion sensor '%s': %w", name, err)
			}
			sensors.Intrusion = append(sensors.Intrusion, sn)
		default:
			continue
		}
	}

	return &sensors, nil
}

func parseVoltage(s *VoltageSensor, values map[string]rawValue) error {
	const div = 1e3 // raw in milli degree Celsius

	for name, val := range values {
		var err error

		s.ReadTime += val.readTime
		v := val.value

		switch name {
		case "label":
			s.Label = v
		case "alarm":
			s.Alarm = ptr(v != "0")
		case "min":
			s.Min, err = parseFloat(v, div)
		case "lcrit":
			s.CritMin, err = parseFloat(v, div)
		case "max":
			s.Max, err = parseFloat(v, div)
		case "crit":
			s.CritMax, err = parseFloat(v, div)
		case "input":
			s.Input, err = parseFloat(v, div)
		case "average":
			s.Average, err = parseFloat(v, div)
		case "lowest":
			s.Lowest, err = parseFloat(v, div)
		case "highest":
			s.Highest, err = parseFloat(v, div)
		}
		if err != nil {
			return fmt.Errorf("subfeature '%s' value '%s': %v", name, v, err)
		}
	}

	return nil
}

func parseFan(s *FanSensor, values map[string]rawValue) error {
	const div = 1 // raw in revolution/min (RPM)

	for name, val := range values {
		var err error

		s.ReadTime += val.readTime
		v := val.value

		switch name {
		case "input":
			s.Input, err = parseFloat(v, div)
		case "min":
			s.Min, err = parseFloat(v, div)
		case "max":
			s.Max, err = parseFloat(v, div)
		case "target":
			s.Target, err = parseFloat(v, div)
		case "alarm":
			s.Alarm = ptr(v != "0")
		case "label":
			s.Label = v
		}
		if err != nil {
			return fmt.Errorf("subfeature '%s' value '%s': %v", name, v, err)
		}
	}

	return nil
}

func parseTemperature(s *TemperatureSensor, values map[string]rawValue) error {
	const div = 1000 // raw in milli degree Celsius

	for name, val := range values {
		var err error

		s.ReadTime += val.readTime
		v := val.value

		switch name {
		case "max":
			s.Max, err = parseFloat(v, div)
		case "min":
			s.Min, err = parseFloat(v, div)
		case "input":
			s.Input, err = parseFloat(v, div)
		case "crit":
			s.CritMax, err = parseFloat(v, div)
		case "emergency":
			s.Emergency, err = parseFloat(v, div)
		case "lcrit":
			s.CritMin, err = parseFloat(v, div)
		case "lowest":
			s.Lowest, err = parseFloat(v, div)
		case "highest":
			s.Highest, err = parseFloat(v, div)
		case "alarm":
			s.Alarm = ptr(v != "0")
		case "type":
			t, err := strconv.Atoi(v)
			if err != nil {
				return err
			}
			s.TempTypeRaw = t
		case "label":
			s.Label = v
		}
		if err != nil {
			return fmt.Errorf("subfeature '%s' value '%s': %v", name, v, err)
		}
	}

	return nil
}

func parseCurrent(s *CurrentSensor, values map[string]rawValue) error {
	const div = 1e3 // raw in milli ampere

	for name, val := range values {
		var err error

		s.ReadTime += val.readTime
		v := val.value

		switch name {
		case "max":
			s.Max, err = parseFloat(v, div)
		case "min":
			s.Min, err = parseFloat(v, div)
		case "lcrit":
			s.CritMin, err = parseFloat(v, div)
		case "crit":
			s.CritMax, err = parseFloat(v, div)
		case "input":
			s.Input, err = parseFloat(v, div)
		case "average":
			s.Average, err = parseFloat(v, div)
		case "lowest":
			s.Lowest, err = parseFloat(v, div)
		case "highest":
			s.Highest, err = parseFloat(v, div)
		case "alarm":
			s.Alarm = ptr(v != "0")
		case "label":
			s.Label = v
		}
		if err != nil {
			return fmt.Errorf("subfeature '%s' value '%s': %v", name, v, err)
		}
	}

	return nil
}

func parsePower(s *PowerSensor, values map[string]rawValue) error {
	const div = 1e6 // raw in microWatt

	for name, val := range values {
		var err error

		s.ReadTime += val.readTime
		v := val.value

		switch name {
		case "label":
			s.Label = v
		case "alarm":
			s.Alarm = ptr(v != "0")
		case "average":
			s.Average, err = parseFloat(v, div)
		case "average_highest":
			s.AverageHighest, err = parseFloat(v, div)
		case "average_lowest":
			s.AverageLowest, err = parseFloat(v, div)
		case "average_max":
			s.AverageMax, err = parseFloat(v, div)
		case "average_min":
			s.AverageMin, err = parseFloat(v, div)
		case "input":
			s.Input, err = parseFloat(v, div)
		case "input_highest":
			s.InputHighest, err = parseFloat(v, div)
		case "input_lowest":
			s.InputLowest, err = parseFloat(v, div)
		case "accuracy":
			v = strings.TrimSuffix(v, "%")
			s.Accuracy, err = parseFloat(v, 1)
		case "cap":
			s.Cap, err = parseFloat(v, div)
		case "cap_max":
			s.CapMax, err = parseFloat(v, div)
		case "cap_min":
			s.CapMin, err = parseFloat(v, div)
		case "max":
			s.Max, err = parseFloat(v, div)
		case "crit":
			s.CritMax, err = parseFloat(v, div)
		}
		if err != nil {
			return fmt.Errorf("subfeature '%s' value '%s': %v", name, v, err)
		}
	}

	return nil
}

func parseEnergy(s *EnergySensor, values map[string]rawValue) error {
	const div = 1e6 // raw in microJoule

	for name, val := range values {
		var err error

		s.ReadTime += val.readTime
		v := val.value

		switch name {
		case "label":
			s.Label = v
		case "input":
			s.Input, err = parseFloat(v, div)
		}
		if err != nil {
			return fmt.Errorf("subfeature '%s' value '%s': %v", name, v, err)
		}
	}

	return nil
}

func parseHumidity(s *HumiditySensor, values map[string]rawValue) error {
	const div = 1e3 // raw in milli percent

	for name, val := range values {
		var err error

		s.ReadTime += val.readTime
		v := val.value

		switch name {
		case "label":
			s.Label = v
		case "input":
			s.Input, err = parseFloat(v, div)
		}
		if err != nil {
			return fmt.Errorf("subfeature '%s' value '%s': %v", name, v, err)
		}
	}

	return nil
}

func parseIntrusion(s *IntrusionSensor, values map[string]rawValue) error {
	for name, val := range values {
		s.ReadTime += val.readTime
		v := val.value

		switch name {
		case "label":
			s.Label = v
		case "alarm":
			s.Alarm = ptr(v != "0")
		}
	}

	return nil
}

func parseFloat(s string, div float64) (*float64, error) {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil, err
	}

	return ptr(f / div), nil
}

func ptr[T any](v T) *T { return &v }
