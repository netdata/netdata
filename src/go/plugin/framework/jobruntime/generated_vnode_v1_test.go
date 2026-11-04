// SPDX-License-Identifier: GPL-3.0-or-later

package jobruntime

import (
	"bytes"
	"context"
	"io"
	"maps"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/hostoutput"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func generatedV1Fixture(out io.Writer) (JobConfig, *v1ModuleOwnedVnodeCollector) {
	charts := collectorapi.Charts{&collectorapi.Chart{
		ID:    "work",
		Title: "Work",
		Units: "units",
		Dims:  collectorapi.Dims{{ID: "value"}},
	}}
	module := &v1ModuleOwnedVnodeCollector{
		MockCollectorV1: collectorapi.MockCollectorV1{
			ChartsFunc:  func() *collectorapi.Charts { return &charts },
			CollectFunc: func(context.Context) map[string]int64 { return map[string]int64{"value": 1} },
		},
		vnode: &vnodes.VirtualNode{
			GUID:     sharedGUID,
			Hostname: "before",
			Labels:   map[string]string{"site": "before", "retired": "yes"},
		},
	}
	return JobConfig{
		PluginName: "go.d",
		Name:       "device",
		ModuleName: "test",
		FullName:   "test_device",
		Module:     module,
		Out:        out,
	}, module
}

func TestJob_GeneratedVnodeRefresh(t *testing.T) {
	for _, replace := range []bool{false, true} {
		name := "in-place mutation"
		if replace {
			name = "replacement snapshot"
		}
		t.Run(name, func(t *testing.T) {
			var out bytes.Buffer
			cfg, module := generatedV1Fixture(&out)
			cycle := 0
			module.CollectFunc = func(context.Context) map[string]int64 {
				cycle++
				if cycle == 2 {
					if replace {
						module.vnode = module.vnode.Copy()
					}
					module.vnode.Hostname = "after"
				}
				if cycle == 3 {
					module.vnode.Labels["site"] = "after"
					delete(module.vnode.Labels, "retired")
				}
				return map[string]int64{"value": 1}
			}
			job := NewJob(cfg)
			require.NoError(t, job.AutoDetectionManaged(context.Background()))
			t.Cleanup(job.Cleanup)
			oracle := newHostProtocolOracle()
			job.runOnce()
			oracle.apply(t, out.String())
			require.Equal(t, "before", oracle.labels[sharedGUID]["_hostname"])
			out.Reset()
			job.runOnce()
			oracle.apply(t, out.String())
			assert.Equal(t, map[string]string{"_hostname": "after", "site": "before", "retired": "yes"}, oracle.labels[sharedGUID])
			assert.Equal(t, 1, strings.Count(out.String(), "HOST_DEFINE '"))
			out.Reset()
			job.runOnce()
			oracle.apply(t, out.String())
			assert.Equal(t, map[string]string{"_hostname": "after", "site": "after"}, oracle.labels[sharedGUID])
			assert.Equal(t, 1, strings.Count(out.String(), "HOST_DEFINE '"))
			assert.NotContains(t, out.String(), "CHART '", "metadata changes must not recreate charts")
			assert.Contains(t, out.String(), "HOST '"+sharedGUID+"'")
			module.vnode.Labels["site"] = "collector-only"
			assert.Equal(t, "after", job.vnode.Labels["site"], "runtime must own the adopted labels")
			module.vnode.Labels["site"] = "after"
			out.Reset()
			job.runOnce()
			assert.NotContains(t, out.String(), "HOST_DEFINE")
		})
	}
}

func TestJob_GeneratedVnodeRetainsAdoptedIdentity(t *testing.T) {
	for name, next := range map[string]*vnodes.VirtualNode{
		"nil":            nil,
		"empty GUID":     {Hostname: "after"},
		"empty hostname": {GUID: sharedGUID},
		"different GUID": {GUID: "22222222-2222-2222-2222-222222222222", Hostname: "after"},
	} {
		t.Run(name, func(t *testing.T) {
			var out bytes.Buffer
			cfg, module := generatedV1Fixture(&out)
			job := NewJob(cfg)
			require.NoError(t, job.AutoDetectionManaged(context.Background()))
			t.Cleanup(job.Cleanup)
			job.runOnce()
			module.vnode = next
			out.Reset()
			job.runOnce()
			assert.NotContains(t, out.String(), "HOST_DEFINE")
			assert.Contains(t, out.String(), "HOST '"+sharedGUID+"'")
			assert.Equal(t, "before", job.hostDefinition.Info().Hostname)
		})
	}
}

func TestJob_GeneratedVnodeRejectsInvalidRefresh(t *testing.T) {
	for name, hostname := range map[string]string{
		"quote":           "bad'hostname",
		"backslash":       "bad\\hostname",
		"newline":         "bad\nhostname",
		"carriage return": "bad\rhostname",
		"NUL":             "bad\x00hostname",
		"whitespace":      "   ",
	} {
		t.Run(name, func(t *testing.T) {
			var out bytes.Buffer
			cfg, module := generatedV1Fixture(&out)
			job := NewJob(cfg)
			require.NoError(t, job.AutoDetectionManaged(context.Background()))
			t.Cleanup(job.Cleanup)
			job.runOnce()
			oracle := newHostProtocolOracle()
			oracle.apply(t, out.String())
			before := maps.Clone(oracle.labels[sharedGUID])
			staleAfter := confopt.Duration(time.Minute)
			module.vnode = &vnodes.VirtualNode{
				GUID:       sharedGUID,
				Hostname:   hostname,
				Labels:     map[string]string{"site": "rejected"},
				StaleAfter: &staleAfter,
			}
			for range 2 {
				out.Reset()
				job.runOnce()
				assert.Contains(t, out.String(), "HOST '"+sharedGUID+"'")
				assert.Contains(t, out.String(), "SET 'value' = 1", "rejected metadata must not interrupt measurements")
				assert.Contains(t, out.String(), "SET 'success' = 1")
				assert.NotContains(t, out.String(), "HOST_DEFINE")
				assert.NotContains(t, out.String(), "CHART '")
				oracle.apply(t, out.String())
				assert.Equal(t, before, oracle.labels[sharedGUID], "retain the whole usable snapshot")
			}
			module.vnode = nil
			out.Reset()
			job.runOnce()
			assert.Contains(t, out.String(), "SET 'value' = 1", "a rejected candidate must not poison subsequent cycles")
			module.vnode = &vnodes.VirtualNode{
				GUID:     sharedGUID,
				Hostname: "recovered",
				Labels:   map[string]string{"site": "recovered"},
			}
			out.Reset()
			job.runOnce()
			assert.Contains(t, out.String(), "HOST_DEFINE '"+sharedGUID+"' 'recovered'")
			assert.Contains(t, out.String(), "SET 'value' = 1")
			assert.NotContains(t, out.String(), "CHART '")
			oracle.apply(t, out.String())
			assert.Equal(t, map[string]string{"_hostname": "recovered", "site": "recovered"}, oracle.labels[sharedGUID])
		})
	}
}

func TestJob_GeneratedVnodeRejectsInvalidRefreshBeforeFirstPublication(t *testing.T) {
	var out bytes.Buffer
	cfg, module := generatedV1Fixture(&out)
	initialCharts := *module.ChartsFunc()
	charts := collectorapi.Charts{}
	module.ChartsFunc = func() *collectorapi.Charts { return &charts }
	collect := module.CollectFunc
	module.CollectFunc = func(context.Context) map[string]int64 { return nil }
	job := NewJob(cfg)
	require.NoError(t, job.AutoDetectionManaged(context.Background()))
	t.Cleanup(job.Cleanup)
	job.runOnce()
	require.NotContains(t, out.String(), "HOST_DEFINE")
	charts = initialCharts
	module.CollectFunc = collect
	module.vnode.Hostname = "bad'hostname"
	out.Reset()
	job.runOnce()
	assert.Contains(t, out.String(), "HOST_DEFINE '"+sharedGUID+"' 'before'")
	assert.Contains(t, out.String(), "SET 'value' = 1")
}

func TestJob_GeneratedVnodeInvalidInitialMetadataDoesNotUseLocalHost(t *testing.T) {
	var out bytes.Buffer
	cfg, module := generatedV1Fixture(&out)
	module.vnode.Hostname = "bad'hostname"
	job := NewJob(cfg)
	require.NoError(t, job.AutoDetectionManaged(context.Background()))
	t.Cleanup(job.Cleanup)
	for range 2 {
		out.Reset()
		job.runOnce()
		assert.Empty(t, out.String(), "an invalid initial target must not route device samples onto the local host")
	}
	module.vnode.Hostname = "recovered"
	job.runOnce()
	assert.Contains(t, out.String(), "HOST_DEFINE '"+sharedGUID+"' 'recovered'")
	assert.Contains(t, out.String(), "SET 'value' = 1")
}

func TestJob_GeneratedVnodeUUIDAliasRetainsRouting(t *testing.T) {
	var out bytes.Buffer
	cfg, module := generatedV1Fixture(&out)
	job := NewJob(cfg)
	require.NoError(t, job.AutoDetectionManaged(context.Background()))
	t.Cleanup(job.Cleanup)
	job.runOnce()
	module.vnode.GUID = strings.ReplaceAll(sharedGUID, "-", "")
	module.vnode.Hostname = "after"
	out.Reset()
	job.runOnce()
	assert.Contains(t, out.String(), "HOST_DEFINE '"+sharedGUID+"' 'after'")
	assert.Contains(t, out.String(), "HOST '"+sharedGUID+"'")
	assert.NotContains(t, out.String(), "CHART '")
}

func TestJob_GeneratedVnodePreservesSuppliedVnode(t *testing.T) {
	for _, name := range []string{"", "configured"} {
		t.Run("reference="+name, func(t *testing.T) {
			var out bytes.Buffer
			cfg, module := generatedV1Fixture(&out)
			cfg.Vnode = vnodes.VirtualNode{
				GUID:     sharedGUID,
				Hostname: "configured",
			}
			cfg.VnodeName = name
			job := NewJob(cfg)
			require.NoError(t, job.AutoDetectionManaged(context.Background()))
			t.Cleanup(job.Cleanup)
			job.runOnce()
			module.vnode.Hostname = "after"
			out.Reset()
			job.runOnce()
			assert.NotContains(t, out.String(), "HOST_DEFINE")
			assert.Equal(t, "configured", job.hostDefinition.Info().Hostname)
		})
	}
}

func TestJob_GeneratedVnodeRefreshRetriesRejectedOutput(t *testing.T) {
	out := &selectiveNativeOutput{}
	cfg, module := generatedV1Fixture(out)
	job := NewJob(cfg)
	require.NoError(t, job.AutoDetectionManaged(context.Background()))
	t.Cleanup(job.Cleanup)
	job.runOnce()
	before := job.hostDefinition.Info()
	module.vnode.Hostname = "after"
	module.vnode.Labels = map[string]string{"site": "after"}
	out.Reset()
	out.reject = "HOST_DEFINE"
	job.runOnce()
	assert.Empty(t, out.String())
	assert.Equal(t, before, job.hostDefinition.Info(), "rejection must preserve committed metadata")
	out.reject = ""
	job.runOnce()
	assert.Contains(t, out.String(), "HOST_DEFINE '"+sharedGUID+"' 'after'")
	assert.NotContains(t, out.String(), "CHART '")
	assert.Equal(t, "after", job.hostDefinition.Info().Hostname)
	out.Reset()
	job.runOnce()
	assert.NotContains(t, out.String(), "HOST_DEFINE")
}

func TestJob_GeneratedVnodeRefreshPreservesConfiguredAuthority(t *testing.T) {
	var out bytes.Buffer
	cfg, module := generatedV1Fixture(&out)
	definition, err := hostoutput.NewDefinition(netdataapi.HostInfo{
		GUID:     sharedGUID,
		Hostname: "configured",
		Labels:   map[string]string{"site": "configured"},
	})
	require.NoError(t, err)
	cfg.Publication = hostoutput.New()
	cfg.Publication.Bind(func(string) *hostoutput.Definition { return definition })
	job := NewJob(cfg)
	require.NoError(t, job.AutoDetectionManaged(context.Background()))
	t.Cleanup(job.Cleanup)
	job.runOnce()
	oracle := newHostProtocolOracle()
	oracle.apply(t, out.String())
	module.vnode.Hostname = "after"
	module.vnode.Labels = map[string]string{"site": "after"}
	out.Reset()
	job.runOnce()
	oracle.apply(t, out.String())
	assert.Equal(t, map[string]string{"_hostname": "configured", "site": "configured"}, oracle.labels[sharedGUID])
	assert.NotContains(t, out.String(), "HOST_DEFINE")
	cfg.Publication.Bind(nil)
	out.Reset()
	job.runOnce()
	oracle.apply(t, out.String())
	assert.Equal(t, map[string]string{"_hostname": "after", "site": "after"}, oracle.labels[sharedGUID])
}

func TestJob_GeneratedVnodeRefreshWaitsForHostOutput(t *testing.T) {
	var out bytes.Buffer
	cfg, module := generatedV1Fixture(&out)
	job := NewJob(cfg)
	require.NoError(t, job.AutoDetectionManaged(context.Background()))
	t.Cleanup(job.Cleanup)
	job.runOnce()
	module.vnode.Hostname = "after"
	collect := module.CollectFunc
	module.CollectFunc = func(context.Context) map[string]int64 { return nil }
	out.Reset()
	job.runOnce()
	assert.NotContains(t, out.String(), "HOST_DEFINE")
	assert.Equal(t, "before", job.hostDefinition.Info().Hostname)
	module.CollectFunc = collect
	out.Reset()
	job.runOnce()
	assert.Contains(t, out.String(), "HOST_DEFINE '"+sharedGUID+"' 'after'")
	assert.Equal(t, "after", job.hostDefinition.Info().Hostname)
}

func TestJob_GeneratedVnodeLifecycleMetadataRefresh(t *testing.T) {
	var out bytes.Buffer
	cfg, module := generatedV1Fixture(&out)
	staleAfter := confopt.Duration(time.Minute)
	module.vnode.StaleAfter = &staleAfter
	job := NewJob(cfg)
	require.NoError(t, job.AutoDetectionManaged(context.Background()))
	t.Cleanup(job.Cleanup)
	job.runOnce()
	oracle := newHostProtocolOracle()
	oracle.apply(t, out.String())
	require.Equal(t, "60", oracle.labels[sharedGUID]["_node_stale_after_seconds"])
	staleAfter = confopt.Duration(2 * time.Minute)
	assert.Equal(t, time.Minute, job.vnode.StaleAfter.Duration(), "runtime must own the lifecycle value")
	out.Reset()
	job.runOnce()
	oracle.apply(t, out.String())
	assert.Equal(t, "120", oracle.labels[sharedGUID]["_node_stale_after_seconds"])
	assert.NotContains(t, out.String(), "CHART '")
	staleAfter = 0
	out.Reset()
	job.runOnce()
	oracle.apply(t, out.String())
	assert.NotContains(t, oracle.labels[sharedGUID], "_node_stale_after_seconds")
}
