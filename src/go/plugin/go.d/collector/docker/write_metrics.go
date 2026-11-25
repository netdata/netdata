// SPDX-License-Identifier: GPL-3.0-or-later

package docker

import (
	"slices"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"

	typesContainer "github.com/moby/moby/api/types/container"
)

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
func (c *Collector) writeMetrics(state *daemonState) {
	meter := c.store.Write().SnapshotMeter("")

	meter.Gauge("containers_running").Observe(float64(state.info.ContainersRunning))
	meter.Gauge("containers_paused").Observe(float64(state.info.ContainersPaused))
	meter.Gauge("containers_exited").Observe(float64(state.info.ContainersStopped))

	healthCounts := make(map[string]int)
	usedImages := make(map[string]bool)
	// A container replaced between two of the list requests is listed twice under one name. Its charts are keyed by
	// name, so only the later listing, the current container, decides what they show.
	latest := make(map[string]listedContainer)

	for _, cntr := range state.containers {
		usedImages[cntr.ImageID] = true
		healthCounts[containerHealthState(cntr)]++

		if len(cntr.Names) > 0 {
			latest[strings.TrimPrefix(cntr.Names[0], "/")] = cntr
		}
	}

	containerVec := meter.Vec("container_name", "image")
	containerState := containerVec.StateSet("container_state", metrix.WithStateSetStates(containerStates...))
	containerHealthStatus := containerVec.StateSet("container_health_status",
		metrix.WithStateSetStates(containerHealthStates...), metrix.WithStateSetMode(metrix.ModeEnum))
	var containerWritableLayerSize metrix.SnapshotGaugeVec
	if c.CollectContainerSize {
		containerWritableLayerSize = containerVec.Gauge("container_writable_layer_size")
	}

	for name, cntr := range latest {
		if !c.isCharted(name, cntr.Summary) {
			continue
		}
		// A state outside the known set leaves every state dimension at zero.
		stateSet := containerState.WithLabelValues(name, cntr.Image)
		if slices.Contains(containerStates, string(cntr.State)) {
			stateSet.Enable(string(cntr.State))
		} else {
			stateSet.ObserveStateSet(metrix.StateSetPoint{})
		}
		containerHealthStatus.WithLabelValues(name, cntr.Image).Enable(containerHealthState(cntr))
		if containerWritableLayerSize != nil {
			containerWritableLayerSize.WithLabelValues(name, cntr.Image).Observe(float64(cntr.SizeRw))
		}
	}

	for _, health := range containerHealthStates {
		meter.Gauge("containers_" + health).Observe(float64(healthCounts[health]))
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

// containerHealthState maps the health status filter that listed the container to its health chart state: "unhealthy"
// counts only running containers; the others count as "not_running_unhealthy".
func containerHealthState(cntr listedContainer) string {
	switch {
	case cntr.healthStatus == typesContainer.Unhealthy && cntr.State != typesContainer.StateRunning:
		return healthStatusNotRunningUnhealthy
	case cntr.healthStatus == typesContainer.NoHealthcheck:
		return healthStatusNoHealthcheck
	default:
		return string(cntr.healthStatus)
	}
}

// isCharted reports whether a container gets its own charts: not ignored by label and matched by the container
// selector.
func (c *Collector) isCharted(name string, cntr typesContainer.Summary) bool {
	return !hasIgnoreLabel(cntr) && c.cntrSr.MatchString(name)
}

func hasIgnoreLabel(cntr typesContainer.Summary) bool {
	v := cntr.Labels["netdata.cloud/ignore"]
	return strings.EqualFold(v, "true") || strings.EqualFold(v, "yes")
}
