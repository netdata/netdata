// SPDX-License-Identifier: GPL-3.0-or-later

package docker

import (
	"context"
	"fmt"
	"strings"

	"github.com/docker/docker/api/types"
	typesContainer "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	typesImage "github.com/docker/docker/api/types/image"
)

func (c *Collector) collect() (map[string]int64, error) {
	if c.client == nil {
		client, err := c.newClient(c.Config)
		if err != nil {
			return nil, err
		}
		c.client = client
	}

	if !c.verNegotiated {
		c.verNegotiated = true
		c.negotiateAPIVersion()
	}

	defer func() { _ = c.client.Close() }()

	mx := make(map[string]int64)

	if err := c.collectInfo(mx); err != nil {
		return nil, err
	}
	if err := c.collectImages(mx); err != nil {
		return nil, err
	}
	if err := c.collectContainers(mx); err != nil {
		return nil, err
	}

	return mx, nil
}

func (c *Collector) collectInfo(mx map[string]int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.Timeout.Duration())
	defer cancel()

	info, err := c.client.Info(ctx)
	if err != nil {
		return err
	}

	mx["containers_state_running"] = int64(info.ContainersRunning)
	mx["containers_state_paused"] = int64(info.ContainersPaused)
	mx["containers_state_exited"] = int64(info.ContainersStopped)

	return nil
}

func (c *Collector) collectImages(mx map[string]int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.Timeout.Duration())
	defer cancel()

	images, err := c.client.ImageList(ctx, typesImage.ListOptions{})
	if err != nil {
		return err
	}

	mx["images_size"] = 0
	mx["images_dangling"] = 0
	mx["images_active"] = 0

	for _, v := range images {
		mx["images_size"] += v.Size
		if v.Containers == 0 {
			mx["images_dangling"]++
		} else {
			mx["images_active"]++
		}
	}

	return nil
}

var (
	containerHealthStatuses = []string{
		types.Healthy,
		types.Unhealthy,
		types.Starting,
		types.NoHealthcheck,
	}
	containerStates = []string{
		"created",
		"running",
		"paused",
		"restarting",
		"removing",
		"exited",
		"dead",
	}
)

func (c *Collector) collectContainers(mx map[string]int64) error {
	containerSet := make(map[string][]types.Container)

	for _, status := range containerHealthStatuses {
		if err := func() error {
			ctx, cancel := context.WithTimeout(context.Background(), c.Timeout.Duration())
			defer cancel()

			containers, err := c.client.ContainerList(ctx, typesContainer.ListOptions{
				All:     true,
				Filters: filters.NewArgs(filters.KeyValuePair{Key: "health", Value: status}),
				Size:    c.CollectContainerSize,
			})
			if err != nil {
				return err
			}
			containerSet[status] = containers
			return nil

		}(); err != nil {
			return err
		}
	}

	seen := make(map[string]bool)

	for _, s := range containerHealthStatuses {
		mx["containers_health_status_"+s] = 0
	}
	mx["containers_health_status_not_running_unhealthy"] = 0

	for status, containers := range containerSet {
		if status != types.Unhealthy {
			mx["containers_health_status_"+status] = int64(len(containers))
		}

		for _, cntr := range containers {
			if status == types.Unhealthy {
				if cntr.State == "running" {
					mx["containers_health_status_"+status] += 1
				} else {
					mx["containers_health_status_not_running_unhealthy"] += 1
				}
			}

			if hasIgnoreLabel(cntr) {
				continue
			}

			if len(cntr.Names) == 0 {
				continue
			}

			name := strings.TrimPrefix(cntr.Names[0], "/")

			if c.cntrSr != nil && !c.cntrSr.MatchString(name) {
				continue
			}

			seen[name] = true

			if !c.containers[name] {
				c.containers[name] = true
				c.addContainerCharts(name, cntr.Image)
			}

			px := fmt.Sprintf("container_%s_", name)

			for _, s := range containerHealthStatuses {
				mx[px+"health_status_"+s] = 0
			}
			mx[px+"health_status_not_running_unhealthy"] = 0
			for _, s := range containerStates {
				mx[px+"state_"+s] = 0
			}

			if status == types.Unhealthy && cntr.State != "running" {
				mx[px+"health_status_not_running_unhealthy"] += 1
			} else {
				mx[px+"health_status_"+status] = 1
			}
			mx[px+"state_"+cntr.State] = 1
			mx[px+"size_rw"] = cntr.SizeRw
			mx[px+"size_root_fs"] = cntr.SizeRootFs
		}
	}

	for name := range c.containers {
		if !seen[name] {
			delete(c.containers, name)
			c.removeContainerCharts(name)
		}
	}

	return nil
}

func (c *Collector) negotiateAPIVersion() {
	ctx, cancel := context.WithTimeout(context.Background(), c.Timeout.Duration())
	defer cancel()

	c.client.NegotiateAPIVersion(ctx)
}

func hasIgnoreLabel(cntr types.Container) bool {
	v, _ := cntr.Labels["netdata.cloud/ignore"]
	return strings.EqualFold(v, "true") || strings.EqualFold(v, "yes")
}
