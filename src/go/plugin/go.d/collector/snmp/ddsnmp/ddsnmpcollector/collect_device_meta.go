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

	for resName, cfg := range prof.Definition.Metadata {
		if !ddprofiledefinition.IsMetadataResourceWithScalarOids(resName) {
			continue
		}
		for name, field := range cfg.Fields {
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

	for resName, cfg := range prof.Definition.Metadata {
		if !ddprofiledefinition.IsMetadataResourceWithScalarOids(resName) {
			continue
		}
		for name, field := range cfg.Fields {
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

func processSymbolTagValue(cfg ddprofiledefinition.SymbolConfig, result map[string]gosnmp.SnmpPDU) (string, error) {
	pdu, ok := result[trimOID(cfg.OID)]
	if !ok {
		return "", nil
	}

	val, err := convPduToStringf(pdu, cfg.Format)
	if err != nil {
		return "", err
	}

	switch {
	case cfg.ExtractValueCompiled != nil:
		if sm := cfg.ExtractValueCompiled.FindStringSubmatch(val); len(sm) > 1 {
			val = sm[1]
		}
	case cfg.MatchPatternCompiled != nil:
		if sm := cfg.MatchPatternCompiled.FindStringSubmatch(val); len(sm) > 0 {
			val = replaceSubmatches(cfg.MatchValue, sm)
		}
	}

	if v, ok := cfg.Mapping[val]; ok {
		val = v
	}

	return val, nil
}
