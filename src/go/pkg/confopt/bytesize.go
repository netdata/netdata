// SPDX-License-Identifier: GPL-3.0-or-later

package confopt

import (
	"encoding/json"
	"fmt"
	"math/big"
	"regexp"
	"strconv"
	"strings"
)

// ByteSize is a byte count that marshals as a lossless human-readable string.
// JSON and YAML accept decimal byte counts or strings with SI (KB, MB, ...) or
// binary (KiB, MiB, ...) units. Fractions must resolve to whole bytes.
// Null resets the value to zero.
type ByteSize int64

const (
	KiB ByteSize = 1 << 10
	MiB ByteSize = 1 << 20
	GiB ByteSize = 1 << 30
	TiB ByteSize = 1 << 40
	PiB ByteSize = 1 << 50
	EiB ByteSize = 1 << 60
)

var reByteSize = regexp.MustCompile(`^([+-]?[0-9]+(?:\.[0-9]+)?)\s*([a-zA-Z]*)$`)

// ParseByteSize parses one signed decimal quantity, with an optional unit.
// Units are case-insensitive; a bare number means bytes. Mixed units, fractional
// bytes, exponential notation and values outside int64 are rejected.
func ParseByteSize(s string) (ByteSize, error) {
	m := reByteSize.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return 0, fmt.Errorf("invalid byte size %q", s)
	}
	unit := strings.ToUpper(m[2])
	var multiplier int64 = 1
	if unit != "" && unit != "B" {
		if len(unit) < 2 || (unit[1:] != "B" && unit[1:] != "IB") {
			return 0, fmt.Errorf("invalid byte size unit %q", m[2])
		}
		power := strings.IndexByte("KMGTPE", unit[0]) + 1
		if power == 0 {
			return 0, fmt.Errorf("invalid byte size unit %q", m[2])
		}
		var base int64 = 1000
		if unit[1:] == "IB" {
			base = 1024
		}
		for range power {
			multiplier *= base
		}
	}
	value, ok := new(big.Rat).SetString(m[1])
	if !ok {
		return 0, fmt.Errorf("invalid byte size %q", s)
	}
	value.Mul(value, new(big.Rat).SetInt64(multiplier))
	if !value.IsInt() || !value.Num().IsInt64() {
		return 0, fmt.Errorf("byte size %q is not a whole int64 byte count", s)
	}
	return ByteSize(value.Num().Int64()), nil
}

func (s ByteSize) Bytes() int64 { return int64(s) }

func (s ByteSize) String() string {
	if s != 0 {
		for i, unit := range [...]ByteSize{EiB, PiB, TiB, GiB, MiB, KiB} {
			if s%unit == 0 {
				return strconv.FormatInt(int64(s/unit), 10) + [...]string{"EiB", "PiB", "TiB", "GiB", "MiB", "KiB"}[i]
			}
		}
	}
	return strconv.FormatInt(int64(s), 10) + "B"
}

func (s *ByteSize) UnmarshalYAML(unmarshal func(any) error) error {
	var value string
	if err := unmarshal(&value); err != nil {
		return err
	}
	parsed, err := ParseByteSize(value)
	if err == nil {
		*s = parsed
	}
	return err
}

func (s ByteSize) MarshalYAML() (any, error) { return s.String(), nil }

func (s *ByteSize) UnmarshalJSON(data []byte) error {
	value := strings.TrimSpace(string(data))
	if value == "null" {
		*s = 0
		return nil
	}
	if strings.HasPrefix(value, `"`) {
		if err := json.Unmarshal(data, &value); err != nil {
			return err
		}
	}
	parsed, err := ParseByteSize(value)
	if err == nil {
		*s = parsed
	}
	return err
}

func (s ByteSize) MarshalJSON() ([]byte, error) { return json.Marshal(s.String()) }
