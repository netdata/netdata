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
	functionadapter "github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/functions"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
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
			if _, err := NewProcess(config); err == nil {
				t.Fatal("invalid initial vnode was accepted")
			}
		})
	}
}

func TestProductionProcessIsSingleUse(t *testing.T) {
	reader, writer := io.Pipe()
	var output bytes.Buffer
	process, err := NewProcess(testProductionProcessConfig(reader, &output))
	if err != nil {
		t.Fatal(err)
	}
	runDone := make(chan error, 1)
	go func() {
		runDone <- process.Run(context.Background())
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	if err := process.Terminate(ctx); err != nil {
		cancel()
		t.Fatal(err)
	}
	cancel()
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := <-runDone; err != nil {
		t.Fatal(err)
	}
	if err := process.Run(context.Background()); err == nil {
		t.Fatal("second production Run was accepted")
	}
}

func TestProductionProcessQuitHasOneCleanTerminalDisposition(t *testing.T) {
	for iteration := 0; iteration < 32; iteration++ {
		process, err := NewProcess(
			testProductionProcessConfig(strings.NewReader("QUIT\n"), io.Discard),
		)
		if err != nil {
			t.Fatal(err)
		}
		if err := process.Run(context.Background()); err != nil {
			t.Fatalf("QUIT iteration %d returned %v", iteration, err)
		}
	}
}

func TestProductionProcessChargesCatalogStorageUntilFinalClose(t *testing.T) {
	process, err := NewProcess(
		testProductionProcessConfig(
			strings.NewReader("QUIT\n"),
			io.Discard,
		),
	)
	if err != nil {
		t.Fatal(err)
	}
	wantProcessBytes := functionadapter.MaximumCatalogStorageBytes
	if census := process.core.admission.Census(); census.ProcessBytes != wantProcessBytes {
		t.Fatalf("construction admission census=%+v", census)
	}
	if err := process.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if census := process.core.admission.Census(); census.ProcessBytes != 0 || census.Phase != "closed" {
		t.Fatalf("final admission census=%+v", census)
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
				t.Fatal("process control was not accepted")
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
					t.Fatal("accepted process control did not complete")
				}
			}
			if returnedEarly {
				t.Fatalf(
					"accepted process control returned before disposition: %v",
					early,
				)
			}
			if early != nil {
				t.Fatalf("process control disposition: %v", early)
			}
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
