// SPDX-License-Identifier: GPL-3.0-or-later

package docker

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

const (
	prioContainersState = module.Priority + iota
	prioContainersHealthy

	prioContainerState
	prioContainerHealthStatus
	prioContainerWritableLayerSize

	prioImagesCount
	prioImagesSize
)

var summaryCharts = module.Charts{
	containersStateChart.Copy(),
	containersHealthyChart.Copy(),

	imagesCountChart.Copy(),
	imagesSizeChart.Copy(),
}

var (
	containersStateChart = module.Chart{
		ID:       "containers_state",
		Title:    "Total number of Docker containers in various states",
		Units:    "containers",
		Fam:      "containers",
		Ctx:      "docker.containers_state",
		Priority: prioContainersState,
		Type:     module.Stacked,
		Dims: module.Dims{
			{ID: "containers_state_running", Name: "running"},
			{ID: "containers_state_paused", Name: "paused"},
			{ID: "containers_state_exited", Name: "exited"},
		},
	}
	containersHealthyChart = module.Chart{
		ID:       "healthy_containers",
		Title:    "Total number of Docker containers in various health states",
		Units:    "containers",
		Fam:      "containers",
		Ctx:      "docker.containers_health_status",
		Priority: prioContainersHealthy,
		Dims: module.Dims{
			{ID: "containers_health_status_healthy", Name: "healthy"},
			{ID: "containers_health_status_unhealthy", Name: "unhealthy"},
			{ID: "containers_health_status_not_running_unhealthy", Name: "not_running_unhealthy"},
			{ID: "containers_health_status_starting", Name: "starting"},
			{ID: "containers_health_status_none", Name: "no_healthcheck"},
		},
	}
)

var (
	imagesCountChart = module.Chart{
		ID:       "images_count",
		Title:    "Total number of Docker images in various states",
		Units:    "images",
		Fam:      "images",
		Ctx:      "docker.images",
		Priority: prioImagesCount,
		Type:     module.Stacked,
		Dims: module.Dims{
			{ID: "images_active", Name: "active"},
			{ID: "images_dangling", Name: "dangling"},
		},
	}
	imagesSizeChart = module.Chart{
		ID:       "images_size",
		Title:    "Total size of all Docker images",
		Units:    "bytes",
		Fam:      "images",
		Ctx:      "docker.images_size",
		Priority: prioImagesSize,
		Dims: module.Dims{
			{ID: "images_size", Name: "size"},
		},
	}
)

var (
	containerChartsTmpl = module.Charts{
		containerStateChartTmpl.Copy(),
		containerHealthStatusChartTmpl.Copy(),
		containerWritableLayerSizeChartTmpl.Copy(),
	}

	containerStateChartTmpl = module.Chart{
		ID:       "container_%s_state",
		Title:    "Docker container state",
		Units:    "state",
		Fam:      "containers",
		Ctx:      "docker.container_state",
		Priority: prioContainerState,
		Dims: module.Dims{
			{ID: "container_%s_state_running", Name: "running"},
			{ID: "container_%s_state_paused", Name: "paused"},
			{ID: "container_%s_state_exited", Name: "exited"},
			{ID: "container_%s_state_created", Name: "created"},
			{ID: "container_%s_state_restarting", Name: "restarting"},
			{ID: "container_%s_state_removing", Name: "removing"},
			{ID: "container_%s_state_dead", Name: "dead"},
		},
	}
	containerHealthStatusChartTmpl = module.Chart{
		ID:       "container_%s_health_status",
		Title:    "Docker container health status",
		Units:    "status",
		Fam:      "containers",
		Ctx:      "docker.container_health_status",
		Priority: prioContainerHealthStatus,
		Dims: module.Dims{
			{ID: "container_%s_health_status_healthy", Name: "healthy"},
			{ID: "container_%s_health_status_unhealthy", Name: "unhealthy"},
			{ID: "container_%s_health_status_not_running_unhealthy", Name: "not_running_unhealthy"},
			{ID: "container_%s_health_status_starting", Name: "starting"},
			{ID: "container_%s_health_status_none", Name: "no_healthcheck"},
		},
	}
	containerWritableLayerSizeChartTmpl = module.Chart{
		ID:       "container_%s_writable_layer_size",
		Title:    "Docker container writable layer size",
		Units:    "bytes",
		Fam:      "containers",
		Ctx:      "docker.container_writeable_layer_size",
		Priority: prioContainerWritableLayerSize,
		Dims: module.Dims{
			{ID: "container_%s_size_rw", Name: "writable_layer"},
		},
	}
)

func (c *Collector) addContainerCharts(name, image string) {
	charts := containerChartsTmpl.Copy()

	if !c.CollectContainerSize {
		_ = charts.Remove(containerWritableLayerSizeChartTmpl.ID)
	}

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, name)
		chart.Labels = []module.Label{
			{Key: "container_name", Value: name},
			{Key: "image", Value: image},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, name)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removeContainerCharts(name string) {
	px := fmt.Sprintf("container_%s", name)

	for _, chart := range *c.Charts() {
		if strings.HasPrefix(chart.ID, px) {
			chart.MarkRemove()
			chart.MarkNotCreated()
		}
	}
}
