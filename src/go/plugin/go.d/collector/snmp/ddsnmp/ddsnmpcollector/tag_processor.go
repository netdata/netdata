// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"errors"

	"github.com/gosnmp/gosnmp"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

type tagAdder struct {
	processing *processingObserver
	tags       map[string]string
	observed   *bool
}

func (ta *tagAdder) addTags(tags map[string]string) {
	for k, v := range tags {
		ta.addTag(k, v)
	}
}

func (ta *tagAdder) addTag(key, value string) {
	if ta.observed != nil && value != "" {
		*ta.observed = true
	}
	if existing, ok := ta.tags[key]; !ok || existing == "" {
		ta.tags[key] = value
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

func (p *globalTagProcessor) processTag(cfg ddprofiledefinition.MetricTagConfig, pdus map[string]gosnmp.SnmpPDU, ta tagAdder) error {
	pdu, ok := pdus[trimOID(cfg.Symbol.OID)]
	if !ok {
		ta.processing.record(metricTagDisplayName(cfg), cfg.Symbol.OID, "missing_input")
		return nil
	}
	return p.tp.processTag(cfg, pdu, ta)
}

func (p *globalTagProcessor) processTagObserved(
	cfg ddprofiledefinition.MetricTagConfig,
	pdus map[string]gosnmp.SnmpPDU,
	ta tagAdder,
) (bool, error) {
	pdu, ok := pdus[trimOID(cfg.Symbol.OID)]
	if !ok {
		ta.processing.record(metricTagDisplayName(cfg), cfg.Symbol.OID, "missing_input")
		return false, nil
	}
	observed := false
	ta.observed = &observed
	err := p.tp.processTag(cfg, pdu, ta)
	return observed, err
}

type tableTagProcessor struct{}

func newTableTagProcessor() *tableTagProcessor {
	return &tableTagProcessor{}
}

func (p *tableTagProcessor) processTag(cfg ddprofiledefinition.MetricTagConfig, pdu gosnmp.SnmpPDU, ta tagAdder) error {
	tagName := ternary(cfg.Tag != "", cfg.Tag, cfg.Symbol.Name)
	if tagName == "" {
		return nil
	}

	val, err := convPduToStringf(pdu, cfg.Symbol.Format)
	if err != nil {
		if errors.Is(err, errNoTextDateValue) {
			ta.processing.record(tagName, pdu.Name, "empty_date")
			return nil
		}
		ta.processing.record(tagName, pdu.Name, "conversion")
		return err
	}

	if cfg.Symbol.ExtractValueCompiled != nil {
		if sm := cfg.Symbol.ExtractValueCompiled.FindStringSubmatch(val); len(sm) > 1 {
			val = sm[1]
		}
	}
	if cfg.Symbol.MatchPatternCompiled != nil {
		sm := cfg.Symbol.MatchPatternCompiled.FindStringSubmatch(val)
		if len(sm) == 0 {
			ta.processing.record(tagName, pdu.Name, "pattern_mismatch")
			return nil
		}
		val = replaceSubmatches(cfg.Symbol.MatchValue, sm)
	}
	if mapped, ok := cfg.Mapping.Lookup(val); ok {
		val = mapped
	}
	if cfg.Pattern != nil {
		if sm := cfg.Pattern.FindStringSubmatch(val); len(sm) > 0 {
			for name, tmpl := range cfg.Tags {
				ta.addTag(name, replaceSubmatches(tmpl, sm))
			}
		} else {
			ta.processing.record(tagName, pdu.Name, "pattern_mismatch")
		}
	} else {
		ta.addTag(tagName, val)
	}

	return nil
}
