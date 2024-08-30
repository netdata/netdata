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

func (d *Docker) collect() (map[string]int64, error) {
	if d.client == nil {
		client, err := d.newClient(d.Config)
		if err != nil {
			return nil, err
		}
		d.client = client
	}

	if !d.verNegotiated {
		d.verNegotiated = true
		d.negotiateAPIVersion()
	}

	defer func() { _ = d.client.Close() }()

	mx := make(map[string]int64)

	if err := d.collectInfo(mx); err != nil {
		return nil, err
	}
	if err := d.collectImages(mx); err != nil {
		return nil, err
	}
	if err := d.collectContainers(mx); err != nil {
		return nil, err
	}

	return mx, nil
}

func (d *Docker) collectInfo(mx map[string]int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), d.Timeout.Duration())
	defer cancel()

	info, err := d.client.Info(ctx)
	if err != nil {
		return err
	}

	mx["containers_state_running"] = int64(info.ContainersRunning)
	mx["containers_state_paused"] = int64(info.ContainersPaused)
	mx["containers_state_exited"] = int64(info.ContainersStopped)

	return nil
}

func (d *Docker) collectImages(mx map[string]int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), d.Timeout.Duration())
	defer cancel()

	images, err := d.client.ImageList(ctx, typesImage.ListOptions{})
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

func (d *Docker) collectContainers(mx map[string]int64) error {
	containerSet := make(map[string][]types.Container)

	for _, status := range containerHealthStatuses {
		if err := func() error {
			ctx, cancel := context.WithTimeout(context.Background(), d.Timeout.Duration())
			defer cancel()

			v, err := d.client.ContainerList(ctx, typesContainer.ListOptions{
				All:     true,
				Filters: filters.NewArgs(filters.KeyValuePair{Key: "health", Value: status}),
				Size:    d.CollectContainerSize,
			})
			if err != nil {
				return err
			}
			containerSet[status] = v
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

			if len(cntr.Names) == 0 {
				continue
			}

			name := strings.TrimPrefix(cntr.Names[0], "/")

			seen[name] = true

			if !d.containers[name] {
				d.containers[name] = true
				d.addContainerCharts(name, cntr.Image)
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

	for name := range d.containers {
		if !seen[name] {
			delete(d.containers, name)
			d.removeContainerCharts(name)
		}
	}

	return nil
}

func (d *Docker) negotiateAPIVersion() {
	ctx, cancel := context.WithTimeout(context.Background(), d.Timeout.Duration())
	defer cancel()

	d.client.NegotiateAPIVersion(ctx)
}
