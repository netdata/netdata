// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/gosnmp/gosnmp"
	snmpmock "github.com/gosnmp/gosnmp/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

const (
	oidPethPsePortEntry    = "1.3.6.1.2.1.105.1.1.1"
	oidCpeExtPsePortEntry  = "1.3.6.1.4.1.9.9.402.1.2.1"
	oidPethMainPseEntry    = "1.3.6.1.2.1.105.1.3.1.1"
	ciscoCat29xxStackSysID = "1.3.6.1.4.1.9.1.1208"
)

type poeFixtureRow struct {
	Tags   map[string]string
	Values map[string]int64
}

// The fixture is a 3-member Catalyst 2960-X stack: PoE rows are indexed (group, port) with the port numbered within
// its stack member, so each row must keep its own chart identity.
func TestCollector_Collect_CiscoCatalystPoE_FromLibreNMSFixture(t *testing.T) {
	profile := matchedProfileByFile(t, ciscoCat29xxStackSysID, "cisco-catalyst.yaml")
	filterProfileForPoE(t, profile)

	pdus := loadSnmprecPDUs(t, "librenms/cisco_2960x_poe_tables.snmprec")
	pdus = append(pdus, pethPsePortRowsFor(pdus)...)

	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()
	expectFixtureAgent(mockHandler, pdus)

	collector := New(Config{
		SnmpClient:  mockHandler,
		Profiles:    []*ddsnmp.Profile{profile},
		Log:         logger.New(),
		SysObjectID: ciscoCat29xxStackSysID,
	})

	results, err := collector.Collect()
	require.NoError(t, err)
	require.Len(t, results, 1)

	columns := fixtureColumns(pdus)
	portIndexes := slices.Sorted(maps.Keys(columns[oidCpeExtPsePortEntry+".9"]))
	require.Len(t, portIndexes, 120)

	wantPower := make(map[string]poeFixtureRow)
	wantFaults := make(map[string]poeFixtureRow)
	wantDetection := make(map[string]map[string]string)
	for _, index := range portIndexes {
		wantPower[index] = poeFixtureRow{
			Tags: poePortTags(index),
			Values: map[string]int64{
				"allocated":   columns[oidCpeExtPsePortEntry+".7"][index],
				"available":   columns[oidCpeExtPsePortEntry+".8"][index],
				"consumption": columns[oidCpeExtPsePortEntry+".9"][index],
				"max_drawn":   columns[oidCpeExtPsePortEntry+".10"][index],
			},
		}
		pethTags := poePortTags(index)
		pethTags["_pse_port_priority"] = "low"
		wantDetection[index] = pethTags
		wantFaults[index] = poeFixtureRow{
			Tags: pethTags,
			Values: map[string]int64{
				"invalid_signature": columns[oidPethPsePortEntry+".11"][index],
				"mps_absent":        columns[oidPethPsePortEntry+".8"][index],
				"overload":          columns[oidPethPsePortEntry+".13"][index],
				"power_denied":      columns[oidPethPsePortEntry+".12"][index],
			},
		}
	}

	wantMain := map[string]int64{
		"pethMainPsePower":            columns[oidPethMainPseEntry+".2"]["1"],
		"pethMainPseConsumptionPower": columns[oidPethMainPseEntry+".4"]["1"],
		"pethMainPseUsageThreshold":   columns[oidPethMainPseEntry+".5"]["1"],
	}

	gotPower := make(map[string]poeFixtureRow)
	gotFaults := make(map[string]poeFixtureRow)
	gotDetection := make(map[string]map[string]string)
	gotMain := make(map[string]int64)
	for _, m := range results[0].Metrics {
		switch m.Name {
		case "cpeExtPsePortPower":
			gotPower[poeRowKey(m.Tags)] = poeFixtureRow{
				Tags:   m.Tags,
				Values: m.MultiValue,
			}
		case "pethPsePortFaults":
			gotFaults[poeRowKey(m.Tags)] = poeFixtureRow{
				Tags:   m.Tags,
				Values: m.MultiValue,
			}
		case "pethPsePortDetectionStatus":
			gotDetection[poeRowKey(m.Tags)] = m.Tags
		case "pethMainPsePower", "pethMainPseConsumptionPower", "pethMainPseUsageThreshold":
			assert.Equal(t, map[string]string{"pse_group_index": "1"}, m.Tags)
			gotMain[m.Name] = m.Value
		}
	}

	assert.Equal(t, wantPower, gotPower)
	assert.Equal(t, wantFaults, gotFaults)
	assert.Equal(t, wantDetection, gotDetection)
	assert.Equal(t, wantMain, gotMain)
}

func filterProfileForPoE(t *testing.T, profile *ddsnmp.Profile) {
	t.Helper()

	poeTables := map[string]bool{"pethMainPseTable": true, "pethPsePortTable": true, "cpeExtPsePortTable": true}

	profile.Definition.Metadata = nil
	profile.Definition.SysobjectIDMetadata = nil
	profile.Definition.MetricTags = nil
	profile.Definition.StaticTags = nil
	profile.Definition.Topology = nil
	profile.Definition.Licensing = nil
	profile.Definition.BGP = nil
	profile.Definition.Metrics = slices.DeleteFunc(
		profile.Definition.Metrics,
		func(m ddprofiledefinition.MetricsConfig) bool {
			return !poeTables[m.Table.Name]
		},
	)
	profile.Definition.VirtualMetrics = slices.DeleteFunc(
		profile.Definition.VirtualMetrics,
		func(vm ddprofiledefinition.VirtualMetricConfig) bool {
			return !slices.ContainsFunc(vm.Sources, func(src ddprofiledefinition.VirtualMetricSourceConfig) bool {
				return poeTables[src.Table]
			})
		},
	)

	require.Len(t, profile.Definition.Metrics, 3)
	require.Len(t, profile.Definition.VirtualMetrics, 2)
}

// No public capture carries Cisco pethPsePortTable rows. cpeExtPsePortEntry AUGMENTS pethPsePortEntry, so the
// standard rows reuse the fixture's (group, port) indexes; each counter value is unique per row.
func pethPsePortRowsFor(pdus []gosnmp.SnmpPDU) []gosnmp.SnmpPDU {
	var rows []gosnmp.SnmpPDU
	for index := range fixtureColumns(pdus)[oidCpeExtPsePortEntry+".9"] {
		group, port, _ := strings.Cut(index, ".")
		g, _ := strconv.Atoi(group)
		p, _ := strconv.Atoi(port)
		seed := uint(g*1000 + p*10)

		rows = append(rows,
			createIntegerPDU(oidPethPsePortEntry+".6."+index, 3),  // pethPsePortDetectionStatus: deliveringPower
			createIntegerPDU(oidPethPsePortEntry+".7."+index, 3),  // pethPsePortPowerPriority: low
			createIntegerPDU(oidPethPsePortEntry+".10."+index, 1), // pethPsePortPowerClassifications: class0
			createCounter32PDU(oidPethPsePortEntry+".8."+index, seed+1),
			createCounter32PDU(oidPethPsePortEntry+".11."+index, seed+2),
			createCounter32PDU(oidPethPsePortEntry+".12."+index, seed+3),
			createCounter32PDU(oidPethPsePortEntry+".13."+index, seed+4),
		)
	}
	return rows
}

// fixtureColumns maps "<entry>.<column>" to row index to integer value for the PoE entries.
func fixtureColumns(pdus []gosnmp.SnmpPDU) map[string]map[string]int64 {
	out := make(map[string]map[string]int64)
	for _, pdu := range pdus {
		name := strings.Trim(pdu.Name, ".")
		for _, entry := range []string{oidPethPsePortEntry, oidCpeExtPsePortEntry, oidPethMainPseEntry} {
			rest, ok := strings.CutPrefix(name, entry+".")
			if !ok {
				continue
			}
			column, index, _ := strings.Cut(rest, ".")
			key := entry + "." + column
			if out[key] == nil {
				out[key] = make(map[string]int64)
			}
			out[key][index] = gosnmp.ToBigInt(pdu.Value).Int64()
		}
	}
	return out
}

func expectFixtureAgent(mockHandler *snmpmock.MockHandler, pdus []gosnmp.SnmpPDU) {
	mockHandler.EXPECT().Version().Return(gosnmp.Version2c).AnyTimes()
	mockHandler.EXPECT().BulkWalkAll(gomock.Any()).DoAndReturn(func(oid string) ([]gosnmp.SnmpPDU, error) {
		return snmprecPDUsWithPrefix(pdus, strings.Trim(oid, ".")+"."), nil
	}).AnyTimes()
}

func poePortTags(index string) map[string]string {
	group, port, _ := strings.Cut(index, ".")
	return map[string]string{"pse_group_index": group, "pse_port_index": port}
}

func poeRowKey(tags map[string]string) string {
	return fmt.Sprintf("%s.%s", tags["pse_group_index"], tags["pse_port_index"])
}
