// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"github.com/gosnmp/gosnmp"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

type mapTagCollector struct {
	tags map[string]string
}

func (c *mapTagCollector) addTag(key, value string) {
	if existing, ok := c.tags[key]; ok || existing == "" {
		c.tags[key] = value
	}
}

type globalTagProcessor struct {
	tp *tableTagProcessor
}

func newGlobalTagProcessor() *globalTagProcessor {
	return &globalTagProcessor{
		tp: newTableTagProcessor(),
	}
}

func (p *globalTagProcessor) processTag(cfg ddprofiledefinition.MetricTagConfig, pdus map[string]gosnmp.SnmpPDU, tc mapTagCollector) error {
	pdu, ok := pdus[trimOID(cfg.Symbol.OID)]
	if !ok {
		return nil
	}
	return p.tp.processTag(cfg, pdu, tc)
}

type tableTagProcessor struct{}

func newTableTagProcessor() *tableTagProcessor {
	return &tableTagProcessor{}
}

func (p *tableTagProcessor) processTag(cfg ddprofiledefinition.MetricTagConfig, pdu gosnmp.SnmpPDU, tc mapTagCollector) error {
	tagName := ternary(cfg.Tag != "", cfg.Tag, cfg.Symbol.Name)
	if tagName == "" {
		return nil
	}

	val, err := convPduToStringf(pdu, cfg.Symbol.Format)
	if err != nil {
		return err
	}

	switch {
	case len(cfg.Mapping) > 0:
		if v, ok := cfg.Mapping[val]; ok {
			val = v
		}
		tc.addTag(tagName, val)
	case cfg.Pattern != nil:
		if sm := cfg.Pattern.FindStringSubmatch(val); len(sm) > 0 {
			for name, tmpl := range cfg.Tags {
				tc.addTag(name, replaceSubmatches(tmpl, sm))
			}
		}
	case cfg.Symbol.ExtractValueCompiled != nil:
		if sm := cfg.Symbol.ExtractValueCompiled.FindStringSubmatch(val); len(sm) > 1 {
			tc.addTag(tagName, sm[1])
		}
	case cfg.Symbol.MatchPatternCompiled != nil:
		if sm := cfg.Symbol.MatchPatternCompiled.FindStringSubmatch(val); len(sm) > 0 {
			tc.addTag(tagName, replaceSubmatches(cfg.Symbol.MatchValue, sm))
		}
	default:
		tc.addTag(tagName, val)
	}

	return nil
}
