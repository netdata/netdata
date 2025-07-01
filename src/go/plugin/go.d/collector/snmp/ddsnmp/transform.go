// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmp

import (
	"errors"
	"fmt"
	"math"
	"net"
	"strconv"
	"strings"
	"text/template"

	"github.com/Masterminds/sprig/v3"
)

// CompileTransforms exported for testing purposes only
func CompileTransforms(prof *Profile) error {
	var errs []error

	for i := range prof.Definition.Metrics {
		metric := &prof.Definition.Metrics[i]

		switch {
		case metric.IsScalar():
			if metric.Symbol.Transform == "" {
				continue
			}
			tmpl, err := compileTransform(metric.Symbol.Transform)
			if err != nil {
				errs = append(errs, fmt.Errorf("metric_transform parsing failed for scalar metric '%s': %v",
					metric.Symbol.Name, err))
				continue
			}
			metric.Symbol.TransformCompiled = tmpl
		case metric.IsColumn():
			for j := range metric.Symbols {
				sym := &metric.Symbols[j]
				if sym.Transform == "" {
					continue
				}
				tmpl, err := compileTransform(sym.Transform)
				if err != nil {
					errs = append(errs, fmt.Errorf("metric_transform parsing failed for table '%s' metric '%s': %v",
						metric.Table.Name, sym.Name, err))
					continue
				}
				sym.TransformCompiled = tmpl
			}
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)

	}

	return nil
}

func compileTransform(transform string) (*template.Template, error) {
	return template.New("metric_transform").
		Option("missingkey=zero").
		Funcs(newMetricTransformFuncMap()).
		Parse(transform)
}

func newMetricTransformFuncMap() template.FuncMap {
	fm := sprig.TxtFuncMap()

	extra := map[string]any{
		"deleteTag": func(m *Metric, key string) interface{} {
			delete(m.Tags, key)
			return nil
		},
		"setName": func(m *Metric, name string) string {
			m.Name = name
			return ""
		},
		"setUnit": func(m *Metric, unit string) string {
			m.Unit = unit
			return ""
		},
		"setFamily": func(m *Metric, family string) string {
			m.Family = family
			return ""
		},
		"setDesc": func(m *Metric, desc string) string {
			m.Description = desc
			return ""
		},
		"setValue": func(m *Metric, value int64) string {
			m.Value = value
			return ""
		},
		"setTag": func(m *Metric, key, value string) string {
			if m.Tags == nil {
				m.Tags = make(map[string]string)
			}
			m.Tags[key] = value
			return ""
		},
		"setMappings": func(m *Metric, mappings map[int64]string) string {
			m.Mappings = mappings
			return ""
		},
		"i64map": func(pairs ...any) map[int64]string {
			m := make(map[int64]string)
			for i := 0; i < len(pairs)-1; i += 2 {
				key := int64(0)
				switch k := pairs[i].(type) {
				case int:
					key = int64(k)
				case int64:
					key = k
				case float64:
					key = int64(k)
				}

				if val, ok := pairs[i+1].(string); ok {
					m[key] = val
				}
			}
			return m
		},
		"mask2cidr": func(mask string) string {
			ip := net.ParseIP(mask)
			if ip == nil {
				return ""
			}
			ip = ip.To4()
			if ip == nil {
				return ""
			}
			prefixLen, bits := net.IPv4Mask(ip[0], ip[1], ip[2], ip[3]).Size()
			if bits == 0 {
				return ""
			}
			return strconv.Itoa(prefixLen)
		},
		"pow": func(base float64, exp int) float64 {
			return math.Pow(base, float64(exp))
		},
		"transformEntitySensorValue": func(m *Metric) string {
			/*
				transformEntitySensorValue normalizes metrics from ENTITY-SENSOR-MIB and
				CISCO-ENTITY-SENSOR-MIB.

				Supported MIBs:
				  - ENTITY-SENSOR-MIB (RFC 3433):       .1.3.6.1.2.1.99.1.1
				  - CISCO-ENTITY-SENSOR-MIB:            .1.3.6.1.4.1.9.9.91.1.1.1

				Expected metric:
				  - entPhySensorValue / entSensorValue: .*.1.4

				Required metric tags:
				  - sensor_type      (entPhySensorType / entSensorType)
				  - sensor_scale     (entPhySensorScale / entSensorScale)
				  - sensor_precision (entPhySensorPrecision / entSensorPrecision)

				What it does:
				  - Sets a descriptive name, unit, family, and description
				  - Applies scaling using sensor_scale and sensor_precision
				  - Applies value mappings for boolean/enumerated types
				  - Works across vendors (Cisco, Juniper, HPE, Dell, etc.)
			*/

			sensorType := m.Tags["rm:sensor_type"]
			sensorScale := m.Tags["rm:sensor_scale"]
			sensorPrecision := m.Tags["rm:sensor_precision"]

			config := map[string]map[string]interface{}{
				"1":  {"name": "unspecified", "family": "Sensor/Generic/Value", "desc": "Unspecified or vendor-specific sensor"},
				"2":  {"name": "unknown", "family": "Sensor/Unknown/Value", "desc": "Unknown sensor type"},
				"3":  {"name": "voltage_ac", "unit": "V", "family": "Sensor/Voltage/AC", "desc": "AC voltage"},
				"4":  {"name": "voltage_dc", "unit": "V", "family": "Sensor/Voltage/DC", "desc": "DC voltage"},
				"5":  {"name": "current", "unit": "A", "family": "Sensor/Current/Value", "desc": "Current draw"},
				"6":  {"name": "power", "unit": "W", "family": "Sensor/Power/Value", "desc": "Power consumption"},
				"7":  {"name": "frequency", "unit": "Hz", "family": "Sensor/Frequency/Value", "desc": "Frequency"},
				"8":  {"name": "temperature", "unit": "Cel", "family": "Sensor/Temperature/Value", "desc": "Temperature reading"},
				"9":  {"name": "humidity", "unit": "%", "family": "Sensor/Humidity/Value", "desc": "Relative humidity"},
				"10": {"name": "fan_speed", "unit": "{revolution}/min", "family": "Sensor/FanSpeed/Value", "desc": "Fan rotation speed"},
				"11": {"name": "airflow", "unit": "m3/min", "family": "Sensor/Airflow/Value", "desc": "Airflow in cubic meters per minute"},
				"12": {"name": "sensor_state", "family": "Sensor/State/Value", "desc": "Boolean sensor state", "mapping": map[int64]string{
					0: "false", 1: "true", 2: "true",
				}},
				"13": {"name": "special_enum", "family": "Sensor/Enum/Value", "desc": "Vendor-specific enumerated sensor"},
				"14": {"name": "power_dbm", "unit": "dBm", "family": "Sensor/Power/Value", "desc": "Power in decibel-milliwatts"},
			}

			conf, ok := config[sensorType]
			if !ok {
				return ""
			}

			switch m.Name {
			case "entPhySensorOperStatus":
				if name, ok := conf["name"].(string); ok {
					m.Name = m.Name + "_" + name
				}
				if family, ok := conf["family"].(string); ok {
					m.Family = strings.TrimSuffix(family, "/Value") + "/Status"
				}
				m.Mappings = map[int64]string{
					1: "ok",
					2: "unavailable",
					3: "nonoperational",
				}
			case "entPhySensorValue", "entSensorValue":
				scaleMap := map[string]float64{
					"1":  1e-24, // yocto (10^-24)
					"2":  1e-21, // zepto (10^-21)
					"3":  1e-18, // atto (10^-18)
					"4":  1e-15, // femto (10^-15)
					"5":  1e-12, // pico (10^-12)
					"6":  1e-9,  // nano (10^-9)
					"7":  1e-6,  // micro (10^-6)
					"8":  1e-3,  // milli (10^-3)
					"9":  1,     // units (10^0)
					"10": 1e3,   // kilo (10^3)
					"11": 1e6,   // mega (10^6)
					"12": 1e9,   // giga (10^9)
					"13": 1e12,  // tera (10^12)
				}

				scale := scaleMap[sensorScale]
				if scale == 0 || scale == 1e-24 {
					// Workaround for Cisco ASA (MIMIC) temperature bug: treat scale=1 (yocto) as units
					scale = 1.0
				}

				precision := 0
				if p, err := strconv.Atoi(sensorPrecision); err == nil {
					precision = p
				}

				if name, ok := conf["name"].(string); ok {
					m.Name = m.Name + "_" + name
				}
				if family, ok := conf["family"].(string); ok {
					m.Family = family
				}
				if desc, ok := conf["desc"].(string); ok {
					m.Description = desc
				}
				if unit, ok := conf["unit"].(string); ok {
					m.Unit = unit
				}
				if mapping, ok := conf["mapping"].(map[int64]string); ok {
					m.Mappings = mapping
				} else {
					val := float64(m.Value) * scale
					if precision > 0 {
						val = val / math.Pow(10, float64(precision))
					}
					m.Value = int64(val)
				}
			}

			return ""
		},
	}

	for name, fn := range extra {
		fm[name] = fn
	}

	return fm
}
