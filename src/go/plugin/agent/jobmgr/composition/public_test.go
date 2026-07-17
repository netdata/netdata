// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"

	agentdiscovery "github.com/netdata/netdata/go/plugins/plugin/agent/discovery"
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
