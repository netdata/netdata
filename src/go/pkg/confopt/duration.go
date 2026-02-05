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
	// Try as JSON string first (handles quoted values like "30m", "5s")
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		if v, err := ParseDuration(s); err == nil {
			*d = Duration(v)
			return nil
		}
		// Try as numeric string (interpret as seconds)
		if v, err := strconv.ParseFloat(s, 64); err == nil {
			*d = Duration(v * float64(time.Second))
			return nil
		}
	}

	// Try as JSON number (handles unquoted values like 5, 1.5)
	var f float64
	if err := json.Unmarshal(b, &f); err == nil {
		*d = Duration(f * float64(time.Second))
		return nil
	}

	return fmt.Errorf("unparsable duration format '%s'", string(b))
}

func (d Duration) MarshalJSON() ([]byte, error) {
	seconds := float64(d) / float64(time.Second)
	return json.Marshal(seconds)
}

// LongDuration is like Duration but marshals to a human-friendly string (e.g., "12h", "30m", "1d").
// Unmarshal accepts both strings ("12h", "1d") and numbers (seconds).
type LongDuration time.Duration

func (d LongDuration) Duration() time.Duration {
	return time.Duration(d)
}

func (d LongDuration) String() string {
	return formatDuration(time.Duration(d))
}

func (d *LongDuration) UnmarshalYAML(unmarshal func(any) error) error {
	var tmp Duration
	if err := tmp.UnmarshalYAML(unmarshal); err != nil {
		return err
	}
	*d = LongDuration(tmp)
	return nil
}

func (d LongDuration) MarshalYAML() (any, error) {
	return formatDuration(time.Duration(d)), nil
}

func (d *LongDuration) UnmarshalJSON(b []byte) error {
	var tmp Duration
	if err := tmp.UnmarshalJSON(b); err != nil {
		return err
	}
	*d = LongDuration(tmp)
	return nil
}

func (d LongDuration) MarshalJSON() ([]byte, error) {
	return json.Marshal(formatDuration(time.Duration(d)))
}

// formatDuration formats a duration as a human-friendly string.
// Uses the largest unit that produces a clean integer value, with preference
// for fractional seconds over milliseconds when value >= 1s.
// Supported units: y (365d), mo (30d), w (7d), d (24h), h, m, s, ms.
func formatDuration(d time.Duration) string {
	if d == 0 {
		return "0s"
	}

	neg := d < 0
	if neg {
		d = -d
	}

	// Units from largest to smallest (excluding ms - handled separately)
	units := []struct {
		suffix string
		value  time.Duration
	}{
		{"y", 365 * 24 * time.Hour},
		{"mo", 30 * 24 * time.Hour},
		{"w", 7 * 24 * time.Hour},
		{"d", 24 * time.Hour},
		{"h", time.Hour},
		{"m", time.Minute},
		{"s", time.Second},
	}

	// Find the largest unit that divides evenly
	for _, u := range units {
		if d >= u.value && d%u.value == 0 {
			val := d / u.value
			if neg {
				return fmt.Sprintf("-%d%s", val, u.suffix)
			}
			return fmt.Sprintf("%d%s", val, u.suffix)
		}
	}

	// For values >= 1s without clean division, prefer fractional seconds over ms
	if d >= time.Second {
		seconds := float64(d) / float64(time.Second)
		if neg {
			return fmt.Sprintf("-%.3gs", seconds)
		}
		return fmt.Sprintf("%.3gs", seconds)
	}

	// For sub-second values, try milliseconds
	if d >= time.Millisecond && d%time.Millisecond == 0 {
		val := d / time.Millisecond
		if neg {
			return fmt.Sprintf("-%dms", val)
		}
		return fmt.Sprintf("%dms", val)
	}

	// Fallback to Go's standard format for sub-millisecond precision
	if neg {
		return "-" + d.String()
	}
	return d.String()
}
