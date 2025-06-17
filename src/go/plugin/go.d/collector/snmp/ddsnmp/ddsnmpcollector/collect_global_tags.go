// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/gosnmp/gosnmp"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

func (c *Collector) collectGlobalTags(prof *ddsnmp.Profile) (map[string]string, error) {
	if len(prof.Definition.MetricTags) == 0 {
		return nil, nil
	}

	var oids []string

	for _, tagCfg := range prof.Definition.MetricTags {
		if tagCfg.Symbol.OID != "" {
			oids = append(oids, tagCfg.Symbol.OID)
		}
	}

	slices.Sort(oids)
	oids = slices.Compact(oids)

	if len(oids) == 0 {
		return nil, nil
	}

	pdus, err := c.snmpGet(oids)
	if err != nil {
		return nil, err
	}

	tags := make(map[string]string)
	var errs []error

	for _, tag := range prof.Definition.StaticTags {
		parts := strings.SplitN(tag, ":", 2)
		if len(parts) == 2 {
			tags[parts[0]] = parts[1]
		}
	}

	for _, cfg := range prof.Definition.MetricTags {
		metricTags, err := processMetricTagValue(cfg, pdus)
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to process tag value for '%s/%s': %v", cfg.Tag, cfg.Symbol.Name, err))
			continue
		}
		for k, val := range metricTags {
			if existing, ok := tags[k]; !ok || existing == "" {
				tags[k] = val
			}
		}
	}

	if len(errs) > 0 && len(tags) == 0 {
		return nil, fmt.Errorf("failed to process any global tags: %v", errors.Join(errs...))
	}

	return tags, nil
}

func processMetricTagValue(cfg ddprofiledefinition.MetricTagConfig, pdus map[string]gosnmp.SnmpPDU) (map[string]string, error) {
	tagName := ternary(cfg.Tag != "", cfg.Tag, cfg.Symbol.Name)
	if tagName == "" {
		return nil, nil
	}

	pdu, ok := pdus[trimOID(cfg.Symbol.OID)]
	if !ok {
		return nil, nil
	}

	val, err := convPduToStringf(pdu, cfg.Symbol.Format)
	if err != nil {
		return nil, err
	}

	tags := make(map[string]string)

	switch {
	case len(cfg.Mapping) > 0:
		if v, ok := cfg.Mapping[val]; ok {
			val = v
		}
		tags[tagName] = val
	case cfg.Pattern != nil:
		if sm := cfg.Pattern.FindStringSubmatch(val); len(sm) > 0 {
			for name, tmpl := range cfg.Tags {
				tags[name] = replaceSubmatches(tmpl, sm)
			}
		}
	case cfg.Symbol.ExtractValueCompiled != nil:
		if sm := cfg.Symbol.ExtractValueCompiled.FindStringSubmatch(val); len(sm) > 1 {
			tags[tagName] = sm[1]
		}
	case cfg.Symbol.MatchPatternCompiled != nil:
		if sm := cfg.Symbol.MatchPatternCompiled.FindStringSubmatch(val); len(sm) > 0 {
			tags[tagName] = replaceSubmatches(cfg.Symbol.MatchValue, sm)
		}
	default:
		tags[tagName] = val
	}

	return tags, nil
}
