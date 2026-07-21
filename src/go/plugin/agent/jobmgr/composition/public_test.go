// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	agentdiscovery "github.com/netdata/netdata/go/plugins/plugin/agent/discovery"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
	"github.com/stretchr/testify/require"
)

func TestProductionProcessRejectsInvalidInitialVnodes(t *testing.T) {
	tests := map[string]struct {
		vnodes map[string]*vnodes.VirtualNode
	}{
		"nil vnode": {
			vnodes: map[string]*vnodes.VirtualNode{"missing": nil},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			config := testProductionProcessConfig(strings.NewReader(""), io.Discard)
			config.InitialVnodes = test.vnodes

			_, err := NewProcess(config)
			require.Error(t, err)
		})
	}
}

func TestProductionProcessIsSingleUse(t *testing.T) {
	reader, writer := io.Pipe()
	var output bytes.Buffer
	process, err := NewProcess(testProductionProcessConfig(reader, &output))
	require.NoError(t, err)
	runDone := make(chan error, 1)
	go func() {
		runDone <- process.Run(context.Background())
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	if err := process.Terminate(ctx); err != nil {
		cancel()
		require.FailNow(t, "test failed", err)
	}
	cancel()

	require.NoError(t, writer.Close())

	require.NoError(t, <-runDone)

	require.Error(t, process.Run(context.Background()))
}

func TestProductionProcessQuitHasOneCleanTerminalDisposition(t *testing.T) {
	for range 32 {
		process, err := NewProcess(testProductionProcessConfig(strings.NewReader("QUIT\n"), io.Discard))
		require.NoError(t, err)

		require.NoError(t, process.Run(context.Background()))
	}
}

func TestProcessControlCancellationAfterHandoffWaitsForDisposition(t *testing.T) {
	tests := map[string]struct {
		call func(*Process, context.Context) error
		want processCommand
	}{
		"restart": {
			call: (*Process).Restart,
			want: processRestart,
		},
		"terminate": {
			call: (*Process).Terminate,
			want: processTerminate,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			started := make(chan struct{})
			close(started)
			process := &Process{
				commands: make(chan processControl),
				started:  started,
				done:     make(chan struct{}),
			}
			accepted := make(chan struct{})
			release := make(chan struct{})
			go func() {
				control := <-process.commands
				if control.command != test.want {
					control.result <- errors.New("unexpected command")
					return
				}
				close(accepted)
				<-release
				control.result <- nil
			}()
			ctx, cancel := context.WithCancel(context.Background())
			result := make(chan error, 1)
			go func() {
				result <- test.call(process, ctx)
			}()
			select {
			case <-accepted:
			case <-time.After(time.Second):
				require.FailNow(t, "test failed", "process control was not accepted")
			}
			cancel()

			var early error
			returnedEarly := false
			select {
			case early = <-result:
				returnedEarly = true
			case <-time.After(100 * time.Millisecond):
			}
			close(release)
			if !returnedEarly {
				select {
				case early = <-result:
				case <-time.After(time.Second):
					require.FailNow(t, "test failed", "accepted process control did not complete")
				}
			}
			require.False(t, returnedEarly)
			require.Nil(t, early)
		})
	}
}

func testProductionProcessConfig(
	input io.Reader,
	output io.Writer,
) Config {
	factory := agentdiscovery.NewProviderFactory(
		"test",
		func(agentdiscovery.BuildContext) (
			agentdiscovery.Discoverer,
			bool,
			error,
		) {
			return runTestDiscoverer{}, true, nil
		},
	)
	return Config{
		Input: input, Output: output,
		PluginName: "go.d",
		Modules:    collectorapi.Registry{"test": {}},
		Defaults:   confgroup.Registry{"test": {}},
		DiscoveryProviders: []agentdiscovery.ProviderFactory{
			factory,
		},
		ShutdownTimeout: time.Second,
	}
}
