// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"math/big"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/internal/registry"
)

const promotedLabelLimit = 256

var redfishHostScopeNamespace = uuid.MustParse("1dc41e5f-4d26-5617-824f-f695cd185e51")
var managerDateTimeOffsetPattern = regexp.MustCompile(`[+-][0-9]{2}:[0-9]{2}$`)

type hardwareObservation struct {
	Metric  string
	Value   float64
	Counter bool
	State   string
	States  []string
	Labels  []metrix.Label
	Scope   metrix.HostScope
}

type hardwareMetrics struct {
	store metrix.CollectorStore
}

func newHardwareMetrics(store metrix.CollectorStore) *hardwareMetrics {
	return &hardwareMetrics{store: store}
}

func (m *hardwareMetrics) observe(observations []hardwareObservation) {
	for _, observation := range observations {
		meter := m.store.Write().SnapshotMeter("").WithHostScope(observation.Scope)
		if len(observation.Labels) > 0 {
			meter = meter.WithLabels(observation.Labels...)
		}
		if observation.State != "" {
			meter.StateSet(
				observation.Metric,
				metrix.WithStateSetMode(metrix.ModeEnum),
				metrix.WithStateSetStates(observation.States...),
			).Enable(observation.State)
			continue
		}
		if observation.Counter {
			meter.Counter(observation.Metric).ObserveTotal(observation.Value)
			continue
		}
		meter.Gauge(observation.Metric).Observe(observation.Value)
	}
}

var standardRegistry = registry.MustCompile()

const (
	maxProtocolNumericTokenBytes  = 128
	maxProtocolDurationTokenBytes = 640
)

var managerClockDescriptor = func() registry.FieldSpec {
	for _, descriptor := range standardRegistry.Fields {
		if descriptor.ID == "manager_datetime_clock_offset" {
			return descriptor
		}
	}
	panic("manager clock descriptor is missing")
}()

type rateBaseline struct {
	Value      *big.Rat
	At         time.Time
	Epoch      string
	Multiplier float64
}

type detailGate struct {
	Count      int
	Open       bool
	Complete   bool
	Generation uint64
	Members    map[string]struct{}
}

type aggregateMember struct {
	Key       string
	Node      *graphNode
	Value     float64
	Readable  bool
	Histogram string
	RangeMin  *float64
	RangeMax  *float64
	Slices    []string
}

type aggregateSnapshot struct {
	OwnerKey            string
	OwnerKind           string
	ChildKind           string
	Semantic            string
	Role                string
	Family              string
	Basis               string
	Units               string
	Source              string
	PhysicalContext     string
	SemanticSourceClass string
	AggregateClass      string
	Additive            bool
	Histogram           string
	States              []string
	OneHot              bool
	Members             map[string]struct{}
	SliceMembers        map[string]map[string]struct{}
}

func (c *protocolClient) hardwareSurface(
	graph *resourceGraph,
	observedAt time.Time,
) ([]hardwareObservation, []map[string]any, error) {
	nodes := c.filterSelectedSystem(graph, graph.emittedNodes())
	if graph.Complete {
		c.pruneRateBaselines(nodes)
	}
	metricNodes := make([]*graphNode, 0, len(nodes))
	for _, node := range nodes {
		if c.metricPlacementReady(node) {
			metricNodes = append(metricNodes, node)
		}
	}
	if err := c.validateHostScopeIdentities(metricNodes); err != nil {
		return nil, nil, err
	}
	readings := make(map[string][]normalizedReading)
	for _, node := range nodes {
		readings[node.Key] = c.readingsForNode(node, observedAt)
	}
	if err := c.validateAndRegisterReadingIdentities(readings); err != nil {
		return nil, nil, err
	}
	evidence := graph.detailEvidence(metricNodes, readings)
	gates, gateRetentionComplete := c.detailGatesWithinBudget(
		metricNodes,
		readings,
		evidence,
		graph.Complete,
		detailGateRetentionBudget,
	)
	if !gateRetentionComplete {
		graph.addDiagnostic("Redfish detail-gate continuity state exceeded its internal retention budget")
	}

	var observations []hardwareObservation
	var rows []map[string]any
	scalarsByNode := make(map[string][]scalarValue)
	for _, node := range nodes {
		gate := gateForNode(node, readings[node.Key], gates)
		row := c.inventoryGraphRow(node, gate, observedAt, readings[node.Key])
		values := c.scalarValues(node, observedAt)
		if value, present, diagnostic := managerClockValue(node); present {
			if diagnostic != "" {
				graph.addDiagnostic(fmt.Sprintf("manager %s clock offset: %s", node.URI, diagnostic))
			}
			if value.Valid {
				values = append(values, value)
			}
		}
		for _, value := range values {
			row[value.Descriptor.Column] = value.Inventory
			if value.Present && !value.Valid {
				markInventoryProtocolError(row)
			}
			if value.Exact != "" {
				row[value.Descriptor.Column+"_exact"] = value.Exact
			}
			if len(value.Descriptor.Candidates) > 1 && value.SelectedSource != "" {
				row[value.Descriptor.Column+"_source"] = value.SelectedSource
			}
			if value.MultiplierColumn != "" {
				row[value.MultiplierColumn] = value.MultiplierValue
			}
			for _, failure := range value.SourceFailures {
				graph.addDiagnostic(failure)
			}
		}
		flags := applyRegisteredFlags(row, node)
		rows = append(rows, row)
		scalarsByNode[node.Key] = values

		if c.metricPlacementReady(node) && detailAllowed(node, gate) {
			observations = append(observations, c.statusObservations(node)...)
			for _, value := range values {
				if !value.Emit || value.Descriptor.Exposure != registry.ExposureOperationalScalar {
					continue
				}
				observations = append(observations, hardwareObservation{
					Metric: value.Descriptor.Metric,
					Value:  value.Value,
					Labels: c.metricLabels(node, nil),
					Scope:  c.scopeForNode(node),
				})
			}
			observations = append(observations, c.flagObservations(node, flags)...)
		}
		for _, reading := range readings[node.Key] {
			graph.addDiagnostic(reading.SourceAlarmDiagnostic)
			readingRow := c.inventoryReadingRow(node, gate, observedAt, reading)
			rows = append(rows, readingRow)
			if c.metricPlacementReady(node) &&
				detailAllowed(node, gate) &&
				(reading.Valid || reading.EffectiveAlarm != "") &&
				reading.Exposure == registry.ExposureOperationalReading {
				observations = append(observations, c.readingObservations(node, reading)...)
			}
		}
	}
	if c.takeRateRetentionOverflow() {
		graph.addDiagnostic("Redfish rate-baseline continuity state exceeded its internal retention budget")
	}
	observations = append(observations, c.detailGateObservations(gates, graph)...)
	if c.config.Charts.aggregatesEnabled() {
		aggregates, err := c.aggregateObservations(
			graph,
			metricNodes,
			readings,
			scalarsByNode,
		)
		if err != nil {
			return observations, rows, err
		}
		observations = append(observations, aggregates...)
	}
	return observations, rows, nil
}

func (c *protocolClient) metricPlacementReady(node *graphNode) bool {
	return c.config.NodeMode != "system_vnodes" ||
		!isSubordinate(node.Kind) ||
		node.PlacementComplete
}

func validateReadingIdentities(readings map[string][]normalizedReading) error {
	seen := make(map[string]string)
	for nodeKey, values := range readings {
		for _, reading := range values {
			preimage := nodeKey + "\x00" + reading.IdentitySource
			if previous, exists := seen[reading.Key]; exists && previous != preimage {
				return fmt.Errorf("Redfish reading-key collision for %s", reading.Key)
			}
			seen[reading.Key] = preimage
		}
	}
	return nil
}

func (c *protocolClient) validateAndRegisterReadingIdentities(
	readings map[string][]normalizedReading,
) error {
	if err := validateReadingIdentities(readings); err != nil {
		return fmt.Errorf("%w: %v", errIdentityIntegrity, err)
	}
	bindings := make([]identityBinding, 0)
	for nodeKey, values := range readings {
		for _, reading := range values {
			bindings = append(bindings, identityBinding{
				Domain:   "reading",
				Key:      reading.Key,
				Preimage: nodeKey + "\x00" + reading.IdentitySource,
			})
		}
	}
	if err := c.identities.register(bindings); err != nil {
		return fmt.Errorf("%w: Redfish reading-key collision", err)
	}
	return nil
}

func (c *protocolClient) pruneRateBaselines(nodes []*graphNode) {
	active := make(map[string]struct{}, len(nodes))
	for _, node := range nodes {
		active[node.Key] = struct{}{}
	}
	c.rateMu.Lock()
	for key := range c.rateBaselines {
		nodeKey, _, ok := strings.Cut(key, "\x00")
		if !ok {
			continue
		}
		if _, exists := active[nodeKey]; !exists {
			delete(c.rateBaselines, key)
		}
	}
	c.rateMu.Unlock()
}

type flagValue struct {
	Set      registry.FlagSetSpec
	Member   registry.FlagMemberSpec
	Value    bool
	Observed bool
	Present  bool
	Emit     bool
}

func flagValues(node *graphNode) []flagValue {
	var result []flagValue
	for _, set := range standardRegistry.Flags {
		if string(set.Kind) != node.Kind {
			continue
		}
		document := node.Data
		if set.Document != "" {
			document = findEnrichment(node, string(set.Document))
		}
		if document == nil {
			continue
		}
		values := make([]flagValue, 0, len(set.Members))
		for _, member := range set.Members {
			raw, present := jsonPath(document, member.Path)
			value, valid := raw.(bool)
			if present && valid && member.Invert {
				value = !value
			}
			values = append(values, flagValue{
				Set: set, Member: member, Value: value,
				Observed: present, Present: present && valid, Emit: present && valid,
			})
		}
		result = append(result, values...)
	}
	return result
}

func applyRegisteredFlags(row map[string]any, node *graphNode) []flagValue {
	values := flagValues(node)
	for _, value := range values {
		if !value.Observed {
			continue
		}
		if value.Present {
			row[value.Member.Column] = value.Value
			continue
		}
		row[value.Member.Column] = nil
		markInventoryProtocolError(row)
	}
	return values
}

func (c *protocolClient) flagObservations(node *graphNode, values []flagValue) []hardwareObservation {
	labels := c.metricLabels(node, nil)
	scope := c.scopeForNode(node)
	result := make([]hardwareObservation, 0, len(values))
	for _, value := range values {
		if !value.Emit || !value.Present {
			continue
		}
		result = append(result, hardwareObservation{
			Metric: value.Set.Metric + "_" + value.Member.Role,
			Value:  boolFloat(value.Value),
			Labels: labels,
			Scope:  scope,
		})
	}
	return result
}

type scalarValue struct {
	Descriptor       registry.FieldSpec
	Value            float64
	Inventory        any
	Exact            string
	SelectedSource   string
	SourceFailures   []string
	MultiplierColumn string
	MultiplierValue  float64
	Present          bool
	Valid            bool
	Emit             bool
}

func (c *protocolClient) scalarValues(node *graphNode, at time.Time) []scalarValue {
	result := make([]scalarValue, 0)
	for _, descriptor := range standardRegistry.Fields {
		if descriptor.Kind != registry.Kind(node.Kind) {
			continue
		}
		if descriptor.ID == managerClockDescriptor.ID {
			// DateTime is observed against the response midpoint by
			// managerClockValue; it is deliberately not a generic numeric field.
			continue
		}
		var selected scalarValue
		var sourceFailures []string
		for _, source := range descriptor.Candidates {
			sourceName := sourcePath(source)
			document := node.Data
			if source.Document != "" {
				document = findEnrichment(node, string(source.Document))
			}
			if document == nil {
				sourceFailures = append(sourceFailures, scalarSourceFailure(descriptor, sourceName, "document unavailable"))
				continue
			}
			if !sourceRequirementsMatch(document, source.Requires) {
				continue
			}
			raw, ok := registeredValueAt(document, source.Path)
			if !ok {
				sourceFailures = append(sourceFailures, scalarSourceFailure(descriptor, sourceName, "property absent"))
				continue
			}
			candidate := scalarValue{
				Descriptor:     descriptor,
				SelectedSource: sourceName,
				Present:        true,
			}
			if raw == nil {
				sourceFailures = append(sourceFailures, scalarSourceFailure(descriptor, sourceName, "property null"))
				candidate.SourceFailures = slices.Clone(sourceFailures)
				if !selected.Present {
					selected = candidate
				}
				continue
			}
			exact, value, ok := numericSourceValue(raw, descriptor.Algorithm)
			if !ok {
				sourceFailures = append(sourceFailures, scalarSourceFailure(descriptor, sourceName, "value malformed, unsupported, or non-finite"))
				candidate.SourceFailures = slices.Clone(sourceFailures)
				if !selected.Present {
					selected = candidate
				}
				continue
			}
			scale := descriptor.Scale
			if source.Scale.Den != 0 {
				scale = source.Scale
			}
			multiplier := float64(scale.Num) / float64(scale.Den)
			if source.MultiplierPath != "" {
				multiplierDocument := document
				if source.MultiplierDocument != "" {
					multiplierDocument = findEnrichment(node, string(source.MultiplierDocument))
				}
				rawMultiplier, present := registeredValueAt(multiplierDocument, source.MultiplierPath)
				_, sourceMultiplier, valid := numericValue(rawMultiplier)
				if !present || !valid || sourceMultiplier <= 0 {
					sourceFailures = append(sourceFailures, scalarSourceFailure(descriptor, sourceName, "normalization multiplier absent or invalid"))
					candidate.SourceFailures = slices.Clone(sourceFailures)
					if !selected.Present {
						selected = candidate
					}
					continue
				}
				scale := source.MultiplierScale
				if scale.Den == 0 {
					scale = registry.Identity
				}
				multiplier *= sourceMultiplier * float64(scale.Num) / float64(scale.Den)
				candidate.MultiplierColumn = source.MultiplierColumn
				candidate.MultiplierValue = sourceMultiplier * float64(scale.Num) / float64(scale.Den)
			}
			normalized := value * multiplier
			candidate.Value = normalized
			candidate.Inventory = normalized
			candidate.Exact = exact
			candidate.Valid = isFinite(normalized)
			if !candidate.Valid {
				sourceFailures = append(sourceFailures, scalarSourceFailure(descriptor, sourceName, "normalized value non-finite"))
			}
			candidate.SourceFailures = slices.Clone(sourceFailures)
			candidate.Emit = candidate.Valid && descriptor.Algorithm == registry.AlgorithmAbsolute
			if descriptor.Algorithm != registry.AlgorithmAbsolute && candidate.Valid {
				rate, emit := c.rateValue(
					node.Key+"\x00"+descriptor.ID,
					exact,
					multiplier,
					at,
					descriptor.Algorithm,
					sourcePath(source)+"\x00"+rateEpoch(document),
				)
				candidate.Value = rate
				candidate.Emit = emit
			}
			selected = candidate
			break
		}
		if selected.Present {
			result = append(result, selected)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Descriptor.Metric < result[j].Descriptor.Metric })
	return result
}

func scalarSourceFailure(descriptor registry.FieldSpec, source, reason string) string {
	return fmt.Sprintf(
		"Redfish compatibility: scalar %s preferred source %s: %s",
		descriptor.ID,
		source,
		reason,
	)
}

func sourceRequirementsMatch(document map[string]any, requirements []registry.SourceRequirement) bool {
	for _, requirement := range requirements {
		value, ok := stringValueAt(document, requirement.Path)
		if !ok || value != requirement.Value {
			return false
		}
	}
	return true
}

func managerClockValue(node *graphNode) (scalarValue, bool, string) {
	if node == nil || node.Kind != "manager" || node.Data == nil {
		return scalarValue{}, false, ""
	}
	raw, present := node.Data["DateTime"]
	if !present || raw == nil {
		return scalarValue{}, false, ""
	}
	text, ok := raw.(string)
	if !ok {
		return scalarValue{}, true, "DateTime is not a string"
	}
	if !strings.HasSuffix(text, "Z") && !managerDateTimeOffsetPattern.MatchString(text) {
		return scalarValue{}, true, "DateTime has no explicit UTC offset"
	}
	managerTime, err := time.Parse(time.RFC3339Nano, text)
	if err != nil {
		return scalarValue{}, true, "DateTime is not valid RFC 3339"
	}
	started, finished := node.Response.StartedAt, node.Response.FinishedAt
	if started.IsZero() || finished.IsZero() || finished.Before(started) {
		return scalarValue{}, true, "request observation interval is unavailable"
	}
	monotonicElapsed := finished.Sub(started)
	wallElapsed := finished.Round(0).Sub(started.Round(0))
	if delta := wallElapsed - monotonicElapsed; delta > time.Millisecond || delta < -time.Millisecond {
		return scalarValue{}, true, "wall clock changed during the request"
	}
	midpoint := started.Round(0).Add(monotonicElapsed / 2)
	offsetDuration := managerTime.Sub(midpoint)
	if offsetDuration == time.Duration(1<<63-1) || offsetDuration == time.Duration(-1<<63) {
		return scalarValue{}, true, "clock offset is outside the supported range"
	}
	offset := offsetDuration.Seconds()
	if !isFinite(offset) {
		return scalarValue{}, true, "clock offset is not finite"
	}
	return scalarValue{
		Descriptor: managerClockDescriptor,
		Value:      offset,
		Inventory:  offset,
		Present:    true,
		Valid:      true,
		Emit:       true,
	}, true, ""
}

func sourcePath(source registry.SourceCandidate) string {
	if source.Document == "" {
		return source.Path
	}
	return string(source.Document) + "." + source.Path
}

func findEnrichment(node *graphNode, kind string) map[string]any {
	var match map[string]any
	found := false
	for key, value := range node.Enrichment {
		if strings.HasPrefix(key, kind+":") || key == kind {
			if found {
				return nil
			}
			match = value
			found = true
		}
	}
	return match
}

func (c *protocolClient) rateValue(
	key, exact string,
	multiplier float64,
	at time.Time,
	algorithm registry.Algorithm,
	epoch string,
) (float64, bool) {
	if !boundedProtocolNumber(exact) {
		return 0, false
	}
	current, ok := new(big.Rat).SetString(exact)
	if !ok {
		return 0, false
	}
	c.rateMu.Lock()
	previous, exists := c.rateBaselines[key]
	limit := c.rateBaselineLimit
	if limit <= 0 {
		limit = rateBaselineRetentionLimit()
		c.rateBaselineLimit = limit
	}
	if !exists && len(c.rateBaselines) >= limit {
		c.rateRetentionOverflow = true
		c.rateMu.Unlock()
		return 0, false
	}
	c.rateBaselines[key] = rateBaseline{
		Value: new(big.Rat).Set(current), At: at, Epoch: epoch, Multiplier: multiplier,
	}
	c.rateMu.Unlock()
	if !exists || previous.Epoch != epoch || previous.Multiplier != multiplier ||
		!at.After(previous.At) || current.Cmp(previous.Value) < 0 {
		return 0, false
	}
	elapsed := at.Sub(previous.At).Seconds()
	delta := new(big.Rat).Sub(current, previous.Value)
	value, _ := delta.Float64()
	value *= multiplier / elapsed
	if algorithm == registry.AlgorithmDurationPercent {
		value *= 100
		if value < 0 || value > 100 {
			return 0, false
		}
	}
	return value, isFinite(value)
}

func (c *protocolClient) takeRateRetentionOverflow() bool {
	c.rateMu.Lock()
	overflow := c.rateRetentionOverflow
	c.rateRetentionOverflow = false
	c.rateMu.Unlock()
	return overflow
}

func rateBaselineRetentionLimit() int {
	perKind := make(map[registry.Kind]int)
	maximum := 0
	for _, field := range standardRegistry.Fields {
		if field.Algorithm == registry.AlgorithmAbsolute {
			continue
		}
		perKind[field.Kind]++
		maximum = max(maximum, perKind[field.Kind])
	}
	if maximum == 0 {
		maximum = 1
	}
	if maximum > math.MaxInt/maxGraphResources {
		return math.MaxInt
	}
	return maxGraphResources * maximum
}

func rateEpoch(document map[string]any) string {
	for _, path := range []string{
		"LifetimeStartDateTime",
		"CurrentPeriod.StartTime",
		"IntervalStartTime",
		"StartTime",
	} {
		if value, ok := stringValueAt(document, path); ok {
			return stableTupleDigest("netdata:redfish:rate-epoch:v1", path, value)
		}
	}
	return ""
}

func numericValue(value any) (string, float64, bool) {
	var exact string
	switch value := value.(type) {
	case interface{ String() string }:
		exact = value.String()
	case float64:
		exact = strconv.FormatFloat(value, 'g', -1, 64)
	case float32:
		exact = strconv.FormatFloat(float64(value), 'g', -1, 32)
	case int:
		exact = strconv.Itoa(value)
	case int64:
		exact = strconv.FormatInt(value, 10)
	case uint64:
		exact = strconv.FormatUint(value, 10)
	default:
		return "", 0, false
	}
	if !boundedProtocolNumber(exact) {
		return "", 0, false
	}
	result, err := strconv.ParseFloat(exact, 64)
	return exact, result, err == nil && isFinite(result)
}

func boundedProtocolNumber(value string) bool {
	return len(value) > 0 && len(value) <= maxProtocolNumericTokenBytes
}

var redfishDurationPattern = regexp.MustCompile(
	`^P(?:(\d+(?:\.\d+)?)D)?(?:T(?:(\d+(?:\.\d+)?)H)?(?:(\d+(?:\.\d+)?)M)?(?:(\d+(?:\.\d+)?)S)?)?$`,
)

func numericSourceValue(value any, algorithm registry.Algorithm) (string, float64, bool) {
	if algorithm != registry.AlgorithmDurationPercent {
		return numericValue(value)
	}
	text, ok := value.(string)
	if !ok {
		return numericValue(value)
	}
	if len(text) > maxProtocolDurationTokenBytes {
		return "", 0, false
	}
	match := redfishDurationPattern.FindStringSubmatch(strings.TrimSpace(text))
	if match == nil || match[1]+match[2]+match[3]+match[4] == "" {
		return "", 0, false
	}
	total := new(big.Rat)
	for index, multiplier := range []int64{86400, 3600, 60, 1} {
		if match[index+1] == "" {
			continue
		}
		if !boundedProtocolNumber(match[index+1]) {
			return "", 0, false
		}
		value, ok := new(big.Rat).SetString(match[index+1])
		if !ok {
			return "", 0, false
		}
		total.Add(total, value.Mul(value, big.NewRat(multiplier, 1)))
	}
	seconds, _ := total.Float64()
	return total.RatString(), seconds, isFinite(seconds)
}

func isFinite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func (c *protocolClient) filterSelectedSystem(graph *resourceGraph, nodes []*graphNode) []*graphNode {
	if c.config.SystemURI == "" {
		return nodes
	}
	selected, _ := normalizeConfiguredResourceURI(c.root, c.config.SystemURI)
	current := make(map[string]struct{})
	for _, node := range nodes {
		if node.Kind == "system" && node.URI != selected {
			continue
		}
		if len(node.SystemOwners) > 0 {
			owned := false
			for _, system := range node.SystemOwners {
				if system.URI == selected {
					owned = true
					break
				}
			}
			if !owned {
				continue
			}
		} else if graph != nil && !graph.Complete &&
			node.Kind != "service" &&
			(node.Kind != "system" || node.URI != selected) {
			// A partial refresh cannot prove that a newly unowned resource is
			// shared. Admit it only after complete ownership evidence; prior
			// selected membership is restored below.
			continue
		}
		current[node.Key] = struct{}{}
	}

	c.selectedSystemMu.Lock()
	if graph != nil && graph.Complete {
		c.selectedSystemIncluded = cloneStringSet(current)
	} else {
		for key := range c.selectedSystemIncluded {
			current[key] = struct{}{}
		}
	}
	c.selectedSystemMu.Unlock()

	result := make([]*graphNode, 0, len(current))
	for _, node := range nodes {
		if _, ok := current[node.Key]; ok {
			result = append(result, node)
		}
	}
	return result
}

func (c *protocolClient) detailGates(
	nodes []*graphNode,
	readings map[string][]normalizedReading,
	evidence map[string]bool,
	graphComplete bool,
) map[string]detailGate {
	result, _ := c.detailGatesWithinBudget(
		nodes,
		readings,
		evidence,
		graphComplete,
		detailGateRetentionBudget,
	)
	return result
}

func (c *protocolClient) detailGatesWithinBudget(
	nodes []*graphNode,
	readings map[string][]normalizedReading,
	evidence map[string]bool,
	graphComplete bool,
	budget retainedStateBudget,
) (map[string]detailGate, bool) {
	current := make(map[string]map[string]struct{})
	for _, node := range nodes {
		if !isSubordinate(node.Kind) {
			continue
		}
		family := componentFamily(node, readings[node.Key])
		owner := node.LogicalOwner
		if owner == nil {
			continue
		}
		key := owner.Key + "\x00" + family
		if current[key] == nil {
			current[key] = make(map[string]struct{})
		}
		current[key][node.Key] = struct{}{}
	}

	c.gateMu.Lock()
	defer c.gateMu.Unlock()
	if c.detailGateState == nil {
		c.detailGateState = make(map[string]detailGate)
	}
	if graphComplete {
		for key := range c.detailGateState {
			if _, exists := current[key]; !exists {
				delete(c.detailGateState, key)
			}
		}
	}
	retainedMembers := 0
	for _, gate := range c.detailGateState {
		retainedMembers += len(gate.Members)
	}
	result := make(map[string]detailGate, len(current))
	keys := make([]string, 0, len(current))
	for key := range current {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	retentionComplete := true
	for _, key := range keys {
		members := current[key]
		previous := c.detailGateState[key]
		if evidence[key] {
			count := len(members)
			cap := dereferenceInt(c.config.Charts.MaxDetailedComponentsPerFamily)
			candidate := detailGate{
				Count:      count,
				Open:       c.config.Charts.detailsEnabled() && (cap == 0 || count <= cap),
				Complete:   true,
				Generation: previous.Generation + 1,
				Members:    cloneStringSet(members),
			}
			_, exists := c.detailGateState[key]
			baseEntries := len(c.detailGateState)
			baseMembers := retainedMembers - len(previous.Members)
			if exists {
				baseEntries--
			}
			if retainedStateFits(baseEntries, baseMembers, 1, len(candidate.Members), budget) {
				c.detailGateState[key] = candidate
				retainedMembers = baseMembers + len(candidate.Members)
			} else {
				if exists {
					delete(c.detailGateState, key)
					retainedMembers = baseMembers
				}
				retentionComplete = false
			}
			previous = candidate
		} else if previous.Members == nil {
			count := len(members)
			previous = detailGate{
				Count:    count,
				Open:     false,
				Complete: false,
				Members:  cloneStringSet(members),
			}
		} else {
			previous.Complete = false
		}
		result[key] = previous
	}
	return result, retentionComplete
}

func cloneStringSet(src map[string]struct{}) map[string]struct{} {
	result := make(map[string]struct{}, len(src))
	for key := range src {
		result[key] = struct{}{}
	}
	return result
}

func componentFamily(node *graphNode, readings []normalizedReading) string {
	if node.Kind == "sensor" {
		if len(readings) > 0 && readings[0].Family != "" {
			return "sensor." + readings[0].Family
		}
		return "sensor.unknown"
	}
	return node.Kind
}

func gateForNode(
	node *graphNode,
	readings []normalizedReading,
	gates map[string]detailGate,
) detailGate {
	if !isSubordinate(node.Kind) {
		return detailGate{Count: 1, Open: true, Complete: node.Complete, Members: map[string]struct{}{node.Key: {}}}
	}
	if node.LogicalOwner == nil {
		return detailGate{Count: 1, Open: false, Complete: false}
	}
	return gates[node.LogicalOwner.Key+"\x00"+componentFamily(node, readings)]
}

func isSubordinate(kind string) bool {
	switch kind {
	case "service", "system", "chassis", "manager":
		return false
	default:
		return true
	}
}

func detailAllowed(node *graphNode, gate detailGate) bool {
	if !isSubordinate(node.Kind) {
		return true
	}
	if !gate.Open {
		return false
	}
	_, ok := gate.Members[node.Key]
	return ok
}

func (c *protocolClient) statusObservations(node *graphNode) []hardwareObservation {
	labels := c.metricLabels(node, nil)
	scope := c.scopeForNode(node)
	prefix := strings.ReplaceAll(node.Kind, "-", "_")
	var result []hardwareObservation
	status := statusForKind(node.Kind)
	if node.AcquisitionState != "" {
		result = append(result, hardwareObservation{
			Metric: prefix + "_acquisition_state",
			State:  normalizedEnum(node.AcquisitionState, acquisitionStates),
			States: acquisitionStates,
			Labels: labels,
			Scope:  scope,
		})
	}
	if status.Status {
		if state, present, _ := categoricalStringState(node.Data, "Status.Health", normalizeHealth); present {
			result = append(result, stateObservation(prefix+"_health", state, healthStates, labels, scope))
		}
		if state, present, _ := categoricalStringState(node.Data, "Status.HealthRollup", normalizeHealth); present {
			result = append(result, stateObservation(prefix+"_health_rollup", state, healthStates, labels, scope))
		}
		if state, present, _ := categoricalStringState(node.Data, "Status.State", normalizeResourceState); present {
			result = append(result, stateObservation(prefix+"_state", state, resourceStates, labels, scope))
		}
	}
	if status.PowerState {
		if state, present, _ := categoricalStringState(node.Data, "PowerState", func(value string) string {
			return normalizedEnum(value, powerStates)
		}); present {
			result = append(result, stateObservation(prefix+"_power_state", state, powerStates, labels, scope))
		}
	}
	if status.FailurePredicted {
		if raw, exists := jsonPath(node.Data, "FailurePredicted"); exists && raw != nil {
			state := "unknown"
			if value, ok := raw.(bool); ok {
				if value {
					state = "predicted"
				} else {
					state = "clear"
				}
			}
			result = append(result, stateObservation(prefix+"_failure_predicted", state, failureStates, labels, scope))
		}
	}
	if status.Status {
		if counts, present, readable := conditionCountsForNode(node); present && readable {
			for role, value := range map[string]int{
				"ok": counts.OK, "warning": counts.Warning, "critical": counts.Critical, "unknown": counts.Unknown,
			} {
				result = append(result, hardwareObservation{
					Metric: prefix + "_conditions_" + role,
					Value:  float64(value),
					Labels: labels,
					Scope:  scope,
				})
			}
		}
	}
	result = append(result, c.additionalStateObservations(node, labels, scope)...)
	return result
}

func statusForKind(kind string) registry.StatusSpec {
	for _, status := range standardRegistry.Status {
		if status.Kind == registry.Kind(kind) {
			return status
		}
	}
	return registry.StatusSpec{}
}

var (
	healthStates      = registry.HealthStates
	resourceStates    = registry.ResourceStates
	powerStates       = registry.PowerStates
	failureStates     = registry.FailureStates
	acquisitionStates = registry.AcquisitionStates
)

func stateObservation(
	metric, state string,
	states []string,
	labels []metrix.Label,
	scope metrix.HostScope,
) hardwareObservation {
	return hardwareObservation{Metric: metric, State: state, States: states, Labels: labels, Scope: scope}
}

func normalizeHealth(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "ok":
		return "ok"
	case "warning":
		return "warning"
	case "critical":
		return "critical"
	default:
		return "unknown"
	}
}

func normalizeResourceState(value string) string {
	return normalizedEnum(value, resourceStates)
}

func normalizedEnum(value string, allowed []string) string {
	value = snakeCase(value)
	if slices.Contains(allowed, value) {
		return value
	}
	return "unknown"
}

type stateSource struct {
	Kind         string
	Document     string
	Path         string
	Metric       string
	States       []string
	BooleanFalse string
	BooleanTrue  string
}

var additionalStateSources = func() []stateSource {
	result := make([]stateSource, 0, len(standardRegistry.States))
	for _, source := range standardRegistry.States {
		result = append(result, stateSource{
			Kind:         string(source.Kind),
			Document:     string(source.Document),
			Path:         source.Path,
			Metric:       source.Metric,
			States:       source.States,
			BooleanFalse: source.BooleanFalse,
			BooleanTrue:  source.BooleanTrue,
		})
	}
	return result
}()

func (c *protocolClient) additionalStateObservations(
	node *graphNode,
	labels []metrix.Label,
	scope metrix.HostScope,
) []hardwareObservation {
	var result []hardwareObservation
	for _, source := range additionalStateSources {
		if source.Kind != node.Kind {
			continue
		}
		document := node.Data
		if source.Document != "" {
			document = findEnrichment(node, source.Document)
		}
		raw, ok := jsonPath(document, source.Path)
		if !ok || raw == nil {
			continue
		}
		var state string
		if source.BooleanFalse != "" || source.BooleanTrue != "" {
			value, ok := raw.(bool)
			if !ok {
				state = "unknown"
			} else if value {
				state = source.BooleanTrue
			} else {
				state = source.BooleanFalse
			}
		} else if value, ok := raw.(string); ok {
			state = normalizedEnum(value, source.States)
		} else {
			state = "unknown"
		}
		result = append(result, stateObservation(source.Metric, state, source.States, labels, scope))
	}
	return result
}

func stringValueAt(data map[string]any, path string) (string, bool) {
	value, ok := jsonPath(data, path)
	if !ok {
		return "", false
	}
	return stringValue(value)
}

func (c *protocolClient) metricLabels(node *graphNode, reading *normalizedReading) []metrix.Label {
	labels := []metrix.Label{
		{Key: "endpoint_key", Value: stableKey("netdata:redfish:endpoint:v1", c.origin, endpointKeyHexChars)},
	}
	addLabel := func(key, value string) {
		labels = upsertLabel(labels, key, value)
	}
	addLabel("resource_key", node.Key)
	addLabel("endpoint_job", c.endpointJob)
	addLabel("resource_kind", node.Kind)
	addLabel("resource_name", node.Doc.Name)
	addLabel("source_model", node.SourceModel)
	for _, kind := range standardRegistry.Kinds {
		if kind.ID == registry.Kind(node.Kind) {
			addLabel("component_family", kind.ComponentFamily)
			break
		}
	}
	if node.LogicalOwner != nil {
		addLabel("logical_owner_key", node.LogicalOwner.Key)
		addLabel("logical_owner_name", node.LogicalOwner.Doc.Name)
	}
	if node.RollupOwner != nil {
		addLabel("rollup_owner_kind", node.RollupOwner.Kind)
		addLabel("rollup_owner_key", node.RollupOwner.Key)
		addLabel("rollup_owner_name", node.RollupOwner.Doc.Name)
	}
	if len(node.SystemOwners) == 1 {
		for _, system := range node.SystemOwners {
			addLabel("system_key", system.Key)
			addLabel("system_name", system.Doc.Name)
		}
	}
	addMetricInventoryLabels(addLabel, node)
	if reading != nil {
		addLabel("reading_key", reading.Key)
		addLabel("physical_context", reading.PhysicalContext)
		addLabel("physical_subcontext", reading.PhysicalSubcontext)
		addLabel("reading_type", reading.Family)
		addLabel("reading_basis", reading.Basis)
		addLabel("reading_role", reading.Role)
		addLabel("reading_source", reading.SourcePath)
		addLabel("semantic_source_class", reading.SemanticSourceClass)
		addLabel("implementation_type", reading.ImplementationType)
		if strings.HasPrefix(reading.Metric, "system_hw_sensor_") {
			addLabel("_collect_module", "redfish")
		}
	}
	return labels
}

func addMetricInventoryLabels(add func(string, string), node *graphNode) {
	if node == nil || node.Data == nil {
		return
	}
	addPath := func(label string, paths ...string) {
		for _, path := range paths {
			value, ok := inventoryValueAt(node.Data, path)
			if !ok {
				continue
			}
			text, ok := stringValue(value)
			if ok {
				add(label, text)
				return
			}
		}
	}
	addPath("manufacturer", "Manufacturer")
	addPath("model", "Model")
	addPath("serial_number", "SerialNumber")
	addPath("asset_tag", "AssetTag")
	addPath("part_number", "PartNumber")
	addPath("spare_part_number", "SparePartNumber")
	addPath("firmware_version", "FirmwareVersion")
	addPath("bios_version", "BiosVersion")
	addPath("slot", "Slot", "DeviceLocator", "Socket")
	addPath("location", "Location.PartLocation.ServiceLabel", "PhysicalLocation.PartLocation.ServiceLabel")
	addPath("mac_address", "MACAddress", "Ethernet.MACAddress")
	addPath("wwn", "FibreChannel.WWPN", "FibreChannel.WWNN")
	addPath("link_type", "LinkNetworkTechnology", "ActiveLinkTechnology")
}

func (c *protocolClient) scopeForNode(node *graphNode) metrix.HostScope {
	if c.config.NodeMode != "system_vnodes" {
		return metrix.HostScope{}
	}
	if len(node.SystemOwners) == 1 {
		for _, system := range node.SystemOwners {
			return c.systemHostScope(system)
		}
	}
	return c.serviceHostScope()
}

func (c *protocolClient) validateHostScopeIdentities(nodes []*graphNode) error {
	if c.config.NodeMode != "system_vnodes" {
		return nil
	}
	seen := make(map[string]string)
	bindings := make([]identityBinding, 0, len(nodes))
	for _, node := range nodes {
		identity := "service:" + stableKey("netdata:redfish:endpoint:v1", c.origin, endpointKeyHexChars)
		if len(node.SystemOwners) == 1 {
			for _, system := range node.SystemOwners {
				identity = "system:" + system.Key
			}
		}
		guid := c.scopeForNode(node).GUID
		if previous, exists := seen[guid]; exists && previous != identity {
			return fmt.Errorf(
				"%w: Redfish HostScope GUID collision between %s and %s",
				errIdentityIntegrity,
				previous,
				identity,
			)
		}
		seen[guid] = identity
		bindings = append(bindings, identityBinding{
			Domain: "host_scope", Key: guid, Preimage: identity,
		})
	}
	if err := c.identities.register(bindings); err != nil {
		return fmt.Errorf("%w: Redfish HostScope GUID collision", err)
	}
	return nil
}

func (c *protocolClient) systemHostScope(system *graphNode) metrix.HostScope {
	guid := uuid.NewSHA1(redfishHostScopeNamespace, []byte("system\x00"+c.origin+"\x00"+system.URI)).String()
	hostname := "redfish-" + system.Key[:min(12, len(system.Key))]
	for _, override := range c.config.HostScopeOverrides {
		uri, _ := normalizeConfiguredResourceURI(c.root, override.ResourceURI)
		if uri != system.URI {
			continue
		}
		if override.GUID != "" {
			guid = override.GUID
		}
		if override.Hostname != "" {
			hostname = override.Hostname
		}
	}
	labels := map[string]string{
		"_vnode_type":    "redfish",
		"endpoint_key":   stableKey("netdata:redfish:endpoint:v1", c.origin, endpointKeyHexChars),
		"redfish_origin": c.origin,
		"system_key":     system.Key,
	}
	addHostLabel(labels, "endpoint_job", c.endpointJob)
	addHostLabel(labels, "system_name", system.Doc.Name)
	addMapHostLabel(labels, system.Data, "system_uuid", "UUID")
	addMapHostLabel(labels, system.Data, "manufacturer", "Manufacturer")
	addMapHostLabel(labels, system.Data, "model", "Model")
	addMapHostLabel(labels, system.Data, "serial_number", "SerialNumber")
	addMapHostLabel(labels, system.Data, "asset_tag", "AssetTag")
	addMapHostLabel(labels, system.Data, "part_number", "PartNumber")
	addMapHostLabel(labels, system.Data, "sku", "SKU")
	addMapHostLabel(labels, system.Data, "bios_version", "BiosVersion")
	return metrix.HostScope{ScopeKey: guid, GUID: guid, Hostname: hostname, Labels: labels}
}

func (c *protocolClient) serviceHostScope() metrix.HostScope {
	guid := uuid.NewSHA1(redfishHostScopeNamespace, []byte("service\x00"+c.origin+"\x00/redfish/v1/")).String()
	key := stableKey("netdata:redfish:endpoint:v1", c.origin, endpointKeyHexChars)
	hostname := "redfish-service-" + key[:min(12, len(key))]
	for _, override := range c.config.HostScopeOverrides {
		uri, _ := normalizeConfiguredResourceURI(c.root, override.ResourceURI)
		if uri != "/redfish/v1" {
			continue
		}
		if override.GUID != "" {
			guid = override.GUID
		}
		if override.Hostname != "" {
			hostname = override.Hostname
		}
	}
	labels := map[string]string{
		"_vnode_type":    "redfish",
		"endpoint_key":   key,
		"redfish_origin": c.origin,
	}
	addHostLabel(labels, "endpoint_job", c.endpointJob)
	c.serviceMetaMu.RLock()
	addHostLabel(labels, "service_name", c.serviceName)
	addHostLabel(labels, "redfish_version", c.redfishVersion)
	c.serviceMetaMu.RUnlock()
	return metrix.HostScope{
		ScopeKey: guid,
		GUID:     guid,
		Hostname: hostname,
		Labels:   labels,
	}
}

func addHostLabel(labels map[string]string, key, value string) {
	value = strings.TrimSpace(value)
	if value != "" && len(value) <= promotedLabelLimit {
		labels[key] = value
	}
}

func addMapHostLabel(labels map[string]string, data map[string]any, key, path string) {
	if value, ok := stringValueAt(data, path); ok {
		addHostLabel(labels, key, value)
	}
}

func snakeCase(value string) string {
	value = strings.TrimSpace(value)
	var result strings.Builder
	for i, r := range value {
		if r == '-' || r == ' ' {
			if result.Len() > 0 {
				result.WriteByte('_')
			}
			continue
		}
		if r >= 'A' && r <= 'Z' {
			if i > 0 && result.Len() > 0 {
				previous := value[i-1]
				if previous >= 'a' && previous <= 'z' {
					result.WriteByte('_')
				}
			}
			result.WriteRune(r + ('a' - 'A'))
			continue
		}
		result.WriteRune(r)
	}
	return result.String()
}

func (c *protocolClient) detailGateObservations(
	gates map[string]detailGate,
	graph *resourceGraph,
) []hardwareObservation {
	var result []hardwareObservation
	keys := make([]string, 0, len(gates))
	for key := range gates {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		gate := gates[key]
		parts := strings.SplitN(key, "\x00", 2)
		if len(parts) != 2 {
			continue
		}
		owner := graph.findKey(parts[0])
		if owner == nil {
			continue
		}
		family := parts[1]
		labels := c.metricLabels(owner, nil)
		labels = upsertLabel(labels, "logical_owner_key", owner.Key)
		labels = upsertLabel(labels, "component_family", family)
		scope := c.scopeForNode(owner)
		result = append(result,
			hardwareObservation{Metric: "collection_detail_components_components", Value: float64(gate.Count), Labels: labels, Scope: scope},
			hardwareObservation{Metric: "collection_detail_components_cap", Value: float64(dereferenceInt(c.config.Charts.MaxDetailedComponentsPerFamily)), Labels: labels, Scope: scope},
			stateObservation("collection_detail_gate", map[bool]string{true: "open", false: "closed"}[gate.Open], []string{"open", "closed"}, labels, scope),
			stateObservation("collection_detail_evidence", map[bool]string{true: "complete", false: "incomplete"}[gate.Complete], []string{"complete", "incomplete"}, labels, scope),
		)
	}
	return result
}

func (g *resourceGraph) findKey(key string) *graphNode {
	g.ensureLookupIndexes()
	return g.ByKey[key]
}

func (c *protocolClient) aggregateObservations(
	graph *resourceGraph,
	nodes []*graphNode,
	readings map[string][]normalizedReading,
	scalars map[string][]scalarValue,
) ([]hardwareObservation, error) {
	nodeByKey := make(map[string]*graphNode, len(nodes))
	for _, node := range nodes {
		nodeByKey[node.Key] = node
	}
	sliceEvidence, nodeSlices := aggregateSliceEvidence(graph, nodes)
	keyRegistry := make(map[string]string)
	type currentGroup struct {
		aggregateSnapshot
		Owner   *graphNode
		Members map[string]aggregateMember
	}
	current := make(map[string]*currentGroup)
	add := func(key string, snapshot aggregateSnapshot, owner *graphNode, member aggregateMember) {
		group := current[key]
		if group == nil {
			snapshot.Members = make(map[string]struct{})
			snapshot.SliceMembers = make(map[string]map[string]struct{})
			group = &currentGroup{
				aggregateSnapshot: snapshot,
				Owner:             owner,
				Members:           make(map[string]aggregateMember),
			}
			current[key] = group
		}
		memberKey := member.Key
		if memberKey == "" {
			memberKey = member.Node.Key
		}
		group.Members[memberKey] = member
		group.aggregateSnapshot.Members[memberKey] = struct{}{}
		for _, sliceKey := range member.Slices {
			members := group.aggregateSnapshot.SliceMembers[sliceKey]
			if members == nil {
				members = make(map[string]struct{})
				group.aggregateSnapshot.SliceMembers[sliceKey] = members
			}
			members[memberKey] = struct{}{}
		}
	}
	for _, node := range nodes {
		if node.RollupOwner == nil {
			continue
		}
		for _, reading := range readings[node.Key] {
			if !reading.Primary ||
				reading.Family == "" ||
				reading.AggregateSemantic == "" ||
				reading.AggregateClass == "" {
				continue
			}
			if !kindContains(reading.AggregateKinds, node.RollupOwner.Kind) {
				continue
			}
			childContext := aggregateReadingContext(node.Kind, reading.Context)
			snapshot := aggregateSnapshot{
				OwnerKey:            node.RollupOwner.Key,
				OwnerKind:           node.RollupOwner.Kind,
				ChildKind:           node.Kind,
				Semantic:            childContext,
				Role:                reading.Role,
				Family:              "sensor." + reading.Family,
				Basis:               reading.Basis,
				Units:               reading.Units,
				Source:              reading.SourcePath,
				PhysicalContext:     reading.PhysicalContext,
				SemanticSourceClass: reading.SemanticSourceClass,
				AggregateClass:      reading.AggregateClass,
				Histogram:           reading.Histogram,
			}
			add(aggregateGroupKey(snapshot), snapshot, node.RollupOwner, aggregateMember{
				Key: reading.Key, Node: node, Value: reading.Value, Readable: reading.Valid, Histogram: reading.Histogram,
				RangeMin: reading.RangeMin, RangeMax: reading.RangeMax, Slices: nodeSlices[node.Key],
			})
		}
		for _, value := range scalars[node.Key] {
			if !value.Present ||
				value.Descriptor.Exposure != registry.ExposureOperationalScalar ||
				value.Descriptor.AggregateClass == "" {
				continue
			}
			if !kindContains(value.Descriptor.AggregateKinds, node.RollupOwner.Kind) {
				continue
			}
			snapshot := aggregateSnapshot{
				OwnerKey:       node.RollupOwner.Key,
				OwnerKind:      node.RollupOwner.Kind,
				ChildKind:      node.Kind,
				Semantic:       value.Descriptor.Context,
				Role:           value.Descriptor.Role,
				Family:         node.Kind,
				Basis:          "zero",
				Units:          value.Descriptor.Units,
				AggregateClass: value.Descriptor.AggregateClass,
				Additive:       value.Descriptor.Additive,
				Histogram:      value.Descriptor.Histogram,
			}
			add(aggregateGroupKey(snapshot), snapshot, node.RollupOwner, aggregateMember{
				Node: node, Value: value.Value, Readable: value.Emit, Histogram: value.Descriptor.Histogram,
				Slices: nodeSlices[node.Key],
			})
		}
	}
	currentKeys := make([]string, 0, len(current))
	for key := range current {
		currentKeys = append(currentKeys, key)
	}
	sort.Strings(currentKeys)
	for _, key := range currentKeys {
		if err := c.registerAggregateKey(keyRegistry, key); err != nil {
			return nil, err
		}
	}

	currentSnapshots := make(map[string]aggregateSnapshot, len(current))
	for key, group := range current {
		currentSnapshots[key] = group.aggregateSnapshot
	}
	c.aggregateMu.Lock()
	effective, aggregateRetentionComplete := reconcileAggregateSnapshots(
		c.aggregateState,
		currentSnapshots,
		sliceEvidence,
		nodeByKey,
		graph.Complete,
		aggregateRetentionBudget,
	)
	c.aggregateMu.Unlock()
	if !aggregateRetentionComplete {
		graph.addDiagnostic("Redfish aggregate continuity state exceeded its internal retention budget")
	}

	var result []hardwareObservation
	keys := make([]string, 0, len(effective))
	for key := range effective {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if err := c.registerAggregateKey(keyRegistry, key); err != nil {
			return nil, err
		}
		info := effective[key]
		if len(info.Members) == 0 {
			continue
		}
		owner := nodeByKey[info.OwnerKey]
		if owner == nil {
			continue
		}
		var readable []aggregateMember
		unreadable, unknown := 0, 0
		currentGroup := current[key]
		for memberKey := range info.Members {
			if currentGroup == nil {
				unknown++
				continue
			}
			member, exists := currentGroup.Members[memberKey]
			if !exists {
				unknown++
				continue
			}
			if !member.Readable {
				unreadable++
				continue
			}
			readable = append(readable, member)
		}
		scope := c.scopeForNode(owner)
		prefix := "aggregate_" + info.AggregateClass
		groupComplete := aggregateSlicesComplete(info.SliceMembers, sliceEvidence) &&
			unreadable == 0 && unknown == 0
		labels := c.aggregateLabels(owner, info, key)
		result = append(result, aggregatePopulationObservations(
			len(info.Members),
			len(readable),
			unreadable,
			unknown,
			groupComplete,
			labels,
			scope,
		)...)
		if len(readable) > 0 {
			minimum, maximum, total := readable[0].Value, readable[0].Value, 0.0
			for _, member := range readable {
				minimum = min(minimum, member.Value)
				maximum = max(maximum, member.Value)
				total += member.Value
			}
			result = append(result,
				hardwareObservation{Metric: prefix + "_minimum", Value: minimum, Labels: labels, Scope: scope},
				hardwareObservation{Metric: prefix + "_average", Value: total / float64(len(readable)), Labels: labels, Scope: scope},
				hardwareObservation{Metric: prefix + "_maximum", Value: maximum, Labels: labels, Scope: scope},
			)
			if info.Additive {
				result = append(result, hardwareObservation{
					Metric: prefix + "_total", Value: total,
					Labels: labels, Scope: scope,
				})
			}
		}
		result = append(result, histogramObservations(
			readable,
			labels,
			scope,
			info.Histogram,
		)...)
	}
	categorical, err := c.categoricalAggregateObservations(
		graph,
		nodes,
		nodeByKey,
		readings,
		sliceEvidence,
		nodeSlices,
		keyRegistry,
	)
	if err != nil {
		return nil, err
	}
	return append(result, categorical...), nil
}

func cloneAggregateSnapshot(src aggregateSnapshot) aggregateSnapshot {
	src.States = append([]string(nil), src.States...)
	src.Members = cloneStringSet(src.Members)
	if src.SliceMembers != nil {
		slices := make(map[string]map[string]struct{}, len(src.SliceMembers))
		for key, members := range src.SliceMembers {
			slices[key] = cloneStringSet(members)
		}
		src.SliceMembers = slices
	}
	return src
}

func aggregateSnapshotWithCurrentMetadata(
	retained aggregateSnapshot,
	current aggregateSnapshot,
) aggregateSnapshot {
	members, slices := retained.Members, retained.SliceMembers
	retained = current
	retained.Members = members
	retained.SliceMembers = slices
	return retained
}

func reconcileAggregateSnapshots(
	retained map[string]aggregateSnapshot,
	current map[string]aggregateSnapshot,
	sliceEvidence map[string]bool,
	nodeByKey map[string]*graphNode,
	graphComplete bool,
	budget retainedStateBudget,
) (map[string]aggregateSnapshot, bool) {
	if graphComplete {
		for key, snapshot := range retained {
			if nodeByKey[snapshot.OwnerKey] == nil {
				delete(retained, key)
			}
		}
	}
	allKeys := make(map[string]struct{}, len(retained)+len(current))
	for key := range retained {
		allKeys[key] = struct{}{}
	}
	for key := range current {
		allKeys[key] = struct{}{}
	}
	keys := make([]string, 0, len(allKeys))
	for key := range allKeys {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	retainedMembers := aggregateRetentionMembers(retained)
	retentionComplete := retainedStateFits(
		0,
		0,
		len(retained),
		retainedMembers,
		budget,
	)
	for _, key := range keys {
		snapshot, exists := retained[key]
		currentSnapshot, currentExists := current[key]
		if !exists {
			if !currentExists {
				continue
			}
			snapshot = cloneAggregateSnapshot(currentSnapshot)
			snapshot.Members = nil
			snapshot.SliceMembers = nil
		}
		if currentExists {
			snapshot = aggregateSnapshotWithCurrentMetadata(snapshot, currentSnapshot)
		}
		if snapshot.SliceMembers == nil {
			snapshot.SliceMembers = make(map[string]map[string]struct{})
		}
		sliceKeys := make(map[string]struct{}, len(snapshot.SliceMembers))
		for sliceKey := range snapshot.SliceMembers {
			sliceKeys[sliceKey] = struct{}{}
		}
		if currentExists {
			for sliceKey := range currentSnapshot.SliceMembers {
				sliceKeys[sliceKey] = struct{}{}
			}
		}
		for sliceKey := range sliceKeys {
			complete, observed := sliceEvidence[sliceKey]
			if !observed || !complete {
				continue
			}
			if !currentExists || len(currentSnapshot.SliceMembers[sliceKey]) == 0 {
				delete(snapshot.SliceMembers, sliceKey)
				continue
			}
			snapshot.SliceMembers[sliceKey] = cloneStringSet(currentSnapshot.SliceMembers[sliceKey])
		}
		refreshAggregateMembers(&snapshot)
		if len(snapshot.Members) == 0 {
			if exists {
				retainedMembers -= aggregateSnapshotRetentionMembers(retained[key])
			}
			delete(retained, key)
			continue
		}
		existingMembers := 0
		baseEntries := len(retained)
		if exists {
			existingMembers = aggregateSnapshotRetentionMembers(retained[key])
			baseEntries--
		}
		baseMembers := retainedMembers - existingMembers
		candidateMembers := aggregateSnapshotRetentionMembers(snapshot)
		if retainedStateFits(baseEntries, baseMembers, 1, candidateMembers, budget) {
			retained[key] = snapshot
			retainedMembers = baseMembers + candidateMembers
			continue
		}
		if exists {
			delete(retained, key)
			retainedMembers = baseMembers
		}
		retentionComplete = false
	}

	effective := make(map[string]aggregateSnapshot, len(retained)+len(current))
	for key, snapshot := range retained {
		effective[key] = cloneAggregateSnapshot(snapshot)
	}
	for key, currentSnapshot := range current {
		snapshot, exists := effective[key]
		if !exists {
			snapshot = cloneAggregateSnapshot(currentSnapshot)
		} else {
			snapshot = aggregateSnapshotWithCurrentMetadata(snapshot, currentSnapshot)
		}
		if snapshot.SliceMembers == nil {
			snapshot.SliceMembers = make(map[string]map[string]struct{})
		}
		for sliceKey, members := range currentSnapshot.SliceMembers {
			if sliceEvidence[sliceKey] {
				continue
			}
			combined := cloneStringSet(snapshot.SliceMembers[sliceKey])
			for memberKey := range members {
				combined[memberKey] = struct{}{}
			}
			snapshot.SliceMembers[sliceKey] = combined
		}
		refreshAggregateMembers(&snapshot)
		effective[key] = snapshot
	}
	return effective, retentionComplete
}

func aggregateRetentionMembers(retained map[string]aggregateSnapshot) int {
	total := 0
	for _, snapshot := range retained {
		total += aggregateSnapshotRetentionMembers(snapshot)
	}
	return total
}

func aggregateSnapshotRetentionMembers(snapshot aggregateSnapshot) int {
	total := len(snapshot.Members)
	for _, members := range snapshot.SliceMembers {
		total += len(members)
	}
	return total
}

func refreshAggregateMembers(snapshot *aggregateSnapshot) {
	snapshot.Members = make(map[string]struct{})
	for _, members := range snapshot.SliceMembers {
		for memberKey := range members {
			snapshot.Members[memberKey] = struct{}{}
		}
	}
}

func aggregateSlicesComplete(
	slices map[string]map[string]struct{},
	evidence map[string]bool,
) bool {
	if len(slices) == 0 {
		return false
	}
	for sliceKey := range slices {
		complete, observed := evidence[sliceKey]
		if !observed || !complete {
			return false
		}
	}
	return true
}

func (c *protocolClient) categoricalAggregateObservations(
	graph *resourceGraph,
	nodes []*graphNode,
	nodeByKey map[string]*graphNode,
	readings map[string][]normalizedReading,
	sliceEvidence map[string]bool,
	nodeSlices map[string][]string,
	keyRegistry map[string]string,
) ([]hardwareObservation, error) {
	type categoryMember struct {
		Counts   map[string]int
		Readable bool
	}
	type categoryGroup struct {
		aggregateSnapshot
		Owner   *graphNode
		Members map[string]categoryMember
	}
	groups := make(map[string]*categoryGroup)
	add := func(
		owner, node *graphNode,
		snapshot aggregateSnapshot,
		state string,
		count int,
		readable bool,
	) {
		if owner == nil || node == nil {
			return
		}
		snapshot.OwnerKey = owner.Key
		snapshot.OwnerKind = owner.Kind
		snapshot.ChildKind = node.Kind
		snapshot.Basis = "state"
		snapshot.Units = categoricalAggregateUnits(snapshot.AggregateClass)
		key := aggregateGroupKey(snapshot)
		group := groups[key]
		if group == nil {
			snapshot.States = append([]string(nil), snapshot.States...)
			snapshot.Members = make(map[string]struct{})
			snapshot.SliceMembers = make(map[string]map[string]struct{})
			group = &categoryGroup{
				aggregateSnapshot: snapshot,
				Owner:             owner, Members: make(map[string]categoryMember),
			}
			groups[key] = group
		}
		member, exists := group.Members[node.Key]
		if !exists {
			member = categoryMember{Counts: make(map[string]int, len(snapshot.States)), Readable: true}
		}
		member.Readable = member.Readable && readable
		if readable && state != "" {
			member.Counts[state] += count
		}
		group.Members[node.Key] = member
		group.aggregateSnapshot.Members[node.Key] = struct{}{}
		for _, sliceKey := range nodeSlices[node.Key] {
			members := group.aggregateSnapshot.SliceMembers[sliceKey]
			if members == nil {
				members = make(map[string]struct{})
				group.aggregateSnapshot.SliceMembers[sliceKey] = members
			}
			members[node.Key] = struct{}{}
		}
	}

	for _, node := range nodes {
		owner := node.RollupOwner
		if owner == nil {
			continue
		}
		family := componentFamily(node, readings[node.Key])
		status := statusForKind(node.Kind)
		statusAggregates := kindContains(status.AggregateKinds, owner.Kind)
		if status.Status && statusAggregates {
			if state, present, readable := categoricalStringState(node.Data, "Status.Health", normalizeHealth); present {
				add(owner, node, aggregateSnapshot{
					Semantic: "redfish." + node.Kind + ".health", Role: "health", Family: family,
					AggregateClass: "health", States: healthStates, OneHot: true,
				}, state, 1, readable)
			}
			if state, present, readable := categoricalStringState(node.Data, "Status.HealthRollup", normalizeHealth); present {
				add(owner, node, aggregateSnapshot{
					Semantic: "redfish." + node.Kind + ".health_rollup", Role: "health_rollup", Family: family,
					AggregateClass: "health_rollup", States: healthStates, OneHot: true,
				}, state, 1, readable)
			}
			if state, present, readable := categoricalStringState(node.Data, "Status.State", normalizeResourceState); present {
				add(owner, node, aggregateSnapshot{
					Semantic: "redfish." + node.Kind + ".state", Role: "state", Family: family,
					AggregateClass: "resource_state", States: resourceStates, OneHot: true,
				}, state, 1, readable)
			}
		}
		if status.PowerState && statusAggregates {
			if state, present, readable := categoricalStringState(node.Data, "PowerState", func(value string) string {
				return normalizedEnum(value, powerStates)
			}); present {
				add(owner, node, aggregateSnapshot{
					Semantic: "redfish." + node.Kind + ".power_state", Role: "power_state", Family: family,
					AggregateClass: "power_state", States: powerStates, OneHot: true,
				}, state, 1, readable)
			}
		}
		if status.FailurePredicted && statusAggregates {
			if raw, exists := jsonPath(node.Data, "FailurePredicted"); exists && raw != nil {
				prediction := "unknown"
				readable := false
				if value, ok := raw.(bool); ok {
					readable = true
					if value {
						prediction = "predicted"
					} else {
						prediction = "clear"
					}
				}
				add(owner, node, aggregateSnapshot{
					Semantic: "redfish." + node.Kind + ".failure_predicted", Role: "failure_predicted", Family: family,
					AggregateClass: "failure_predicted", States: failureStates, OneHot: true,
				}, prediction, 1, readable)
			}
		}
		if node.AcquisitionState != "" && statusAggregates {
			acquisition := normalizedEnum(node.AcquisitionState, acquisitionStates)
			add(owner, node, aggregateSnapshot{
				Semantic: "redfish." + node.Kind + ".acquisition_state", Role: "acquisition_state", Family: family,
				AggregateClass: "acquisition_state", States: acquisitionStates, OneHot: true,
			}, acquisition, 1, true)
		}
		if status.Status && statusAggregates {
			if conditions, present, readable := conditionCountsForNode(node); present {
				semantic := "redfish." + node.Kind + ".conditions"
				for state, count := range map[string]int{
					"ok": conditions.OK, "warning": conditions.Warning,
					"critical": conditions.Critical, "unknown": conditions.Unknown,
				} {
					add(owner, node, aggregateSnapshot{
						Semantic: semantic, Role: "conditions", Family: family,
						AggregateClass: "conditions", States: healthStates,
					}, state, count, readable)
				}
			}
		}

		for _, stateSpec := range standardRegistry.States {
			if string(stateSpec.Kind) != node.Kind ||
				!kindContains(stateSpec.AggregateKinds, owner.Kind) {
				continue
			}
			document := node.Data
			if stateSpec.Document != "" {
				document = findEnrichment(node, string(stateSpec.Document))
			}
			raw, present := jsonPath(document, stateSpec.Path)
			if !present || raw == nil {
				continue
			}
			state := "unknown"
			readable := false
			if stateSpec.BooleanFalse != "" || stateSpec.BooleanTrue != "" {
				if value, ok := raw.(bool); ok {
					readable = true
					if value {
						state = stateSpec.BooleanTrue
					} else {
						state = stateSpec.BooleanFalse
					}
				}
			} else if value, ok := raw.(string); ok {
				state, readable = normalizedEnum(value, stateSpec.States), true
			}
			role := strings.TrimPrefix(stateSpec.Context, "redfish."+node.Kind+".")
			add(owner, node, aggregateSnapshot{
				Semantic: stateSpec.Context, Role: role, Family: family,
				AggregateClass: stateSpec.Metric, States: stateSpec.States, OneHot: true,
			}, state, 1, readable)
		}

		for _, flag := range flagValues(node) {
			if !flag.Emit || !kindContains(flag.Set.AggregateKinds, owner.Kind) {
				continue
			}
			count := 0
			if flag.Present && flag.Value {
				count = 1
			}
			roles := make([]string, 0, len(flag.Set.Members))
			for _, member := range flag.Set.Members {
				roles = append(roles, member.Role)
			}
			role := strings.TrimPrefix(flag.Set.Context, "redfish."+node.Kind+".")
			add(owner, node, aggregateSnapshot{
				Semantic: flag.Set.Context, Role: role, Family: family,
				AggregateClass: flag.Set.Metric, States: roles,
			}, flag.Member.Role, count, true)
		}

		for _, reading := range readings[node.Key] {
			if !reading.Primary || reading.AggregateSemantic == "" || reading.EffectiveAlarm == "" ||
				!kindContains(reading.AggregateKinds, owner.Kind) {
				continue
			}
			add(owner, node, aggregateSnapshot{
				Semantic: aggregateReadingContext(node.Kind, reading.Context), Role: "alarm",
				Family: "sensor." + reading.Family, AggregateClass: "reading_alarm",
				States: registry.AlarmStates, OneHot: true,
			}, reading.EffectiveAlarm, 1, true)
		}
	}
	groupKeys := make([]string, 0, len(groups))
	for key := range groups {
		groupKeys = append(groupKeys, key)
	}
	sort.Strings(groupKeys)
	for _, key := range groupKeys {
		if err := c.registerAggregateKey(keyRegistry, key); err != nil {
			return nil, err
		}
	}

	currentSnapshots := make(map[string]aggregateSnapshot, len(groups))
	for key, group := range groups {
		currentSnapshots[key] = group.aggregateSnapshot
	}
	c.aggregateMu.Lock()
	effective, aggregateRetentionComplete := reconcileAggregateSnapshots(
		c.categoricalAggregateState,
		currentSnapshots,
		sliceEvidence,
		nodeByKey,
		graph.Complete,
		aggregateRetentionBudget,
	)
	c.aggregateMu.Unlock()
	if !aggregateRetentionComplete {
		graph.addDiagnostic("Redfish aggregate continuity state exceeded its internal retention budget")
	}

	keys := make([]string, 0, len(effective))
	for key := range effective {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var result []hardwareObservation
	for _, key := range keys {
		if err := c.registerAggregateKey(keyRegistry, key); err != nil {
			return nil, err
		}
		info := effective[key]
		owner := nodeByKey[info.OwnerKey]
		if owner == nil {
			continue
		}
		counts := make(map[string]int, len(info.States))
		readable, unreadable, unknown := 0, 0, 0
		currentGroup := groups[key]
		for memberKey := range info.Members {
			if currentGroup == nil {
				unknown++
				continue
			}
			member, exists := currentGroup.Members[memberKey]
			if !exists {
				unknown++
				continue
			}
			if !member.Readable {
				unreadable++
				continue
			}
			readable++
			for state, count := range member.Counts {
				counts[state] += count
			}
		}
		if info.OneHot && slices.Contains(info.States, "unknown") {
			counts["unknown"] += unknown + unreadable
		}
		complete := aggregateSlicesComplete(info.SliceMembers, sliceEvidence) &&
			unreadable == 0 && unknown == 0
		labels := c.aggregateLabels(owner, info, key)
		scope := c.scopeForNode(owner)
		result = append(result, aggregatePopulationObservations(
			len(info.Members), readable, unreadable, unknown, complete, labels, scope,
		)...)
		result = append(result, aggregateNoHistogramObservations(readable, labels, scope)...)
		for _, state := range info.States {
			result = append(result, hardwareObservation{
				Metric: "aggregate_" + info.AggregateClass + "_" + state,
				Value:  float64(counts[state]),
				Labels: labels,
				Scope:  scope,
			})
		}
	}
	return result, nil
}

func categoricalStringState(
	document map[string]any,
	path string,
	normalize func(string) string,
) (state string, present, readable bool) {
	raw, present := jsonPath(document, path)
	if !present || raw == nil {
		return "", false, false
	}
	value, ok := raw.(string)
	if !ok {
		return "unknown", true, true
	}
	return normalize(value), true, true
}

func categoricalAggregateUnits(class string) string {
	if class == "conditions" {
		return "conditions"
	}
	return "components"
}

func conditionCountsForNode(node *graphNode) (conditionCounts, bool, bool) {
	if node == nil {
		return conditionCounts{}, false, false
	}
	raw, present := jsonPath(node.Data, "Status.Conditions")
	if !present || raw == nil {
		return conditionCounts{}, false, false
	}
	if _, ok := raw.([]any); !ok {
		return conditionCounts{}, true, false
	}
	return conditionCountsFrom(node.Doc.Status.Conditions), true, true
}

func aggregatePopulationObservations(
	total, readable, unreadable, unknown int,
	complete bool,
	labels []metrix.Label,
	scope metrix.HostScope,
) []hardwareObservation {
	return []hardwareObservation{
		{Metric: "aggregate_population_total", Value: float64(total), Labels: labels, Scope: scope},
		{Metric: "aggregate_population_readable", Value: float64(readable), Labels: labels, Scope: scope},
		{Metric: "aggregate_population_unreadable", Value: float64(unreadable), Labels: labels, Scope: scope},
		{Metric: "aggregate_population_unknown", Value: float64(unknown), Labels: labels, Scope: scope},
		{Metric: "aggregate_completeness_complete", Value: boolFloat(complete), Labels: labels, Scope: scope},
		{Metric: "aggregate_completeness_incomplete", Value: boolFloat(!complete), Labels: labels, Scope: scope},
	}
}

func aggregateNoHistogramObservations(
	readable int,
	labels []metrix.Label,
	scope metrix.HostScope,
) []hardwareObservation {
	return []hardwareObservation{
		{Metric: "aggregate_population_histogram_eligible", Value: 0, Labels: labels, Scope: scope},
		{Metric: "aggregate_population_histogram_ineligible", Value: float64(readable), Labels: labels, Scope: scope},
		{Metric: "aggregate_completeness_histogram_available", Value: 0, Labels: labels, Scope: scope},
		{Metric: "aggregate_completeness_histogram_unavailable", Value: 1, Labels: labels, Scope: scope},
	}
}

func kindContains(values []registry.Kind, value string) bool {
	for _, candidate := range values {
		if string(candidate) == value {
			return true
		}
	}
	return false
}

func aggregateGroupKey(snapshot aggregateSnapshot) string {
	// Every field before PhysicalContext is a hash, registry value, or normalized
	// enum and cannot contain NUL. PhysicalContext is the only raw BMC value and
	// is final, so the shipped aggregate identity remains unambiguous.
	return strings.Join([]string{
		snapshot.OwnerKey,
		snapshot.OwnerKind,
		snapshot.Semantic,
		snapshot.Role,
		snapshot.Family,
		snapshot.Basis,
		snapshot.Units,
		snapshot.Source,
		snapshot.PhysicalContext,
	}, "\x00")
}

func aggregateReadingContext(childKind, context string) string {
	return "redfish." + childKind + "." + strings.TrimPrefix(context, "redfish.")
}

func aggregateSliceEvidence(
	graph *resourceGraph,
	nodes []*graphNode,
) (map[string]bool, map[string][]string) {
	evidence := make(map[string]bool)
	nodeSlices := make(map[string][]string)
	nodeByKey := make(map[string]*graphNode, len(nodes))
	for _, node := range nodes {
		nodeByKey[node.Key] = node
	}
	for _, slice := range graph.Slices {
		sliceKey := aggregateSliceKey(slice)
		complete, exists := evidence[sliceKey]
		if !exists {
			complete = true
		}
		evidence[sliceKey] = complete && slice.Complete
		for _, memberKey := range slice.Members {
			node := nodeByKey[memberKey]
			if node == nil || node.RollupOwner == nil || node.RollupOwner.Key != slice.ParentKey {
				continue
			}
			nodeSlices[memberKey] = appendUniqueString(nodeSlices[memberKey], sliceKey)
		}
	}
	for _, node := range nodes {
		if node.RollupOwner == nil || len(nodeSlices[node.Key]) != 0 {
			continue
		}
		// Production graph placement derives a rollup owner from a contributing
		// slice. Keep a conservative identity for synthetic/test graphs rather
		// than treating their membership as authoritative.
		sliceKey := strings.Join([]string{
			"unattributed", node.RollupOwner.Key, node.Key,
		}, "\x00")
		nodeSlices[node.Key] = []string{sliceKey}
		evidence[sliceKey] = false
	}
	return evidence, nodeSlices
}

func aggregateSliceKey(slice graphSlice) string {
	return strings.Join([]string{
		slice.ParentKey,
		slice.Path,
		slice.ChildKind,
		slice.Family,
		string(slice.Mode),
		slice.Source,
	}, "\x00")
}

func appendUniqueString(values []string, value string) []string {
	if slices.Contains(values, value) {
		return values
	}
	return append(values, value)
}

func (c *protocolClient) aggregateLabels(
	owner *graphNode,
	info aggregateSnapshot,
	groupKey string,
) []metrix.Label {
	labels := c.metricLabels(owner, nil)
	labels = upsertLabel(labels, "rollup_owner_kind", owner.Kind)
	labels = upsertLabel(labels, "rollup_owner_key", owner.Key)
	labels = upsertLabel(labels, "rollup_owner_name", owner.Doc.Name)
	labels = upsertLabel(labels, "aggregate_key", c.aggregateKeyDigest(groupKey))
	labels = upsertLabel(labels, "component_family", info.Family)
	labels = upsertLabel(labels, "aggregate_class", info.AggregateClass)
	labels = upsertLabel(labels, "aggregate_semantic", info.Semantic)
	labels = upsertLabel(labels, "aggregate_role", info.Role)
	labels = upsertLabel(labels, "aggregate_units", info.Units)
	labels = upsertLabel(labels, "child_resource_kind", info.ChildKind)
	labels = upsertLabel(labels, "reading_basis", info.Basis)
	labels = upsertLabel(labels, "reading_source", info.Source)
	labels = upsertLabel(labels, "physical_context", info.PhysicalContext)
	labels = upsertLabel(labels, "semantic_source_class", info.SemanticSourceClass)
	return labels
}

func histogramObservations(
	members []aggregateMember,
	labels []metrix.Label,
	scope metrix.HostScope,
	kind string,
) []hardwareObservation {
	if kind == "" {
		return []hardwareObservation{
			{Metric: "aggregate_population_histogram_eligible", Value: 0, Labels: labels, Scope: scope},
			{Metric: "aggregate_population_histogram_ineligible", Value: float64(len(members)), Labels: labels, Scope: scope},
			{Metric: "aggregate_completeness_histogram_available", Value: 0, Labels: labels, Scope: scope},
			{Metric: "aggregate_completeness_histogram_unavailable", Value: 1, Labels: labels, Scope: scope},
		}
	}
	histogram, ok := registry.Histogram(kind)
	if !ok {
		return nil
	}
	counts := make([]int, len(histogram.Buckets))
	eligible := 0
	for _, member := range members {
		value, ok := aggregateHistogramValue(member, kind)
		if !ok {
			continue
		}
		eligible++
		index := len(histogram.Buckets) - 1
		for candidate, bucket := range histogram.Buckets {
			if bucket.UpperExclusive == nil {
				index = candidate
				break
			}
			if value < *bucket.UpperExclusive ||
				(bucket.UpperInclusive && value == *bucket.UpperExclusive) {
				index = candidate
				break
			}
		}
		counts[index]++
	}
	result := []hardwareObservation{
		{Metric: "aggregate_population_histogram_eligible", Value: float64(eligible), Labels: labels, Scope: scope},
		{Metric: "aggregate_population_histogram_ineligible", Value: float64(len(members) - eligible), Labels: labels, Scope: scope},
		{Metric: "aggregate_completeness_histogram_available", Value: boolFloat(eligible > 0), Labels: labels, Scope: scope},
		{Metric: "aggregate_completeness_histogram_unavailable", Value: boolFloat(eligible == 0), Labels: labels, Scope: scope},
	}
	if eligible == 0 {
		return result
	}
	for i, count := range counts {
		result = append(result, hardwareObservation{
			Metric: "aggregate_" + kind + "_distribution_" + histogram.Buckets[i].ID,
			Value:  float64(count),
			Labels: labels,
			Scope:  scope,
		})
	}
	return result
}

func aggregateHistogramValue(member aggregateMember, kind string) (float64, bool) {
	if member.Histogram != kind || !member.Readable || !isFinite(member.Value) {
		return 0, false
	}
	if kind != "range_percentage" {
		return member.Value, true
	}
	if member.RangeMin == nil || member.RangeMax == nil ||
		!isFinite(*member.RangeMin) || !isFinite(*member.RangeMax) ||
		*member.RangeMax <= *member.RangeMin {
		return 0, false
	}
	value := 100 * (member.Value - *member.RangeMin) / (*member.RangeMax - *member.RangeMin)
	return value, isFinite(value)
}

func boolFloat(value bool) float64 {
	if value {
		return 1
	}
	return 0
}

func upsertLabel(labels []metrix.Label, key, value string) []metrix.Label {
	value = strings.TrimSpace(value)
	valid := value != "" && len(value) <= promotedLabelLimit
	for i := range labels {
		if labels[i].Key == key {
			if !valid {
				return append(labels[:i], labels[i+1:]...)
			}
			labels[i].Value = value
			return labels
		}
	}
	if !valid {
		return labels
	}
	return append(labels, metrix.Label{Key: key, Value: value})
}

// Keep the locks with the protocol state. Collection is serial today, while
// explicit locking preserves correctness if framework scheduling changes.
type hardwareState struct {
	rateMu                    sync.Mutex
	rateBaselines             map[string]rateBaseline
	rateBaselineLimit         int
	rateRetentionOverflow     bool
	gateMu                    sync.Mutex
	detailGateState           map[string]detailGate
	aggregateMu               sync.Mutex
	aggregateState            map[string]aggregateSnapshot
	categoricalAggregateState map[string]aggregateSnapshot
	aggregateDigest           func(string) string
}

func (s *hardwareState) initialize() {
	if s.rateBaselines == nil {
		s.rateBaselines = make(map[string]rateBaseline)
	}
	if s.rateBaselineLimit <= 0 {
		s.rateBaselineLimit = rateBaselineRetentionLimit()
	}
	if s.detailGateState == nil {
		s.detailGateState = make(map[string]detailGate)
	}
	if s.aggregateState == nil {
		s.aggregateState = make(map[string]aggregateSnapshot)
	}
	if s.categoricalAggregateState == nil {
		s.categoricalAggregateState = make(map[string]aggregateSnapshot)
	}
	if s.aggregateDigest == nil {
		s.aggregateDigest = func(preimage string) string {
			return stableKey("netdata:redfish:aggregate:v1", preimage, 32)
		}
	}
}

func (c *protocolClient) aggregateKeyDigest(preimage string) string {
	if c.aggregateDigest == nil {
		return stableKey("netdata:redfish:aggregate:v1", preimage, 32)
	}
	return c.aggregateDigest(preimage)
}

func (c *protocolClient) registerAggregateKey(registry map[string]string, preimage string) error {
	digest := c.aggregateKeyDigest(preimage)
	if previous, exists := registry[digest]; exists && previous != preimage {
		return fmt.Errorf("%w: Redfish aggregate-key collision", errIdentityIntegrity)
	}
	registry[digest] = preimage
	if err := c.identities.register([]identityBinding{{
		Domain: "aggregate", Key: digest, Preimage: preimage,
	}}); err != nil {
		return fmt.Errorf("%w: Redfish aggregate-key collision", err)
	}
	return nil
}

func (c *protocolClient) inventoryGraphRow(
	node *graphNode,
	gate detailGate,
	observedAt time.Time,
	readings []normalizedReading,
) map[string]any {
	hostURI, hostKey, hostName := "/redfish/v1/", resourceKey(c.origin, "service", "/redfish/v1/"), "Redfish Service"
	if len(node.SystemOwners) == 1 {
		for _, system := range node.SystemOwners {
			hostURI, hostKey, hostName = system.URI, system.Key, system.Doc.Name
		}
	}
	row := c.inventoryResourceRow(node, observedAt, inventoryHost{
		uri: hostURI, key: hostKey, name: hostName,
	})
	row["acquisition_state"] = node.AcquisitionState
	row["error_class"] = emptyToNil(node.ErrorClass)
	row["identity_quality"] = node.IdentityQuality
	row["source_container_uri"] = emptyToNil(node.SourceContainer)
	row["source_property_path"] = emptyToNil(node.SourcePath)
	if node.SourcePosition != nil {
		row["source_position"] = *node.SourcePosition
	}
	if node.RollupOwner != nil {
		row["rollup_owner_kind"] = node.RollupOwner.Kind
		row["rollup_owner_key"] = node.RollupOwner.Key
		row["rollup_owner_name"] = emptyToNil(node.RollupOwner.Doc.Name)
		row["rollup_owner_uri"] = emptyToNil(node.RollupOwner.URI)
	}
	if node.LogicalOwner != nil {
		row["logical_owner_key"] = node.LogicalOwner.Key
		row["logical_owner_name"] = emptyToNil(node.LogicalOwner.Doc.Name)
		row["logical_owner_uri"] = emptyToNil(node.LogicalOwner.URI)
	}
	row["logical_owner_reason"] = emptyToNil(node.LogicalReason)
	if len(node.LogicalCandidates) > 0 {
		if encoded, err := json.Marshal(node.LogicalCandidates); err == nil {
			row["logical_owner_candidates"] = string(encoded)
		}
	}
	row["component_family"] = componentFamily(node, readings)
	row["detail_gate"] = map[bool]string{true: "open", false: "closed"}[gate.Open]
	row["detail_component_count"] = gate.Count
	row["detail_component_cap"] = dereferenceInt(c.config.Charts.MaxDetailedComponentsPerFamily)
	row["source_schema_type"] = emptyToNil(node.Doc.ODataType)
	row["source_schema"] = emptyToNil(node.Doc.ODataType)
	row["source_model"] = node.SourceModel
	row["source_models"] = compactJSONArray(node.SourceModels)
	row["source_uris"] = compactJSONArray(node.SourceURIs)
	row["response_content_type_state"] = emptyToNil(node.Response.ContentTypeState)
	row["response_odata_version_state"] = emptyToNil(node.Response.ODataVersionState)
	applyCommonInventory(row, node)
	applyRegisteredInventory(row, node)
	applyRegisteredStates(row, node)
	return row
}

func compactJSONArray(values []string) any {
	if len(values) == 0 {
		return nil
	}
	encoded, err := json.Marshal(mergeStringSets(values))
	if err != nil {
		return nil
	}
	return string(encoded)
}

func applyCommonInventory(row map[string]any, node *graphNode) {
	fields := []struct {
		column string
		path   string
		typ    registry.ColumnType
	}{
		{"id", "Id", registry.ColumnString},
		{"name", "Name", registry.ColumnString},
		{"description", "Description", registry.ColumnString},
		{"manufacturer", "Manufacturer", registry.ColumnString},
		{"model", "Model", registry.ColumnString},
		{"serial_number", "SerialNumber", registry.ColumnString},
		{"part_number", "PartNumber", registry.ColumnString},
		{"spare_part_number", "SparePartNumber", registry.ColumnString},
		{"sku", "SKU", registry.ColumnString},
		{"asset_tag", "AssetTag", registry.ColumnString},
		{"firmware_version", "FirmwareVersion", registry.ColumnString},
		{"uuid", "UUID", registry.ColumnString},
		{"hot_pluggable", "HotPluggable", registry.ColumnBoolean},
		{"replaceable", "Replaceable", registry.ColumnBoolean},
		{"ready_to_remove", "ReadyToRemove", registry.ColumnBoolean},
		{"location_indicator_active", "LocationIndicatorActive", registry.ColumnBoolean},
		{"location_service_label", "Location.PartLocation.ServiceLabel", registry.ColumnString},
		{"location_ordinal", "Location.PartLocation.LocationOrdinalValue", registry.ColumnInteger},
		{"location_type", "Location.PartLocation.LocationType", registry.ColumnEnum},
		{"location_orientation", "Location.PartLocation.Orientation", registry.ColumnEnum},
		{"location_reference", "Location.PartLocation.Reference", registry.ColumnEnum},
		{"location_part_context", "Location.PartLocationContext", registry.ColumnString},
		{"location_rack", "Location.Placement.Rack", registry.ColumnString},
		{"location_rack_offset", "Location.Placement.RackOffset", registry.ColumnInteger},
		{"location_rack_offset_units", "Location.Placement.RackOffsetUnits", registry.ColumnEnum},
		{"physical_context", "PhysicalContext", registry.ColumnEnum},
		{"physical_subcontext", "PhysicalSubContext", registry.ColumnEnum},
	}
	for _, field := range fields {
		if value, ok := inventoryValueAt(node.Data, field.path); ok {
			row[field.column] = normalizeInventoryValue(value, field.typ, false)
		}
	}
	if node.Kind == "drive" {
		for _, field := range fields {
			if !strings.HasPrefix(field.path, "Location.") {
				continue
			}
			if value, ok := inventoryValueAt(node.Data, "Physical"+field.path); ok {
				row[field.column] = normalizeInventoryValue(value, field.typ, false)
			}
		}
	}
	hardwareVersionPaths := map[string]string{
		"assembly": "Version", "battery": "Version", "chassis": "Version",
		"drive": "HardwareVersion", "manager": "Version", "power_supply": "Version",
		"processor": "Version", "pump": "Version",
	}
	if path := hardwareVersionPaths[node.Kind]; path != "" {
		if value, ok := inventoryValueAt(node.Data, path); ok {
			row["hardware_version"] = normalizeInventoryValue(value, registry.ColumnString, false)
		}
	}
}

func applyRegisteredInventory(row map[string]any, node *graphNode) {
	for _, field := range standardRegistry.Inventory {
		if field.Kind != registry.Kind(node.Kind) {
			continue
		}
		value, ok := inventoryValueAt(node.Data, field.Path)
		if !ok {
			continue
		}
		normalized := normalizeInventoryFieldValue(value, field)
		row[field.Column] = normalized
		if normalized == nil {
			markInventoryProtocolError(row)
		}
	}
}

func normalizeInventoryFieldValue(value any, field registry.InventoryFieldSpec) any {
	sourceType := field.SourceType
	if sourceType == "" {
		sourceType = field.Type
	}
	if sourceType == registry.ColumnFloat && field.Type == registry.ColumnInteger && !field.Structured {
		return scaleInventoryNumberToInteger(value, field.Scale)
	}
	return scaleInventoryValue(normalizeInventoryValue(value, sourceType, field.Structured), field.Scale)
}

func scaleInventoryNumberToInteger(value any, scale registry.Rational) any {
	if value == nil || scale.Num == 0 || scale.Den <= 0 {
		return nil
	}
	var text string
	switch current := value.(type) {
	case json.Number:
		text = current.String()
	case float64:
		if !isFinite(current) {
			return nil
		}
		text = strconv.FormatFloat(current, 'g', -1, 64)
	case float32:
		if !isFinite(float64(current)) {
			return nil
		}
		text = strconv.FormatFloat(float64(current), 'g', -1, 32)
	default:
		return nil
	}
	if !boundedProtocolNumber(text) {
		return nil
	}
	valueRat, ok := new(big.Rat).SetString(text)
	if !ok {
		return nil
	}
	valueRat.Mul(valueRat, new(big.Rat).SetFrac(big.NewInt(scale.Num), big.NewInt(scale.Den)))
	if !valueRat.IsInt() || !valueRat.Num().IsInt64() {
		return nil
	}
	return valueRat.Num().Int64()
}

func applyRegisteredStates(row map[string]any, node *graphNode) {
	for _, state := range standardRegistry.States {
		if state.Kind != registry.Kind(node.Kind) {
			continue
		}
		document := node.Data
		if state.Document != "" {
			document = findEnrichment(node, string(state.Document))
		}
		value, ok := registeredValueAt(document, state.Path)
		if !ok {
			continue
		}
		typ := registry.ColumnEnum
		if state.BooleanFalse != "" || state.BooleanTrue != "" {
			typ = registry.ColumnBoolean
		}
		normalized := normalizeInventoryValue(value, typ, false)
		row[state.Column] = normalized
		if normalized == nil {
			markInventoryProtocolError(row)
		}
	}
}

func markInventoryProtocolError(row map[string]any) {
	if row["error_class"] == nil {
		row["error_class"] = "protocol"
	}
}

func scaleInventoryValue(value any, scale registry.Rational) any {
	if value == nil || scale.Num == 0 || scale.Den == 0 {
		return value
	}
	switch current := value.(type) {
	case int64:
		numerator := new(big.Int).Mul(big.NewInt(current), big.NewInt(scale.Num))
		quotient, remainder := new(big.Int), new(big.Int)
		quotient.QuoRem(numerator, big.NewInt(scale.Den), remainder)
		if remainder.Sign() != 0 || !quotient.IsInt64() {
			return nil
		}
		return quotient.Int64()
	case float64:
		result := current * float64(scale.Num) / float64(scale.Den)
		if !isFinite(result) {
			return nil
		}
		return result
	default:
		return value
	}
}

func inventoryValueAt(data map[string]any, path string) (any, bool) {
	return registeredValueAt(data, path)
}

func registeredValueAt(data map[string]any, path string) (any, bool) {
	if data == nil || path == "" {
		return nil, false
	}
	const countAnnotation = ".@odata.count"
	if before, ok := strings.CutSuffix(path, countAnnotation); ok {
		propertyPath := before
		parent := data
		if index := strings.LastIndexByte(propertyPath, '.'); index >= 0 {
			value, ok := jsonPath(data, propertyPath[:index])
			if !ok {
				return nil, false
			}
			parent, ok = value.(map[string]any)
			if !ok {
				return nil, false
			}
			propertyPath = propertyPath[index+1:]
		}
		value, ok := parent[propertyPath+"@odata.count"]
		return value, ok
	}
	return jsonPath(data, path)
}

func normalizeInventoryValue(value any, typ registry.ColumnType, structured bool) any {
	if value == nil {
		return nil
	}
	if structured {
		switch value.(type) {
		case map[string]any, []any:
		default:
			return nil
		}
		raw, err := json.Marshal(value)
		if err != nil {
			return nil
		}
		return string(raw)
	}
	switch typ {
	case registry.ColumnString, registry.ColumnEnum:
		text, ok := value.(string)
		if !ok {
			return nil
		}
		return text
	case registry.ColumnBoolean:
		boolean, ok := value.(bool)
		if !ok {
			return nil
		}
		return boolean
	case registry.ColumnTimestamp:
		text, ok := value.(string)
		if !ok {
			return nil
		}
		parsed, err := time.Parse(time.RFC3339Nano, text)
		if err != nil {
			return nil
		}
		return parsed.UnixMilli()
	case registry.ColumnInteger:
		switch current := value.(type) {
		case json.Number:
			if !boundedProtocolNumber(current.String()) {
				return nil
			}
			if integer, err := current.Int64(); err == nil {
				return integer
			}
		case float64:
			if !isFinite(current) || current != math.Trunc(current) ||
				current < -float64(uint64(1)<<63) || current >= float64(uint64(1)<<63) {
				return nil
			}
			return int64(current)
		case float32:
			value := float64(current)
			if !isFinite(value) || value != math.Trunc(value) ||
				value < -float64(uint64(1)<<63) || value >= float64(uint64(1)<<63) {
				return nil
			}
			return int64(current)
		}
	case registry.ColumnFloat:
		if _, number, ok := numericValue(value); ok {
			return number
		}
	}
	return nil
}

func (c *protocolClient) inventoryReadingRow(
	node *graphNode,
	gate detailGate,
	observedAt time.Time,
	reading normalizedReading,
) map[string]any {
	row := c.inventoryGraphRow(node, gate, observedAt, []normalizedReading{reading})
	row["row_type"] = "reading"
	row["reading_key"] = reading.Key
	row["row_key"] = stableKey("netdata:redfish:inventory-row:v1", "reading\x00"+node.Key+"\x00"+reading.Key, 64)
	row["reading_source_path"] = reading.SourcePath
	row["reading_source_type"] = reading.SourceType
	row["reading_source_units"] = reading.SourceUnits
	row["reading_source_basis"] = reading.SourceBasis
	row["reading_type"] = reading.Family
	row["reading_units"] = reading.Units
	row["reading_basis"] = reading.Basis
	if reading.Valid {
		row["reading_source_value"] = reading.SourceValue
		row["reading_value"] = reading.Value
	} else {
		markInventoryProtocolError(row)
	}
	row["physical_context"] = emptyToNil(reading.PhysicalContext)
	row["physical_subcontext"] = emptyToNil(reading.PhysicalSubcontext)
	row["implementation_type"] = emptyToNil(reading.ImplementationType)
	row["source_alarm_state"] = emptyToNil(reading.SourceAlarm)
	row["derived_alarm_state"] = emptyToNil(reading.DerivedAlarm)
	row["effective_alarm_state"] = emptyToNil(reading.EffectiveAlarm)
	row["effective_alarm_source"] = emptyToNil(reading.EffectiveAlarmSource)
	row["effective_alarm_reason"] = emptyToNil(reading.EffectiveAlarmReason)
	if reading.RangeMin != nil {
		row["reading_range_min"] = *reading.RangeMin
	}
	if reading.RangeMax != nil {
		row["reading_range_max"] = *reading.RangeMax
	}
	maps.Copy(row, reading.Inventory)
	severity, rank := readingSeverity(row["severity"].(string), reading.EffectiveAlarm)
	row["severity"], row["severity_rank"] = severity, rank
	row["sort_key"] = fmt.Sprintf("%d\x00%s\x00%s\x00%s", rank, node.Kind, strings.ToLower(node.Doc.Name), row["row_key"])
	return row
}

func readingSeverity(resource, alarm string) (string, int) {
	switch alarm {
	case "critical", "emergency", "fault":
		return "critical", 0
	case "warning", "cap", "alarm":
		return "warning", 1
	}
	switch resource {
	case "critical":
		return "critical", 0
	case "warning":
		return "warning", 1
	default:
		return "normal", 2
	}
}
