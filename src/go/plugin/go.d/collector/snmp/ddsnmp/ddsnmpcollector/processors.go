// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"fmt"
	"strconv"

	"github.com/gosnmp/gosnmp"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

// ValueProcessor is the interface for different value processing strategies
type ValueProcessor interface {
	Process(pdu gosnmp.SnmpPDU, sym ddprofiledefinition.SymbolConfig) (int64, error)
}

// NumericValueProcessor handles numeric PDU types
type NumericValueProcessor struct{}

func (p *NumericValueProcessor) Process(pdu gosnmp.SnmpPDU, sym ddprofiledefinition.SymbolConfig) (int64, error) {
	switch pdu.Type {
	case gosnmp.OpaqueFloat:
		return p.processOpaqueFloat(pdu, sym)
	case gosnmp.OpaqueDouble:
		return p.processOpaqueDouble(pdu, sym)
	default:
		return p.processInteger(pdu, sym)
	}
}

func (p *NumericValueProcessor) processOpaqueFloat(pdu gosnmp.SnmpPDU, sym ddprofiledefinition.SymbolConfig) (int64, error) {
	floatVal, ok := pdu.Value.(float32)
	if !ok {
		return 0, fmt.Errorf("OpaqueFloat has unexpected type %T", pdu.Value)
	}

	if sym.ScaleFactor != 0 {
		return int64(float64(floatVal) * sym.ScaleFactor), nil
	}
	return int64(floatVal), nil
}

func (p *NumericValueProcessor) processOpaqueDouble(pdu gosnmp.SnmpPDU, sym ddprofiledefinition.SymbolConfig) (int64, error) {
	floatVal, ok := pdu.Value.(float64)
	if !ok {
		return 0, fmt.Errorf("OpaqueDouble has unexpected type %T", pdu.Value)
	}

	if sym.ScaleFactor != 0 {
		return int64(floatVal * sym.ScaleFactor), nil
	}
	return int64(floatVal), nil
}

func (p *NumericValueProcessor) processInteger(pdu gosnmp.SnmpPDU, sym ddprofiledefinition.SymbolConfig) (int64, error) {
	value := gosnmp.ToBigInt(pdu.Value).Int64()

	// Apply numeric mapping if exists
	if len(sym.Mapping) > 0 {
		s := strconv.FormatInt(value, 10)
		if v, ok := sym.Mapping[s]; ok && isInt(v) {
			value, _ = strconv.ParseInt(v, 10, 64)
		}
	}

	// Apply scale factor
	if sym.ScaleFactor != 0 {
		value = int64(float64(value) * sym.ScaleFactor)
	}

	return value, nil
}

// StringValueProcessor handles string-based PDU types
type StringValueProcessor struct{}

func (p *StringValueProcessor) Process(pdu gosnmp.SnmpPDU, sym ddprofiledefinition.SymbolConfig) (int64, error) {
	s, err := convPduToStringf(pdu, sym.Format)
	if err != nil {
		return 0, err
	}

	// Apply extract_value pattern
	if sym.ExtractValueCompiled != nil {
		if sm := sym.ExtractValueCompiled.FindStringSubmatch(s); len(sm) > 1 {
			s = sm[1]
		}
	}

	// Apply match_pattern and match_value
	if sym.MatchPatternCompiled != nil {
		if sm := sym.MatchPatternCompiled.FindStringSubmatch(s); len(sm) > 0 {
			s = replaceSubmatches(sym.MatchValue, sm)
		}
	}

	// Apply string to int mapping
	if v, ok := sym.Mapping[s]; ok && isInt(v) {
		s = v
	}

	value, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("cannot convert '%s' to int64: %w", s, err)
	}

	if sym.ScaleFactor != 0 {
		value = int64(float64(value) * sym.ScaleFactor)
	}

	return value, nil
}

// ValueProcessorFactory selects the appropriate processor based on PDU type
type ValueProcessorFactory struct {
	numericProcessor *NumericValueProcessor
	stringProcessor  *StringValueProcessor
}

func NewValueProcessorFactory() *ValueProcessorFactory {
	return &ValueProcessorFactory{
		numericProcessor: &NumericValueProcessor{},
		stringProcessor:  &StringValueProcessor{},
	}
}

func (f *ValueProcessorFactory) GetProcessor(pdu gosnmp.SnmpPDU) ValueProcessor {
	if isPduNumericType(pdu) {
		return f.numericProcessor
	}
	return f.stringProcessor
}

// processSymbolValue is the main entry point for value processing
func processSymbolValue(sym ddprofiledefinition.SymbolConfig, pdu gosnmp.SnmpPDU) (int64, error) {
	return NewValueProcessorFactory().
		GetProcessor(pdu).
		Process(pdu, sym)
}

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
