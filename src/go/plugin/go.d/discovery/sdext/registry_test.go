// SPDX-License-Identifier: GPL-3.0-or-later

package sdext

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery/sd/model"
	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery/sd/pipeline"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/discovery/sdext/discoverer/dockersd"
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
