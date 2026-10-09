// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package jetson

import (
	"math"
	"regexp"
	"strconv"
	"strings"
)

var (
	// A record starts with the RAM header, optionally after a timestamp, and has a
	// CPU array. Both remain present in records that omit GPU and EMC, so requiring
	// them keeps warnings and fragments from refreshing telemetry.
	reRecordHeader = regexp.MustCompile(
		`^(?:[0-9]+-[0-9]+-[0-9]+\s+[0-9]+:[0-9]+:[0-9]+\s+)?RAM\s+[0-9]+/[0-9]+MB\s+\(lfb\s+[0-9]+x[0-9]+MB\)`,
	)
	reRecordCPU    = regexp.MustCompile(`\sCPU\s+\[[^\]\r\n]+\](?:\s|@|$)`)
	reReadingKey   = regexp.MustCompile(`(?:^|\s)(GR3D_FREQ|EMC_FREQ)\b`)
	reReadingValue = regexp.MustCompile(`^\s+(\S*\[[^\]\r\n]*\]\S*|\S+)`)
)

// parseRecord parses one line of tegrastats output. It reports false for a line
// that is not a record; a record without supported readings yields an
// empty sample, which still replaces the previous one.
func parseRecord(line string) (sample, bool) {
	line = strings.TrimSpace(line)
	if !reRecordHeader.MatchString(line) || !reRecordCPU.MatchString(line) {
		return sample{}, false
	}

	var s sample
	keys := reReadingKey.FindAllStringSubmatchIndex(line, -1)
	for i, key := range keys {
		// Bound each value by the next key, so a missing value or a malformed
		// array cannot consume the other reading.
		end := len(line)
		if i+1 < len(keys) {
			end = keys[i+1][0]
		}
		value := reReadingValue.FindStringSubmatch(line[key[1]:end])
		if value == nil {
			continue
		}
		// A reading is "<util>%", "@<freq>" or "<util>%@<freq>".
		util, freq, _ := strings.Cut(value[1], "@")
		switch line[key[2]:key[3]] {
		case "GR3D_FREQ":
			s.GPUUtilization = parseUtilization(util)
			s.GPUFrequency, s.GPCFrequencies = parseGPUFrequency(freq)
		case "EMC_FREQ":
			s.EMCUtilization = parseUtilization(util)
			s.EMCFrequency = parseNonNegative(freq)
		}
	}
	s.PowerRails = parsePowerRails(line)
	return s, true
}

// parseGPUFrequency parses a scalar GPU clock or a per-GPC array "[f0,f1,...]".
func parseGPUFrequency(s string) (scalar *float64, gpcs []*float64) {
	inner, ok := strings.CutPrefix(s, "[")
	if !ok {
		return parseNonNegative(s), nil
	}
	if inner, ok = strings.CutSuffix(inner, "]"); !ok || strings.TrimSpace(inner) == "" {
		return nil, nil
	}
	for v := range strings.SplitSeq(inner, ",") {
		gpcs = append(gpcs, parseNonNegative(strings.TrimSpace(v)))
	}
	return nil, gpcs
}

// parseUtilization parses a "<percent>%" reading in the 0-100 range.
func parseUtilization(s string) *float64 {
	s, ok := strings.CutSuffix(s, "%")
	if !ok {
		return nil
	}
	if v := parseNonNegative(s); v != nil && *v <= 100 {
		return v
	}
	return nil
}

// parseNonNegative parses a finite, non-negative number.
func parseNonNegative(s string) *float64 {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
		return nil
	}
	return &v
}

// parsePowerRails reads instantaneous/average milliwatt pairs. Rail keys come
// from NVIDIA's documented families and the attributed legacy fixtures.
func parsePowerRails(line string) map[string]float64 {
	var rails map[string]float64
	seen := make(map[string]bool)
	var name string
	for value := range strings.FieldsSeq(line) {
		if isPowerRail(name) && strings.Contains(value, "/") {
			if seen[name] {
				// The name cannot identify two rails; choosing one would hide ambiguity.
				delete(rails, name)
			} else {
				seen[name] = true
				if watts := parsePower(value); watts != nil {
					if rails == nil {
						rails = make(map[string]float64)
					}
					rails[name] = *watts
				}
			}
		}
		name = value
	}
	if len(rails) == 0 {
		return nil
	}
	return rails
}

func isPowerRail(name string) bool {
	switch name {
	case "VIN", "GPU", "CPU", "SOC", "CV", "VDDRQ", "SYS5V":
		return true
	}
	for _, prefix := range []string{"VDD_", "VDDQ_", "VIN_", "POM_"} {
		suffix, ok := strings.CutPrefix(name, prefix)
		if !ok || suffix == "" {
			continue
		}
		for _, ch := range suffix {
			if !(ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '_') {
				return false
			}
		}
		return true
	}
	return false
}

// parsePower returns only the current value. The source's average window is
// unknown; unfamiliar units and three-value forms are deliberately unsupported.
func parsePower(value string) *float64 {
	current, average, ok := strings.Cut(value, "/")
	if !ok || average == "" || strings.Contains(average, "/") {
		return nil
	}
	current, currentUnit := strings.CutSuffix(current, "mW")
	average, averageUnit := strings.CutSuffix(average, "mW")
	if currentUnit != averageUnit || parseNonNegative(average) == nil {
		return nil
	}
	if power := parseNonNegative(current); power != nil {
		*power /= 1000
		return power
	}
	return nil
}
