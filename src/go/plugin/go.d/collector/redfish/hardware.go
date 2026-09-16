// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"fmt"
	"math"
	"math/big"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

const promotedLabelLimit = 256

var managerDateTimeOffsetPattern = regexp.MustCompile(`[+-][0-9]{2}:[0-9]{2}$`)

type hardwareObservation struct {
	Metric string
	Value  float64
	State  string
	States []string
	Labels []metrix.Label
}

type hardwareMetrics struct {
	meter  metrix.SnapshotMeter
	gauges map[string]metrix.SnapshotGauge
	states map[string]metrix.StateSetInstrument
}

func newHardwareMetrics(store metrix.CollectorStore) *hardwareMetrics {
	meter := store.Write().SnapshotMeter("")
	result := &hardwareMetrics{meter: meter, gauges: make(map[string]metrix.SnapshotGauge), states: make(map[string]metrix.StateSetInstrument)}
	gauge := func(metric string) {
		if _, exists := result.gauges[metric]; !exists {
			result.gauges[metric] = meter.Gauge(metric)
		}
	}
	states := func(metric string, values []string) {
		if _, exists := result.states[metric]; !exists {
			result.states[metric] = meter.StateSet(metric, metrix.WithStateSetMode(metrix.ModeEnum), metrix.WithStateSetStates(values...))
		}
	}
	for _, field := range scalarFields {
		gauge(field.Metric)
	}
	for _, reading := range readingDescriptors {
		gauge(reading.Metric)
		if reading.AlarmMetric != "" {
			states(reading.AlarmMetric, alarmStates)
		}
	}
	for kind, status := range sourceStatusByKind {
		states(kind+"_acquisition_state", acquisitionStates)
		if status.Status {
			states(kind+"_health", healthStates)
			states(kind+"_health_rollup", healthStates)
			states(kind+"_state", resourceStates)
			for _, state := range healthStates {
				gauge(kind + "_conditions_" + state)
			}
		}
		if status.PowerState {
			states(kind+"_power_state", powerStates)
		}
		if status.FailurePredicted {
			states(kind+"_failure_predicted", failureStates)
		}
	}
	for _, source := range additionalStateSources {
		states(source.Metric, source.States)
	}
	for _, set := range sourceFlagSets {
		for _, member := range set.Members {
			gauge(set.Metric + "_" + member.Role)
		}
	}
	return result
}

func (m *hardwareMetrics) observe(observations []hardwareObservation) {
	for _, observation := range observations {
		labels := m.meter.LabelSet(observation.Labels...)
		if observation.State != "" {
			m.states[observation.Metric].ObserveStateSet(metrix.StateSetPoint{States: map[string]bool{observation.State: true}}, labels)
		} else {
			m.gauges[observation.Metric].Observe(observation.Value, labels)
		}
	}
}

const (
	maxProtocolNumericTokenBytes  = 128
	maxProtocolDurationTokenBytes = 640
)

var managerClockDescriptor = func() sourceField {
	for _, descriptor := range scalarFields {
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

func (c *protocolClient) hardwareSurface(graph *resourceGraph, observedAt time.Time) ([]hardwareObservation, error) {
	nodes := graph.emittedNodes()
	if graph.Complete {
		c.pruneRateBaselines(nodes)
	}
	readings := make(map[string][]normalizedReading, len(nodes))
	for _, node := range nodes {
		readings[node.Key] = c.readingsForNode(node, observedAt)
	}
	if err := c.validateAndRegisterReadingIdentities(readings); err != nil {
		return nil, err
	}
	var observations []hardwareObservation
	for _, node := range nodes {
		values := c.scalarValues(node, observedAt)
		if value, present, diagnostic := managerClockValue(node); present {
			if diagnostic != "" {
				graph.addDiagnostic(fmt.Sprintf("manager %s clock offset: %s", node.URI, diagnostic))
			}
			if value.Valid {
				values = append(values, value)
			}
		}
		observations = append(observations, c.statusObservations(node)...)
		labels := c.metricLabels(node, nil)
		for _, value := range values {
			for _, failure := range value.SourceFailures {
				graph.addDiagnostic(failure)
			}
			if value.Emit {
				observations = append(observations, hardwareObservation{Metric: value.Descriptor.Metric, Value: value.Value, Labels: labels})
			}
		}
		observations = append(observations, c.flagObservations(node, flagValues(node))...)
		for _, reading := range readings[node.Key] {
			graph.addDiagnostic(reading.SourceAlarmDiagnostic)
			if reading.Valid || reading.SourceAlarm != "" {
				observations = append(observations, c.readingObservations(node, reading)...)
			}
		}
	}
	return observations, nil
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
	Set     flagSet
	Member  flagMember
	Value   bool
	Present bool
	Emit    bool
}

func flagValues(node *graphNode) []flagValue {
	var result []flagValue
	for _, set := range sourceFlagSets {
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
				Present: present && valid, Emit: present && valid,
			})
		}
		result = append(result, values...)
	}
	return result
}

func (c *protocolClient) flagObservations(node *graphNode, values []flagValue) []hardwareObservation {
	labels := c.metricLabels(node, nil)
	result := make([]hardwareObservation, 0, len(values))
	for _, value := range values {
		if !value.Emit || !value.Present {
			continue
		}
		result = append(result, hardwareObservation{
			Metric: value.Set.Metric + "_" + value.Member.Role,
			Value:  boolFloat(value.Value),
			Labels: labels,
		})
	}
	return result
}

type scalarValue struct {
	Descriptor     sourceField
	Value          float64
	SourceFailures []string
	Present        bool
	Valid          bool
	Emit           bool
}

func (c *protocolClient) scalarValues(node *graphNode, at time.Time) []scalarValue {
	result := make([]scalarValue, 0)
	for _, descriptor := range scalarFieldsByKind[node.Kind] {
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
				Descriptor: descriptor,
				Present:    true,
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
					scale = identityScale
				}
				multiplier *= sourceMultiplier * float64(scale.Num) / float64(scale.Den)
			}
			normalized := value * multiplier
			candidate.Value = normalized

			candidate.Valid = isFinite(normalized)
			if !candidate.Valid {
				sourceFailures = append(sourceFailures, scalarSourceFailure(descriptor, sourceName, "normalized value non-finite"))
			}
			candidate.SourceFailures = slices.Clone(sourceFailures)
			candidate.Emit = candidate.Valid && descriptor.Algorithm == algorithmAbsolute
			if descriptor.Algorithm != algorithmAbsolute && candidate.Valid {
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

func scalarSourceFailure(descriptor sourceField, source, reason string) string {
	return fmt.Sprintf(
		"Redfish compatibility: scalar %s preferred source %s: %s",
		descriptor.ID,
		source,
		reason,
	)
}

func sourceRequirementsMatch(document map[string]any, requirements []sourceRequirement) bool {
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
		Present:    true,
		Valid:      true,
		Emit:       true,
	}, true, ""
}

func sourcePath(source scalarSource) string {
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
	algorithm scalarAlgorithm,
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
	if algorithm == algorithmDurationPercent {
		value *= 100
		if value < 0 || value > 100 {
			return 0, false
		}
	}
	return value, isFinite(value)
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

func numericSourceValue(value any, algorithm scalarAlgorithm) (string, float64, bool) {
	if algorithm != algorithmDurationPercent {
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

func (c *protocolClient) statusObservations(node *graphNode) []hardwareObservation {
	labels := c.metricLabels(node, nil)
	prefix := strings.ReplaceAll(node.Kind, "-", "_")
	var result []hardwareObservation
	status := statusForKind(node.Kind)
	if node.AcquisitionState != "" {
		result = append(result, hardwareObservation{
			Metric: prefix + "_acquisition_state",
			State:  normalizedEnum(node.AcquisitionState, acquisitionStates),
			States: acquisitionStates,
			Labels: labels,
		})
	}
	if status.Status {
		if state, present, _ := categoricalStringState(node.Data, "Status.Health", normalizeHealth); present {
			result = append(result, stateObservation(prefix+"_health", state, healthStates, labels))
		}
		if state, present, _ := categoricalStringState(node.Data, "Status.HealthRollup", normalizeHealth); present {
			result = append(result, stateObservation(prefix+"_health_rollup", state, healthStates, labels))
		}
		if state, present, _ := categoricalStringState(node.Data, "Status.State", normalizeResourceState); present {
			result = append(result, stateObservation(prefix+"_state", state, resourceStates, labels))
		}
	}
	if status.PowerState {
		if state, present, _ := categoricalStringState(node.Data, "PowerState", func(value string) string {
			return normalizedEnum(value, powerStates)
		}); present {
			result = append(result, stateObservation(prefix+"_power_state", state, powerStates, labels))
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
			result = append(result, stateObservation(prefix+"_failure_predicted", state, failureStates, labels))
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
				})
			}
		}
	}
	result = append(result, c.additionalStateObservations(node, labels)...)
	return result
}

func statusForKind(kind string) statusDescriptor { return sourceStatusByKind[kind] }

func stateObservation(
	metric, state string,
	states []string,
	labels []metrix.Label,
) hardwareObservation {
	return hardwareObservation{Metric: metric, State: state, States: states, Labels: labels}
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

func (c *protocolClient) additionalStateObservations(
	node *graphNode,
	labels []metrix.Label,
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
		result = append(result, stateObservation(source.Metric, state, source.States, labels))
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
	addLabel("component_family", componentFamilies[node.Kind])
	addMetricResourceLabels(addLabel, node)
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

func addMetricResourceLabels(add func(string, string), node *graphNode) {
	if node == nil || node.Data == nil {
		return
	}
	addPath := func(label string, paths ...string) {
		for _, path := range paths {
			value, ok := registeredValueAt(node.Data, path)
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

func (g *resourceGraph) findKey(key string) *graphNode {
	g.ensureLookupIndexes()
	return g.ByKey[key]
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
	rateMu        sync.Mutex
	rateBaselines map[string]rateBaseline
}

func (s *hardwareState) initialize() {
	if s.rateBaselines == nil {
		s.rateBaselines = make(map[string]rateBaseline)
	}
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
