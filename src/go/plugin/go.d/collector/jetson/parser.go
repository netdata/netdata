// SPDX-License-Identifier: GPL-3.0-or-later

package jetson

import (
	"math"
	"regexp"
	"strconv"
	"strings"
)

var (
	// RAM and CPU remain present in the captured records that omit GPU and EMC.
	// Requiring the record envelope prevents diagnostics from refreshing telemetry.
	recordHeader  = regexp.MustCompile(`^(?:[0-9]+-[0-9]+-[0-9]+\s+[0-9]+:[0-9]+:[0-9]+\s+)?RAM\s+[0-9]+/[0-9]+MB\s+\(lfb\s+[0-9]+x[0-9]+MB\)`)
	recordCPU     = regexp.MustCompile(`\sCPU\s+\[[^\]\r\n]+\](?:\s|@|$)`)
	recordMetrics = regexp.MustCompile(`(?:^|\s)(GR3D_FREQ|EMC_FREQ)\b`)
	recordValue   = regexp.MustCompile(`^\s+(\S*\[[^\]\r\n]*\]\S*|\S+)`)
)

// parseSample distinguishes a recognized record with unavailable measurements
// from unrelated output. The caller replaces its snapshot even for an empty record.
func parseSample(line string) (sample, bool) {
	line = strings.TrimSpace(line)
	if !recordHeader.MatchString(line) || !recordCPU.MatchString(line) {
		return sample{}, false
	}

	var s sample
	fields := recordMetrics.FindAllStringSubmatchIndex(line, -1)
	for i, field := range fields {
		// Bound each value by the next supported field, so a missing value or
		// malformed array cannot consume the other family's measurements.
		end := len(line)
		if i+1 < len(fields) {
			end = fields[i+1][0]
		}
		value := recordValue.FindStringSubmatch(line[field[1]:end])
		if value == nil {
			continue
		}
		utilization, frequency, hasFrequency := strings.Cut(value[1], "@")
		var util *float64
		if strings.HasSuffix(utilization, "%") {
			util = parseMeasurement(strings.TrimSuffix(utilization, "%"), true)
		}
		switch line[field[2]:field[3]] {
		case "EMC_FREQ":
			s.EMCUtilization = util
			if hasFrequency {
				s.EMCFrequency = parseMeasurement(frequency, false)
			}
		case "GR3D_FREQ":
			s.GPUUtilization = util
			if !hasFrequency {
				continue
			}
			if strings.HasPrefix(frequency, "[") && strings.HasSuffix(frequency, "]") {
				values := strings.TrimSpace(frequency[1 : len(frequency)-1])
				if values == "" {
					continue
				}
				for _, value := range strings.Split(values, ",") {
					s.GPCFrequencies = append(s.GPCFrequencies, parseMeasurement(strings.TrimSpace(value), false))
				}
			} else {
				s.GPUFrequency = parseMeasurement(frequency, false)
			}
		}
	}
	return s, true
}

func parseMeasurement(raw string, percent bool) *float64 {
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || (percent && value > 100) {
		return nil
	}
	return &value
}
