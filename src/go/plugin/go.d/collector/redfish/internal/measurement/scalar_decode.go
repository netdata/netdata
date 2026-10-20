// SPDX-License-Identifier: GPL-3.0-or-later

package measurement

import (
	"math"
	"math/big"
	"regexp"
	"strconv"
	"strings"
)

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

const (
	maxProtocolNumericTokenBytes  = 128
	maxProtocolDurationTokenBytes = 640
)

var redfishDurationPattern = regexp.MustCompile(
	`^P(?:(\d+(?:\.\d+)?)D)?(?:T(?:(\d+(?:\.\d+)?)H)?(?:(\d+(?:\.\d+)?)M)?(?:(\d+(?:\.\d+)?)S)?)?$`,
)

// Complete means numeric decoding and multiplier selection succeeded. A
// non-finite scaled value still ends fallback selection, matching source priority.
type scalarCandidate struct {
	Present    bool
	Complete   bool
	Value      float64
	Exact      string
	Multiplier float64
	Epoch      string
}

func decodeScalarCandidate(node *Resource, descriptor sourceField, source scalarSource) (scalarCandidate, string) {
	document := node.Data
	if source.Document != "" {
		document = findEnrichment(node, string(source.Document))
	}
	if document == nil {
		return scalarCandidate{}, ""
	}
	if !sourceRequirementsMatch(document, source.Requires) {
		return scalarCandidate{}, ""
	}
	raw, present := registeredValueAt(document, source.Path)
	if !present {
		return scalarCandidate{}, ""
	}
	candidate := scalarCandidate{
		Present: true,
	}
	if raw == nil {
		return candidate, "property null"
	}
	exact, value, valid := numericSourceValue(raw, descriptor.Algorithm)
	if !valid {
		return candidate, "value malformed, unsupported, or non-finite"
	}
	multiplier, valid := scalarMultiplier(node, document, descriptor, source)
	if !valid {
		return candidate, "normalization multiplier absent or invalid"
	}
	candidate.Complete = true
	candidate.Exact = exact
	candidate.Multiplier = multiplier
	candidate.Value = value * multiplier
	if !isFinite(candidate.Value) {
		return candidate, "normalized value non-finite"
	}
	if descriptor.Algorithm != algorithmAbsolute {
		candidate.Epoch = rateEpoch(document)
	}
	return candidate, ""
}

func scalarMultiplier(
	node *Resource,
	document map[string]any,
	descriptor sourceField,
	source scalarSource,
) (float64, bool) {
	scale := descriptor.Scale
	if source.Scale.Den != 0 {
		scale = source.Scale
	}
	multiplier := float64(scale.Num) / float64(scale.Den)
	if source.MultiplierPath == "" {
		return multiplier, true
	}
	if source.MultiplierDocument != "" {
		document = findEnrichment(node, string(source.MultiplierDocument))
	}
	raw, present := registeredValueAt(document, source.MultiplierPath)
	_, sourceMultiplier, valid := numericValue(raw)
	if !present || !valid || sourceMultiplier <= 0 {
		return 0, false
	}
	scale = source.MultiplierScale
	if scale.Den == 0 {
		scale = identityScale
	}
	return multiplier * (sourceMultiplier * float64(scale.Num) / float64(scale.Den)), true
}
