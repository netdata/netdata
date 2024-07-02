// SPDX-License-Identifier: GPL-3.0-or-later

package logs

import (
	"bufio"
	"fmt"
	"io"
	"strconv"

	"github.com/valyala/fastjson"
)

type JSONConfig struct {
	Mapping map[string]string `yaml:"mapping" json:"mapping"`
}

type JSONParser struct {
	reader  *bufio.Reader
	parser  fastjson.Parser
	buf     []byte
	mapping map[string]string
}

func NewJSONParser(config JSONConfig, in io.Reader) (*JSONParser, error) {
	parser := &JSONParser{
		reader:  bufio.NewReader(in),
		mapping: config.Mapping,
		buf:     make([]byte, 0, 100),
	}
	return parser, nil
}

func (p *JSONParser) ReadLine(line LogLine) error {
	row, err := p.reader.ReadSlice('\n')
	if err != nil && len(row) == 0 {
		return err
	}
	if len(row) > 0 && row[len(row)-1] == '\n' {
		row = row[:len(row)-1]
	}
	return p.Parse(row, line)
}

func (p *JSONParser) Parse(row []byte, line LogLine) error {
	val, err := p.parser.ParseBytes(row)
	if err != nil {
		return err
	}

	if err := p.parseObject("", val, line); err != nil {
		return &ParseError{msg: fmt.Sprintf("json parse: %v", err), err: err}
	}

	return nil
}

func (p *JSONParser) parseObject(prefix string, val *fastjson.Value, line LogLine) error {
	obj, err := val.Object()
	if err != nil {
		return err
	}

	obj.Visit(func(key []byte, v *fastjson.Value) {
		if err != nil {
			return
		}

		k := jsonObjKey(prefix, string(key))

		switch v.Type() {
		case fastjson.TypeString, fastjson.TypeNumber:
			err = p.parseStringNumber(k, v, line)
		case fastjson.TypeArray:
			err = p.parseArray(k, v, line)
		case fastjson.TypeObject:
			err = p.parseObject(k, v, line)
		default:
			return
		}
	})

	return err
}

func jsonObjKey(prefix, key string) string {
	if prefix == "" {
		return key
	}
	return prefix + "." + key
}

func (p *JSONParser) parseArray(key string, val *fastjson.Value, line LogLine) error {
	arr, err := val.Array()
	if err != nil {
		return err
	}

	for i, v := range arr {
		k := jsonObjKey(key, strconv.Itoa(i))

		switch v.Type() {
		case fastjson.TypeString, fastjson.TypeNumber:
			err = p.parseStringNumber(k, v, line)
		case fastjson.TypeArray:
			err = p.parseArray(k, v, line)
		case fastjson.TypeObject:
			err = p.parseObject(k, v, line)
		default:
			continue
		}

		if err != nil {
			return err
		}
	}

	return err
}

func (p *JSONParser) parseStringNumber(key string, val *fastjson.Value, line LogLine) error {
	if mapped, ok := p.mapping[key]; ok {
		key = mapped
	}

	p.buf = p.buf[:0]
	if p.buf = val.MarshalTo(p.buf); len(p.buf) == 0 {
		return nil
	}

	if val.Type() == fastjson.TypeString {
		// trim "
		return line.Assign(key, string(p.buf[1:len(p.buf)-1]))
	}
	return line.Assign(key, string(p.buf))
}

func (p *JSONParser) Info() string {
	return fmt.Sprintf("json: %q", p.mapping)
}
