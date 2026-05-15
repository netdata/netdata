// SPDX-License-Identifier: GPL-3.0-or-later

package dockerfunc

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFuncContainers_HandleUnavailable(t *testing.T) {
	r := newRouter(mockDeps{err: errors.New("collector docker client is not ready")})

	resp := r.Handle(context.Background(), containersMethodID, nil)

	require.NotNil(t, resp)
	assert.Equal(t, 503, resp.Status)
	assert.Contains(t, resp.Message, "initializing")
}

func TestFuncContainers_HandleTimeout(t *testing.T) {
	r := newRouter(mockDeps{
		client: mockDockerClient{listErr: context.DeadlineExceeded},
	})

	resp := r.Handle(context.Background(), containersMethodID, nil)

	require.NotNil(t, resp)
	assert.Equal(t, 504, resp.Status)
}

func TestFuncContainers_HandleSuccess(t *testing.T) {
	now := time.Now()
	newerCreated := now.Add(-2 * time.Hour).Unix()
	olderCreated := now.Add(-24 * time.Hour).Unix()

	r := newRouter(mockDeps{
		client: mockDockerClient{containers: []types.Container{
			{
				ID:      "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				Image:   "postgres:16",
				Command: "docker-entrypoint.sh postgres",
				Created: olderCreated,
				Status:  "Exited (0) 4 days ago",
				State:   "exited",
				Ports: []types.Port{
					{IP: "[::]", PrivatePort: 5432, PublicPort: 5432, Type: "tcp"},
					{IP: "0.0.0.0", PrivatePort: 5432, PublicPort: 5432, Type: "tcp"},
				},
				Names: []string{"/postgres-dev"},
			},
			{
				ID:      "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
				Image:   "almalinux:8",
				Command: "sleep infinity",
				Created: newerCreated,
				State:   "exited",
				Names:   []string{"/alma8"},
			},
		}},
	})

	resp := r.Handle(context.Background(), containersMethodID, nil)

	require.NotNil(t, resp)
	assert.Equal(t, 200, resp.Status)
	assert.Equal(t, containersColCreatedUnix, resp.DefaultSortColumn)
	assert.Equal(t, containersMethodHelp, resp.Help)

	columns := resp.Columns
	require.NotNil(t, columns)
	for _, key := range []string{
		containersColID,
		containersColImage,
		containersColCommand,
		containersColCreated,
		containersColStatus,
		containersColState,
		containersColPorts,
		containersColNames,
		containersColIDFull,
		containersColCreatedUnix,
	} {
		_, ok := columns[key]
		assert.True(t, ok, "missing column %s", key)
	}

	data, ok := resp.Data.([][]any)
	require.True(t, ok)
	require.Len(t, data, 2)

	// sorted by Created desc: "alma8" first
	assert.Equal(t, "alma8", data[0][colNames])
	assert.Equal(t, newerCreated, data[0][colCreatedUnix])
	assert.Equal(t, "bbbbbbbbbbbb", data[0][colContainerID])
	assert.Contains(t, data[0][colCreated].(string), "ago")
	assert.Equal(t, "sleep infinity", data[0][colCommand])
	assert.Equal(t, "Exited", data[0][colStatus])
	assert.Equal(t, "exited", data[0][colState])

	// second row includes formatted ports and truncated command
	assert.Equal(t, "postgres-dev", data[1][colNames])
	assert.Equal(t, "0.0.0.0:5432->5432/tcp, [::]:5432->5432/tcp", data[1][colPorts])
	assert.Equal(t, "aaaaaaaaaaaa", data[1][colContainerID])
	assert.Equal(t, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", data[1][colContainerIDFull])
	assert.Equal(t, "docker-entrypoint.sh postgres", data[1][colCommand])
	assert.Equal(t, "exited", data[1][colState])
}

func TestFormatHelpers(t *testing.T) {
	t.Run("shortContainerID", func(t *testing.T) {
		assert.Equal(t, "123456789012", shortContainerID("1234567890123456"))
		assert.Equal(t, "abc", shortContainerID("abc"))
	})

	t.Run("formatContainerNames", func(t *testing.T) {
		assert.Equal(t, "a, b", formatContainerNames([]string{"/a", " /b "}))
		assert.Equal(t, "", formatContainerNames(nil))
	})

	t.Run("formatCreated", func(t *testing.T) {
		now := time.Unix(1_730_000_000, 0)
		assert.Equal(t, "", formatCreated(0, now))
		assert.Contains(t, formatCreated(now.Add(-48*time.Hour).Unix(), now), "ago")
	})

	t.Run("formatPorts", func(t *testing.T) {
		ports := []types.Port{
			{IP: "[::]", PrivatePort: 5432, PublicPort: 5432, Type: "tcp"},
			{IP: "0.0.0.0", PrivatePort: 5432, PublicPort: 5432, Type: "tcp"},
			{PrivatePort: 80, Type: "tcp"},
		}
		assert.Equal(t, "80/tcp, 0.0.0.0:5432->5432/tcp, [::]:5432->5432/tcp", formatPorts(ports))
		assert.Equal(t, "", formatPorts(nil))
	})
}
