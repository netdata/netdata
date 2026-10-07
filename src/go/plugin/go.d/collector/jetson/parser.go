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
// that is not a record; a record without valid GPU or EMC readings yields an
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
