// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"errors"
	"fmt"
	"slices"

	"github.com/gosnmp/gosnmp"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

func (c *Collector) collectDeviceMetadata(prof *ddsnmp.Profile) (map[string]string, error) {
	if len(prof.Definition.Metadata) == 0 {
		return nil, nil
	}

	var tagOIDs []string
	tags := make(map[string]string)

	for resName, metaCfg := range prof.Definition.Metadata {
		if !ddprofiledefinition.IsMetadataResourceWithScalarOids(resName) {
			continue
		}
		for name, field := range metaCfg.Fields {
			switch {
			case field.Value != "":
				tags[name] = field.Value
			case field.Symbol.OID != "":
				tagOIDs = append(tagOIDs, field.Symbol.OID)
			case len(field.Symbols) > 0:
				for _, sym := range field.Symbols {
					if sym.OID != "" {
						tagOIDs = append(tagOIDs, sym.OID)
					}
				}
			}
		}
	}

	slices.Sort(tagOIDs)
	tagOIDs = slices.Compact(tagOIDs)

	if len(tagOIDs) == 0 {
		return ternary(len(tags) > 0, tags, nil), nil
	}

	pdus, err := c.snmpGet(tagOIDs)
	if err != nil {
		return nil, err
	}

	var errs []error

	for resName, metaCfg := range prof.Definition.Metadata {
		if !ddprofiledefinition.IsMetadataResourceWithScalarOids(resName) {
			continue
		}
		for name, field := range metaCfg.Fields {
			switch {
			case field.Symbol.OID != "":
				v, err := processSymbolTagValue(field.Symbol, pdus)
				if err != nil {
					errs = append(errs, fmt.Errorf("failed to process meta device tag value for '%s': %v", name, err))
					continue
				}
				tags[name] = v
			case len(field.Symbols) > 0:
				for _, sym := range field.Symbols {
					v, err := processSymbolTagValue(sym, pdus)
					if err != nil {
						errs = append(errs, fmt.Errorf("failed to process meta device tag value for '%s': %v", name, err))
						continue
					}
					tags[name] = v
				}
			}
		}
	}

	if len(errs) > 0 && len(tags) == 0 {
		return nil, fmt.Errorf("failed to process any meta device tags: %v", errors.Join(errs...))
	}

	return tags, nil
}

func processSymbolTagValue(symCfg ddprofiledefinition.SymbolConfig, result map[string]gosnmp.SnmpPDU) (string, error) {
	pdu, ok := result[trimOID(symCfg.OID)]
	if !ok {
		return "", nil
	}

	val, err := convPduToStringf(pdu, symCfg.Format)
	if err != nil {
		return "", err
	}

	switch {
	case symCfg.ExtractValueCompiled != nil:
		if sm := symCfg.ExtractValueCompiled.FindStringSubmatch(val); len(sm) > 1 {
			return sm[1], nil
		}
	case symCfg.MatchPatternCompiled != nil:
		if sm := symCfg.MatchPatternCompiled.FindStringSubmatch(val); len(sm) > 0 {
			return replaceSubmatches(symCfg.MatchValue, sm), nil
		}
	}
	return val, nil
}
