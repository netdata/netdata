// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"fmt"
	"net/netip"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/netdata/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	snmpmock "github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils/snmpmock"

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

// testRequests is the request configuration of the tests' mock sessions:
// SNMPv2c and 10 OIDs per Get.
var testRequests = requestConfig{version: gosnmp.Version2c, maxOIDs: 10}

func setupMockSession(t *testing.T) (*gomock.Controller, *snmpmock.MockSession) {
	ctrl := gomock.NewController(t)
	mockHandler := snmpmock.NewMockSession(ctrl)
	return ctrl, mockHandler
}

func matchedProfileByFile(t *testing.T, sysObjectID, profileFile string) *ddsnmp.Profile {
	t.Helper()

	matched := ddsnmp.FindProfiles(sysObjectID, "", nil)
	for _, prof := range matched {
		if strings.HasSuffix(prof.SourceFile, profileFile) {
			return prof
		}
	}

	require.FailNowf(t, "missing profile", "expected %s for %s", profileFile, sysObjectID)
	return nil
}

func oidWithIndex(baseOID, index string) string {
	return strings.Trim(baseOID, ".") + "." + strings.Trim(index, ".")
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

func expectSNMPGet(mockHandler *snmpmock.MockSession, oids []string, pdus []gosnmp.VarBind) {
	mockHandler.EXPECT().Get(gomock.Any(), gomock.InAnyOrder(oids)).Return(
		&gosnmp.Message{VarBinds: pdus}, nil,
	)
}

func expectSNMPGetError(mockHandler *snmpmock.MockSession, oids []string, err error) {
	mockHandler.EXPECT().Get(gomock.Any(), gomock.InAnyOrder(oids)).Return(nil, err)
}

func expectSystemMetadataGets(mockHandler *snmpmock.MockSession, sysObjectID, sysName string) {
	mockHandler.EXPECT().Get(gomock.Any(), gomock.Any()).Return(&gosnmp.Message{VarBinds: []gosnmp.VarBind{
		createStringPDU("1.0.8802.1.1.2.1.3.1.0", sysName),
		createStringPDU("1.0.8802.1.1.2.1.3.2.0", sysName),
		createStringPDU("1.0.8802.1.1.2.1.3.3.0", sysName),
		createStringPDU("1.0.8802.1.1.2.1.3.4.0", sysName),
		createStringPDU("1.0.8802.1.1.2.1.3.5.0", sysName),
		createStringPDU("1.0.8802.1.1.2.1.3.6.0", sysName),
		createStringPDU("1.3.6.1.2.1.1.1.0", "SNMP device"),
		createPDU("1.3.6.1.2.1.1.2.0", gosnmp.ObjectIdentifier, sysObjectID),
		createStringPDU("1.3.6.1.2.1.1.5.0", sysName),
		createStringPDU("1.3.6.1.2.1.1.6.0", "lab"),
	}}, nil).AnyTimes()
}

func expectSNMPWalk(mockHandler *snmpmock.MockSession, version gosnmp.Version, oid string, pdus []gosnmp.VarBind) {
	if version == gosnmp.Version1 {
		mockHandler.EXPECT().WalkAll(gomock.Any(), oid).Return(pdus, gosnmp.WalkEnd{}, nil)
	} else {
		mockHandler.EXPECT().BulkWalkAll(gomock.Any(), oid, gomock.Any()).Return(pdus, gosnmp.WalkEnd{}, nil)
	}
}

func expectSNMPWalkError(mockHandler *snmpmock.MockSession, version gosnmp.Version, oid string, err error) {
	if version == gosnmp.Version1 {
		mockHandler.EXPECT().WalkAll(gomock.Any(), oid).Return(nil, gosnmp.WalkEnd{}, err)
	} else {
		mockHandler.EXPECT().BulkWalkAll(gomock.Any(), oid, gomock.Any()).Return(nil, gosnmp.WalkEnd{}, err)
	}
}

func createPDU(name string, pduType gosnmp.ValueType, value any) gosnmp.VarBind {
	return gosnmp.VarBind{
		Name:  name,
		Value: testValue(pduType, value),
	}
}

// testValue builds the value of pduType from a Go value as the tests write it:
// a number of any integer or float type, octets as []byte or string, an OID
// as text, an address as text or as 4 or 16 octets (nil or "" for the empty
// IpAddress some devices send).
func testValue(pduType gosnmp.ValueType, value any) gosnmp.Value {
	switch pduType {
	case gosnmp.Integer:
		return gosnmp.IntegerValue(int32(testNumber[int64](value)))
	case gosnmp.Counter32:
		return gosnmp.Counter32Value(uint32(testNumber[uint64](value)))
	case gosnmp.Gauge32:
		return gosnmp.Gauge32Value(uint32(testNumber[uint64](value)))
	case gosnmp.TimeTicks:
		return gosnmp.TimeTicksValue(uint32(testNumber[uint64](value)))
	case gosnmp.Uinteger32:
		return gosnmp.Uinteger32Value(uint32(testNumber[uint64](value)))
	case gosnmp.Counter64:
		return gosnmp.Counter64Value(testNumber[uint64](value))
	case gosnmp.OpaqueFloat:
		return gosnmp.OpaqueFloatValue(float32(testNumber[float64](value)))
	case gosnmp.OpaqueDouble:
		return gosnmp.OpaqueDoubleValue(testNumber[float64](value))
	case gosnmp.OctetString:
		return gosnmp.OctetStringValue(testOctets(value))
	case gosnmp.Opaque:
		return gosnmp.OpaqueValue(testOctets(value))
	case gosnmp.ObjectIdentifier:
		return gosnmp.ObjectIdentifierValue(value.(string))
	case gosnmp.IPAddress:
		switch v := value.(type) {
		case nil:
			return gosnmp.IPAddressValue(netip.Addr{})
		case string:
			if v == "" {
				return gosnmp.IPAddressValue(netip.Addr{})
			}
			return gosnmp.IPAddressValue(netip.MustParseAddr(v))
		case []byte:
			addr, ok := netip.AddrFromSlice(v)
			if !ok {
				panic(fmt.Sprintf("test IpAddress of %d octets", len(v)))
			}
			return gosnmp.IPAddressValue(addr)
		}
	case gosnmp.Null:
		return gosnmp.NullValue()
	case gosnmp.NoSuchObject:
		return gosnmp.NoSuchObjectValue()
	case gosnmp.NoSuchInstance:
		return gosnmp.NoSuchInstanceValue()
	case gosnmp.EndOfMibView:
		return gosnmp.EndOfMibViewValue()
	}
	panic(fmt.Sprintf("test value %v of type %T for %v", value, value, pduType))
}

// answeredError is err as the client returns it for a request the agent's
// answer ended, such as an SNMPv3 Report.
func answeredError(err error) error {
	return fmt.Errorf("%w (%w)", err, gosnmp.ErrAnswered)
}

func testNumber[T int64 | uint64 | float64](value any) T {
	switch v := value.(type) {
	case int:
		return T(v)
	case int32:
		return T(v)
	case int64:
		return T(v)
	case uint:
		return T(v)
	case uint32:
		return T(v)
	case uint64:
		return T(v)
	case float32:
		return T(v)
	case float64:
		return T(v)
	case nil:
		return 0
	}
	panic(fmt.Sprintf("test number of type %T", value))
}

func testOctets(value any) []byte {
	switch v := value.(type) {
	case []byte:
		return v
	case string:
		return []byte(v)
	case nil:
		return nil
	}
	panic(fmt.Sprintf("test octets of type %T", value))
}

func createStringPDU(name string, value string) gosnmp.VarBind {
	return createPDU(name, gosnmp.OctetString, []byte(value))
}

func createIntegerPDU(name string, value int) gosnmp.VarBind {
	return createPDU(name, gosnmp.Integer, value)
}

func createCounter32PDU(name string, value uint) gosnmp.VarBind {
	return createPDU(name, gosnmp.Counter32, value)
}

func createCounter64PDU(name string, value uint64) gosnmp.VarBind {
	return createPDU(name, gosnmp.Counter64, value)
}

func createGauge32PDU(name string, value uint) gosnmp.VarBind {
	return createPDU(name, gosnmp.Gauge32, value)
}

func createTimeTicksPDU(name string, value uint32) gosnmp.VarBind {
	return createPDU(name, gosnmp.TimeTicks, value)
}

func createDateAndTimePDU(name string, value time.Time) gosnmp.VarBind {
	return createPDU(name, gosnmp.OctetString, []byte{
		byte(value.Year() >> 8),
		byte(value.Year()),
		byte(value.Month()),
		byte(value.Day()),
		byte(value.Hour()),
		byte(value.Minute()),
		byte(value.Second()),
		0,
	})
}

func createNoSuchObjectPDU(name string) gosnmp.VarBind {
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
