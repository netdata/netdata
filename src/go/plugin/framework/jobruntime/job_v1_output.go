// SPDX-License-Identifier: GPL-3.0-or-later

package jobruntime

import (
	"maps"
	"slices"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/hostoutput"
)

// The collector owns desired chart objects. Only emitted definitions belong in
// cleanup inventory: collectors can request a refresh or remove objects before a write.
type jobV1ChartInventory map[string]netdataapi.ChartOpts

func (inventory jobV1ChartInventory) record(opts netdataapi.ChartOpts, obsolete bool) {
	// Match CHART's wire identity: pluginsd_chart in src/plugins.d/pluginsd_parser.c
	// splits this token at its first dot; pairs yielding the same token are one chart.
	key := opts.TypeID + "." + opts.ID
	if obsolete {
		delete(inventory, key)
	} else {
		inventory[key] = opts
	}
}

func (inventory jobV1ChartInventory) cleanup(api *netdataapi.API) {
	for _, key := range slices.Sorted(maps.Keys(inventory)) {
		opts := inventory[key]
		if opts.Options != "" {
			opts.Options += " "
		}
		opts.Options += "obsolete"
		api.CHART(opts)
		_ = api.EMPTYLINE()
	}
}

type jobV1ChartChange struct {
	chart                     *collectorapi.Chart
	created, updated, ignored bool
	retries, priority         int
	typeID, id                string
	pruneDims                 bool
	definition                *netdataapi.ChartOpts
	global, obsolete          bool
}

func (change *jobV1ChartChange) prepare(chart *collectorapi.Chart, j *Job) {
	*change = jobV1ChartChange{
		chart:    chart,
		created:  chart.IsCreated(),
		updated:  chart.IsUpdated(),
		ignored:  chart.IsIgnored(),
		retries:  chart.Retries,
		priority: chart.Priority,
		typeID:   getChartType(chart, j),
		id:       getChartID(chart),
	}
}

// A reusable attempt is safe because output settles synchronously on the job loop.
// The journal retains changed scalars and emitted definitions, never cloned chart graphs.
type jobV1Emission struct {
	job         *Job
	changes     []jobV1ChartChange
	owner       *hostoutput.Owner
	definition  *hostoutput.Definition
	guid        string
	priority    int
	started     time.Time
	hostBytes   int
	pruneCharts bool
	settled     bool
}

func (j *Job) prepareEmission(started time.Time) (*jobV1Emission, error) {
	tx := &j.emission
	tx.job = j
	tx.settled = false
	tx.started = started
	tx.priority = j.priority
	tx.guid = j.vnode.GUID
	tx.owner = j.hostOwner
	if tx.guid == "" {
		tx.owner = nil
	} else {
		if tx.owner == nil || j.hostGUID != tx.guid {
			tx.owner = j.publication.NewOwner(tx.guid)
		}
		labels := j.vnode.Labels
		if j.vnode.StaleAfter != nil {
			labels = j.vnode.HostLabels()
		}
		var err error
		tx.definition, err = tx.owner.Prepare(netdataapi.HostInfo{
			GUID:     tx.guid,
			Hostname: j.vnode.Hostname,
			Labels:   labels,
		})
		if err != nil {
			_ = tx.Abort()
			return nil, err
		}
	}
	return tx, nil
}

func (tx *jobV1Emission) record(change *jobV1ChartChange) {
	c := change.chart
	if change.definition != nil || change.pruneDims || change.created != c.IsCreated() || change.updated != c.IsUpdated() ||
		change.ignored != c.IsIgnored() ||
		change.retries != c.Retries ||
		change.priority != c.Priority ||
		change.typeID != c.CachedType() ||
		c.IDSep && change.id != c.CachedID() {
		tx.changes = append(tx.changes, *change)
	}
}

func (tx *jobV1Emission) Commit() error {
	if tx.settled {
		return nil
	}
	j := tx.job
	if tx.hostBytes > 0 {
		if tx.guid != j.hostGUID {
			clear(j.hostCharts)
		}
		if tx.owner != j.hostOwner {
			j.hostOwner.Release()
		}
		j.hostOwner = tx.owner
		j.hostGUID = tx.guid
		j.hostDefinition = tx.definition
	} else if tx.owner != j.hostOwner {
		tx.owner.Release()
	}
	for _, change := range tx.changes {
		c := change.chart
		c.SetCreated(change.created)
		c.SetUpdated(change.updated)
		c.SetIgnored(change.ignored)
		c.Retries = change.retries
		c.Priority = change.priority
		c.SetCachedType(change.typeID)
		if c.IDSep {
			c.SetCachedID(change.id)
		}
		if change.pruneDims {
			kept := c.Dims[:0]
			for _, dim := range c.Dims {
				if !dim.IsRemoved() {
					kept = append(kept, dim)
				}
			}
			clear(c.Dims[len(kept):])
			c.Dims = kept
		}
		if change.definition != nil {
			inventory := j.hostCharts
			if change.global {
				inventory = j.selfCharts
			}
			inventory.record(*change.definition, change.obsolete)
		}
	}
	if tx.pruneCharts {
		charts := *j.charts
		kept := charts[:0]
		for _, chart := range charts {
			if !chart.IsRemoved() {
				kept = append(kept, chart)
			}
		}
		clear(charts[len(kept):])
		*j.charts = kept
	}
	j.priority = tx.priority
	j.prevRun = tx.started
	tx.finish()
	return nil
}

func (tx *jobV1Emission) Abort() error {
	if tx.settled {
		return nil
	}
	if tx.owner != tx.job.hostOwner {
		tx.owner.Release()
	}
	tx.finish()
	return nil
}

func (tx *jobV1Emission) finish() {
	clear(tx.changes)
	tx.changes = tx.changes[:0]
	tx.owner = nil
	tx.definition = nil
	tx.guid = ""
	tx.hostBytes = 0
	tx.pruneCharts = false
	tx.settled = true
}

func (j *Job) clearOutputState() {
	j.hostOwner.Release()
	j.hostOwner = nil
	j.hostGUID = ""
	j.hostDefinition = nil
	clear(j.hostCharts)
	clear(j.selfCharts)
}
