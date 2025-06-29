// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmp

import (
	"errors"
	"fmt"
	"math"
	"net"
	"strconv"
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
		"transformEntPhySensor": func(m *Metric) string {
			/*
				transformEntPhySensor applies standardized normalization to metrics
				from the ENTITY-SENSOR-MIB's entPhySensorTable:

				OID base: .1.3.6.1.2.1.99.1.1
				Spec: https://datatracker.ietf.org/doc/html/rfc3433
				Applies to: entPhySensorValue (.1.3.6.1.2.1.99.1.1.1.4)

				Uses the following tags:
				  - sensor_type      (entPhySensorType: integer value 1–12)
				  - sensor_scale     (entPhySensorScale: integer value 1–14)
				  - sensor_precision (entPhySensorPrecision: integer)

				This function:
				  - Assigns proper name, unit, family, description
				  - Applies scaling using sensor_scale and sensor_precision
				  - Optionally maps values to human-readable labels (e.g., "true"/"false")

				This supports multiple vendors implementing ENTITY-SENSOR-MIB:
				  Cisco, Juniper, HPE, Dell, Supermicro, etc.
			*/
			sensorType := m.Tags["sensor_type"]
			sensorScale := m.Tags["sensor_scale"]
			sensorPrecision := m.Tags["sensor_precision"]

			defer func() {
				delete(m.Tags, "sensor_type")
				delete(m.Tags, "sensor_scale")
				delete(m.Tags, "sensor_precision")
			}()

			config := map[string]map[string]interface{}{
				"1":  {"name": "unspecified", "family": "Generic", "desc": "Unspecified or vendor-specific sensor"},
				"2":  {"name": "unknown", "family": "Generic", "desc": "Unknown sensor type"},
				"3":  {"name": "voltage_ac", "unit": "volts", "family": "Power", "desc": "AC voltage"},
				"4":  {"name": "voltage_dc", "unit": "volts", "family": "Power", "desc": "DC voltage"},
				"5":  {"name": "current", "unit": "amperes", "family": "Power", "desc": "Current draw"},
				"6":  {"name": "power", "unit": "watts", "family": "Power", "desc": "Power consumption"},
				"7":  {"name": "frequency", "unit": "hertz", "family": "Power", "desc": "Frequency"},
				"8":  {"name": "temperature", "unit": "celsius", "family": "Temperature", "desc": "Temperature reading"},
				"9":  {"name": "humidity", "unit": "percentage", "family": "Environment", "desc": "Relative humidity"},
				"10": {"name": "fan_speed", "unit": "rpm", "family": "Fan", "desc": "Fan rotation speed"},
				"11": {"name": "airflow", "unit": "cmm", "family": "Environment", "desc": "Airflow in cubic meters per minute"},
				"12": {"name": "sensor_state", "family": "Status", "desc": "Boolean sensor state", "mapping": map[int64]string{
					0: "false", 1: "true", 2: "true",
				}},
			}

			scaleMap := map[string]float64{
				"1": 1.0, "2": 0.001, "3": 0.000001, "4": 0.000000001,
				"5": 0.000000000001, "6": 0.000000000000001, "7": 0.000000000000000001,
				"8": 0.000000000000000000001, "9": 0.000000000000000000000001,
				"10": 0.1, "11": 0.01, "12": 1000.0, "13": 1000000.0, "14": 1000000000.0,
			}
			scale := scaleMap[sensorScale]
			if scale == 0 {
				scale = 1.0
			}

			precision := 0
			if p, err := strconv.Atoi(sensorPrecision); err == nil {
				precision = p
			}

			conf, ok := config[sensorType]
			if !ok {
				return ""
			}

			if name, ok := conf["name"].(string); ok {
				m.Name = m.Name + "_" + name
			}
			if family, ok := conf["family"].(string); ok {
				m.Family = "Sensors/" + family
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

			return ""
		},
	}

	for name, fn := range extra {
		fm[name] = fn
	}

	return fm
}
