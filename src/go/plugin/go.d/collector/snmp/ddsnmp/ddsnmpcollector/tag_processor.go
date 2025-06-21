// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"github.com/gosnmp/gosnmp"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

type globalTagProcessor struct {
	tp *tableTagProcessor
}

func newGlobalTagProcessor() *globalTagProcessor {
	return &globalTagProcessor{
		tp: newTableTagProcessor(),
	}
}

func (p *globalTagProcessor) processTag(cfg ddprofiledefinition.MetricTagConfig, pdus map[string]gosnmp.SnmpPDU) (map[string]string, error) {
	pdu, ok := pdus[trimOID(cfg.Symbol.OID)]
	if !ok {
		return nil, nil
	}
	return p.tp.processTag(cfg, pdu)
}

// tagProcessorFactory selects the appropriate processor
type tableTagProcessor struct {
	mappingProcessor *mappingTagProcessor
	patternProcessor *patternTagProcessor
	extractProcessor *extractValueTagProcessor
	matchProcessor   *matchPatternTagProcessor
	defaultProcessor *defaultTagProcessor
}

func newTableTagProcessor() *tableTagProcessor {
	return &tableTagProcessor{
		mappingProcessor: &mappingTagProcessor{},
		patternProcessor: &patternTagProcessor{},
		extractProcessor: &extractValueTagProcessor{},
		matchProcessor:   &matchPatternTagProcessor{},
		defaultProcessor: &defaultTagProcessor{},
	}
}

func (p *tableTagProcessor) processTag(cfg ddprofiledefinition.MetricTagConfig, pdu gosnmp.SnmpPDU) (map[string]string, error) {
	tagName := ternary(cfg.Tag != "", cfg.Tag, cfg.Symbol.Name)
	if tagName == "" {
		return nil, nil
	}

	val, err := convPduToStringf(pdu, cfg.Symbol.Format)
	if err != nil {
		return nil, err
	}

	switch {
	case len(cfg.Mapping) > 0:
		return p.mappingProcessor.processTag(tagName, val, cfg)
	case cfg.Pattern != nil:
		return p.patternProcessor.processTag(val, cfg)
	case cfg.Symbol.ExtractValueCompiled != nil:
		return p.extractProcessor.processTag(tagName, val, cfg)
	case cfg.Symbol.MatchPatternCompiled != nil:
		return p.matchProcessor.processTag(tagName, val, cfg)
	default:
		return p.defaultProcessor.processTag(tagName, val)
	}
}

// mappingTagProcessor handles simple value mapping
type mappingTagProcessor struct{}

func (p *mappingTagProcessor) processTag(tagName, value string, cfg ddprofiledefinition.MetricTagConfig) (map[string]string, error) {
	tags := make(map[string]string)

	if v, ok := cfg.Mapping[value]; ok {
		value = v
	}
	tags[tagName] = value

	return tags, nil
}

// patternTagProcessor handles regex pattern matching with multiple tags
type patternTagProcessor struct{}

func (p *patternTagProcessor) processTag(value string, cfg ddprofiledefinition.MetricTagConfig) (map[string]string, error) {
	tags := make(map[string]string)

	if sm := cfg.Pattern.FindStringSubmatch(value); len(sm) > 0 {
		for name, tmpl := range cfg.Tags {
			tags[name] = replaceSubmatches(tmpl, sm)
		}
	}

	return tags, nil
}

// extractValueTagProcessor handles extract_value pattern
type extractValueTagProcessor struct{}

func (p *extractValueTagProcessor) processTag(tagName, value string, cfg ddprofiledefinition.MetricTagConfig) (map[string]string, error) {
	tags := make(map[string]string)

	if sm := cfg.Symbol.ExtractValueCompiled.FindStringSubmatch(value); len(sm) > 1 {
		tags[tagName] = sm[1]
	}

	return tags, nil
}

// matchPatternTagProcessor handles match_pattern and match_value
type matchPatternTagProcessor struct{}

func (p *matchPatternTagProcessor) processTag(tagName, value string, cfg ddprofiledefinition.MetricTagConfig) (map[string]string, error) {
	tags := make(map[string]string)

	if sm := cfg.Symbol.MatchPatternCompiled.FindStringSubmatch(value); len(sm) > 0 {
		tags[tagName] = replaceSubmatches(cfg.Symbol.MatchValue, sm)
	}

	return tags, nil
}

// defaultTagProcessor handles tags with no transformation
type defaultTagProcessor struct{}

func (p *defaultTagProcessor) processTag(tagName, value string) (map[string]string, error) {
	return map[string]string{tagName: value}, nil
}
