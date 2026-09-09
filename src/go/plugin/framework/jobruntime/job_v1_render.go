// SPDX-License-Identifier: GPL-3.0-or-later

package jobruntime

import (
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/oldmetrix"
)

func (tx *jobV1Emission) processMetrics(mx collectedMetrics, sinceLastRun int) bool {
	j := tx.job
	createCharts := j.hostGUID != tx.guid
	// pluginsd_validate_machine_guid in src/plugins.d/pluginsd_parser.c canonicalizes
	// HOST and HOST_DEFINE alike, so compact UUIDs select the same host here.
	j.api.HOST(tx.guid)
	hostHeader := j.buf.Len()
	elapsed := int64(durationTo(time.Since(tx.started), time.Millisecond))
	updated, created := 0, 0
	for _, chart := range *j.charts {
		var change jobV1ChartChange
		change.prepare(chart, j)
		if !change.created || createCharts {
			typeID := change.typeID + "." + change.id
			if len(typeID) >= NetdataChartIDMaxLength {
				j.Warningf(
					"chart 'type.id' length (%d) >= max allowed (%d), the chart is ignored (%s)",
					len(typeID),
					NetdataChartIDMaxLength,
					typeID,
				)
				change.ignored = true
			}
			tx.createChart(&change)
			if change.definition != nil {
				created++
			}
		}
		if chart.IsRemoved() {
			tx.pruneCharts = true
		} else if len(mx.intMetrics)+len(mx.floatMetrics) > 0 && !chart.Obsolete {
			if tx.updateChart(&change, mx, sinceLastRun) {
				updated++
			}
		}
		tx.record(&change)
	}
	if j.buf.Len() == hostHeader || updated == 0 && created == 0 && tx.guid != "" {
		j.buf.Reset()
	}
	tx.hostBytes = j.buf.Len()
	j.api.HOST("")
	var status jobV1ChartChange
	status.prepare(j.collectStatusChart, j)
	status.global = true
	var duration jobV1ChartChange
	duration.prepare(j.collectDurationChart, j)
	duration.global = true
	if !status.created || createCharts {
		tx.createChart(&status)
	}
	if !duration.created || createCharts {
		tx.createChart(&duration)
	}
	intMx := collectedMetrics{
		intMetrics: map[string]int64{"success": oldmetrix.Bool(updated > 0), "failed": oldmetrix.Bool(updated == 0)},
	}
	tx.updateChart(&status, intMx, sinceLastRun)
	tx.record(&status)
	if updated > 0 {
		tx.updateChart(&duration, collectedMetrics{
			intMetrics: map[string]int64{"duration": elapsed},
		}, sinceLastRun)
	}
	tx.record(&duration)
	return updated > 0
}

func (tx *jobV1Emission) createChart(change *jobV1ChartChange) {
	j := tx.job
	chart := change.chart
	change.created = true
	if change.ignored {
		return
	}

	if change.priority == 0 {
		change.priority = tx.priority
		tx.priority++
	}
	updateEvery := j.updateEvery
	if chart.UpdateEvery > 0 {
		updateEvery = chart.UpdateEvery
	}

	opts := netdataapi.ChartOpts{
		TypeID:      change.typeID,
		ID:          change.id,
		Name:        chart.OverID,
		Title:       chart.Title,
		Units:       chart.Units,
		Family:      chart.Fam,
		Context:     chart.Ctx,
		ChartType:   chart.Type.String(),
		Priority:    change.priority,
		UpdateEvery: updateEvery,
		Options:     chart.Opts.String(),
		Plugin:      j.pluginName,
		Module:      j.moduleName,
	}
	change.definition = &opts
	change.obsolete = chart.Obsolete
	j.api.CHART(opts)

	if chart.Obsolete {
		_ = j.api.EMPTYLINE()
		return
	}

	seen := make(map[string]bool)
	for _, l := range chart.Labels {
		if l.Key != "" {
			seen[l.Key] = true
			ls := l.Source
			// the default should be auto
			// https://github.com/netdata/netdata/blob/cc2586de697702f86a3c34e60e23652dd4ddcb42/database/rrd.h#L205
			if ls == 0 {
				ls = collectorapi.LabelSourceAuto
			}
			j.api.CLABEL(l.Key, lblValueReplacer.Replace(l.Value), ls)
		}
	}
	for k, v := range j.labels {
		if !seen[k] {
			j.api.CLABEL(k, lblValueReplacer.Replace(v), collectorapi.LabelSourceConf)
		}
	}
	j.api.CLABEL("_collect_job", lblValueReplacer.Replace(j.Name()), collectorapi.LabelSourceAuto)
	j.api.CLABELCOMMIT()

	for _, dim := range chart.Dims {
		j.api.DIMENSION(netdataapi.DimensionOpts{
			ID:         firstNotEmpty(dim.Name, dim.ID),
			Name:       dim.Name,
			Algorithm:  dim.Algo.String(),
			Multiplier: handleZero(dim.Mul),
			Divisor:    handleZero(dim.Div),
			Options:    dim.DimOpts.String(),
		})
	}
	for _, v := range chart.Vars {
		name := firstNotEmpty(v.Name, v.ID)
		j.api.VARIABLE(name, v.Value)
	}
	_ = j.api.EMPTYLINE()
}

func (tx *jobV1Emission) updateChart(change *jobV1ChartChange, mx collectedMetrics, sinceLastRun int) bool {
	j := tx.job
	chart := change.chart
	if change.ignored {
		for _, dim := range chart.Dims {
			if dim.IsRemoved() {
				change.pruneDims = true
				break
			}
		}
		return false
	}

	// Handle SkipGaps: check if any dimension has data
	if chart.SkipGaps {
		hasData := false
		for _, dim := range chart.Dims {
			if dim.IsRemoved() {
				continue
			}
			if _, hasData = mx.getValue(dim.ID); hasData {
				break
			}
		}
		if !hasData {
			// No dimensions have data - skip this chart entirely
			return false
		}
		// At least one dimension has data - proceed with deltaTime=0
		sinceLastRun = 0
	} else if !chart.IsUpdated() {
		sinceLastRun = 0
	}

	j.api.BEGIN(change.typeID, change.id, sinceLastRun)

	var updated int
	for _, dim := range chart.Dims {
		if dim.IsRemoved() {
			change.pruneDims = true
			continue
		}

		name := firstNotEmpty(dim.Name, dim.ID)
		v, ok := mx.getValue(dim.ID)
		if !ok {
			j.api.SETEMPTY(name)
			continue
		}
		updated++
		if dim.Float {
			j.api.SETFLOAT(name, v)
		} else {
			j.api.SET(name, int64(v))
		}
	}

	for _, vr := range chart.Vars {
		if v, ok := mx.getValue(vr.ID); ok {
			name := firstNotEmpty(vr.Name, vr.ID)
			j.api.VARIABLE(name, v)
		}
	}

	j.api.END()

	change.updated = updated > 0
	if change.updated {
		change.retries = 0
	} else {
		change.retries++
	}
	return change.updated
}
