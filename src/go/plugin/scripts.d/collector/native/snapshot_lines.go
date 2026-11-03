// SPDX-License-Identifier: GPL-3.0-or-later

package native

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// decodeLines groups scalar records into the same model as JSON. Metadata is
// merged before validation applies defaults; nothing carries over from a prior
// snapshot. Cost follows input bytes and samples, with one index per family.
func decodeLines(data []byte) (snapshot, error) {
	result := snapshot{
		Version: "v1",
	}
	if !utf8.Valid(data) {
		return result, errors.New("invalid UTF-8 in line snapshot")
	}
	indices := make(map[string]int)
	observed := false
	lineNumber := 0
	for raw := range bytes.SplitSeq(data, []byte{'\n'}) {
		lineNumber++
		line := string(bytes.Trim(raw, " \t\r"))
		if line == "" {
			continue
		}
		observed = true
		if strings.HasPrefix(line, "#") {
			continue
		}
		family, err := parseMetricLine(line)
		if err != nil {
			return result, fmt.Errorf("line %d: %w", lineNumber, err)
		}
		index, exists := indices[family.Name]
		if !exists {
			indices[family.Name] = len(result.Metrics)
			result.Metrics = append(result.Metrics, family)
			continue
		}
		current := &result.Metrics[index]
		if !mergeLineContract(&current.metricContract, family.metricContract) {
			return result, fmt.Errorf("line %d: conflicting metric metadata", lineNumber)
		}
		current.Samples = append(current.Samples, family.Samples[0])
	}
	if !observed {
		return result, errors.New("empty line snapshot; use a comment for an empty observation")
	}
	return result, result.validate()
}

// Empty metadata is rejected by the line grammar, so zero values mean absent.
func mergeLineContract(dst *metricContract, src metricContract) bool {
	if dst.Type != src.Type {
		return false
	}
	for _, pair := range []struct {
		dst *string
		src string
	}{
		{&dst.Unit, src.Unit}, {&dst.ChartMeta.Title, src.ChartMeta.Title}, {&dst.ChartMeta.Family, src.ChartMeta.Family},
	} {
		if *pair.dst != "" && pair.src != "" && *pair.dst != pair.src {
			return false
		}
		if pair.src != "" {
			*pair.dst = pair.src
		}
	}
	if src.ChartMeta.Priority != nil {
		if dst.ChartMeta.Priority != nil && *dst.ChartMeta.Priority != *src.ChartMeta.Priority {
			return false
		}
		dst.ChartMeta.Priority = src.ChartMeta.Priority
	}
	return true
}

func parseMetricLine(line string) (metricFamily, error) {
	var family metricFamily
	for _, c := range line {
		if c < 0x20 || c == 0x7f {
			return family, errors.New("raw control character in metric line")
		}
	}
	p := lineParser{
		rest: line,
	}
	name, ok := p.until(':')
	if !ok || !reMetricName.MatchString(name) {
		return family, errors.New("invalid metric name")
	}
	family.Name = strings.Clone(name)
	number, ok := p.until('|')
	value, err := strconv.ParseFloat(number, 64)
	if !ok || err != nil || strings.TrimSpace(number) != number || !json.Valid([]byte(number)) {
		return family, errors.New("value must be a finite JSON number")
	}
	kind, rest, _ := strings.Cut(p.rest, "|")
	p.rest = rest
	if kind != metricGauge && kind != metricCounter {
		return family, errors.New("line type must be gauge or counter")
	}
	family.Type = strings.Clone(kind)
	sample := metricSample{
		Value: &value,
	}
	var seen uint8
	// A trailing separator is an empty field, including immediately after type.
	if strings.HasSuffix(line, "|") {
		return family, errors.New("empty line field")
	}
	for p.rest != "" {
		var field uint8
		if p.rest[0] == '#' {
			field = 1
			p.rest = p.rest[1:]
			sample.Labels, err = p.labels()
		} else {
			key, found := p.until(':')
			if !found {
				return family, errors.New("expected metadata field")
			}
			text, valueErr := p.value(false)
			if valueErr != nil || text == "" {
				return family, errors.New("metadata requires a nonempty value")
			}
			switch key {
			case "unit":
				field, family.Unit = 2, text
			case "title":
				field, family.ChartMeta.Title = 4, text
			case "family":
				field, family.ChartMeta.Family = 8, text
			case "priority":
				field = 16
				priority, parseErr := strconv.Atoi(text)
				if parseErr != nil || priority <= 0 || strconv.Itoa(priority) != text {
					return family, errors.New("priority must be a positive decimal integer")
				}
				family.ChartMeta.Priority = &priority
			default:
				return family, errors.New("unknown line field")
			}
		}
		if err != nil {
			return family, err
		}
		if seen&field != 0 {
			return family, errors.New("duplicate line field")
		}
		seen |= field
		if p.rest != "" {
			p.rest = p.rest[1:] // value/labels leave the separating pipe
		}
	}
	family.Samples = []metricSample{sample}
	return family, nil
}

type lineParser struct{ rest string }

// until consumes a required delimiter; callers validate the preceding token.
func (p *lineParser) until(delimiter byte) (string, bool) {
	i := strings.IndexByte(p.rest, delimiter)
	if i < 0 {
		return "", false
	}
	token := p.rest[:i]
	p.rest = p.rest[i+1:]
	return token, true
}

// value leaves the delimiter unconsumed. Quoted values use ordinary JSON string
// escaping, including Unicode escapes, exactly as the JSON snapshot encoding.
func (p *lineParser) value(label bool) (string, error) {
	delimiters := "|"
	if label {
		delimiters = ",|"
	}
	if p.rest == "" {
		return "", errors.New("missing field value")
	}
	if p.rest[0] != '"' {
		i := strings.IndexAny(p.rest, delimiters)
		if i < 0 {
			i = len(p.rest)
		}
		value := p.rest[:i]
		p.rest = p.rest[i:]
		if value == "" || strings.TrimSpace(value) != value {
			return "", errors.New("unquoted value must be nonempty without surrounding whitespace")
		}
		return strings.Clone(value), nil
	}
	for i := 1; i < len(p.rest); i++ {
		switch p.rest[i] {
		case '\\':
			i++
		case '"':
			var value string
			if json.Unmarshal([]byte(p.rest[:i+1]), &value) != nil {
				return "", errors.New("invalid JSON string")
			}
			p.rest = p.rest[i+1:]
			if p.rest != "" && !strings.ContainsRune(delimiters, rune(p.rest[0])) {
				return "", errors.New("expected delimiter after quoted value")
			}
			return value, nil
		}
	}
	return "", errors.New("unterminated quoted value")
}

func (p *lineParser) labels() (map[string]string, error) {
	labels := make(map[string]string)
	for {
		key, ok := p.until(':')
		if !ok || !reIdentifier.MatchString(key) {
			return nil, errors.New("invalid label key")
		}
		if _, exists := labels[key]; exists {
			return nil, errors.New("duplicate label key")
		}
		value, err := p.value(true)
		if err != nil {
			return nil, err
		}
		labels[strings.Clone(key)] = value
		if p.rest == "" || p.rest[0] == '|' {
			return labels, nil
		}
		p.rest = p.rest[1:] // comma requires another label
	}
}
