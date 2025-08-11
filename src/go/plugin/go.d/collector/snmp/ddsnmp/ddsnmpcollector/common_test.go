// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"regexp"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	snmpmock "github.com/gosnmp/gosnmp/mocks"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

func mustCompileRegex(pattern string) *regexp.Regexp {
	re, err := regexp.Compile(pattern)
	if err != nil {
		panic(err)
	}
	return re
}

func setupMockHandler(t *testing.T) (*gomock.Controller, *snmpmock.MockHandler) {
	ctrl := gomock.NewController(t)
	mockHandler := snmpmock.NewMockHandler(ctrl)
	mockHandler.EXPECT().MaxOids().Return(10).AnyTimes()
	return ctrl, mockHandler
}

func createTestProfile(sourceFile string, metrics []ddprofiledefinition.MetricsConfig) *ddsnmp.Profile {
	return &ddsnmp.Profile{
		SourceFile: sourceFile,
		Definition: &ddprofiledefinition.ProfileDefinition{
			Metrics: metrics,
		},
	}
}

func createScalarMetric(oid, name string) ddprofiledefinition.MetricsConfig {
	return ddprofiledefinition.MetricsConfig{
		Symbol: ddprofiledefinition.SymbolConfig{
			OID:  oid,
			Name: name,
		},
	}
}

func expectSNMPGet(mockHandler *snmpmock.MockHandler, oids []string, pdus []gosnmp.SnmpPDU) {
	mockHandler.EXPECT().Get(gomock.InAnyOrder(oids)).Return(
		&gosnmp.SnmpPacket{Variables: pdus}, nil,
	)
}

func expectSNMPGetError(mockHandler *snmpmock.MockHandler, oids []string, err error) {
	mockHandler.EXPECT().Get(gomock.InAnyOrder(oids)).Return(nil, err)
}

func expectSNMPWalk(mockHandler *snmpmock.MockHandler, version gosnmp.SnmpVersion, oid string, pdus []gosnmp.SnmpPDU) {
	mockHandler.EXPECT().Version().Return(version)
	if version == gosnmp.Version1 {
		mockHandler.EXPECT().WalkAll(oid).Return(pdus, nil)
	} else {
		mockHandler.EXPECT().BulkWalkAll(oid).Return(pdus, nil)
	}
}

func expectSNMPWalkError(mockHandler *snmpmock.MockHandler, version gosnmp.SnmpVersion, oid string, err error) {
	mockHandler.EXPECT().Version().Return(version)
	if version == gosnmp.Version1 {
		mockHandler.EXPECT().WalkAll(oid).Return(nil, err)
	} else {
		mockHandler.EXPECT().BulkWalkAll(oid).Return(nil, err)
	}
}

func createPDU(name string, pduType gosnmp.Asn1BER, value interface{}) gosnmp.SnmpPDU {
	return gosnmp.SnmpPDU{
		Name:  name,
		Type:  pduType,
		Value: value,
	}
}

func createStringPDU(name string, value string) gosnmp.SnmpPDU {
	return createPDU(name, gosnmp.OctetString, []byte(value))
}

func createIntegerPDU(name string, value int) gosnmp.SnmpPDU {
	return createPDU(name, gosnmp.Integer, value)
}

func createCounter32PDU(name string, value uint) gosnmp.SnmpPDU {
	return createPDU(name, gosnmp.Counter32, value)
}

func createCounter64PDU(name string, value uint64) gosnmp.SnmpPDU {
	return createPDU(name, gosnmp.Counter64, value)
}

func createGauge32PDU(name string, value uint) gosnmp.SnmpPDU {
	return createPDU(name, gosnmp.Gauge32, value)
}

func createTimeTicksPDU(name string, value uint32) gosnmp.SnmpPDU {
	return createPDU(name, gosnmp.TimeTicks, value)
}

func createNoSuchObjectPDU(name string) gosnmp.SnmpPDU {
	return createPDU(name, gosnmp.NoSuchObject, nil)
}

func assertMetricsEqual(t *testing.T, expected, actual []ddsnmp.Metric) {
	t.Helper()
	assert.Equal(t, len(expected), len(actual))
	require.Equal(t, len(expected), len(actual), "number of metrics")

	// Sort metrics for consistent comparison

	for i := range expected {
		assert.Equal(t, expected[i].Name, actual[i].Name, "metric name")
		assert.Equal(t, expected[i].Value, actual[i].Value, "metric value")
		assert.Equal(t, expected[i].MetricType, actual[i].MetricType, "metric type")
		assert.Equal(t, expected[i].Tags, actual[i].Tags, "metric tags")
		assert.Equal(t, expected[i].StaticTags, actual[i].StaticTags, "metric static tags")
		assert.Equal(t, expected[i].IsTable, actual[i].IsTable, "metric is table")
		assert.Equal(t, expected[i].Unit, actual[i].Unit, "metric unit")
		assert.Equal(t, expected[i].Family, actual[i].Family, "metric family")
		assert.Equal(t, expected[i].Description, actual[i].Description, "metric description")
		assert.Equal(t, expected[i].MultiValue, actual[i].MultiValue, "metric multi value")
	}
}

func assertTableMetricsEqual(t *testing.T, expected, actual []ddsnmp.Metric) {
	t.Helper()
	assert.Equal(t, len(expected), len(actual), "number of metrics")

	// Use ElementsMatch for unordered comparison
	assert.ElementsMatch(t, expected, actual, "table metrics should match (unordered)")
}
