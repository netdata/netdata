// SPDX-License-Identifier: GPL-3.0-or-later

package collector

import (
	"reflect"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp"
	snmptopology "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology"
	snmptraps "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSNMPFamilyRegistrationUsesSharedDependencies(t *testing.T) {
	snmpCreator := requireCreator(t, "snmp")
	topologyCreator := requireCreator(t, "snmp_topology")
	trapsCreator := requireCreator(t, "snmp_traps")

	assert.NotNil(t, snmpCreator.Create)
	assert.Nil(t, snmpCreator.CreateV2)
	assert.NotNil(t, snmpCreator.Config)
	assert.NotNil(t, snmpCreator.SharedFunctions)
	assert.Nil(t, snmpCreator.AgentFunctions)
	assert.NotNil(t, snmpCreator.MethodHandler)
	assert.Equal(t, 10, snmpCreator.Defaults.UpdateEvery)

	assert.Nil(t, topologyCreator.Create)
	assert.NotNil(t, topologyCreator.CreateV2)
	assert.Equal(t, collectorapi.InstancePolicySingle, topologyCreator.InstancePolicy)
	assert.False(t, topologyCreator.FunctionOnly)
	assert.NotNil(t, topologyCreator.SharedFunctions)
	assert.Nil(t, topologyCreator.AgentFunctions)
	assert.NotNil(t, topologyCreator.MethodHandler)
	assert.Equal(t, 60, topologyCreator.Defaults.UpdateEvery)

	assert.Nil(t, trapsCreator.Create)
	assert.NotNil(t, trapsCreator.CreateV2)
	assert.Nil(t, trapsCreator.SharedFunctions)
	assert.NotNil(t, trapsCreator.AgentFunctions)
	assert.NotNil(t, trapsCreator.MethodHandler)
	assert.Equal(t, 1, trapsCreator.Defaults.UpdateEvery)

	snmpCollector, ok := snmpCreator.Create().(*snmp.Collector)
	require.True(t, ok)
	topologyCollector, ok := topologyCreator.CreateV2().(*snmptopology.Collector)
	require.True(t, ok)
	trapsCollector, ok := trapsCreator.CreateV2().(*snmptraps.Collector)
	require.True(t, ok)

	deviceStore := pointerField(t, snmpCollector, "deviceStore")
	require.NotZero(t, deviceStore)
	assert.Equal(t, deviceStore, interfacePointerField(t, topologyCollector, "deviceSource"))
	assert.Equal(t, deviceStore, interfacePointerField(t, trapsCollector, "deviceLookup"))

	trapEnrichment := pointerField(t, topologyCollector, "trapEnrichment")
	require.NotZero(t, trapEnrichment)
	assert.Equal(t, trapEnrichment, interfacePointerField(t, trapsCollector, "topologyEnricher"))
}

func requireCreator(t *testing.T, module string) collectorapi.Creator {
	t.Helper()
	creator, ok := collectorapi.DefaultRegistry.Lookup(module)
	require.True(t, ok, "collector %q is not registered", module)
	return creator
}

func pointerField(t *testing.T, obj any, name string) uintptr {
	t.Helper()
	field := reflect.ValueOf(obj).Elem().FieldByName(name)
	require.True(t, field.IsValid(), "field %q not found", name)
	require.Equal(t, reflect.Pointer, field.Kind(), "field %q", name)
	require.False(t, field.IsNil(), "field %q is nil", name)
	return field.Pointer()
}

func interfacePointerField(t *testing.T, obj any, name string) uintptr {
	t.Helper()
	field := reflect.ValueOf(obj).Elem().FieldByName(name)
	require.True(t, field.IsValid(), "field %q not found", name)
	require.Equal(t, reflect.Interface, field.Kind(), "field %q", name)
	require.False(t, field.IsNil(), "field %q is nil", name)

	elem := field.Elem()
	require.Equal(t, reflect.Pointer, elem.Kind(), "field %q concrete value", name)
	require.False(t, elem.IsNil(), "field %q concrete value is nil", name)
	return elem.Pointer()
}
