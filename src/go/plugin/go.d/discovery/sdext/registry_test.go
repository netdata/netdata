// SPDX-License-Identifier: GPL-3.0-or-later

package sdext

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery/sd/model"
	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery/sd/pipeline"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/discovery/sdext/discoverer/dockersd"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/discovery/sdext/discoverer/netlistensd"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegistry_DockerInclusion(t *testing.T) {
	withDocker := Registry(true)
	assert.Contains(t, withDocker.Types(), discovererHTTP)
	assert.Contains(t, withDocker.Types(), discovererDocker)

	withoutDocker := Registry(false)
	assert.Contains(t, withoutDocker.Types(), discovererHTTP)
	assert.NotContains(t, withoutDocker.Types(), discovererDocker)
}

func TestRegistry_DockerOperationalTestRejectsUnreachableEndpoint(t *testing.T) {
	raw, err := json.Marshal(dockersd.Config{
		Address: fmt.Sprintf("unix:///tmp/netdata-sd-%d.sock", time.Now().UnixNano()),
		Timeout: confopt.Duration(50 * time.Millisecond),
	})
	require.NoError(t, err)
	registry := Registry(true)

	candidate, err := pipeline.New(
		pipeline.Config{
			Name: "docker-test",
			Discoverer: pipeline.DiscovererPayload{
				Kind:   discovererDocker,
				Config: raw,
			},
			Services: []pipeline.ServiceRuleConfig{{
				ID:    "test",
				Match: "true",
			}},
		},
		func(payload pipeline.DiscovererPayload, source string) ([]model.Discoverer, error) {
			descriptor, ok := registry.Get(payload.Type())
			require.True(t, ok)
			config, err := descriptor.ParseJSONConfig(payload.Config)
			if err != nil {
				return nil, err
			}
			return descriptor.NewDiscoverers(config, source)
		},
	)
	require.NoError(t, err)

	fullyTested, err := candidate.Test(t.Context())

	require.False(t, fullyTested)
	require.Error(t, err)
	require.ErrorContains(t, err, "cannot connect to the configured Docker endpoint")
}

func TestRegistry_NetListenersOperationalTestUsesShippedConstructionPath(t *testing.T) {
	fixture := prepareNetListenersExecutableFixture(t)

	raw, err := json.Marshal(netlistensd.Config{
		Timeout: confopt.Duration(5 * time.Second),
	})
	require.NoError(t, err)
	registry := Registry(true)
	descriptor, ok := registry.Get(discovererNetListeners)
	require.True(t, ok)
	config, err := descriptor.ParseJSONConfig(raw)
	require.NoError(t, err)
	discoverers, err := descriptor.NewDiscoverers(config, "dyncfg=user=test")
	require.NoError(t, err)
	require.Len(t, discoverers, 1)
	_, ok = discoverers[0].(dyncfg.Testable)
	require.True(t, ok)

	candidate, err := pipeline.New(
		pipeline.Config{
			Name: "net-listeners-test",
			Discoverer: pipeline.DiscovererPayload{
				Kind:   discovererNetListeners,
				Config: raw,
			},
			Services: []pipeline.ServiceRuleConfig{{
				ID:    "test",
				Match: "true",
			}},
		},
		func(payload pipeline.DiscovererPayload, source string) ([]model.Discoverer, error) {
			descriptor, ok := registry.Get(payload.Type())
			require.True(t, ok)
			config, err := descriptor.ParseJSONConfig(payload.Config)
			if err != nil {
				return nil, err
			}
			return descriptor.NewDiscoverers(config, source)
		},
	)
	require.NoError(t, err)

	fullyTested, err := candidate.Test(t.Context())

	require.NoError(t, err)
	require.True(t, fullyTested)
	invocations, err := os.ReadFile(fixture.invocationsPath)
	require.NoError(t, err)
	require.Equal(t, "nd-run\ninvoked\n", string(invocations))
	args, err := os.ReadFile(fixture.argsPath)
	require.NoError(t, err)
	require.Equal(
		t,
		"no-udp6 no-local no-inbound no-outbound no-namespaces",
		strings.TrimSpace(string(args)),
	)
}

func TestRegistry_NetListenersOperationalTestHonorsConfiguredTimeout(t *testing.T) {
	prepareNetListenersExecutableFixture(t)
	t.Setenv("NETDATA_TEST_LOCAL_LISTENER_MODE", "timeout")

	raw, err := json.Marshal(netlistensd.Config{
		Timeout: confopt.Duration(100 * time.Millisecond),
	})
	require.NoError(t, err)
	descriptor, ok := Registry(true).Get(discovererNetListeners)
	require.True(t, ok)
	config, err := descriptor.ParseJSONConfig(raw)
	require.NoError(t, err)
	discoverers, err := descriptor.NewDiscoverers(config, "dyncfg=user=test")
	require.NoError(t, err)
	require.Len(t, discoverers, 1)
	testable, ok := discoverers[0].(dyncfg.Testable)
	require.True(t, ok)

	err = testable.Test(t.Context())

	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Equal(t, "local listener inspection did not complete before the timeout", err.Error())
}

type netListenersExecutableFixture struct {
	invocationsPath string
	argsPath        string
}

func prepareNetListenersExecutableFixture(t *testing.T) netListenersExecutableFixture {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("uses shell executable fixtures")
	}

	tmp := t.TempDir()
	fixture := netListenersExecutableFixture{
		invocationsPath: filepath.Join(tmp, "invocations"),
		argsPath:        filepath.Join(tmp, "args"),
	}
	t.Setenv("NETDATA_PLUGINS_DIR", tmp)
	t.Setenv("NETDATA_TEST_LOCAL_LISTENER_INVOCATIONS", fixture.invocationsPath)
	t.Setenv("NETDATA_TEST_LOCAL_LISTENER_ARGS", fixture.argsPath)

	localListeners := filepath.Join(tmp, "local-listeners")
	require.NoError(t, os.WriteFile(localListeners, []byte(`#!/bin/sh
printf 'invoked\n' >> "$NETDATA_TEST_LOCAL_LISTENER_INVOCATIONS"
printf '%s\n' "$*" >> "$NETDATA_TEST_LOCAL_LISTENER_ARGS"
case "$NETDATA_TEST_LOCAL_LISTENER_MODE" in
failure)
	printf 'private helper failure [REDACTED_SECRET]\n' >&2
	exit 1
	;;
invalid)
	printf 'invalid [REDACTED_SECRET]\n'
	;;
timeout)
	sleep 30
	;;
*)
	printf 'TCP|127.0.0.1|19999|/usr/bin/example\n'
	;;
esac
`), 0o755))

	ndRun := filepath.Join(tmp, "nd-run")
	require.NoError(t, os.WriteFile(ndRun, []byte(`#!/bin/sh
printf 'nd-run\n' >> "$NETDATA_TEST_LOCAL_LISTENER_INVOCATIONS"
exec "$@"
`), 0o755))
	t.Cleanup(ndexec.SetRunnerPathsForTests(ndRun, ""))

	return fixture
}
