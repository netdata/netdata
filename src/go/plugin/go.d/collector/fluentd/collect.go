// SPDX-License-Identifier: GPL-3.0-or-later

package fluentd

import "fmt"

func (f *Fluentd) collect() (map[string]int64, error) {
	info, err := f.apiClient.getPluginsInfo()
	if err != nil {
		return nil, err
	}

	mx := make(map[string]int64)

	for _, p := range info.Payload {
		// TODO: if p.Category == "input" ?
		if !p.hasCategory() && !p.hasBufferQueueLength() && !p.hasBufferTotalQueuedSize() {
			continue
		}

		if f.permitPlugin != nil && !f.permitPlugin.MatchString(p.ID) {
			f.Debugf("plugin id: '%s', type: '%s', category: '%s' denied", p.ID, p.Type, p.Category)
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

		if !f.activePlugins[id] {
			f.activePlugins[id] = true
			f.addPluginToCharts(p)
		}

	}

	return mx, nil
}

func (f *Fluentd) addPluginToCharts(p pluginData) {
	id := fmt.Sprintf("%s_%s_%s", p.ID, p.Type, p.Category)

	if p.hasCategory() {
		chart := f.charts.Get("retry_count")
		_ = chart.AddDim(&Dim{ID: id + "_retry_count", Name: p.ID})
		chart.MarkNotCreated()
	}
	if p.hasBufferQueueLength() {
		chart := f.charts.Get("buffer_queue_length")
		_ = chart.AddDim(&Dim{ID: id + "_buffer_queue_length", Name: p.ID})
		chart.MarkNotCreated()
	}
	if p.hasBufferTotalQueuedSize() {
		chart := f.charts.Get("buffer_total_queued_size")
		_ = chart.AddDim(&Dim{ID: id + "_buffer_total_queued_size", Name: p.ID})
		chart.MarkNotCreated()
	}
}
