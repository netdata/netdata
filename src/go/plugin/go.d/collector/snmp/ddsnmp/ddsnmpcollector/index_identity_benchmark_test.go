// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

type decodedTagWorkload struct {
	name, index, want string
	config            ddprofiledefinition.MetricTagConfig
}

func decodedTagWorkloads() []decodedTagWorkload {
	fixed48 := uint(48)
	octets := func(n int) string { return strings.TrimSuffix(strings.Repeat("97.", n), ".") }
	config := func(component uint, components ...ddprofiledefinition.MetricIndexComponent) ddprofiledefinition.MetricTagConfig {
		return ddprofiledefinition.MetricTagConfig{
			Tag:         "identity",
			IndexDecode: &ddprofiledefinition.MetricIndexDecode{Component: component, Components: components},
			Symbol:      ddprofiledefinition.SymbolConfigCompat{Format: "octet_string"},
		}
	}
	integer := ddprofiledefinition.MetricIndexComponent{Type: "integer"}
	variable := ddprofiledefinition.MetricIndexComponent{Type: "octet_string"}
	return []decodedTagWorkload{
		{
			name:   "multiple-variable-strings",
			index:  "3.70.79.79.3.98.97.114.2",
			want:   "bar",
			config: config(2, variable, variable, integer),
		},
		{
			name:   "variable-112-octets",
			index:  "1.112." + octets(112),
			want:   strings.Repeat("a", 112),
			config: config(2, integer, variable),
		},
		{
			name:  "fixed-48-octets",
			index: octets(48) + ".7",
			want:  strings.Repeat("a", 48),
			config: config(
				1,
				ddprofiledefinition.MetricIndexComponent{Type: "octet_string", Length: &fixed48},
				integer,
			),
		},
	}
}

// Full-layout validation and final text formatting are O(index arcs), with a constant allocation count. These
// opt-in layouts do not add decoding work to legacy single-index tags. ns/op is a workstation trend, not a CI gate.
func BenchmarkIndexIdentityDecodedTag(b *testing.B) {
	processor := newTableRowProcessor(logger.New())
	for _, tc := range decodedTagWorkloads() {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				_, value, err := processor.processIndexTag(tc.config, tc.index)
				if err != nil || value != tc.want {
					b.Fatalf("decoded identity %q: %v", value, err)
				}
			}
		})
	}
}

type scopedIdentityWorkload struct {
	resolver                                   *crossTableResolver
	config                                     ddprofiledefinition.MetricTagConfig
	pdus                                       map[string]gosnmp.SnmpPDU
	sourceIndexes, lookupValues, targetIndexes []string
}

func newScopedIdentityWorkload(rows int) scopedIdentityWorkload {
	const root = "1.3.6.1.4.1.99999.990"
	w := scopedIdentityWorkload{
		resolver: newCrossTableResolver(logger.New()),
		config: ddprofiledefinition.MetricTagConfig{
			Tag: "description", Table: "target", Symbol: ddprofiledefinition.SymbolConfigCompat{OID: root + ".1.2", Name: "description"},
			LookupSymbol: ddprofiledefinition.SymbolConfigCompat{OID: root + ".1.1", Name: "address"},
			LookupScope: &ddprofiledefinition.MetricLookupScope{
				SourceIndexTransform: []ddprofiledefinition.MetricIndexTransform{{Start: 1, End: 1}},
				TargetIndexTransform: []ddprofiledefinition.MetricIndexTransform{{Start: 0, End: 0}},
			},
		},
		pdus: make(map[string]gosnmp.SnmpPDU, rows*2),
	}
	// Every peer value repeats in eight VRFs. The scope must select the matching VRF without scanning target rows.
	for i := range rows {
		scope, peer := strconv.Itoa(i%8+1), strconv.Itoa(i/8+1)
		targetIndex := scope + "." + peer
		w.sourceIndexes = append(w.sourceIndexes, peer+"."+scope)
		w.lookupValues = append(w.lookupValues, "peer_"+peer)
		w.targetIndexes = append(w.targetIndexes, targetIndex)
		lookupOID := root + ".1.1." + targetIndex
		w.pdus[lookupOID] = createStringPDU(lookupOID, "peer_"+peer)
		valueOID := root + ".1.2." + targetIndex
		w.pdus[valueOID] = createStringPDU(valueOID, "description_"+targetIndex)
	}
	return w
}

func (w scopedIdentityWorkload) query(ctx *crossTableContext, i int) (string, error) {
	return w.resolver.resolveLookupIndexByValue(
		w.config,
		w.lookupValues[i],
		w.sourceIndexes[i],
		"1.3.6.1.4.1.99999.990",
		w.pdus,
		ctx,
	)
}

// One reverse index is built per collection: O(target rows + source queries) time and O(target rows) memory.
// The warm-query case reuses that collection-owned index; its cost and allocations must not grow with target rows.
func BenchmarkIndexIdentityScopedLookup(b *testing.B) {
	for _, rows := range []int{32, 512, 4096} {
		workload := newScopedIdentityWorkload(rows)
		b.Run(fmt.Sprintf("collection/rows=%d", rows), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				ctx := newCrossTableContext(nil, nil)
				for i, want := range workload.targetIndexes {
					got, err := workload.query(ctx, i)
					if err != nil || got != want {
						b.Fatalf("scope result %q, want %q: %v", got, want, err)
					}
				}
			}
		})
		b.Run(fmt.Sprintf("warm-query/rows=%d", rows), func(b *testing.B) {
			ctx := newCrossTableContext(nil, nil)
			got, err := workload.query(ctx, 0)
			require.NoError(b, err)
			require.Equal(b, workload.targetIndexes[0], got)
			b.ReportAllocs()
			i := 0
			for b.Loop() {
				got, err := workload.query(ctx, i)
				if err != nil || got != workload.targetIndexes[i] {
					b.Fatalf("scope result %q: %v", got, err)
				}
				i = (i + 1) % rows
			}
		})
	}
}

func TestIndexIdentityAllocationEnvelopes(t *testing.T) {
	processor := newTableRowProcessor(logger.New())
	legacy := ddprofiledefinition.MetricTagConfig{Tag: "identity", Index: 1}
	var value string
	var err error
	legacyAllocs := testing.AllocsPerRun(100, func() { _, value, err = processor.processIndexTag(legacy, "1.2.3.4") })
	require.NoError(t, err)
	require.Equal(t, "1", value)
	require.Zero(t, legacyAllocs, "single-index common path stays allocation-free")
	for _, tc := range decodedTagWorkloads() {
		t.Run(tc.name, func(t *testing.T) {
			allocs := testing.AllocsPerRun(
				100,
				func() { _, value, err = processor.processIndexTag(tc.config, tc.index) },
			)
			require.NoError(t, err)
			require.Equal(t, tc.want, value)
			require.LessOrEqual(
				t,
				allocs,
				float64(5),
				"split/layout selection and final octet conversion have a constant allocation envelope",
			)
			t.Logf("decoded tag: %.0f allocs", allocs)
		})
	}
	for _, rows := range []int{32, 512, 4096} {
		t.Run(fmt.Sprintf("scope/rows=%d", rows), func(t *testing.T) {
			workload := newScopedIdentityWorkload(rows)
			ctx := newCrossTableContext(nil, nil)
			got, err := workload.query(ctx, 0)
			require.NoError(t, err)
			require.Equal(t, workload.targetIndexes[0], got)
			allocs := testing.AllocsPerRun(100, func() { value, err = workload.query(ctx, rows-1) })
			require.NoError(t, err)
			require.Equal(t, workload.targetIndexes[rows-1], value)
			require.LessOrEqual(
				t,
				allocs,
				float64(4),
				"warm scoped queries must not allocate or scan proportional to target rows",
			)
			t.Logf("warm scoped query: %.0f allocs", allocs)
			coldAllocs := testing.AllocsPerRun(5, func() {
				collection := newCrossTableContext(nil, nil)
				for i := range rows {
					value, err = workload.query(collection, i)
					if err != nil {
						return
					}
				}
			})
			require.NoError(t, err)
			require.LessOrEqual(
				t,
				coldAllocs,
				float64(rows*9+32),
				"a collection builds a linear reverse index once, not once per source query",
			)
			t.Logf("collection: %.0f allocs (%.2f per row)", coldAllocs, coldAllocs/float64(rows))
		})
	}
}
