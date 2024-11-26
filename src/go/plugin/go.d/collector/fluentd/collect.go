// SPDX-License-Identifier: GPL-3.0-or-later

package fluentd

import "fmt"

func (c *Collector) collect() (map[string]int64, error) {
	info, err := c.apiClient.getPluginsInfo()
	if err != nil {
		return nil, err
	}

	mx := make(map[string]int64)

	for _, p := range info.Payload {
		// TODO: if p.Category == "input" ?
		if !p.hasCategory() && !p.hasBufferQueueLength() && !p.hasBufferTotalQueuedSize() {
			continue
		}

		if c.permitPlugin != nil && !c.permitPlugin.MatchString(p.ID) {
			c.Debugf("plugin id: '%s', type: '%s', category: '%s' denied", p.ID, p.Type, p.Category)
			continue
		}

		id := fmt.Sprintf("%s_%s_%s", p.ID, p.Type, p.Category)

		if p.hasCategory() {
			mx[id+"_retry_count"] = *p.RetryCount
		}
		if p.hasBufferQueueLength() {
			mx[id+"_buffer_queue_length"] = *p.BufferQueueLength
		}
		if p.hasBufferTotalQueuedSize() {
			mx[id+"_buffer_total_queued_size"] = *p.BufferTotalQueuedSize
		}

		if !c.activePlugins[id] {
			c.activePlugins[id] = true
			c.addPluginToCharts(p)
		}

	}

	return mx, nil
}

func (c *Collector) addPluginToCharts(p pluginData) {
	id := fmt.Sprintf("%s_%s_%s", p.ID, p.Type, p.Category)

	if p.hasCategory() {
		chart := c.charts.Get("retry_count")
		_ = chart.AddDim(&Dim{ID: id + "_retry_count", Name: p.ID})
		chart.MarkNotCreated()
	}
	if p.hasBufferQueueLength() {
		chart := c.charts.Get("buffer_queue_length")
		_ = chart.AddDim(&Dim{ID: id + "_buffer_queue_length", Name: p.ID})
		chart.MarkNotCreated()
	}
	if p.hasBufferTotalQueuedSize() {
		chart := c.charts.Get("buffer_total_queued_size")
		_ = chart.AddDim(&Dim{ID: id + "_buffer_total_queued_size", Name: p.ID})
		chart.MarkNotCreated()
	}
}
