// SPDX-License-Identifier: GPL-3.0-or-later

package weblog

import (
	"errors"
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/logs"
)

type pattern struct {
	name string
	matcher.Matcher
}

func newPattern(up userPattern) (*pattern, error) {
	if up.Name == "" || up.Match == "" {
		return nil, errors.New("empty 'name' or 'match'")
	}

	m, err := matcher.Parse(up.Match)
	if err != nil {
		return nil, err
	}
	return &pattern{name: up.Name, Matcher: m}, nil
}

func (w *WebLog) createURLPatterns() error {
	if len(w.URLPatterns) == 0 {
		w.Debug("skipping URL patterns creating, no patterns provided")
		return nil
	}
	w.Debug("starting URL patterns creating")
	for _, up := range w.URLPatterns {
		p, err := newPattern(up)
		if err != nil {
			return fmt.Errorf("create pattern %+v: %v", up, err)
		}
		w.Debugf("created pattern '%s', type '%T', match '%s'", p.name, p.Matcher, up.Match)
		w.urlPatterns = append(w.urlPatterns, p)
	}
	w.Debugf("created %d URL pattern(s)", len(w.URLPatterns))
	return nil
}

func (w *WebLog) createCustomFields() error {
	if len(w.CustomFields) == 0 {
		w.Debug("skipping custom fields creating, no custom fields provided")
		return nil
	}

	w.Debug("starting custom fields creating")
	w.customFields = make(map[string][]*pattern)
	for i, cf := range w.CustomFields {
		if cf.Name == "" {
			return fmt.Errorf("create custom field: name not set (field %d)", i+1)
		}
		for _, up := range cf.Patterns {
			p, err := newPattern(up)
			if err != nil {
				return fmt.Errorf("create field '%s' pattern %+v: %v", cf.Name, up, err)
			}
			w.Debugf("created field '%s', pattern '%s', type '%T', match '%s'", cf.Name, p.name, p.Matcher, up.Match)
			w.customFields[cf.Name] = append(w.customFields[cf.Name], p)
		}
	}
	w.Debugf("created %d custom field(s)", len(w.CustomFields))
	return nil
}

func (w *WebLog) createCustomTimeFields() error {
	if len(w.CustomTimeFields) == 0 {
		w.Debug("skipping custom time fields creating, no custom time fields provided")
		return nil
	}

	w.Debug("starting custom time fields creating")
	w.customTimeFields = make(map[string][]float64)
	for i, ctf := range w.CustomTimeFields {
		if ctf.Name == "" {
			return fmt.Errorf("create custom field: name not set (field %d)", i+1)
		}
		w.customTimeFields[ctf.Name] = ctf.Histogram
		w.Debugf("created time field '%s', histogram '%v'", ctf.Name, ctf.Histogram)
	}
	w.Debugf("created %d custom time field(s)", len(w.CustomTimeFields))
	return nil
}

func (w *WebLog) createCustomNumericFields() error {
	if len(w.CustomNumericFields) == 0 {
		w.Debug("no custom time fields provided")
		return nil
	}

	w.Debugf("creating custom numeric fields for '%+v'", w.CustomNumericFields)

	w.customNumericFields = make(map[string]bool)

	for i := range w.CustomNumericFields {
		v := w.CustomNumericFields[i]
		if v.Name == "" {
			return fmt.Errorf("custom numeric field (%d): 'name' not set", i+1)
		}
		if v.Units == "" {
			return fmt.Errorf("custom numeric field (%s): 'units' not set", v.Name)
		}
		if v.Multiplier <= 0 {
			v.Multiplier = 1
		}
		if v.Divisor <= 0 {
			v.Divisor = 1
		}
		w.CustomNumericFields[i] = v
		w.customNumericFields[v.Name] = true
	}

	return nil
}

func (w *WebLog) createLogLine() {
	w.line = newEmptyLogLine()

	for v := range w.customFields {
		w.line.custom.fields[v] = struct{}{}
	}
	for v := range w.customTimeFields {
		w.line.custom.fields[v] = struct{}{}
	}
	for v := range w.customNumericFields {
		w.line.custom.fields[v] = struct{}{}
	}
}

func (w *WebLog) createLogReader() error {
	w.Cleanup()
	w.Debug("starting log reader creating")

	reader, err := logs.Open(w.Path, w.ExcludePath, w.Logger)
	if err != nil {
		return fmt.Errorf("creating log reader: %v", err)
	}

	w.Debugf("created log reader, current file '%s'", reader.CurrentFilename())
	w.file = reader

	return nil
}

func (w *WebLog) createParser() error {
	w.Debug("starting parser creating")

	const readLinesNum = 100

	lines, err := logs.ReadLastLines(w.file.CurrentFilename(), readLinesNum)
	if err != nil {
		return fmt.Errorf("failed to read last lines: %v", err)
	}

	var found bool
	for _, line := range lines {
		if line = strings.TrimSpace(line); line == "" {
			continue
		}
		w.Debugf("last line: '%s'", line)

		w.parser, err = w.newParser([]byte(line))
		if err != nil {
			w.Debugf("failed to create parser from line: %v", err)
			continue
		}

		w.line.reset()

		if err = w.parser.Parse([]byte(line), w.line); err != nil {
			w.Debugf("failed to parse line: %v", err)
			continue
		}

		if err = w.line.verify(); err != nil {
			w.Debugf("failed to verify line: %v", err)
			continue
		}

		found = true
		break
	}

	if !found {
		return fmt.Errorf("failed to create log parser (file '%s')", w.file.CurrentFilename())
	}

	return nil
}
