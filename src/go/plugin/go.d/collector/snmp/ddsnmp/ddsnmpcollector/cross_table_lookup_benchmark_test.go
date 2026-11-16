// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/gosnmp/gosnmp"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

// The legacy direct path remains O(rows), including allocations. ns/op is a local
// trend, not a CI gate.
func BenchmarkCrossTableResolver_Direct(b *testing.B) {
	for _, rows := range []int{32, 4096} {
		b.Run(fmt.Sprintf("rows=%d", rows), func(b *testing.B) {
			tag := ddprofiledefinition.MetricTagConfig{
				Tag:   "label",
				Table: "target",
				Symbol: ddprofiledefinition.SymbolConfigCompat{
					OID:  "1.2.1",
					Name: "label",
				},
			}
			pdus := make(map[string]gosnmp.SnmpPDU)
			indexes := make([]string, rows)
			for i := range rows {
				indexes[i] = strconv.Itoa(i + 1)
				pdus["1.2.1."+indexes[i]] = createStringPDU("1.2.1."+indexes[i], "label")
			}
			ctx := newCrossTableContext(
				map[string]map[string]gosnmp.SnmpPDU{"1.2": pdus},
				map[string]string{"target": "1.2"},
			)
			ctx.rowTags = make(map[string]string)
			r := newCrossTableResolver(logger.New())
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				for _, index := range indexes {
					if err := r.resolveCrossTableTag(tag, index, ctx, nil); err != nil {
						b.Fatal(err)
					}
				}
			}
		})
	}
}

func BenchmarkCrossTableResolver_JuniperPeerFamilyLookupScaling(b *testing.B) {
	const (
		refTableOID     = "1.3.6.1.4.1.2636.5.1.1.2.1.1"
		lookupColumnOID = refTableOID + ".1.14"
		targetFields    = 8
	)

	for _, peers := range []int{32, 128, 512} {
		b.Run(fmt.Sprintf("peers=%d", peers), func(b *testing.B) {
			resolver := newCrossTableResolver(logger.New())
			refTablePDUs := make(map[string]gosnmp.SnmpPDU, peers*(targetFields+1))
			lookupValues := make([]string, peers)
			tagConfigs := make([]ddprofiledefinition.MetricTagConfig, targetFields)

			for field := range targetFields {
				tagConfigs[field] = lookupTestTagConfig(
					fmt.Sprintf("field_%d", field),
					fmt.Sprintf("%s.1.%d", refTableOID, field+20),
					"peer_index",
					lookupColumnOID,
				)
				tagConfigs[field].LookupSymbol.Format = ""
				tagConfigs[field].LookupSymbol.ExtractValue = ""
				tagConfigs[field].LookupSymbol.ExtractValueCompiled = nil
			}

			for peer := range peers {
				rowIndex := strconv.Itoa(peer + 1)
				lookupValues[peer] = rowIndex
				lookupOID := lookupColumnOID + "." + rowIndex
				refTablePDUs[lookupOID] = createStringPDU(lookupOID, rowIndex)

				for field, tagCfg := range tagConfigs {
					targetOID := trimOID(tagCfg.Symbol.OID) + "." + rowIndex
					refTablePDUs[targetOID] = createGauge32PDU(targetOID, uint(field+1))
				}
			}

			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				ctx := newCrossTableContext(nil, nil)
				for _, lookupValue := range lookupValues {
					for _, tagCfg := range tagConfigs {
						if _, err := resolver.resolveLookupIndexByValue(
							tagCfg,
							lookupValue,
							refTableOID,
							refTablePDUs,
							ctx,
						); err != nil {
							b.Fatal(err)
						}
					}
				}
			}
		})
	}
}
