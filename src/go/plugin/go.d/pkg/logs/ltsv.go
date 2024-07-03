// SPDX-License-Identifier: GPL-3.0-or-later

package logs

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"unsafe"

	"github.com/Wing924/ltsv"
)

type (
	LTSVConfig struct {
		FieldDelimiter string            `yaml:"field_delimiter" json:"field_delimiter"`
		ValueDelimiter string            `yaml:"value_delimiter" json:"value_delimiter"`
		Mapping        map[string]string `yaml:"mapping" json:"mapping"`
	}

	LTSVParser struct {
		r       *bufio.Reader
		parser  ltsv.Parser
		mapping map[string]string
	}
)

func NewLTSVParser(config LTSVConfig, in io.Reader) (*LTSVParser, error) {
	p := ltsv.Parser{
		FieldDelimiter: ltsv.DefaultParser.FieldDelimiter,
		ValueDelimiter: ltsv.DefaultParser.ValueDelimiter,
		StrictMode:     false,
	}
	if config.FieldDelimiter != "" {
		if d, err := parseLTSVDelimiter(config.FieldDelimiter); err == nil {
			p.FieldDelimiter = d
		}
	}
	if config.ValueDelimiter != "" {
		if d, err := parseLTSVDelimiter(config.ValueDelimiter); err == nil {
			p.ValueDelimiter = d
		}
	}
	parser := &LTSVParser{
		r:       bufio.NewReader(in),
		parser:  p,
		mapping: config.Mapping,
	}
	return parser, nil
}

func (p *LTSVParser) ReadLine(line LogLine) error {
	row, err := p.r.ReadSlice('\n')
	if err != nil && len(row) == 0 {
		return err
	}
	if len(row) > 0 && row[len(row)-1] == '\n' {
		row = row[:len(row)-1]
	}
	return p.Parse(row, line)
}

func (p *LTSVParser) Parse(row []byte, line LogLine) error {
	err := p.parser.ParseLine(row, func(label []byte, value []byte) error {
		s := *(*string)(unsafe.Pointer(&label)) // no alloc, same as in fmt.Builder.String()
		if v, ok := p.mapping[s]; ok {
			s = v
		}
		return line.Assign(s, string(value))
	})
	if err != nil {
		return &ParseError{msg: fmt.Sprintf("ltsv parse: %v", err), err: err}
	}
	return nil
}

func (p LTSVParser) Info() string {
	return fmt.Sprintf("ltsv: %q", p.mapping)
}

func parseLTSVDelimiter(s string) (byte, error) {
	if isNumber(s) {
		d, err := strconv.ParseUint(s, 10, 8)
		if err != nil {
			return 0, fmt.Errorf("invalid LTSV delimiter: %v", err)
		}
		return byte(d), nil
	}
	if len(s) != 1 {
		return 0, errors.New("invalid LTSV delimiter: must be a single character")
	}
	return s[0], nil
}
