// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"testing"
	"time"
)

func TestParseHumanSize(t *testing.T) {
	tests := map[string]struct {
		input    string
		expected uint64
		wantErr  bool
	}{
		"10GB":         {"10GB", 10 * bytesPerGB, false},
		"1GB":          {"1GB", 1 * bytesPerGB, false},
		"100MB":        {"100MB", 100 * bytesPerMB, false},
		"5MB":          {"5MB", 5 * bytesPerMB, false},
		"200MB":        {"200MB", 200 * bytesPerMB, false},
		"1KB":          {"1KB", 1 * bytesPerKB, false},
		"512B":         {"512B", 512, false},
		"1024":         {"1024", 1024, false},
		"empty":        {"", 0, true},
		"null":         {"null", 0, true},
		"garbage":      {"xyz", 0, true},
		"negative":     {"-1GB", 0, true},
		"float_gb":     {"1.5GB", uint64(1.5 * bytesPerGB), false},
		"float_mb":     {"0.5MB", uint64(0.5 * bytesPerMB), false},
		"lowercase_gb": {"10gb", 10 * bytesPerGB, false},
		"lowercase_mb": {"100mb", 100 * bytesPerMB, false},
		"mixedcase_gb": {"10Gb", 10 * bytesPerGB, false},
		"mixedcase_mb": {"100Mb", 100 * bytesPerMB, false},
		"mixedcase_kb": {"64Kb", 64 * bytesPerKB, false},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := parseHumanSize(tc.input)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error, got %d", got)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !tc.wantErr && got != tc.expected {
				t.Fatalf("expected %d, got %d", tc.expected, got)
			}
		})
	}
}

func TestParseHumanDuration(t *testing.T) {
	tests := map[string]struct {
		input    string
		expected time.Duration
		wantErr  bool
	}{
		"1h":       {"1h", 1 * time.Hour, false},
		"30m":      {"30m", 30 * time.Minute, false},
		"90s":      {"90s", 90 * time.Second, false},
		"1h30m":    {"1h30m", 1*time.Hour + 30*time.Minute, false},
		"7d":       {"7d", 7 * 24 * time.Hour, false},
		"30d":      {"30d", 30 * 24 * time.Hour, false},
		"1w":       {"1w", 7 * 24 * time.Hour, false},
		"0":        {"0", 0, false},
		"empty":    {"", 0, true},
		"null":     {"null", 0, true},
		"garbage":  {"xyz", 0, true},
		"negative": {"-1h", 0, true},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := parseHumanDuration(tc.input)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error, got %v", got)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !tc.wantErr && got != tc.expected {
				t.Fatalf("expected %v, got %v", tc.expected, got)
			}
		})
	}
}

func TestValidateRetention(t *testing.T) {
	tests := map[string]struct {
		rc      RetentionConfig
		wantErr bool
	}{
		"defaults": {
			rc: RetentionConfig{
				MaxSize:     uint64Ptr(defaultMaxSize),
				MaxDuration: nil,
				RotateSize:  nil,
				RotateDur:   durationPtr(defaultRotateDur),
			},
			wantErr: false,
		},
		"both_null": {
			rc: RetentionConfig{
				MaxSize:     nil,
				MaxDuration: nil,
				RotateSize:  nil,
				RotateDur:   durationPtr(defaultRotateDur),
			},
			wantErr: false,
		},
		"zero_max_size": {
			rc: RetentionConfig{
				MaxSize:     uint64Ptr(0),
				MaxDuration: nil,
				RotateSize:  nil,
				RotateDur:   durationPtr(defaultRotateDur),
			},
			wantErr: true,
		},
		"negative_max_duration": {
			rc: RetentionConfig{
				MaxSize:     uint64Ptr(defaultMaxSize),
				MaxDuration: durationPtr(-1 * time.Second),
				RotateSize:  nil,
				RotateDur:   durationPtr(defaultRotateDur),
			},
			wantErr: true,
		},
		"zero_rotation_duration": {
			rc: RetentionConfig{
				MaxSize:     uint64Ptr(defaultMaxSize),
				MaxDuration: nil,
				RotateSize:  nil,
				RotateDur:   durationPtr(0),
			},
			wantErr: false,
		},
		"negative_rotation_duration": {
			rc: RetentionConfig{
				MaxSize:     uint64Ptr(defaultMaxSize),
				MaxDuration: nil,
				RotateSize:  nil,
				RotateDur:   durationPtr(-1 * time.Hour),
			},
			wantErr: true,
		},
		"very_short_max_duration": {
			rc: RetentionConfig{
				MaxSize:     uint64Ptr(defaultMaxSize),
				MaxDuration: durationPtr(500 * time.Millisecond),
				RotateSize:  nil,
				RotateDur:   durationPtr(defaultRotateDur),
			},
			wantErr: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			err := validateRetention(tc.rc)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestEffectiveRotateSize(t *testing.T) {
	tests := map[string]struct {
		rc       RetentionConfig
		expected uint64
	}{
		"default_10GB": {
			rc:       RetentionConfig{MaxSize: uint64Ptr(10 * bytesPerGB)},
			expected: maxRotationSize,
		},
		"1GB_min_clamp": {
			rc:       RetentionConfig{MaxSize: uint64Ptr(1 * bytesPerGB)},
			expected: uint64(1 * bytesPerGB / rotationSizeDiv),
		},
		"100GB_max_clamp": {
			rc:       RetentionConfig{MaxSize: uint64Ptr(100 * bytesPerGB)},
			expected: maxRotationSize,
		},
		"null_size_uses_upper_clamp": {
			rc:       RetentionConfig{MaxSize: nil},
			expected: maxRotationSize,
		},
		"explicit_rotation_overrides_auto": {
			rc: RetentionConfig{
				MaxSize:    uint64Ptr(10 * bytesPerGB),
				RotateSize: uint64Ptr(100 * bytesPerMB),
			},
			expected: 100 * bytesPerMB,
		},
		"small_200MB": {
			rc:       RetentionConfig{MaxSize: uint64Ptr(200 * bytesPerMB)},
			expected: uint64(200 * bytesPerMB / rotationSizeDiv),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := tc.rc.EffectiveRotateSize()
			if got != tc.expected {
				t.Fatalf("expected %d, got %d", tc.expected, got)
			}
		})
	}
}

func TestParseRetentionConfig(t *testing.T) {
	jc := jsonRetentionConfig{
		MaxSize:     stringPtr("10GB"),
		MaxDuration: nil,
		RotateSize:  nil,
		RotateDur:   stringPtr("1h"),
	}
	rc, err := parseRetentionConfig(jc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rc.MaxSize == nil || *rc.MaxSize != 10*bytesPerGB {
		t.Fatalf("expected max_size 10GB, got %v", rc.MaxSize)
	}
	if rc.MaxDuration != nil {
		t.Fatalf("expected nil max_duration")
	}
	if rc.RotateDur == nil || *rc.RotateDur != 1*time.Hour {
		t.Fatalf("expected rotate_dur 1h, got %v", rc.RotateDur)
	}
}

func TestParseRetentionConfigBothNull(t *testing.T) {
	jc := jsonRetentionConfig{
		MaxSize:     stringPtr("null"),
		MaxDuration: nil,
		RotateSize:  nil,
		RotateDur:   stringPtr("1h"),
	}
	rc, err := parseRetentionConfig(jc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rc.MaxSize != nil {
		t.Fatalf("expected nil max_size, got %v", *rc.MaxSize)
	}
}

func TestParseRetentionConfigInvalidSize(t *testing.T) {
	jc := jsonRetentionConfig{
		MaxSize:   stringPtr("xyz"),
		RotateDur: stringPtr("1h"),
	}
	_, err := parseRetentionConfig(jc)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestParseRetentionConfigInvalidDuration(t *testing.T) {
	jc := jsonRetentionConfig{
		RotateDur: stringPtr("xyz"),
	}
	_, err := parseRetentionConfig(jc)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestParseRetentionConfigRotationDurationDisabled(t *testing.T) {
	tests := map[string]struct {
		value string
	}{
		"zero": {"0"},
		"null": {"null"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			rc, err := parseRetentionConfig(jsonRetentionConfig{RotateDur: stringPtr(tc.value)})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if rc.RotateDur == nil || *rc.RotateDur != 0 {
				t.Fatalf("expected disabled rotation duration, got %v", rc.RotateDur)
			}
		})
	}
}

func TestJournalRoot(t *testing.T) {
	root := journalRoot("local")
	if root == "" {
		t.Fatal("expected non-empty journal root")
	}
	if root[len(root)-6:] != "/local" {
		t.Fatalf("expected suffix /local, got %q", root)
	}
}

func stringPtr(s string) *string {
	return &s
}
