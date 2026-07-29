// SPDX-License-Identifier: GPL-3.0-or-later

package dockersd

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"

	docker "github.com/moby/moby/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiscoverer_Test(t *testing.T) {
	tests := map[string]struct {
		listErr error
		wantErr string
	}{
		"reachable endpoint": {},
		"unreachable endpoint": {
			listErr: errors.New("connection refused"),
			wantErr: "list Docker containers: connection refused",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			d, err := NewDiscoverer(Config{Timeout: confopt.Duration(100 * time.Millisecond)})
			require.NoError(t, err)

			client := &testDockerClient{listErr: test.listErr}
			d.newDockerClient = func(string) (dockerClient, error) {
				return client, nil
			}

			err = d.Test(context.Background())
			if test.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.EqualError(t, err, test.wantErr)
			}
			assert.True(t, client.hadDeadline, "ContainerList context has a deadline")
			assert.True(t, client.closed, "Docker client is closed")
		})
	}
}

func TestDiscoverer_Test_ClientCreationFailure(t *testing.T) {
	d, err := NewDiscoverer(Config{})
	require.NoError(t, err)

	d.newDockerClient = func(string) (dockerClient, error) {
		return nil, errors.New("invalid endpoint")
	}

	require.EqualError(t, d.Test(context.Background()), "create Docker client: invalid endpoint")
}

func TestConfig_UnreachableEndpoint(t *testing.T) {
	socket, err := os.CreateTemp("/tmp", "netdata-missing-docker-*.sock")
	require.NoError(t, err)
	socketPath := socket.Name()
	require.NoError(t, socket.Close())
	require.NoError(t, os.Remove(socketPath))

	cfg := Config{
		Address: "unix://" + socketPath,
		Timeout: confopt.Duration(100 * time.Millisecond),
	}

	err = TestConfig(context.Background(), cfg)
	require.ErrorContains(t, err, "list Docker containers:")
}

type testDockerClient struct {
	listErr     error
	hadDeadline bool
	closed      bool
}

func (c *testDockerClient) ContainerList(ctx context.Context, _ docker.ContainerListOptions) (docker.ContainerListResult, error) {
	_, c.hadDeadline = ctx.Deadline()
	return docker.ContainerListResult{}, c.listErr
}

func (c *testDockerClient) Close() error {
	c.closed = true
	return nil
}
