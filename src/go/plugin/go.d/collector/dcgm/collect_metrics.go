// SPDX-License-Identifier: GPL-3.0-or-later

package dcgm

import (
	"math"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	promlabels "github.com/prometheus/prometheus/model/labels"
)

type metricKey struct {
	name, instance, dimension, source string
}

type transportKey struct {
	entity              metricEntity
	instance, transport string
}

type counterSample struct {
	value float64
	at    time.Time
}
type collectedValue struct {
	instance, parent entityInstance
	spec             metricSpec
	value            float64
	source           string
}

func (c *Collector) normalizeValue(value float64, spec metricSpec, key metricKey, now time.Time, next map[metricKey]counterSample) (float64, bool) {
	if spec.DecodeBER {
		if value < 0 || value > 65535 || math.Trunc(value) != value {
			return 0, false
		}
		packed := uint64(value)
		value = float64((packed>>8)&15) * math.Pow10(-int(packed&255))
	}
	if spec.Context.Units == "errors per trillion bits" && (value < 0 || value > 1) {
		return 0, false
	}
	if spec.CounterRate {
		if value < 0 {
			return 0, false
		}
		old, ok := c.counterSamples[key]
		next[key] = counterSample{
			value: value,
			at:    now,
		}
		if !ok || value < old.value || !now.After(old.at) {
			return 0, false
		}
		value = (value - old.value) / now.Sub(old.at).Seconds()
	}
	value *= spec.Scale
	return value, !math.IsNaN(value) && !math.IsInf(value, 0)
}

func metricInstances(lbls promlabels.Labels, def fieldDefinition) (entityInstance, entityInstance) {
	if def.Host {
		var host string
		for _, l := range lbls {
			if strings.EqualFold(l.Name, "hostname") {
				host = l.Value
				break
			}
		}
		key := "global"
		if host != "" {
			key = "hostname=" + url.QueryEscape(host)
		}
		return entityInstance{
			entity: entityHost,
			key:    key,
		}, entityInstance{}
	}
	if def.Link != "" {
		labels := make(promlabels.Labels, 0, len(lbls)+1)
		for _, l := range lbls {
			if strings.EqualFold(l.Name, "nvlink") {
				continue
			}
			if strings.EqualFold(l.Name, "uuid") {
				l.Name = "gpu_uuid"
			}
			labels = append(labels, l)
		}
		labels = append(labels, promlabels.Label{
			Name:  "nvlink",
			Value: def.Link,
		})
		lbls = labels
	}
	instance := resolveEntityInstance(lbls)
	var parent entityInstance
	if instance.entity == entityNVLink {
		labels := make(promlabels.Labels, 0, len(lbls))
		switchLink := false
		for _, l := range lbls {
			if strings.EqualFold(l.Name, "nvswitch") {
				switchLink = true
			}
			if strings.EqualFold(l.Name, "nvlink") || strings.EqualFold(l.Name, "nvswitch") {
				continue
			}
			if strings.EqualFold(l.Name, "gpu_uuid") {
				l.Name = "uuid"
			}
			labels = append(labels, l)
		}
		if !switchLink {
			parent = resolveEntityInstance(labels)
			if parent.entity != entityGPU && parent.entity != entityMIG {
				parent = entityInstance{}
			}
		}
	} else if instance.entity == entityGPU || instance.entity == entityMIG {
		parent = instance
	}
	if def.Window {
		for _, l := range lbls {
			if l.Name == "window_size_in_ms" {
				instance.key += "|window_size_in_ms=" + url.QueryEscape(l.Value)
				break
			}
		}
	}
	return instance, parent
}

func dimensionName(spec metricSpec, lbls promlabels.Labels) string {
	dim := spec.DimName
	var tokens []string
	for _, l := range lbls {
		key := strings.ToLower(l.Name)
		keep := false
		for _, semantic := range spec.Labels {
			if key == semantic {
				keep = true
				break
			}
		}
		if spec.Raw {
			keep = !isIdentityOrMetadataLabel(key)
		}
		if keep && l.Value != "" {
			if spec.Raw {
				key = escapeDimensionLabel(l.Name)
			}
			// Equals is outside the escaped alphabet, keeping the key/value boundary unambiguous.
			tokens = append(tokens, key+"="+escapeDimensionLabel(l.Value))
		}
	}
	sort.Strings(tokens)
	if len(tokens) > 0 {
		dim += "_" + strings.Join(tokens, "__")
	}
	return dim
}

// Escape punctuation without merging distinct source labels or changing case.
func escapeDimensionLabel(value string) string {
	const hex = "0123456789abcdef"
	var b strings.Builder
	for i := 0; i < len(value); i++ {
		c := value[i]
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' {
			b.WriteByte(c)
		} else {
			b.WriteByte('_')
			b.WriteByte(hex[c>>4])
			b.WriteByte(hex[c&15])
		}
	}
	return b.String()
}
func isIdentityOrMetadataLabel(key string) bool {
	switch key {
	case "gpu", "uuid", "gpu_uuid", "gpu_i_id", "gpu_instance_id", "gpu_i_profile", "gpu_instance_profile", "nvlink", "nvswitch", "cpu", "cpucore", "hostname", "__name__", "namespace", "pod", "container", "job", "hpc_job", "hpc_job_id", "modelname", "model_name", "device", "pci_bus_id":
		return true
	}
	return strings.HasPrefix(key, "dcgm_fi_")
}

func selectValue(selected map[metricKey]collectedValue, v collectedValue) {
	key := metricKey{
		name:      v.spec.Context.ID,
		instance:  v.instance.key,
		dimension: v.spec.DimName,
		source:    v.spec.RateSource,
	}
	old, ok := selected[key]
	// Aliases and duplicated exporter samples are alternatives, never additive.
	if !ok || v.spec.Priority > old.spec.Priority || v.spec.Priority == old.spec.Priority && (v.source < old.source || v.source == old.source && v.value > old.value) {
		selected[key] = v
	}
}

func (c *Collector) emitValue(mx map[string]int64, v collectedValue) {
	if len(v.spec.States) > 0 {
		valid := v.value >= 0 && math.Trunc(v.value) == v.value
		for i, state := range v.spec.States {
			active := valid && v.value == float64(i)
			if v.spec.Bitmask {
				active = valid && uint64(v.value)&(uint64(1)<<i) != 0
			}
			child := v
			child.spec.States = nil
			child.spec.Bitmask = false
			child.spec.DimName = state
			if v.spec.DimName != "" && v.spec.DimName != "reason" {
				child.spec.DimName = v.spec.DimName + "_" + state
			}
			child.value = 0
			if active {
				child.value = 1
			}
			c.emitValue(mx, child)
		}
		// Keep unrecognized enum values visible without guessing their meaning.
		unknown := !valid
		if v.spec.Bitmask {
			unknown = unknown || v.value >= math.Pow(2, float64(len(v.spec.States)))
		} else {
			unknown = unknown || v.value >= float64(len(v.spec.States))
		}
		child := v
		child.spec.States = nil
		child.spec.Bitmask = false
		child.spec.DimName = "unknown"
		if v.spec.DimName != "" && v.spec.DimName != "reason" {
			child.spec.DimName = v.spec.DimName + "_unknown"
		}
		child.value = 0
		if unknown {
			child.value = 1
		}
		c.emitValue(mx, child)
		return
	}
	scaled := v.value * v.spec.Precision
	if math.IsNaN(scaled) || math.IsInf(scaled, 0) || scaled >= float64(math.MaxInt64) || scaled < float64(math.MinInt64) {
		return
	}
	key, ch := c.ensureChart(v.instance, v.spec.Context)
	dimID := c.ensureDim(key, ch, v.spec)
	mx[dimID] = int64(math.Round(scaled))
	if v.spec.TotalGroup != "" {
		v.spec.Context = contextCatalog["dcgm."+string(v.instance.entity)+"."+v.spec.TotalGroup]
		v.spec.TotalGroup = ""
		v.spec.Kind = sampleGauge
		c.emitValue(mx, v)
	}
}

func (c *Collector) ensureChart(instance entityInstance, spec contextSpec) (chartKey, *collectorapi.Chart) {
	key := chartKey{
		context:  spec.ID,
		instance: instance.key,
	}
	if ch, ok := c.cache.getChart(key); ok {
		return key, ch.chart
	}
	chart := &collectorapi.Chart{
		ID:       makeID(spec.ID, instance.key),
		Title:    spec.Title,
		Units:    spec.Units,
		Fam:      spec.Family,
		Ctx:      spec.ID,
		Type:     spec.Type,
		Priority: spec.Priority,
		Labels:   append([]collectorapi.Label(nil), instance.chartLabels...),
	}
	if err := c.Charts().Add(chart); err != nil {
		c.Warning(err)
	}
	return key, c.cache.putChart(key, chart).chart
}
func (c *Collector) ensureDim(key chartKey, chart *collectorapi.Chart, spec metricSpec) string {
	dimID := makeID(chart.ID, spec.DimName)
	ch := c.cache.charts[key]
	if !ch.touchDim(dimID) {
		algo := collectorapi.Absolute
		if spec.Kind == sampleCounter {
			algo = collectorapi.Incremental
		}
		dim := &collectorapi.Dim{
			ID:   dimID,
			Name: spec.DimName,
			Div:  int(spec.Precision),
			Algo: algo,
		}
		if err := chart.AddDim(dim); err != nil {
			c.Warning(err)
		} else {
			sort.Slice(chart.Dims, func(i, j int) bool { return chart.Dims[i].Name < chart.Dims[j].Name })
			chart.MarkNotCreated()
		}
	}
	return dimID
}

type directionValues struct {
	rx, tx, total          float64
	hasRX, hasTX, hasTotal bool
	priority               int
}

func (d *directionValues) set(direction string, value float64) {
	switch direction {
	case "rx":
		d.rx = value
		d.hasRX = true
	case "tx":
		d.tx = value
		d.hasTX = true
	case "total":
		d.total = value
		d.hasTotal = true
	}
}
func (d directionValues) sum() (float64, bool) {
	if d.hasRX && d.hasTX {
		return d.rx + d.tx, true
	}
	return d.total, d.hasTotal
}

type transportValues struct {
	instance  entityInstance
	transport string
	aggregate map[string]directionValues
	links     map[string]map[string]directionValues
}

func (c *Collector) emitInterconnectTotals(mx map[string]int64, selected map[metricKey]collectedValue) {
	totals := make(map[transportKey]*transportValues)
	for _, v := range selected {
		if v.spec.Transport == "" || v.parent.entity == "" {
			continue
		}
		key := transportKey{
			entity:    v.parent.entity,
			instance:  v.parent.key,
			transport: v.spec.Transport,
		}
		total := totals[key]
		if total == nil {
			total = &transportValues{
				instance:  v.parent,
				transport: v.spec.Transport,
				aggregate: make(map[string]directionValues),
			}
			totals[key] = total
		}
		if v.instance.entity == entityNVLink {
			if total.links == nil {
				total.links = make(map[string]map[string]directionValues)
			}
			sources := total.links[v.instance.key]
			if sources == nil {
				sources = make(map[string]directionValues)
				total.links[v.instance.key] = sources
			}
			addDirection(sources, v)
		} else {
			addDirection(total.aggregate, v)
		}
	}
	for _, total := range totals {
		value, ok := completeThroughput(total.aggregate)
		if !ok && len(total.links) > 0 {
			value, ok = 0, true
			for _, link := range total.links {
				v, complete := completeThroughput(link)
				if !complete {
					ok = false
					break
				}
				value += v
			}
		}
		if !ok {
			continue
		}
		spec := metricSpec{
			Context: contextCatalog["dcgm."+string(total.instance.entity)+".interconnect.total.throughput"],
			fieldDefinition: fieldDefinition{
				DimName:   total.transport,
				Kind:      sampleGauge,
				Scale:     1,
				Precision: precision,
			},
		}
		c.emitValue(mx, collectedValue{
			instance: total.instance,
			spec:     spec,
			value:    value,
		})
	}
}

func addDirection(sources map[string]directionValues, v collectedValue) {
	d := sources[v.spec.RateSource]
	d.set(v.spec.Direction, v.value)
	if v.spec.Priority > d.priority {
		d.priority = v.spec.Priority
	}
	sources[v.spec.RateSource] = d
}
func completeThroughput(sources map[string]directionValues) (float64, bool) {
	value, priority := 0.0, -1
	for _, d := range sources {
		if sum, ok := d.sum(); ok && d.priority > priority {
			value, priority = sum, d.priority
		}
	}
	return value, priority >= 0
}
