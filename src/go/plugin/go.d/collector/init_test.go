// SPDX-License-Identifier: GPL-3.0-or-later

package collector

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery/sd/pipeline"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp"
	snmptopology "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology"
	snmptraps "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

var serviceModulePattern = regexp.MustCompile(`(?m)^\s*(?:-\s*)?module:\s*([A-Za-z0-9_.-]+)\s*$`)

func TestStockServiceDiscoveryRulesTargetRegisteredCollectors(t *testing.T) {
	files, err := filepath.Glob("../config/go.d/sd/*.conf")
	require.NoError(t, err)
	require.NotEmpty(t, files)

	for _, file := range files {
		bs, err := os.ReadFile(file)
		require.NoError(t, err, file)

		var cfg pipeline.Config
		require.NoError(t, yaml.Unmarshal(bs, &cfg), file)

		for _, rule := range cfg.Services {
			if strings.TrimSpace(rule.ConfigTemplate) == "" {
				continue
			}
			if strings.Contains(rule.ConfigTemplate, "toYaml") {
				// Pass-through templates provide the complete job, including module,
				// from the discovered item.
				continue
			}

			modules := serviceModulePattern.FindAllStringSubmatch(rule.ConfigTemplate, -1)
			if len(modules) == 0 {
				modules = [][]string{{"", rule.ID}}
			}
			for _, match := range modules {
				_, ok := collectorapi.DefaultRegistry.Lookup(match[1])
				assert.Truef(t, ok, "%s service rule %q targets unregistered collector %q", filepath.Base(file), rule.ID, match[1])
			}
		}
	}
}

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

	trapEnrichment := pointerField(t, topologyCollector, "trapEnrichment")
	require.NotZero(t, trapEnrichment)
	require.NotZero(t, nestedPointerField(t, trapsCollector, "services", "enricher"))

	topologyReverseDNS := nestedPointerField(t, topologyCollector, "topologyRegistry", "reverseDNS")
	trapsReverseDNS := nestedInterfacePointerField(t, trapsCollector, "services", "enricher", "reverseDNS")
	require.NotZero(t, topologyReverseDNS)
	assert.Equal(t, topologyReverseDNS, trapsReverseDNS)
	negativeTTL := durationField(t, topologyCollector, "topologyRegistry", "reverseDNS", "negativeTTL")
	assert.Equal(t, 5*time.Minute, negativeTTL)
}

func requireCreator(t *testing.T, module string) collectorapi.Creator {
	t.Helper()
	creator, ok := collectorapi.DefaultRegistry.Lookup(module)
	require.True(t, ok, "collector %q is not registered", module)
	return creator
}

func pointerField(t *testing.T, obj any, name string) uintptr {
	t.Helper()
	field := fieldPath(t, obj, name)
	require.Equal(t, reflect.Pointer, field.Kind(), "field %q", name)
	require.False(t, field.IsNil(), "field %q is nil", name)
	return field.Pointer()
}

func interfacePointerField(t *testing.T, obj any, name string) uintptr {
	t.Helper()
	field := fieldPath(t, obj, name)
	require.Equal(t, reflect.Interface, field.Kind(), "field %q", name)
	require.False(t, field.IsNil(), "field %q is nil", name)

	elem := field.Elem()
	require.Equal(t, reflect.Pointer, elem.Kind(), "field %q concrete value", name)
	require.False(t, elem.IsNil(), "field %q concrete value is nil", name)
	return elem.Pointer()
}

func nestedPointerField(t *testing.T, obj any, outerName, innerName string) uintptr {
	t.Helper()
	inner := fieldPath(t, obj, outerName, innerName)
	require.Equal(t, reflect.Pointer, inner.Kind(), "field %q.%q", outerName, innerName)
	require.False(t, inner.IsNil(), "field %q.%q is nil", outerName, innerName)
	return inner.Pointer()
}

func nestedInterfacePointerField(t *testing.T, obj any, names ...string) uintptr {
	t.Helper()
	inner := fieldPath(t, obj, names...)
	name := strings.Join(names, ".")
	require.Equal(t, reflect.Interface, inner.Kind(), "field %q", name)
	require.False(t, inner.IsNil(), "field %q is nil", name)

	elem := inner.Elem()
	require.Equal(t, reflect.Pointer, elem.Kind(), "field %q concrete value", name)
	require.False(t, elem.IsNil(), "field %q concrete value is nil", name)
	return elem.Pointer()
}

func durationField(t *testing.T, obj any, names ...string) time.Duration {
	t.Helper()
	field := fieldPath(t, obj, names...)
	require.Equal(t, reflect.Int64, field.Kind(), "field %q", strings.Join(names, "."))
	return time.Duration(field.Int())
}

func fieldPath(t *testing.T, obj any, names ...string) reflect.Value {
	t.Helper()
	value := reflect.ValueOf(obj)
	for _, name := range names {
		for value.Kind() == reflect.Pointer || value.Kind() == reflect.Interface {
			require.False(t, value.IsNil(), "field path %q contains nil", strings.Join(names, "."))
			value = value.Elem()
		}
		require.Equal(t, reflect.Struct, value.Kind(), "field path %q", strings.Join(names, "."))
		value = value.FieldByName(name)
		require.True(t, value.IsValid(), "field path %q not found", strings.Join(names, "."))
	}
	return value
}
