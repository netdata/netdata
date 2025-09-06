// SPDX-License-Identifier: GPL-3.0-or-later

package logs

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
)

type (
	RegExpConfig struct {
		Pattern string `yaml:"pattern" json:"pattern"`
	}

	RegExpParser struct {
		r       *bufio.Reader
		pattern *regexp.Regexp
	}
)

func NewRegExpParser(config RegExpConfig, in io.Reader) (*RegExpParser, error) {
	if config.Pattern == "" {
		return nil, errors.New("empty pattern")
	}

	pattern, err := regexp.Compile(config.Pattern)
	if err != nil {
		return nil, fmt.Errorf("compile: %w", err)
	}

	if hasNamed := slices.ContainsFunc(pattern.SubexpNames(), func(s string) bool { return s != "" }); !hasNamed {
		return nil, errors.New("pattern has no named subgroups")
	}

	p := &RegExpParser{
		r:       bufio.NewReader(in),
		pattern: pattern,
	}
	return p, nil
}

func (p *RegExpParser) ReadLine(line LogLine) error {
	row, err := p.r.ReadSlice('\n')
	if err != nil && len(row) == 0 {
		return err
	}
	if len(row) > 0 && row[len(row)-1] == '\n' {
		row = row[:len(row)-1]
	}
	return p.Parse(row, line)
}

func (p *RegExpParser) Parse(row []byte, line LogLine) error {
	match := p.pattern.FindSubmatch(row)
	if len(match) == 0 {
		return &ParseError{msg: "regexp parse: unmatched line"}
	}

	for i, name := range p.pattern.SubexpNames() {
		if name == "" || match[i] == nil {
			continue
		}
		err := line.Assign(name, string(match[i]))
		if err != nil {
			return &ParseError{msg: fmt.Sprintf("regexp parse: %v", err), err: err}
		}
	}
	return nil
}

func (p RegExpParser) Info() string {
	return fmt.Sprintf("regexp: %s", p.pattern)
}
