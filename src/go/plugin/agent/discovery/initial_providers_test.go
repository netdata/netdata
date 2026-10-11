// SPDX-License-Identifier: GPL-3.0-or-later

package discovery_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery"
	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery/dummy"
	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery/file"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/stretchr/testify/require"
)

func TestFileAndDummyInitialSnapshotsReachAppliedBoundary(t *testing.T) {
	for _, empty := range []bool{false, true} {
		name := "configured"
		if empty {
			name = "empty"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			readPath := filepath.Join(t.TempDir(), "module.conf")
			watchPath := filepath.Join(t.TempDir(), "module.conf")
			if !empty {
				require.NoError(t, os.WriteFile(readPath, []byte("jobs:\n - name: read-job\n"), 0600))
				require.NoError(t, os.WriteFile(watchPath, []byte("jobs:\n - name: watched-job\n"), 0600))
			}
			reg := confgroup.Registry{"module": {}}
			f, err := file.NewDiscovery(file.Config{Registry: reg, Read: []string{readPath}, Watch: []string{watchPath}})
			require.NoError(t, err)
			d, err := dummy.NewDiscovery(dummy.Config{Registry: reg, Names: []string{"module"}})
			require.NoError(t, err)
			catalog, err := discovery.NewProviderCatalog([]discovery.ProviderFactory{
				discovery.NewProviderFactory("file", func(discovery.BuildContext) (discovery.Discoverer, bool, error) { return f, true, nil }),
				discovery.NewProviderFactory("dummy", func(discovery.BuildContext) (discovery.Discoverer, bool, error) { return d, true, nil }),
			})
			require.NoError(t, err)
			initial := make(chan struct{})
			pipeline, err := discovery.NewPipelineGeneration(discovery.PipelineConfig{Providers: catalog, BuildContext: discovery.BuildContext{Registry: reg}, InitialApplied: func() error { close(initial); return nil }})
			require.NoError(t, err)
			seen := map[string]bool{}
			done := make(chan error, 3)
			go func() {
				done <- pipeline.Run(ctx, func(_ context.Context, groups []*confgroup.Group) error {
					for _, group := range groups {
						seen[group.Source] = true
					}
					return nil
				})
			}()
			for _, provider := range pipeline.ProviderNames() {
				go func() { done <- pipeline.RunProvider(ctx, provider) }()
			}
			select {
			case <-initial:
			case <-time.After(5 * time.Second):
				t.Fatal("initial snapshot did not finish")
			}
			cancel()
			for range 3 {
				require.NoError(t, <-done)
			}
			require.True(t, seen["internal"])
			if !empty {
				require.True(t, seen[readPath])
				require.True(t, seen[watchPath])
			}
		})
	}
}
