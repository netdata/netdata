// SPDX-License-Identifier: GPL-3.0-or-later

package docker

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"

	typesContainer "github.com/moby/moby/api/types/container"
	typesSystem "github.com/moby/moby/api/types/system"
	docker "github.com/moby/moby/client"
)

// Every Docker API call of a cycle runs before any metric is written, so a failed call aborts the cycle as a whole.
func (c *Collector) collect(ctx context.Context) error {
	info, err := c.info(ctx)
	if err != nil {
		return err
	}
	if err := c.refreshImages(ctx); err != nil {
		return err
	}
	containers, err := c.listContainers(ctx)
	if err != nil {
		return err
	}

	c.writeMetrics(info, containers)

	return nil
}

func (c *Collector) info(ctx context.Context) (typesSystem.Info, error) {
	if c.client == nil {
		client, err := c.newClient(c.Config)
		if err != nil {
			return typesSystem.Info{}, err
		}
		c.client = client
	}

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

// containersByHealth holds the containers listed with each health status filter.
type containersByHealth map[typesContainer.HealthStatus][]typesContainer.Summary

// listContainers lists the containers once per health status, so each container comes with its health status.
func (c *Collector) listContainers(ctx context.Context) (containersByHealth, error) {
	containers := make(containersByHealth)

	for _, status := range containerHealthStatuses {
		if err := func() error {
			ctx, cancel := context.WithTimeout(ctx, c.Timeout.Duration())
			defer cancel()

			result, err := c.client.ContainerList(ctx, docker.ContainerListOptions{
				All:     true,
				Filters: make(docker.Filters).Add("health", string(status)),
				Size:    c.CollectContainerSize,
			})
			if err != nil {
				return err
			}
			containers[status] = result.Items
			return nil
		}(); err != nil {
			return nil, err
		}
	}

	return containers, nil
}

const (
	healthStatusNotRunningUnhealthy = "not_running_unhealthy"
	healthStatusNoHealthcheck       = "no_healthcheck"
)

var (
	// containerStates are the states of the Docker container state machine.
	containerStates = []string{
		string(typesContainer.StateRunning),
		string(typesContainer.StatePaused),
		string(typesContainer.StateExited),
		string(typesContainer.StateCreated),
		string(typesContainer.StateRestarting),
		string(typesContainer.StateRemoving),
		string(typesContainer.StateDead),
	}
	containerHealthStates = []string{
		string(typesContainer.Healthy),
		string(typesContainer.Unhealthy),
		healthStatusNotRunningUnhealthy,
		string(typesContainer.Starting),
		healthStatusNoHealthcheck,
	}
)

// writeMetrics builds the instruments every cycle: containers come and go, so a long-lived Vec would keep a handle for
// every container ever seen.
func (c *Collector) writeMetrics(info typesSystem.Info, containers containersByHealth) {
	meter := c.store.Write().SnapshotMeter("")

	meter.Gauge("containers_running").Observe(float64(info.ContainersRunning))
	meter.Gauge("containers_paused").Observe(float64(info.ContainersPaused))
	meter.Gauge("containers_exited").Observe(float64(info.ContainersStopped))

	containerVec := meter.Vec("container_name", "image")
	containerState := containerVec.StateSet("container_state", metrix.WithStateSetStates(containerStates...))
	containerHealthStatus := containerVec.StateSet("container_health_status",
		metrix.WithStateSetStates(containerHealthStates...), metrix.WithStateSetMode(metrix.ModeEnum))
	var containerWritableLayerSize metrix.SnapshotGaugeVec
	if c.CollectContainerSize {
		containerWritableLayerSize = containerVec.Gauge("container_writable_layer_size")
	}

	healthCounts := make(map[string]int)
	usedImages := make(map[string]bool)

	for status, cntrs := range containers {
		for _, cntr := range cntrs {
			usedImages[cntr.ImageID] = true

			// "unhealthy" counts only running containers; the others count as "not_running_unhealthy".
			health := string(status)
			switch {
			case status == typesContainer.Unhealthy && cntr.State != typesContainer.StateRunning:
				health = healthStatusNotRunningUnhealthy
			case status == typesContainer.NoHealthcheck:
				health = healthStatusNoHealthcheck
			}
			healthCounts[health]++

			if hasIgnoreLabel(cntr) || len(cntr.Names) == 0 {
				continue
			}
			name := strings.TrimPrefix(cntr.Names[0], "/")
			if c.cntrSr != nil && !c.cntrSr.MatchString(name) {
				continue
			}

			// A state outside the known set leaves every state dimension at zero.
			state := containerState.WithLabelValues(name, cntr.Image)
			if slices.Contains(containerStates, string(cntr.State)) {
				state.Enable(string(cntr.State))
			} else {
				state.ObserveStateSet(metrix.StateSetPoint{})
			}
			containerHealthStatus.WithLabelValues(name, cntr.Image).Enable(health)
			if containerWritableLayerSize != nil {
				containerWritableLayerSize.WithLabelValues(name, cntr.Image).Observe(float64(cntr.SizeRw))
			}
		}
	}

	for _, status := range containerHealthStates {
		meter.Gauge("containers_" + status).Observe(float64(healthCounts[status]))
	}

	// An image is active when a container was created from it. The daemon's own per-image container count is not
	// used: it is -1 when the negotiated API is older than 1.51.
	var active, dangling int
	for _, id := range c.images.ids {
		if usedImages[id] {
			active++
		} else {
			dangling++
		}
	}
	meter.Gauge("images_active").Observe(float64(active))
	meter.Gauge("images_dangling").Observe(float64(dangling))
	meter.Gauge("images_size").Observe(float64(c.images.size))
}

func hasIgnoreLabel(cntr typesContainer.Summary) bool {
	v := cntr.Labels["netdata.cloud/ignore"]
	return strings.EqualFold(v, "true") || strings.EqualFold(v, "yes")
}
