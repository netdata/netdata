// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmp

import (
	"errors"
	"fmt"
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
	}

	for name, fn := range extra {
		fm[name] = fn
	}

	return fm
}
