// SPDX-License-Identifier: GPL-3.0-or-later

package measurement

import (
	"regexp"
	"sort"
	"strings"
	"time"
)

type scalarValue struct {
	Descriptor     sourceField
	Value          float64
	SourceFailures []string
	Present        bool
	Valid          bool
	Emit           bool
}

// scalarValues selects candidates in source priority order. Invalid present
// sources retain their diagnostics until a fully decoded fallback is selected.
func (c *Projector) scalarValues(node *Resource, at time.Time) []scalarValue {
	result := make([]scalarValue, 0)
	for _, descriptor := range scalarFieldsByKind[node.Kind] {
		if descriptor.ID == managerClockDescriptor.ID {
			// DateTime is measured against the request midpoint, not as a numeric field.
			continue
		}
		var selected scalarValue
		var sourceFailures []string
		for _, source := range descriptor.Candidates {
			decoded, failure := decodeScalarCandidate(node, descriptor, source)
			if failure != "" {
				sourceFailures = append(sourceFailures, scalarSourceFailure(descriptor, sourcePath(source), failure))
			}
			if !decoded.Present {
				continue
			}
			candidate := scalarValue{
				Descriptor: descriptor,
				Present:    true,
				Value:      decoded.Value,
				Valid:      decoded.Complete && failure == "",
			}
			if !decoded.Complete {
				if !selected.Present {
					selected = candidate
				}
				continue
			}
			candidate.Emit = candidate.Valid && descriptor.Algorithm == algorithmAbsolute
			if descriptor.Algorithm != algorithmAbsolute && candidate.Valid {
				candidate.Value, candidate.Emit = c.rateValue(
					node.Key+"\x00"+descriptor.ID, decoded.Exact, decoded.Multiplier, at,
					descriptor.Algorithm, sourcePath(source)+"\x00"+decoded.Epoch,
				)
			}
			selected = candidate
			break
		}
		if selected.Present {
			selected.SourceFailures = sourceFailures
			result = append(result, selected)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Descriptor.Metric < result[j].Descriptor.Metric })
	return result
}

func scalarSourceFailure(descriptor sourceField, source, reason string) string {
	return "Redfish compatibility: scalar " + descriptor.ID + " preferred source " + source + ": " + reason
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

func managerClockValue(node *Resource) (scalarValue, bool, string) {
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

func findEnrichment(node *Resource, kind string) map[string]any {
	var match map[string]any
	found := false
	for key, value := range node.Enrichment {
		if strings.HasPrefix(key, kind+":") || key == kind {
			if found {
				return nil
			}
			match = value.Data
			found = true
		}
	}
	return match
}

func stringValueAt(data map[string]any, path string) (string, bool) {
	value, ok := Properties(data).Lookup(path)
	if !ok {
		return "", false
	}
	return stringValue(value)
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
			value, ok := Properties(data).Lookup(propertyPath[:index])
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
	return Properties(data).Lookup(path)
}

var managerDateTimeOffsetPattern = regexp.MustCompile(`[+-][0-9]{2}:[0-9]{2}$`)

var managerClockDescriptor = func() sourceField {
	for _, descriptor := range scalarFields {
		if descriptor.ID == "manager_datetime_clock_offset" {
			return descriptor
		}
	}
	panic("manager clock descriptor is missing")
}()
