// SPDX-License-Identifier: GPL-3.0-or-later

package sensors

import (
	"errors"
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/modules/sensors/lmsensors"
)

func (s *Sensors) collectSysfs() (map[string]int64, error) {
	if s.sc == nil {
		return nil, errors.New("sysfs scanner is not initialized")
	}

	s.Debugf("using sysfs scan to collect metrics")

	devices, err := s.sc.Scan()
	if err != nil {
		return nil, err
	}

	if len(devices) == 0 {
		return nil, errors.New("sysfs scanner: devices found")
	}

	seen := make(map[string]bool)
	mx := make(map[string]int64)

	for _, dev := range devices {
		for _, sn := range dev.Sensors {
			var key string

			switch v := sn.(type) {
			case *lmsensors.TemperatureSensor:
				key = snakeCase(fmt.Sprintf("sensor_chip_%s_feature_%s_subfeature_%s_input", dev.Name, firstNotEmpty(v.Label, v.Name), v.Name))
				mx[key] = int64(v.Input * precision)
			case *lmsensors.VoltageSensor:
				key = snakeCase(fmt.Sprintf("sensor_chip_%s_feature_%s_subfeature_%s_input", dev.Name, firstNotEmpty(v.Label, v.Name), v.Name))
				mx[key] = int64(v.Input * precision)
			case *lmsensors.CurrentSensor:
				key = snakeCase(fmt.Sprintf("sensor_chip_%s_feature_%s_subfeature_%s_input", dev.Name, firstNotEmpty(v.Label, v.Name), v.Name))
				mx[key] = int64(v.Input * precision)
			case *lmsensors.PowerSensor:
				key = snakeCase(fmt.Sprintf("sensor_chip_%s_feature_%s_subfeature_%s_average", dev.Name, firstNotEmpty(v.Label, v.Name), v.Name))
				mx[key] = int64(v.Average * precision)
			case *lmsensors.FanSensor:
				key = snakeCase(fmt.Sprintf("sensor_chip_%s_feature_%s_subfeature_%s_input", dev.Name, firstNotEmpty(v.Label, v.Name), v.Name))
				mx[key] = int64(v.Input * precision)
			case *lmsensors.IntrusionSensor:
				key = snakeCase(fmt.Sprintf("sensor_chip_%s_feature_%s_subfeature_%s_alarm", dev.Name, firstNotEmpty(v.Label, v.Name), v.Name))
				mx[key+"_triggered"] = boolToInt(v.Alarm)
				mx[key+"_clear"] = boolToInt(!v.Alarm)
			default:
				s.Debugf("unexpected sensor type: %T", v)
				continue
			}

			seen[key] = true

			if !s.sensors[key] {
				s.sensors[key] = true
				s.addSysfsSensorChart(dev.Name, sn)
			}
		}
	}

	if len(mx) == 0 {
		return nil, errors.New("sysfs scanner: no metrics collected")
	}

	for k := range s.sensors {
		if !seen[k] {
			delete(s.sensors, k)
			s.removeSensorChart(k)
		}
	}

	return mx, nil
}

func firstNotEmpty(s ...string) string {
	for _, v := range s {
		if v != "" {
			return v
		}
	}
	return ""
}

func boolToInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}
