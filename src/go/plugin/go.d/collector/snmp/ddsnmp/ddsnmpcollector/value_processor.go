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
