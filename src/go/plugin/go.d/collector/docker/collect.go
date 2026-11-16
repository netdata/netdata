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
	// collectContainers adds and removes charts, so every call that can fail the collection runs before it.
	if err := c.refreshImages(); err != nil {
		return nil, err
	}
	usedImages, err := c.collectContainers(mx)
	if err != nil {
		return nil, err
	}
	c.collectImages(mx, usedImages)

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

type imagesSnapshot struct {
	ids  []string
	size int64
}

func (c *Collector) refreshImages() error {
	now := c.now()
	if now.Before(c.imagesNextRefresh) {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), c.Timeout.Duration())
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

// collectImages counts an image as active when a container in usedImages (keyed by image ID) was created from it.
// The daemon's own per-image container count is not used: it is -1 when the negotiated API is older than 1.51.
func (c *Collector) collectImages(mx map[string]int64, usedImages map[string]bool) {
	mx["images_size"] = c.images.size
	mx["images_dangling"] = 0
	mx["images_active"] = 0

	for _, id := range c.images.ids {
		if usedImages[id] {
			mx["images_active"]++
		} else {
			mx["images_dangling"]++
		}
	}
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

func (c *Collector) collectContainers(mx map[string]int64) (map[string]bool, error) {
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
			return nil, err
		}
	}

	seen := make(map[string]bool)
	usedImages := make(map[string]bool)

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
			usedImages[cntr.ImageID] = true

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

	return usedImages, nil
}

func hasIgnoreLabel(cntr typesContainer.Summary) bool {
	v := cntr.Labels["netdata.cloud/ignore"]
	return strings.EqualFold(v, "true") || strings.EqualFold(v, "yes")
}
