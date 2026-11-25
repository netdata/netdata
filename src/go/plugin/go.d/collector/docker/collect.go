// SPDX-License-Identifier: GPL-3.0-or-later

package docker

import (
	"context"
	"fmt"
	"time"

	typesContainer "github.com/moby/moby/api/types/container"
	typesSystem "github.com/moby/moby/api/types/system"
	docker "github.com/moby/moby/client"
)

func (c *Collector) collect(ctx context.Context) error {
	state, err := c.queryDaemon(ctx)
	if err != nil {
		return err
	}

	c.writeMetrics(state)

	return nil
}

// daemonState is what one collection reads from the Docker daemon. Images come from the cached snapshot in c.images.
type daemonState struct {
	info       typesSystem.Info
	containers []listedContainer
}

// listedContainer is a container as returned by the list request filtered by its health status.
type listedContainer struct {
	typesContainer.Summary
	healthStatus typesContainer.HealthStatus
}

// queryDaemon makes every Docker API call of a collection, so a failed call aborts the collection before any metric
// is written.
func (c *Collector) queryDaemon(ctx context.Context) (*daemonState, error) {
	if c.client == nil {
		client, err := c.newClient(c.Config)
		if err != nil {
			return nil, fmt.Errorf("creating Docker client: %w", err)
		}
		c.client = client
	}

	info, err := c.systemInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting system info: %w", err)
	}
	if err := c.refreshImages(ctx); err != nil {
		return nil, fmt.Errorf("listing images: %w", err)
	}
	containers, err := c.listContainers(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing containers: %w", err)
	}

	return &daemonState{
		info:       info,
		containers: containers,
	}, nil
}

func (c *Collector) systemInfo(ctx context.Context) (typesSystem.Info, error) {
	ctx, cancel := context.WithTimeout(ctx, c.Timeout.Duration())
	defer cancel()

	result, err := c.client.Info(ctx, docker.InfoOptions{})
	if err != nil {
		return typesSystem.Info{}, err
	}
	return result.Info, nil
}

// On Docker Engine 29+ with the containerd image store, listing images can cost the daemon over a second of CPU time
// per call (moby/moby#53077). Image metrics change rarely, so the list is refreshed at most this often.
const imagesRefreshEvery = 5 * time.Minute

type imagesSnapshot struct {
	ids  []string
	size int64
}

func (c *Collector) refreshImages(ctx context.Context) error {
	now := c.now()
	if now.Before(c.imagesNextRefresh) {
		return nil
	}

	ctx, cancel := context.WithTimeout(ctx, c.Timeout.Duration())
	defer cancel()

	result, err := c.client.ImageList(ctx, docker.ImageListOptions{})
	if err != nil {
		return err
	}

	var images imagesSnapshot
	for _, v := range result.Items {
		images.ids = append(images.ids, v.ID)
		images.size += v.Size
	}
	c.images = images
	c.imagesNextRefresh = now.Add(imagesRefreshEvery)

	return nil
}

var containerHealthStatuses = []typesContainer.HealthStatus{
	typesContainer.Healthy,
	typesContainer.Unhealthy,
	typesContainer.Starting,
	typesContainer.NoHealthcheck,
}

// listContainers lists the containers once per health status, in containerHealthStatuses order, so each container
// comes with its health status.
func (c *Collector) listContainers(ctx context.Context) ([]listedContainer, error) {
	var containers []listedContainer

	for _, status := range containerHealthStatuses {
		items, err := c.listContainersWithHealth(ctx, status)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			containers = append(containers, listedContainer{
				Summary:      item,
				healthStatus: status,
			})
		}
	}

	return containers, nil
}

func (c *Collector) listContainersWithHealth(
	ctx context.Context,
	status typesContainer.HealthStatus,
) ([]typesContainer.Summary, error) {
	ctx, cancel := context.WithTimeout(ctx, c.Timeout.Duration())
	defer cancel()

	result, err := c.client.ContainerList(ctx, docker.ContainerListOptions{
		All:     true,
		Filters: make(docker.Filters).Add("health", string(status)),
		Size:    c.CollectContainerSize,
	})
	if err != nil {
		return nil, err
	}
	return result.Items, nil
}
