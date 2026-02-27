// SPDX-License-Identifier: GPL-3.0-or-later

package output

import (
	"math"
	"strconv"
	"strings"
	"unicode"
)

// ParsedOutput represents the structured form of a Nagios plugin output.
type ParsedOutput struct {
	StatusLine string
	LongOutput string
	Perfdata   []PerfDatum
}

// PerfDatum represents a single perfdata entry.
type PerfDatum struct {
	Label string
	Unit  string
	Value float64
	Warn  *ThresholdRange
	Crit  *ThresholdRange
	Min   *float64
	Max   *float64
}

// ThresholdRange captures the Nagios range grammar semantics.
type ThresholdRange struct {
	Raw       string
	Inclusive bool
	Low       *float64
	High      *float64
}

// Defined reports whether the range field was present (non-empty and not U).
func (r *ThresholdRange) Defined() bool {
	return r != nil
}

// Parse converts raw plugin output into status, long output, and perfdata sections.
func Parse(raw []byte) ParsedOutput {
	text := strings.ReplaceAll(string(raw), "\r\n", "\n")
	text = strings.TrimSpace(text)
	if text == "" {
		return ParsedOutput{}
	}

	lines := strings.Split(text, "\n")
	var perfSections []string

	firstLine := lines[0]
	if idx := strings.Index(firstLine, "|"); idx >= 0 {
		perfSections = append(perfSections, firstLine[idx+1:])
		firstLine = firstLine[:idx]
	}

	status := strings.TrimSpace(firstLine)

	var longLines []string
	if len(lines) > 1 {
		for _, line := range lines[1:] {
			trimmed := line
			if idx := strings.Index(trimmed, "|"); idx >= 0 {
				perfSections = append(perfSections, trimmed[idx+1:])
				trimmed = trimmed[:idx]
			}
			longLines = append(longLines, strings.TrimRightFunc(trimmed, unicode.IsSpace))
		}
	}

	perfTokens := tokenizePerfdata(strings.Join(perfSections, " "))

	var perfdata []PerfDatum
	for _, token := range perfTokens {
		if datum, ok := parsePerfToken(token); ok {
			perfdata = append(perfdata, datum)
		}
	}

	longOutput := strings.Join(longLines, "\n")
	longOutput = strings.TrimRightFunc(longOutput, unicode.IsSpace)

	return ParsedOutput{
		StatusLine: status,
		LongOutput: longOutput,
		Perfdata:   perfdata,
	}
}

func tokenizePerfdata(s string) []string {
	var tokens []string
	var cur strings.Builder
	var quote rune

	flush := func() {
		if cur.Len() > 0 {
			tokens = append(tokens, cur.String())
			cur.Reset()
		}
	}

	for _, r := range s {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote = r
		case unicode.IsSpace(r):
			flush()
		default:
			cur.WriteRune(r)
		}
	}
	flush()

	return tokens
}

func parsePerfToken(token string) (PerfDatum, bool) {
	parts := strings.SplitN(token, "=", 2)
	if len(parts) != 2 {
		return PerfDatum{}, false
	}

	label := strings.Trim(parts[0], "\"'")
	if label == "" {
		return PerfDatum{}, false
	}

	fields := strings.SplitN(parts[1], ";", 5)
	valueStr := fields[0]
	warnStr, critStr, minStr, maxStr := getField(fields, 1), getField(fields, 2), getField(fields, 3), getField(fields, 4)

	val, unit, ok := parseValueUnit(valueStr)
	if !ok {
		return PerfDatum{}, false
	}

	datum := PerfDatum{
		Label: label,
		Unit:  unit,
		Value: val,
		Warn:  parseRange(warnStr),
		Crit:  parseRange(critStr),
		Min:   parseFloatPtr(minStr),
		Max:   parseFloatPtr(maxStr),
	}

	return datum, true
}

func getField(fields []string, idx int) string {
	if idx >= len(fields) {
		return ""
	}
	return fields[idx]
}

func parseValueUnit(value string) (float64, string, bool) {
	if value == "" {
		return 0, "", false
	}
	i := 0
	for i < len(value) {
		if isNumberChar(value[i]) {
			i++
			continue
		}
		break
	}
	numStr := value[:i]
	unit := value[i:]
	if numStr == "" {
		return 0, unit, false
	}
	v, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return 0, unit, false
	}
	return v, unit, true
}

func parseFloatPtr(val string) *float64 {
	val = strings.TrimSpace(val)
	if val == "" || strings.EqualFold(val, "u") {
		return nil
	}
	v, err := strconv.ParseFloat(val, 64)
	if err != nil {
		return nil
	}
	return &v
}

func parseRange(val string) *ThresholdRange {
	s := strings.TrimSpace(val)
	if s == "" || strings.EqualFold(s, "u") {
		return nil
	}
	rng := &ThresholdRange{Raw: s}
	if strings.HasPrefix(s, "@") {
		rng.Inclusive = true
		s = strings.TrimSpace(s[1:])
	}
	if s == "" {
		return nil
	}
	if strings.Contains(s, ":") {
		parts := strings.SplitN(s, ":", 2)
		start := strings.TrimSpace(parts[0])
		end := strings.TrimSpace(parts[1])
		if start == "" {
			zero := 0.0
			rng.Low = &zero
		} else if start != "~" {
			if v, ok := parseRangeNumber(start); ok {
				rng.Low = &v
			} else {
				return nil
			}
		}
		if end != "" && end != "~" {
			if v, ok := parseRangeNumber(end); ok {
				rng.High = &v
			} else {
				return nil
			}
		}
	} else {
		v, ok := parseRangeNumber(s)
		if !ok {
			return nil
		}
		zero := 0.0
		rng.Low = &zero
		rng.High = &v
	}
	return rng
}

func parseRangeNumber(val string) (float64, bool) {
	if val == "" {
		return 0, false
	}
	switch strings.ToLower(val) {
	case "inf", "+inf", "infinity":
		return math.Inf(1), true
	case "-inf":
		return math.Inf(-1), true
	}
	v, err := strconv.ParseFloat(val, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

func isNumberChar(b byte) bool {
	switch b {
	case '+', '-', '.', 'e', 'E':
		return true
	}
	return b >= '0' && b <= '9'
}
