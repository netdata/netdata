// SPDX-License-Identifier: GPL-3.0-or-later

package sd

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/netdata/netdata/go/plugins/plugin/framework/functions"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDyncfgConfigReturnsAfterQueuedCommandCompletes(t *testing.T) {
	var output bytes.Buffer
	discovery, err := NewServiceDiscovery(Config{
		PluginName:   "test",
		DyncfgOutput: dyncfg.NewProtocolOutput(&output),
		Discoverers:  NewRegistry(),
	})
	require.NoError(t, err)
	discovery.ctx = context.Background()
	done := make(chan struct{})
	go func() {
		discovery.dyncfgConfig(dyncfg.NewFunction(functions.Function{
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

func TestDyncfgAdmissionRejectsCommandsAfterShutdown(t *testing.T) {
	var output bytes.Buffer
	discovery, err := NewServiceDiscovery(Config{
		PluginName:   "test",
		DyncfgOutput: dyncfg.NewProtocolOutput(&output),
		Discoverers:  NewRegistry(),
	})
	require.NoError(t, err)
	discovery.ctx = context.Background()
	discovery.failPendingDyncfg()

	done := make(chan struct{})
	go func() {
		discovery.enqueueDyncfgFunction(dyncfg.NewFunction(functions.Function{
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
