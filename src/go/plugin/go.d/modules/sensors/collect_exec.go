// SPDX-License-Identifier: GPL-3.0-or-later

package sensors

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const (
	sensorTypeTemp      = "temperature"
	sensorTypeVoltage   = "voltage"
	sensorTypePower     = "power"
	sensorTypeHumidity  = "humidity"
	sensorTypeFan       = "fan"
	sensorTypeCurrent   = "current"
	sensorTypeEnergy    = "energy"
	sensorTypeIntrusion = "intrusion"
)

type execSensor struct {
	chip       string
	feature    string
	subfeature string
	value      string
}

func (s *execSensor) String() string {
	return fmt.Sprintf("chip:%s feat:%s subfeat:%s value:%s", s.chip, s.feature, s.subfeature, s.value)
}

func (s *execSensor) sensorType() string {
	switch {
	case strings.HasPrefix(s.subfeature, "temp"):
		return sensorTypeTemp
	case strings.HasPrefix(s.subfeature, "intrusion"):
		return sensorTypeIntrusion
	case strings.HasPrefix(s.subfeature, "in"):
		return sensorTypeVoltage
	case strings.HasPrefix(s.subfeature, "power"):
		return sensorTypePower
	case strings.HasPrefix(s.subfeature, "humidity"):
		return sensorTypeHumidity
	case strings.HasPrefix(s.subfeature, "fan"):
		return sensorTypeFan
	case strings.HasPrefix(s.subfeature, "curr"):
		return sensorTypeCurrent
	case strings.HasPrefix(s.subfeature, "energy"):
		return sensorTypeEnergy
	default:
		return ""
	}
}

func (s *execSensor) limits() (minVal float64, maxVal float64, ok bool) {
	switch s.sensorType() {
	case sensorTypeTemp:
		return -127, 1000, true
	case sensorTypeVoltage:
		return -400, 400, true
	case sensorTypeCurrent:
		return -127, 127, true
	case sensorTypeFan:
		return 0, 65535, true
	default:
		return 0, 0, false
	}
}

func (s *Sensors) collectExec() (map[string]int64, error) {
	if s.exec == nil {
		return nil, errors.New("exec sensor is not initialized")
	}

	s.Debugf("using sensors binary to collect metrics")

	bs, err := s.exec.sensorsInfo()
	if err != nil {
		return nil, err
	}

	if len(bs) == 0 {
		return nil, errors.New("empty response from sensors")
	}

	sensors, err := parseExecSensors(bs)
	if err != nil {
		return nil, err
	}
	if len(sensors) == 0 {
		return nil, errors.New("no sensors found")
	}

	mx := make(map[string]int64)
	seen := make(map[string]bool)

	for _, sn := range sensors {
		var sx string

		switch sn.sensorType() {
		case "":
			s.Debugf("can not find type for sensor '%s'", sn)
			continue
		case sensorTypePower:
			sx = "_average"
		case sensorTypeIntrusion:
			sx = "_alarm"
		default:
			sx = "_input"
		}

		if !strings.HasSuffix(sn.subfeature, sx) {
			s.Debugf("skipping sensor: '%s'", sn)
			continue
		}

		v, err := strconv.ParseFloat(sn.value, 64)
		if err != nil {
			s.Debugf("parsing value for sensor '%s': %v", sn, err)
			continue
		}

		if minVal, maxVal, ok := sn.limits(); ok && (v < minVal || v > maxVal) {
			s.Debugf("value outside limits [%d/%d] for sensor '%s'", int64(minVal), int64(maxVal), sn)
			continue
		}

		key := fmt.Sprintf("sensor_chip_%s_feature_%s_subfeature_%s", sn.chip, sn.feature, sn.subfeature)
		key = snakeCase(key)

		if !s.sensors[key] {
			s.sensors[key] = true
			s.addExecSensorChart(sn)
		}

		seen[key] = true

		if sn.sensorType() == sensorTypeIntrusion {
			mx[key+"_triggered"] = boolToInt(v != 0)
			mx[key+"_clear"] = boolToInt(v == 0)
		} else {
			mx[key] = int64(v * precision)
		}
	}

	for k := range s.sensors {
		if !seen[k] {
			delete(s.sensors, k)
			s.removeSensorChart(k)
		}
	}

	return mx, nil
}

func snakeCase(n string) string {
	return strings.ToLower(strings.ReplaceAll(n, " ", "_"))
}

func parseExecSensors(output []byte) ([]execSensor, error) {
	var sensors []execSensor

	sc := bufio.NewScanner(bytes.NewReader(output))

	var chip, feat string

	for sc.Scan() {
		text := sc.Text()
		if text == "" {
			chip, feat = "", ""
			continue
		}

		switch {
		case strings.HasPrefix(text, "  ") && chip != "" && feat != "":
			parts := strings.Split(text, ":")
			if len(parts) != 2 {
				continue
			}
			subfeat, value := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
			sensors = append(sensors, execSensor{
				chip:       chip,
				feature:    feat,
				subfeature: subfeat,
				value:      value,
			})
		case strings.HasSuffix(text, ":") && chip != "":
			feat = strings.TrimSpace(strings.TrimSuffix(text, ":"))
		default:
			chip = text
			feat = ""
		}
	}

	return sensors, nil
}
