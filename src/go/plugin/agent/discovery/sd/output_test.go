// SPDX-License-Identifier: GPL-3.0-or-later

package sd

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/netdata/netdata/go/plugins/plugin/framework/functions"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDyncfgConfigReturnsAfterQueuedCommandCompletes(t *testing.T) {
	var output bytes.Buffer
	discovery, err := NewServiceDiscovery(Config{
		Epoch:        1,
		Attempts:     newTestAttemptAuthority(t),
		PluginName:   "test",
		DyncfgOutput: dyncfg.NewProtocolOutput(&output),
		Discoverers:  NewRegistry(),
	})
	require.NoError(t, err)
	discovery.ctx = context.Background()
	done := make(chan struct{})
	go func() {
		discovery.dyncfgConfig(dyncfg.NewFunction(t.Context(), functions.Function{
			UID:  "queued",
			Name: "config",
			Args: []string{
				"test:sd:type:name",
				string(dyncfg.CommandRestart),
			},
		}))
		close(done)
	}()
	var command dyncfg.Function
	select {
	case command = <-discovery.dyncfgCh:
	case <-time.After(time.Second):
		t.Fatal("dyncfg command was not queued")
	}
	select {
	case <-done:
		t.Fatal("dyncfg handler returned before queued execution")
	default:
	}
	discovery.dyncfgSeqExec(command)
	discovery.completeDyncfg(command)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("dyncfg handler did not observe queued completion")
	}
	assert.Contains(
		t,
		output.String(),
		"FUNCTION_RESULT_BEGIN queued 501 application/json",
	)
}

func TestCanceledQueuedDyncfgCommandReturnsWithoutApplying(t *testing.T) {
	var output bytes.Buffer
	discovery, err := NewServiceDiscovery(Config{
		Epoch:        1,
		Attempts:     newTestAttemptAuthority(t),
		PluginName:   "test",
		DyncfgOutput: dyncfg.NewProtocolOutput(&output),
		Discoverers:  NewRegistry(),
	})
	require.NoError(t, err)
	discovery.ctx = context.Background()
	discovery.mgr = NewPipelineManager(discovery.Logger, func(context.Context, []*confgroup.Group) {})

	config, err := newSDConfigFromJSON(
		[]byte(`{
			"discoverer":{"fixture":{}},
			"services":[{"id":"service","match":"true"}]
		}`),
		"job",
		"user=test",
		"dyncfg",
		"fixture",
		pipelineKey("fixture", "job"),
	)
	require.NoError(t, err)
	discovery.handler.AddDiscoveredConfig(config, dyncfg.StatusRunning)

	ctx, cancel := context.WithCancel(context.Background())
	fn := dyncfg.NewFunction(ctx, functions.Function{
		UID:  "queued-disable",
		Name: "config",
		Args: []string{
			discovery.dyncfgJobID("fixture", "job"),
			string(dyncfg.CommandDisable),
		},
	})
	done := make(chan struct{})
	go func() {
		discovery.dyncfgConfig(fn)
		close(done)
	}()

	var command dyncfg.Function
	select {
	case command = <-discovery.dyncfgCh:
	case <-time.After(time.Second):
		t.Fatal("dyncfg command was not queued")
	}
	cancel()

	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	select {
	case <-done:
	case <-timer.C:
		discovery.dyncfgSeqExec(command)
		discovery.completeDyncfg(command)
		<-done
		t.Fatal("canceled dyncfg handler waited for queued execution")
	}

	_, admission := discovery.beginDyncfg(dyncfg.NewFunction(
		context.Background(),
		functions.Function{UID: fn.UID()},
	))
	require.Equal(t, dyncfgAdmissionDuplicate, admission)
	discovery.failPendingDyncfg()
	discovery.dyncfgSeqExec(command)
	discovery.completeDyncfg(command)
	entry, ok := discovery.exposed.LookupByKey(config.ExposedKey())
	require.True(t, ok)
	require.Equal(t, dyncfg.StatusRunning, entry.Status)
	require.Empty(t, output.String())
}

func TestDyncfgAdmissionRejectsCommandsAfterShutdown(t *testing.T) {
	var output bytes.Buffer
	discovery, err := NewServiceDiscovery(Config{
		Epoch:        1,
		Attempts:     newTestAttemptAuthority(t),
		PluginName:   "test",
		DyncfgOutput: dyncfg.NewProtocolOutput(&output),
		Discoverers:  NewRegistry(),
	})
	require.NoError(t, err)
	discovery.ctx = context.Background()
	discovery.failPendingDyncfg()

	done := make(chan struct{})
	go func() {
		discovery.enqueueDyncfgFunction(dyncfg.NewFunction(t.Context(), functions.Function{
			UID:  "late",
			Name: "config",
			Args: []string{"test:sd:type:name", string(dyncfg.CommandRestart)},
		}))
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("late dyncfg admission blocked after shutdown")
	}
	assert.Contains(t, output.String(), "FUNCTION_RESULT_BEGIN late 503 application/json")
	assert.Empty(t, discovery.dyncfgCh)
}

func TestNewServiceDiscoveryUsesConfiguredDyncfgOutput(t *testing.T) {
	const pluginName = "test"

	var buf bytes.Buffer
	sd, err := NewServiceDiscovery(Config{
		Epoch:        1,
		Attempts:     newTestAttemptAuthority(t),
		PluginName:   pluginName,
		DyncfgOutput: dyncfg.NewProtocolOutput(&buf),
		Discoverers:  NewRegistry(),
	})
	require.NoError(t, err)

	sd.dyncfgApi.ConfigCreate(netdataapi.ConfigOpts{
		ID:                "test:sd:discoverer",
		Status:            dyncfg.StatusAccepted.String(),
		ConfigType:        dyncfg.ConfigTypeTemplate.String(),
		Path:              fmt.Sprintf(dyncfgSDPath, pluginName),
		SourceType:        "internal",
		Source:            "internal",
		SupportedCommands: "schema",
	})

	assert.Contains(t, buf.String(), "CONFIG test:sd:discoverer create accepted template /collectors/test/ServiceDiscovery")
}
