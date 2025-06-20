// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"fmt"

	"github.com/gosnmp/gosnmp"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

// TagProcessor processes tag values from PDUs
type TagProcessor interface {
	Process(tagName, value string, cfg ddprofiledefinition.MetricTagConfig) (map[string]string, error)
}

// MappingTagProcessor handles simple value mapping
type MappingTagProcessor struct{}

func (p *MappingTagProcessor) Process(tagName, value string, cfg ddprofiledefinition.MetricTagConfig) (map[string]string, error) {
	tags := make(map[string]string)

	if v, ok := cfg.Mapping[value]; ok {
		value = v
	}
	tags[tagName] = value

	return tags, nil
}

// PatternTagProcessor handles regex pattern matching with multiple tags
type PatternTagProcessor struct{}

func (p *PatternTagProcessor) Process(tagName, value string, cfg ddprofiledefinition.MetricTagConfig) (map[string]string, error) {
	tags := make(map[string]string)

	if sm := cfg.Pattern.FindStringSubmatch(value); len(sm) > 0 {
		for name, tmpl := range cfg.Tags {
			tags[name] = replaceSubmatches(tmpl, sm)
		}
	}

	return tags, nil
}

// ExtractValueTagProcessor handles extract_value pattern
type ExtractValueTagProcessor struct{}

func (p *ExtractValueTagProcessor) Process(tagName, value string, cfg ddprofiledefinition.MetricTagConfig) (map[string]string, error) {
	tags := make(map[string]string)

	if sm := cfg.Symbol.ExtractValueCompiled.FindStringSubmatch(value); len(sm) > 1 {
		tags[tagName] = sm[1]
	}

	return tags, nil
}

// MatchPatternTagProcessor handles match_pattern and match_value
type MatchPatternTagProcessor struct{}

func (p *MatchPatternTagProcessor) Process(tagName, value string, cfg ddprofiledefinition.MetricTagConfig) (map[string]string, error) {
	tags := make(map[string]string)

	if sm := cfg.Symbol.MatchPatternCompiled.FindStringSubmatch(value); len(sm) > 0 {
		tags[tagName] = replaceSubmatches(cfg.Symbol.MatchValue, sm)
	}

	return tags, nil
}

// DefaultTagProcessor handles tags with no transformation
type DefaultTagProcessor struct{}

func (p *DefaultTagProcessor) Process(tagName, value string, cfg ddprofiledefinition.MetricTagConfig) (map[string]string, error) {
	return map[string]string{tagName: value}, nil
}

// TagProcessorFactory selects the appropriate processor
type TagProcessorFactory struct {
	mappingProcessor *MappingTagProcessor
	patternProcessor *PatternTagProcessor
	extractProcessor *ExtractValueTagProcessor
	matchProcessor   *MatchPatternTagProcessor
	defaultProcessor *DefaultTagProcessor
}

func NewTagProcessorFactory() *TagProcessorFactory {
	return &TagProcessorFactory{
		mappingProcessor: &MappingTagProcessor{},
		patternProcessor: &PatternTagProcessor{},
		extractProcessor: &ExtractValueTagProcessor{},
		matchProcessor:   &MatchPatternTagProcessor{},
		defaultProcessor: &DefaultTagProcessor{},
	}
}

func (f *TagProcessorFactory) GetProcessor(cfg ddprofiledefinition.MetricTagConfig) TagProcessor {
	switch {
	case len(cfg.Mapping) > 0:
		return f.mappingProcessor
	case cfg.Pattern != nil:
		return f.patternProcessor
	case cfg.Symbol.ExtractValueCompiled != nil:
		return f.extractProcessor
	case cfg.Symbol.MatchPatternCompiled != nil:
		return f.matchProcessor
	default:
		return f.defaultProcessor
	}
}

// processTableMetricTagValue is the main entry point for tag processing
func processTableMetricTagValue(cfg ddprofiledefinition.MetricTagConfig, pdu gosnmp.SnmpPDU) (map[string]string, error) {
	// Convert PDU to string
	val, err := convPduToStringf(pdu, cfg.Symbol.Format)
	if err != nil {
		return nil, err
	}

	// Determine tag name
	tagName := ternary(cfg.Tag != "", cfg.Tag, cfg.Symbol.Name)

	// Process the tag value
	factory := NewTagProcessorFactory()
	processor := factory.GetProcessor(cfg)
	return processor.Process(tagName, val, cfg)
}

// processMetricTagValue processes global metric tags
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

	factory := NewTagProcessorFactory()
	processor := factory.GetProcessor(cfg)
	return processor.Process(tagName, val, cfg)
}

// processIndexTag processes index-based tags
func processIndexTag(cfg ddprofiledefinition.MetricTagConfig, index string) (string, string, bool) {
	indexValue, ok := getIndexPosition(index, cfg.Index)
	if !ok {
		return "", "", false
	}

	tagName := ternary(cfg.Tag != "", cfg.Tag, fmt.Sprintf("index%d", cfg.Index))

	if v, ok := cfg.Mapping[indexValue]; ok {
		indexValue = v
	}

	return tagName, indexValue, true
}
