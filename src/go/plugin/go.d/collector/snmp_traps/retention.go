// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"
)

const (
	defaultMaxSize  = uint64(10 * 1000 * 1000 * 1000) // 10 GB
	minRotationSize = uint64(5 * 1024 * 1024)         // 5 MB
	maxRotationSize = uint64(200 * 1024 * 1024)       // 200 MB
	rotationSizeDiv = 20
	bytesPerKB      = 1024
	bytesPerMB      = 1024 * 1024
	bytesPerGB      = 1024 * 1024 * 1024
)

type RetentionConfig struct {
	MaxSize     *uint64
	MaxDuration *time.Duration
	RotateSize  *uint64
	RotateDur   *time.Duration
}

func (rc RetentionConfig) EffectiveMaxSize() uint64 {
	if rc.MaxSize == nil {
		return defaultMaxSize
	}
	return *rc.MaxSize
}

func (rc RetentionConfig) EffectiveMaxDuration() time.Duration {
	if rc.MaxDuration == nil {
		return 0
	}
	return *rc.MaxDuration
}

func (rc RetentionConfig) EffectiveRotateSize() uint64 {
	if rc.RotateSize != nil {
		return *rc.RotateSize
	}
	maxSize := rc.EffectiveMaxSize()
	rot := min(max(maxSize/rotationSizeDiv, minRotationSize), maxRotationSize)
	return rot
}

func (rc RetentionConfig) EffectiveRotateDur() time.Duration {
	if rc.RotateDur != nil {
		return *rc.RotateDur
	}
	return 0
}

func validateRetention(rc RetentionConfig) error {
	if rc.MaxSize != nil && *rc.MaxSize == 0 {
		return errors.New("retention.max_size must be null or positive")
	}
	if rc.MaxDuration != nil {
		d := *rc.MaxDuration
		if d < 0 {
			return errors.New("retention.max_duration must be null or positive")
		}
		if d > 0 && d < time.Second {
			return errors.New("retention.max_duration must be at least 1s")
		}
	}
	if rc.RotateSize != nil && *rc.RotateSize == 0 {
		return errors.New("retention.rotation_size must be null or positive")
	}
	if rc.RotateDur != nil {
		d := *rc.RotateDur
		if d < 0 {
			return errors.New("retention.rotation_duration must be zero or positive")
		}
	}
	return nil
}

func parseHumanSize(s string) (uint64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, errors.New("empty size string")
	}
	if strings.EqualFold(s, "null") || strings.EqualFold(s, "none") {
		return 0, fmt.Errorf("not a size: %q", s)
	}

	var mult uint64 = 1
	upper := strings.ToUpper(s)
	switch {
	case strings.HasSuffix(upper, "GB"):
		mult = bytesPerGB
		s = s[:len(s)-2]
	case strings.HasSuffix(upper, "MB"):
		mult = bytesPerMB
		s = s[:len(s)-2]
	case strings.HasSuffix(upper, "KB"):
		mult = bytesPerKB
		s = s[:len(s)-2]
	case strings.HasSuffix(upper, "B"):
		mult = 1
		s = s[:len(s)-1]
	}

	s = strings.TrimSpace(s)
	val, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid size %q: %w", s, err)
	}
	if val < 0 {
		return 0, fmt.Errorf("negative size: %q", s)
	}
	result := uint64(val * float64(mult))
	return result, nil
}

func parseHumanDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, errors.New("empty duration string")
	}
	if strings.EqualFold(s, "null") || strings.EqualFold(s, "none") {
		return 0, fmt.Errorf("not a duration: %q", s)
	}
	if s == "0" {
		return 0, nil
	}
	d, err := time.ParseDuration(s)
	if err == nil {
		if d < 0 {
			return 0, fmt.Errorf("negative duration: %q", s)
		}
		return d, nil
	}
	d, err = parseComplexDuration(s)
	if err != nil {
		return 0, err
	}
	if d < 0 {
		return 0, fmt.Errorf("negative duration: %q", s)
	}
	return d, nil
}

func parseComplexDuration(s string) (time.Duration, error) {
	var total time.Duration
	rest := s
	for len(rest) > 0 {
		numEnd := 0
		for numEnd < len(rest) && (rest[numEnd] == '.' || rest[numEnd] == '-' || (rest[numEnd] >= '0' && rest[numEnd] <= '9')) {
			numEnd++
		}
		if numEnd == 0 {
			return 0, fmt.Errorf("invalid duration %q: expected number at %q", s, rest)
		}
		numStr := rest[:numEnd]
		rest = rest[numEnd:]

		unitEnd := 0
		for unitEnd < len(rest) && unicode.IsLetter(rune(rest[unitEnd])) {
			unitEnd++
		}
		if unitEnd == 0 {
			return 0, fmt.Errorf("invalid duration %q: expected unit after %q", s, numStr)
		}
		unitStr := rest[:unitEnd]
		rest = rest[unitEnd:]

		var unit time.Duration
		switch strings.ToLower(unitStr) {
		case "ns":
			unit = time.Nanosecond
		case "us", "µs":
			unit = time.Microsecond
		case "ms":
			unit = time.Millisecond
		case "s":
			unit = time.Second
		case "m":
			unit = time.Minute
		case "h":
			unit = time.Hour
		case "d":
			unit = 24 * time.Hour
		case "w":
			unit = 7 * 24 * time.Hour
		default:
			return 0, fmt.Errorf("invalid duration %q: unknown unit %q", s, unitStr)
		}

		val, err := strconv.ParseFloat(numStr, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid duration %q: %w", s, err)
		}
		total += time.Duration(val * float64(unit))
	}
	return total, nil
}

func formatHumanSize(bytes uint64) string {
	switch {
	case bytes >= bytesPerGB:
		return fmt.Sprintf("%dGB", bytes/bytesPerGB)
	case bytes >= bytesPerMB:
		return fmt.Sprintf("%dMB", bytes/bytesPerMB)
	case bytes >= bytesPerKB:
		return fmt.Sprintf("%dKB", bytes/bytesPerKB)
	default:
		return fmt.Sprintf("%dB", bytes)
	}
}

//go:fix inline
func uint64Ptr(u uint64) *uint64 {
	return new(u)
}

//go:fix inline
func durationPtr(d time.Duration) *time.Duration {
	return new(d)
}

func humanDuration(d time.Duration) string {
	if d == 0 {
		return "0s"
	}
	if d < time.Microsecond {
		return d.String()
	}
	var parts []string
	remaining := d

	days := remaining / (24 * time.Hour)
	if days > 0 {
		parts = append(parts, fmt.Sprintf("%dd", days))
		remaining -= days * 24 * time.Hour
	}
	hours := remaining / time.Hour
	if hours > 0 {
		parts = append(parts, fmt.Sprintf("%dh", hours))
		remaining -= hours * time.Hour
	}
	minutes := remaining / time.Minute
	if minutes > 0 {
		parts = append(parts, fmt.Sprintf("%dm", minutes))
		remaining -= minutes * time.Minute
	}
	seconds := remaining / time.Second
	if seconds > 0 {
		parts = append(parts, fmt.Sprintf("%ds", seconds))
	}
	if len(parts) == 0 {
		ms := remaining / time.Millisecond
		if ms > 0 {
			parts = append(parts, fmt.Sprintf("%dms", ms))
		}
	}
	if len(parts) == 0 {
		return "0s"
	}
	return strings.Join(parts, "")
}
