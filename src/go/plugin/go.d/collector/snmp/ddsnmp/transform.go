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

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/metrix"
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
		"setMultivalue": func(m *Metric, mappings map[int64]string) string {
			m.MultiValue = make(map[string]int64)
			for k, v := range mappings {
				if _, ok := m.MultiValue[v]; !ok || m.Value == k {
					m.MultiValue[v] = metrix.Bool(m.Value == k)
				}
			}
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

			famPrefix := "Hardware/Sensor/"
			config := map[string]map[string]interface{}{
				"1":  {"name": "unspecified", "family": "Generic", "desc": "Unspecified or vendor-specific sensor"},
				"2":  {"name": "unknown", "family": "Unknown", "desc": "Unknown sensor type"},
				"3":  {"name": "voltage_ac", "unit": "V", "family": "Voltage/AC", "desc": "AC voltage"},
				"4":  {"name": "voltage_dc", "unit": "V", "family": "Voltage/DC", "desc": "DC voltage"},
				"5":  {"name": "current", "unit": "A", "family": "Current", "desc": "Current draw"},
				"6":  {"name": "power", "unit": "W", "family": "Power", "desc": "Power consumption"},
				"7":  {"name": "frequency", "unit": "Hz", "family": "Frequency", "desc": "Frequency"},
				"8":  {"name": "temperature", "unit": "Cel", "family": "Temperature", "desc": "Temperature reading"},
				"9":  {"name": "humidity", "unit": "%", "family": "Humidity", "desc": "Relative humidity"},
				"10": {"name": "fan_speed", "unit": "{revolution}/min", "family": "FanSpeed", "desc": "Fan rotation speed"},
				"11": {"name": "airflow", "unit": "m3/min", "family": "Airflow", "desc": "Airflow in cubic meters per minute"},
				"12": {"name": "sensor_state", "family": "State", "desc": "Boolean sensor state", "mapping": map[int64]string{
					0: "false", 1: "true", 2: "true",
				}},
				"13": {"name": "special_enum", "family": "Enum", "desc": "Vendor-specific enumerated sensor"},
				"14": {"name": "power_dbm", "unit": "dBm", "family": "Power", "desc": "Power in decibel-milliwatts"},
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
					m.Family = famPrefix + family + "/Status"
				}
				m.MultiValue = map[string]int64{
					"ok":             metrix.Bool(m.Value == 1),
					"unavailable":    metrix.Bool(m.Value == 2),
					"nonoperational": metrix.Bool(m.Value == 3),
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
					m.Family = famPrefix + family + "/Value"
				}
				if desc, ok := conf["desc"].(string); ok {
					m.Description = desc
				}
				if unit, ok := conf["unit"].(string); ok {
					m.Unit = unit
				}
				if mapping, ok := conf["mapping"].(map[int64]string); ok {
					m.MultiValue = make(map[string]int64)
					for k, v := range mapping {
						if _, ok := m.MultiValue[v]; !ok || m.Value == k {
							m.MultiValue[v] = metrix.Bool(m.Value == k)
						}
					}
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
		"transformHrStorage": func(m *Metric) string {
			// hrStorageType → friendly name/family
			// https://oidref.com/1.3.6.1.2.1.25.2.1
			typeMap := map[string]map[string]string{
				"1.3.6.1.2.1.25.2.1.1":  {"key": "other", "family": "System/Memory/Other", "desc": "Other memory"},
				"1.3.6.1.2.1.25.2.1.2":  {"key": "ram", "family": "System/Memory/RAM", "desc": "Physical system memory"},
				"1.3.6.1.2.1.25.2.1.3":  {"key": "virtual_memory", "family": "System/Memory/Swap", "desc": "Virtual memory space"},
				"1.3.6.1.2.1.25.2.1.4":  {"key": "fixed_disk", "family": "System/Storage/Disk", "desc": "Fixed disk storage"},
				"1.3.6.1.2.1.25.2.1.5":  {"key": "removable_disk", "family": "System/Storage/Removable", "desc": "Removable disk storage"},
				"1.3.6.1.2.1.25.2.1.6":  {"key": "floppy", "family": "System/Storage/Removable", "desc": "Floppy disk storage"},
				"1.3.6.1.2.1.25.2.1.7":  {"key": "cdrom", "family": "System/Storage/Removable", "desc": "CD-ROM storage"},
				"1.3.6.1.2.1.25.2.1.8":  {"key": "ram_disk", "family": "System/Storage/RAMDisk", "desc": "RAM-based storage"},
				"1.3.6.1.2.1.25.2.1.9":  {"key": "flash", "family": "System/Storage/Flash", "desc": "Flash-based storage"},
				"1.3.6.1.2.1.25.2.1.10": {"key": "network_disk", "family": "System/Storage/NetworkDisk", "desc": "Network-mounted storage"},
			}

			storageTypeOID := m.Tags["rm:storage_type"]
			allocUnit, _ := strconv.ParseInt(m.Tags["rm:storage_alloc_unit"], 10, 64)

			if storageTypeOID == "" || allocUnit <= 0 || m.Value < 0 {
				return ""
			}

			tconf, ok := typeMap[storageTypeOID]
			if !ok {
				tconf = map[string]string{"key": "unknown", "family": "System/Storage/Unknown", "desc": "Unknown storage type"}
			}

			bytesVal := m.Value * allocUnit // to bytes
			baseKey := tconf["key"]
			fam := tconf["family"]
			desc := tconf["desc"]

			switch m.Name {
			case "hrStorageSize":
				m.Family = fam + "/Total"
				m.Description = desc + " - Total"
			case "hrStorageUsed":
				m.Family = fam + "/Used"
				m.Description = desc + " - Used"
			default:
				return ""
			}

			m.Name = m.Name + "_" + baseKey
			m.Value = bytesVal

			return ""
		},
	}

	for name, fn := range extra {
		fm[name] = fn
	}

	return fm
}
