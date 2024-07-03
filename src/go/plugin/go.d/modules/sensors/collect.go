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

type sensorStats struct {
	chip       string
	feature    string
	subfeature string
	value      string
}

func (s *sensorStats) String() string {
	return fmt.Sprintf("chip:%s feat:%s subfeat:%s value:%s", s.chip, s.feature, s.subfeature, s.value)
}

const (
	sensorTypeTemp     = "temperature"
	sensorTypeVoltage  = "voltage"
	sensorTypePower    = "power"
	sensorTypeHumidity = "humidity"
	sensorTypeFan      = "fan"
	sensorTypeCurrent  = "current"
	sensorTypeEnergy   = "energy"
)

const precision = 1000

func (s *Sensors) collect() (map[string]int64, error) {
	bs, err := s.exec.sensorsInfo()
	if err != nil {
		return nil, err
	}

	if len(bs) == 0 {
		return nil, errors.New("empty response from sensors")
	}

	sensors, err := parseSensors(bs)
	if err != nil {
		return nil, err
	}
	if len(sensors) == 0 {
		return nil, errors.New("no sensors found")
	}

	mx := make(map[string]int64)
	seen := make(map[string]bool)

	for _, sn := range sensors {
		// TODO: Most likely we need different values depending on the type of sensor.
		if !strings.HasSuffix(sn.subfeature, "_input") {
			s.Debugf("skipping non input sensor: '%s'", sn)
			continue
		}

		v, err := strconv.ParseFloat(sn.value, 64)
		if err != nil {
			s.Debugf("parsing value for sensor '%s': %v", sn, err)
			continue
		}

		if sensorType(sn) == "" {
			s.Debugf("can not find type for sensor '%s'", sn)
			continue
		}

		if minVal, maxVal, ok := sensorLimits(sn); ok && (v < minVal || v > maxVal) {
			s.Debugf("value outside limits [%d/%d] for sensor '%s'", int64(minVal), int64(maxVal), sn)
			continue
		}

		key := fmt.Sprintf("sensor_chip_%s_feature_%s_subfeature_%s", sn.chip, sn.feature, sn.subfeature)
		key = snakeCase(key)
		if !s.sensors[key] {
			s.sensors[key] = true
			s.addSensorChart(sn)
		}

		seen[key] = true

		mx[key] = int64(v * precision)
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

func sensorLimits(sn sensorStats) (minVal float64, maxVal float64, ok bool) {
	switch sensorType(sn) {
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

func sensorType(sn sensorStats) string {
	switch {
	case strings.HasPrefix(sn.subfeature, "temp"):
		return sensorTypeTemp
	case strings.HasPrefix(sn.subfeature, "in"):
		return sensorTypeVoltage
	case strings.HasPrefix(sn.subfeature, "power"):
		return sensorTypePower
	case strings.HasPrefix(sn.subfeature, "humidity"):
		return sensorTypeHumidity
	case strings.HasPrefix(sn.subfeature, "fan"):
		return sensorTypeFan
	case strings.HasPrefix(sn.subfeature, "curr"):
		return sensorTypeCurrent
	case strings.HasPrefix(sn.subfeature, "energy"):
		return sensorTypeEnergy
	default:
		return ""
	}
}

func parseSensors(output []byte) ([]sensorStats, error) {
	var sensors []sensorStats

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
			sensors = append(sensors, sensorStats{
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
