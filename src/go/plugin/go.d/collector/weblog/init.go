// SPDX-License-Identifier: GPL-3.0-or-later

package weblog

import (
	"context"
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

func (c *Collector) createURLPatterns() error {
	if len(c.URLPatterns) == 0 {
		c.Debug("skipping URL patterns creating, no patterns provided")
		return nil
	}
	c.Debug("starting URL patterns creating")
	for _, up := range c.URLPatterns {
		p, err := newPattern(up)
		if err != nil {
			return fmt.Errorf("create pattern %+v: %v", up, err)
		}
		c.Debugf("created pattern '%s', type '%T', match '%s'", p.name, p.Matcher, up.Match)
		c.urlPatterns = append(c.urlPatterns, p)
	}
	c.Debugf("created %d URL pattern(s)", len(c.URLPatterns))
	return nil
}

func (c *Collector) createCustomFields() error {
	if len(c.CustomFields) == 0 {
		c.Debug("skipping custom fields creating, no custom fields provided")
		return nil
	}

	c.Debug("starting custom fields creating")
	c.customFields = make(map[string][]*pattern)
	for i, cf := range c.CustomFields {
		if cf.Name == "" {
			return fmt.Errorf("create custom field: name not set (field %d)", i+1)
		}
		for _, up := range cf.Patterns {
			p, err := newPattern(up)
			if err != nil {
				return fmt.Errorf("create field '%s' pattern %+v: %v", cf.Name, up, err)
			}
			c.Debugf("created field '%s', pattern '%s', type '%T', match '%s'", cf.Name, p.name, p.Matcher, up.Match)
			c.customFields[cf.Name] = append(c.customFields[cf.Name], p)
		}
	}
	c.Debugf("created %d custom field(s)", len(c.CustomFields))
	return nil
}

func (c *Collector) createCustomTimeFields() error {
	if len(c.CustomTimeFields) == 0 {
		c.Debug("skipping custom time fields creating, no custom time fields provided")
		return nil
	}

	c.Debug("starting custom time fields creating")
	c.customTimeFields = make(map[string][]float64)
	for i, ctf := range c.CustomTimeFields {
		if ctf.Name == "" {
			return fmt.Errorf("create custom field: name not set (field %d)", i+1)
		}
		c.customTimeFields[ctf.Name] = ctf.Histogram
		c.Debugf("created time field '%s', histogram '%v'", ctf.Name, ctf.Histogram)
	}
	c.Debugf("created %d custom time field(s)", len(c.CustomTimeFields))
	return nil
}

func (c *Collector) createCustomNumericFields() error {
	if len(c.CustomNumericFields) == 0 {
		c.Debug("no custom time fields provided")
		return nil
	}

	c.Debugf("creating custom numeric fields for '%+v'", c.CustomNumericFields)

	c.customNumericFields = make(map[string]bool)

	for i := range c.CustomNumericFields {
		v := c.CustomNumericFields[i]
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
		c.CustomNumericFields[i] = v
		c.customNumericFields[v.Name] = true
	}

	return nil
}

func (c *Collector) createLogLine() {
	c.line = newEmptyLogLine()

	for v := range c.customFields {
		c.line.custom.fields[v] = struct{}{}
	}
	for v := range c.customTimeFields {
		c.line.custom.fields[v] = struct{}{}
	}
	for v := range c.customNumericFields {
		c.line.custom.fields[v] = struct{}{}
	}
}

func (c *Collector) createLogReader() error {
	c.Cleanup(context.Background())
	c.Debug("starting log reader creating")

	reader, err := logs.Open(c.Path, c.ExcludePath, c.Logger)
	if err != nil {
		return fmt.Errorf("creating log reader: %v", err)
	}

	c.Debugf("created log reader, current file '%s'", reader.CurrentFilename())
	c.file = reader

	return nil
}

func (c *Collector) createParser() error {
	c.Debug("starting parser creating")

	const readLinesNum = 100

	lines, err := logs.ReadLastLines(c.file.CurrentFilename(), readLinesNum)
	if err != nil {
		return fmt.Errorf("failed to read last lines: %v", err)
	}

	var found bool
	for _, line := range lines {
		if line = strings.TrimSpace(line); line == "" {
			continue
		}
		c.Debugf("last line: '%s'", line)

		c.parser, err = c.newParser([]byte(line))
		if err != nil {
			c.Debugf("failed to create parser from line: %v", err)
			continue
		}

		c.line.reset()

		if err = c.parser.Parse([]byte(line), c.line); err != nil {
			c.Debugf("failed to parse line: %v", err)
			continue
		}

		if err = c.line.verify(); err != nil {
			c.Debugf("failed to verify line: %v", err)
			continue
		}

		found = true
		break
	}

	if !found {
		return fmt.Errorf("failed to create log parser (file '%s')", c.file.CurrentFilename())
	}

	return nil
}
