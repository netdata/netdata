// SPDX-License-Identifier: GPL-3.0-or-later

package confopt

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var reDuration = regexp.MustCompile(`(\d+(?:\.\d+)?)\s*(ns|us|µs|μs|ms|s|mo|m|h|d|wk|w|M|y)`)

// ParseDuration parses a duration string with units.
func ParseDuration(s string) (time.Duration, error) {
	orig := s

	if s = strings.ReplaceAll(s, " ", ""); s == "" {
		return 0, fmt.Errorf("empty duration string")
	}

	neg := s[0] == '-'
	if neg {
		s = s[1:]
	}

	unitMap := map[string]time.Duration{
		"d":  24 * time.Hour,
		"w":  7 * 24 * time.Hour,
		"wk": 7 * 24 * time.Hour,
		"mo": 30 * 24 * time.Hour,
		"M":  30 * 24 * time.Hour,
		"y":  365 * 24 * time.Hour,
	}

	matches := reDuration.FindAllStringSubmatch(s, -1)

	if len(matches) == 0 {
		return 0, fmt.Errorf("invalid duration format: '%s'", orig)
	}

	var total time.Duration

	for _, m := range matches {
		value, unit := m[1], m[2]

		val, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid number: %s", value)
		}

		if multiplier, ok := unitMap[unit]; ok {
			total += time.Duration(val * float64(multiplier))
		} else {
			dur, err := time.ParseDuration(value + unit)
			if err != nil {
				return 0, fmt.Errorf("invalid duration unit: %s", value+unit)
			}
			total += dur
		}
	}

	if neg {
		total = -total
	}

	return total, nil
}

type Duration time.Duration

func (d Duration) Duration() time.Duration {
	return time.Duration(d)
}

func (d Duration) String() string {
	return d.Duration().String()
}

func (d *Duration) UnmarshalYAML(unmarshal func(any) error) error {
	var s string

	if err := unmarshal(&s); err != nil {
		return err
	}

	if v, err := ParseDuration(s); err == nil {
		*d = Duration(v)
		return nil
	}
	if v, err := strconv.ParseInt(s, 10, 64); err == nil {
		*d = Duration(time.Duration(v) * time.Second)
		return nil
	}
	if v, err := strconv.ParseFloat(s, 64); err == nil {
		*d = Duration(v * float64(time.Second))
		return nil
	}

	return fmt.Errorf("unparsable duration format '%s'", s)
}

func (d Duration) MarshalYAML() (any, error) {
	seconds := float64(d) / float64(time.Second)
	return seconds, nil
}

func (d *Duration) UnmarshalJSON(b []byte) error {
	s := string(b)

	if v, err := time.ParseDuration(s); err == nil {
		*d = Duration(v)
		return nil
	}
	if v, err := strconv.ParseInt(s, 10, 64); err == nil {
		*d = Duration(time.Duration(v) * time.Second)
		return nil
	}
	if v, err := strconv.ParseFloat(s, 64); err == nil {
		*d = Duration(v * float64(time.Second))
		return nil
	}

	return fmt.Errorf("unparsable duration format '%s'", s)
}

func (d Duration) MarshalJSON() ([]byte, error) {
	seconds := float64(d) / float64(time.Second)
	return json.Marshal(seconds)
}
