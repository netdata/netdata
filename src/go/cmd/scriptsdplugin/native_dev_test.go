// SPDX-License-Identifier: GPL-3.0-or-later

//go:build scripts_native_dev

package main

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/cmd/internal/discoveryproviders"
	"github.com/netdata/netdata/go/plugins/pkg/multipath"
	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func TestNativeAvailableInDevelopmentRegistry(t *testing.T) {
	_, nagios := collectorapi.DefaultRegistry.Lookup("nagios")
	creator, native := collectorapi.DefaultRegistry.Lookup("native")
	require.True(t, nagios)
	require.True(t, native)
	require.NotNil(t, creator.CreateV2)
}

func TestPackageInventoryAndExplicitExecution(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture uses a Unix interpreter")
	}
	dir := t.TempDir()
	manifest := filepath.Join(dir, "manifest.yaml")
	require.NoError(
		t,
		os.WriteFile(manifest, []byte("version: v1\ncommand: [/bin/sh]\n"), 0644),
	)
	inventory := filepath.Join(dir, "scripts.d.packages.yaml")
	require.NoError(
		t,
		os.WriteFile(
			inventory,
			[]byte("version: v1\npackages:\n  - name: example\n    manifest: "+manifest+"\n"),
			0644,
		),
	)
	base := collectorapi.Registry{
		"nagios": collectorapi.DefaultRegistry["nagios"],
	}
	registry, provider, err := configurePackages(multipath.MultiPath{dir}, base)
	require.NoError(t, err)
	require.Contains(t, registry, "native-example")
	require.NotContains(t, base, "native-example")
	defaults := confgroup.Registry{
		"nagios":         {},
		"native-example": {},
	}
	discoverer, enabled, err := provider.Build(
		discovery.BuildContext{
			Registry:   defaults,
			DummyNames: []string{"native-example", "nagios"},
		},
	)
	require.NoError(t, err)
	require.True(t, enabled)
	output := make(chan []*confgroup.Group, 1)
	discoverer.Run(context.Background(), output)
	groups := <-output
	require.Len(t, groups, 1)
	require.Len(t, groups[0].Configs, 1)
	require.Equal(t, "nagios", groups[0].Configs[0].Module())
	_, enabled, err = provider.Build(discovery.BuildContext{
		Registry:   defaults,
		DummyNames: []string{"native-example"},
	})
	require.NoError(t, err)
	require.False(t, enabled, "registration alone must not create a job")
	// The ordinary file provider still discovers explicitly configured package jobs.
	config := filepath.Join(dir, "native-example.conf")
	require.NoError(t, os.WriteFile(config, []byte("jobs:\n  - name: target\n    update_every: 10\n"), 0644))
	fileProvider := discoveryproviders.File()
	files, enabled, err := fileProvider.Build(discovery.BuildContext{
		Registry:  defaults,
		ReadPaths: []string{config},
	})
	require.NoError(t, err)
	require.True(t, enabled)
	output = make(chan []*confgroup.Group, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { defer close(done); files.Run(ctx, output) }()
	select {
	case groups = <-output:
		require.Len(t, groups, 1)
		require.Len(t, groups[0].Configs, 1)
		require.Equal(t, "native-example", groups[0].Configs[0].Module())
	case <-time.After(3 * time.Second):
		t.Fatal("file config not discovered")
	}
	cancel()
	<-done
	// A malformed higher-priority inventory must fail, never fall through to another directory.
	high := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(high, "scripts.d.packages.yaml"), []byte("invalid: true"), 0644))
	_, _, err = configurePackages(multipath.MultiPath{high, dir}, base)
	require.Error(t, err)
}

func TestCommandPackageInventory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture uses Unix scripts")
	}
	dir := t.TempDir()
	shim := filepath.Join(dir, "nd-run")
	require.NoError(t, os.WriteFile(shim, []byte("#!/bin/sh\nexec \"$@\"\n"), 0755))
	t.Cleanup(ndexec.SetRunnerPathsForTests(shim, ""))
	script := filepath.Join(dir, "package.sh")
	require.NoError(t, os.WriteFile(script, []byte(`#!/bin/sh
[ "$1" = describe ] || exit 2
printf describe >> "$(dirname "$0")/described"
printf '%s\n' 'version: v1'
`), 0755))
	data, err := yaml.Marshal(
		map[string]any{
			"version":  "v1",
			"packages": []any{map[string]any{"name": "example", "command": []string{"/bin/sh", script}}},
		},
	)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "scripts.d.packages.yaml"), data, 0644))
	registry, _, err := configurePackages(multipath.MultiPath{dir}, collectorapi.Registry{})
	require.NoError(t, err)
	c := registry["native-example"].CreateV2()
	require.NoError(t, c.Init(context.Background()))
	require.NoError(t, c.Check(context.Background()))
	observed, err := os.ReadFile(filepath.Join(dir, "described"))
	require.NoError(t, err)
	require.Equal(t, "describe", string(observed))
}
