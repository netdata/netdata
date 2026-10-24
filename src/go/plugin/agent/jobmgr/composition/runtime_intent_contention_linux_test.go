// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gofrs/flock"
	agentdiscovery "github.com/netdata/netdata/go/plugins/plugin/agent/discovery"
	"github.com/netdata/netdata/go/plugins/plugin/dem/collector/lighthouse"
	managedruntime "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runtime"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runtimehost"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/stretchr/testify/require"
)

type graphContentionObservedOwner struct {
	host    *runtimehost.Host
	entered chan struct{}
}

func (o *graphContentionObservedOwner) Prepare(previous, next collectorapi.JobConfigIdentity, config map[string]any) (collectorapi.PreparedJobConfigIntent, error) {
	if next.Valid() {
		o.entered <- struct{}{}
	}
	return o.host.Prepare(previous, next, config)
}

func TestRuntimeIntentReservationYieldsGraphClaim(t *testing.T) {
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		t.Skip("managed browser runtime requires amd64 or arm64")
	}
	storePath := filepath.Join(t.TempDir(), "runtime")
	store, err := managedruntime.Open(managedruntime.DefaultConfig(storePath))
	require.NoError(t, err)
	manifest := managedruntime.Manifest{
		Format: 1, Version: "graph-contention", Platform: "linux/" + runtime.GOARCH,
		Archive:  managedruntime.Archive{SHA256: strings.Repeat("a", 64), Bytes: 1, TarBytes: 1024, ExpandedBytes: 1, Entries: 5},
		Layout:   managedruntime.Layout{Node: "node", Browser: "browser", Loader: "loader", Dependencies: "dependencies", Fonts: "fonts"},
		Bindings: []managedruntime.Binding{{Path: "node", Expected: []byte{0}}, {Path: "browser", Expected: []byte{0}}},
	}
	require.NoError(t, manifest.Validate())
	host := runtimehost.New(runtimehost.Config{Store: store, Manifest: &manifest})
	owner := &graphContentionObservedOwner{host: host, entered: make(chan struct{}, 4)}
	reader, writer := io.Pipe()
	output := newProcessSynchronizedBuffer()
	cfg := testProductionProcessConfig(reader, output)
	cfg.AutoEnable = false
	cfg.Services = []ProcessService{host}
	creator := lighthouse.Creator(lighthouse.Dependencies{})
	creator.JobConfigIntent = owner
	creator.ActivationDependencies = host.Dependencies
	cfg.Modules["test"] = creator
	cfg.DiscoveryProviders = []agentdiscovery.ProviderFactory{agentdiscovery.NewProviderFactory("test", func(agentdiscovery.BuildContext) (agentdiscovery.Discoverer, bool, error) {
		var configs []confgroup.Config
		for _, name := range []string{"browser", "other"} {
			configs = append(configs, confgroup.Config{"module": "test", "name": name, "url": "https://example.com", "__source_type__": "user", "__source__": "test", "__provider__": "test"})
		}
		return runTestDiscoverer{configs: configs}, true, nil
	})}
	process, err := NewProcess(cfg)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	done := make(chan error, 1)
	go func() { done <- process.Run(ctx) }()
	coordination := flock.New(filepath.Join(storePath, "coordination.lock"))
	t.Cleanup(func() {
		require.NoError(t, coordination.Close())
		shutdown, stop := context.WithTimeout(context.Background(), 3*time.Second)
		defer stop()
		defer cancel()
		defer reader.Close()
		defer writer.Close()
		require.NoError(t, process.Terminate(shutdown))
		select {
		case err := <-done:
			require.NoError(t, err)
		case <-shutdown.Done():
			t.Error("process did not join")
		}
		require.NoError(t, host.Close())
		require.NoError(t, store.Close())
		if t.Failed() {
			t.Log(output.String())
		}
	})
	output.waitContains(t, "CONFIG go.d:collector:test:browser create accepted")
	output.waitContains(t, "CONFIG go.d:collector:test:other create accepted")
	require.NoError(t, coordination.Lock())
	_, err = fmt.Fprintln(writer, `FUNCTION browser-enable 8 "config go.d:collector:test:browser enable" 0xFFFF "user=test"`)
	require.NoError(t, err)
	select {
	case <-owner.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("browser did not enter actual runtime intent owner")
	}
	start := time.Now()
	_, err = fmt.Fprintln(writer, `FUNCTION other-disable 8 "config go.d:collector:test:other disable" 0xFFFF "user=test"`)
	require.NoError(t, err)
	otherDone := func() bool { return strings.Contains(output.String(), "FUNCTION_RESULT_BEGIN other-disable 200 ") }
	require.Eventually(t, otherDone, time.Second, 5*time.Millisecond, "unrelated graph mutation blocked behind runtime reservation")
	t.Logf("unrelated mutation completed while coordination held: %s", time.Since(start))
	_, err = fmt.Fprintln(writer, `FUNCTION browser-disable 8 "config go.d:collector:test:browser disable" 0xFFFF "user=test"`)
	require.NoError(t, err)
	require.Never(t, func() bool { return strings.Contains(output.String(), "FUNCTION_RESULT_BEGIN browser-disable ") }, 50*time.Millisecond, time.Millisecond, "same-job mutation bypassed the reservation lane")
	require.NotContains(t, output.String(), "FUNCTION_RESULT_BEGIN browser-enable 202 ")
	require.NoError(t, coordination.Unlock())
	output.waitContains(t, "FUNCTION_RESULT_BEGIN browser-enable 202 ")
	output.waitContains(t, "FUNCTION_RESULT_BEGIN other-disable 200 ")
	output.waitContains(t, "FUNCTION_RESULT_BEGIN browser-disable 200 ")
	t.Logf("both mutations completed after coordination release: %s", time.Since(start))
}
