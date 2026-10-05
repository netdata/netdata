// SPDX-License-Identifier: GPL-3.0-or-later

package docker

import (
	"context"
	"fmt"
	"strings"
	"time"

	typesContainer "github.com/moby/moby/api/types/container"
	docker "github.com/moby/moby/client"
)

func (c *Collector) collect() (map[string]int64, error) {
	if c.client == nil {
		client, err := c.newClient(c.Config)
		if err != nil {
			return nil, err
		}
		c.client = client
	}

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

	result, err := c.client.Info(ctx, docker.InfoOptions{})
	if err != nil {
		return err
	}
	info := result.Info

	mx["containers_state_running"] = int64(info.ContainersRunning)
	mx["containers_state_paused"] = int64(info.ContainersPaused)
	mx["containers_state_exited"] = int64(info.ContainersStopped)

	return nil
}

// On Docker Engine 29+ with the containerd image store, listing images can cost the daemon over a second of CPU time
// per call (moby/moby#53077). Image metrics change rarely, so the list is refreshed at most this often.
const imagesRefreshEvery = 5 * time.Minute

type imagesStats struct {
	size     int64
	dangling int64
	active   int64
}

func (c *Collector) collectImages(mx map[string]int64) error {
	if now := c.now(); !now.Before(c.imagesNextRefresh) {
		stats, err := c.listImages()
		if err != nil {
			return err
		}
		c.images = stats
		c.imagesNextRefresh = now.Add(imagesRefreshEvery)
	}

	mx["images_size"] = c.images.size
	mx["images_dangling"] = c.images.dangling
	mx["images_active"] = c.images.active

	return nil
}

func (c *Collector) listImages() (imagesStats, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.Timeout.Duration())
	defer cancel()

	result, err := c.client.ImageList(ctx, docker.ImageListOptions{})
	if err != nil {
		return imagesStats{}, err
	}

	var stats imagesStats
	for _, v := range result.Items {
		stats.size += v.Size
		if v.Containers == 0 {
			stats.dangling++
		} else {
			stats.active++
		}
	}

	return stats, nil
}

var (
	containerHealthStatuses = []typesContainer.HealthStatus{
		typesContainer.Healthy,
		typesContainer.Unhealthy,
		typesContainer.Starting,
		typesContainer.NoHealthcheck,
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
	containerSet := make(map[typesContainer.HealthStatus][]typesContainer.Summary)

	for _, status := range containerHealthStatuses {
		if err := func() error {
			ctx, cancel := context.WithTimeout(context.Background(), c.Timeout.Duration())
			defer cancel()

			result, err := c.client.ContainerList(ctx, docker.ContainerListOptions{
				All:     true,
				Filters: make(docker.Filters).Add("health", string(status)),
				Size:    c.CollectContainerSize,
			})
			if err != nil {
				return err
			}
			containerSet[status] = result.Items
			return nil

		}(); err != nil {
			return err
		}
	}

	seen := make(map[string]bool)

	for _, s := range containerHealthStatuses {
		mx["containers_health_status_"+string(s)] = 0
	}
	mx["containers_health_status_not_running_unhealthy"] = 0

	for status, containers := range containerSet {
		statusKey := string(status)
		if status != typesContainer.Unhealthy {
			mx["containers_health_status_"+statusKey] = int64(len(containers))
		}

		for _, cntr := range containers {
			state := string(cntr.State)
			if status == typesContainer.Unhealthy {
				if cntr.State == "running" {
					mx["containers_health_status_"+statusKey] += 1
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
				mx[px+"health_status_"+string(s)] = 0
			}
			mx[px+"health_status_not_running_unhealthy"] = 0
			for _, s := range containerStates {
				mx[px+"state_"+s] = 0
			}

			if status == typesContainer.Unhealthy && cntr.State != "running" {
				mx[px+"health_status_not_running_unhealthy"] += 1
			} else {
				mx[px+"health_status_"+statusKey] = 1
			}
			mx[px+"state_"+state] = 1
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

func hasIgnoreLabel(cntr typesContainer.Summary) bool {
	v := cntr.Labels["netdata.cloud/ignore"]
	return strings.EqualFold(v, "true") || strings.EqualFold(v, "yes")
}
