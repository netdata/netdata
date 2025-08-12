// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"fmt"
	"strconv"

	"github.com/gosnmp/gosnmp"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

// valueProcessorFactory selects the appropriate processor based on PDU type
type valueProcessor struct {
	numericProcessor *numericValueProcessor
	stringProcessor  *stringValueProcessor
}

func newValueProcessor() *valueProcessor {
	return &valueProcessor{
		numericProcessor: &numericValueProcessor{},
		stringProcessor:  &stringValueProcessor{},
	}
}

func (p *valueProcessor) processValue(sym ddprofiledefinition.SymbolConfig, pdu gosnmp.SnmpPDU) (int64, error) {
	if isPduNumericType(pdu) {
		return p.numericProcessor.processValue(sym, pdu)
	}
	return p.stringProcessor.processValue(sym, pdu)
}

// numericValueProcessor handles numeric PDU types
type numericValueProcessor struct{}

func (p *numericValueProcessor) processValue(sym ddprofiledefinition.SymbolConfig, pdu gosnmp.SnmpPDU) (int64, error) {
	switch pdu.Type {
	case gosnmp.OpaqueFloat:
		return p.processOpaqueFloat(sym, pdu)
	case gosnmp.OpaqueDouble:
		return p.processOpaqueDouble(sym, pdu)
	default:
		return p.processInteger(sym, pdu)
	}
}

func (p *numericValueProcessor) processOpaqueFloat(sym ddprofiledefinition.SymbolConfig, pdu gosnmp.SnmpPDU) (int64, error) {
	floatVal, ok := pdu.Value.(float32)
	if !ok {
		return 0, fmt.Errorf("OpaqueFloat has unexpected type %T", pdu.Value)
	}

	if sym.ScaleFactor != 0 {
		return int64(float64(floatVal) * sym.ScaleFactor), nil
	}
	return int64(floatVal), nil
}

func (p *numericValueProcessor) processOpaqueDouble(sym ddprofiledefinition.SymbolConfig, pdu gosnmp.SnmpPDU) (int64, error) {
	floatVal, ok := pdu.Value.(float64)
	if !ok {
		return 0, fmt.Errorf("OpaqueDouble has unexpected type %T", pdu.Value)
	}

	if sym.ScaleFactor != 0 {
		return int64(floatVal * sym.ScaleFactor), nil
	}
	return int64(floatVal), nil
}

func (p *numericValueProcessor) processInteger(sym ddprofiledefinition.SymbolConfig, pdu gosnmp.SnmpPDU) (int64, error) {
	value := gosnmp.ToBigInt(pdu.Value).Int64()

	if len(sym.Mapping) > 0 {
		s := strconv.FormatInt(value, 10)
		if v, ok := sym.Mapping[s]; ok && isInt(v) {
			value, _ = strconv.ParseInt(v, 10, 64)
		}
	}

	if sym.ScaleFactor != 0 {
		value = int64(float64(value) * sym.ScaleFactor)
	}

	return value, nil
}

// stringValueProcessor handles string-based PDU types
type stringValueProcessor struct{}

func (p *stringValueProcessor) processValue(sym ddprofiledefinition.SymbolConfig, pdu gosnmp.SnmpPDU) (int64, error) {
	s, err := convPduToStringf(pdu, sym.Format)
	if err != nil {
		return 0, err
	}

	if sym.ExtractValueCompiled != nil {
		if sm := sym.ExtractValueCompiled.FindStringSubmatch(s); len(sm) > 1 {
			s = sm[1]
		}
	}

	if sym.MatchPatternCompiled != nil {
		sm := sym.MatchPatternCompiled.FindStringSubmatch(s)
		if len(sm) == 0 {
			// Pattern didn't match - cannot extract expected value
			return 0, fmt.Errorf("match_pattern '%s' did not match value '%s'", sym.MatchPattern, s)
		}
		s = replaceSubmatches(sym.MatchValue, sm)
	}

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
