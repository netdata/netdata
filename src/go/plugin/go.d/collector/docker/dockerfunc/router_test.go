// SPDX-License-Identifier: GPL-3.0-or-later

package dockerfunc

import (
	"context"
	"errors"
	"testing"

	typesContainer "github.com/moby/moby/api/types/container"
	docker "github.com/moby/moby/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockDeps struct {
	client DockerClient
	err    error
}

func (m mockDeps) DockerClient() (DockerClient, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.client == nil {
		return nil, errors.New("collector docker client is not ready")
	}
	return m.client, nil
}

type mockDockerClient struct {
	containers []typesContainer.Summary
	listErr    error
}

func (m mockDockerClient) ContainerList(ctx context.Context, opts docker.ContainerListOptions) (docker.ContainerListResult, error) {
	if m.listErr != nil {
		return docker.ContainerListResult{}, m.listErr
	}
	return docker.ContainerListResult{Items: m.containers}, nil
}

func (m mockDockerClient) ImageList(ctx context.Context, opts docker.ImageListOptions) (docker.ImageListResult, error) {
	return docker.ImageListResult{}, nil
}

func TestDockerMethods(t *testing.T) {
	methods := Methods()

	require.Len(t, methods, 1)
	assert.Equal(t, containersMethodID, methods[0].ID)
	assert.Equal(t, "Containers", methods[0].Name)
	assert.Empty(t, methods[0].RequiredParams)
}

func TestRouter_NotFound(t *testing.T) {
	r := newRouter(mockDeps{client: mockDockerClient{}})
	resp := r.Handle(context.Background(), "unknown-method", nil)
	require.NotNil(t, resp)
	assert.Equal(t, 404, resp.Status)
}

func TestRouter_MethodParamsUnknownMethod(t *testing.T) {
	r := newRouter(mockDeps{client: mockDockerClient{}})
	_, err := r.MethodParams(context.Background(), "unknown-method")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown method")
}
