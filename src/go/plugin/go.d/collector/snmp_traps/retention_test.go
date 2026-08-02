// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/output/journal"
)

func TestParseHumanSize(t *testing.T) {
	tests := map[string]struct {
		input    string
		expected uint64
		wantErr  bool
	}{
		"10GB":         {"10GB", 10 * (1024 * 1024 * 1024), false},
		"1GB":          {"1GB", 1 * (1024 * 1024 * 1024), false},
		"100MB":        {"100MB", 100 * (1024 * 1024), false},
		"5MB":          {"5MB", 5 * (1024 * 1024), false},
		"200MB":        {"200MB", 200 * (1024 * 1024), false},
		"1KB":          {"1KB", 1 * (1024), false},
		"512B":         {"512B", 512, false},
		"1024":         {"1024", 1024, false},
		"empty":        {"", 0, true},
		"null":         {"null", 0, true},
		"garbage":      {"xyz", 0, true},
		"negative":     {"-1GB", 0, true},
		"float_gb":     {"1.5GB", uint64(1.5 * (1024 * 1024 * 1024)), false},
		"float_mb":     {"0.5MB", uint64(0.5 * (1024 * 1024)), false},
		"lowercase_gb": {"10gb", 10 * (1024 * 1024 * 1024), false},
		"lowercase_mb": {"100mb", 100 * (1024 * 1024), false},
		"mixedcase_gb": {"10Gb", 10 * (1024 * 1024 * 1024), false},
		"mixedcase_mb": {"100Mb", 100 * (1024 * 1024), false},
		"mixedcase_kb": {"64Kb", 64 * (1024), false},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := journal.ParseSize(tc.input)
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
			got, err := journal.ParseDuration(tc.input)
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
		rc      journal.Retention
		wantErr bool
	}{
		"defaults": {
			rc: journal.Retention{
				MaxSize:     new(journal.DefaultMaxSize),
				MaxDuration: nil,
				RotateSize:  nil,
				RotateDur:   nil,
			},
			wantErr: false,
		},
		"both_null": {
			rc: journal.Retention{
				MaxSize:     nil,
				MaxDuration: nil,
				RotateSize:  nil,
				RotateDur:   nil,
			},
			wantErr: false,
		},
		"zero_max_size": {
			rc: journal.Retention{
				MaxSize:     new(uint64(0)),
				MaxDuration: nil,
				RotateSize:  nil,
				RotateDur:   nil,
			},
			wantErr: true,
		},
		"negative_max_duration": {
			rc: journal.Retention{
				MaxSize:     new(journal.DefaultMaxSize),
				MaxDuration: new(-1 * time.Second),
				RotateSize:  nil,
				RotateDur:   nil,
			},
			wantErr: true,
		},
		"zero_rotation_duration": {
			rc: journal.Retention{
				MaxSize:     new(journal.DefaultMaxSize),
				MaxDuration: nil,
				RotateSize:  nil,
				RotateDur:   new(time.Duration(0)),
			},
			wantErr: false,
		},
		"negative_rotation_duration": {
			rc: journal.Retention{
				MaxSize:     new(journal.DefaultMaxSize),
				MaxDuration: nil,
				RotateSize:  nil,
				RotateDur:   new(-1 * time.Hour),
			},
			wantErr: true,
		},
		"very_short_max_duration": {
			rc: journal.Retention{
				MaxSize:     new(journal.DefaultMaxSize),
				MaxDuration: new(500 * time.Millisecond),
				RotateSize:  nil,
				RotateDur:   nil,
			},
			wantErr: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			err := journal.ValidateRetention(tc.rc)
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
		rc       journal.Retention
		expected uint64
	}{
		"default_10GB": {
			rc:       journal.Retention{MaxSize: new(uint64(10 * (1024 * 1024 * 1024)))},
			expected: (200 * 1024 * 1024),
		},
		"1GB_min_clamp": {
			rc:       journal.Retention{MaxSize: new(uint64(1 * (1024 * 1024 * 1024)))},
			expected: uint64(1 * (1024 * 1024 * 1024) / 20),
		},
		"100GB_max_clamp": {
			rc:       journal.Retention{MaxSize: new(uint64(100 * (1024 * 1024 * 1024)))},
			expected: (200 * 1024 * 1024),
		},
		"null_size_uses_upper_clamp": {
			rc:       journal.Retention{MaxSize: nil},
			expected: (200 * 1024 * 1024),
		},
		"explicit_rotation_overrides_auto": {
			rc: journal.Retention{
				MaxSize:    new(uint64(10 * (1024 * 1024 * 1024))),
				RotateSize: new(uint64(100 * (1024 * 1024))),
			},
			expected: 100 * (1024 * 1024),
		},
		"small_200MB": {
			rc:       journal.Retention{MaxSize: new(uint64(200 * (1024 * 1024)))},
			expected: uint64(200 * (1024 * 1024) / 20),
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

func TestEffectiveRotateDurationDefaultDisabled(t *testing.T) {
	if got := (journal.Retention{}).EffectiveRotateDur(); got != 0 {
		t.Fatalf("expected disabled default rotate duration, got %v", got)
	}
	if got := (journal.Retention{RotateDur: new(1 * time.Hour)}).EffectiveRotateDur(); got != time.Hour {
		t.Fatalf("expected explicit rotate duration 1h, got %v", got)
	}
}

func TestParseRetentionConfigDefaults(t *testing.T) {
	jc := jsonRetentionConfig{
		MaxSize:     new("10GB"),
		MaxDuration: nil,
		RotateSize:  nil,
		RotateDur:   nil,
	}
	rc, err := parseRetentionConfig(jc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rc.MaxSize == nil || *rc.MaxSize != 10*(1024*1024*1024) {
		t.Fatalf("expected max_size 10GB, got %v", rc.MaxSize)
	}
	if rc.MaxDuration != nil {
		t.Fatalf("expected nil max_duration")
	}
	if rc.RotateDur != nil {
		t.Fatalf("expected nil rotate_dur default, got %v", rc.RotateDur)
	}
	if got := rc.EffectiveRotateDur(); got != 0 {
		t.Fatalf("expected disabled effective rotate_dur default, got %v", got)
	}
}

func TestParseRetentionConfigExplicitRotationDuration(t *testing.T) {
	jc := jsonRetentionConfig{
		RotateDur: new("1h"),
	}
	rc, err := parseRetentionConfig(jc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rc.RotateDur == nil || *rc.RotateDur != 1*time.Hour {
		t.Fatalf("expected rotate_dur 1h, got %v", rc.RotateDur)
	}
}

func TestParseRetentionConfigBothNull(t *testing.T) {
	jc := jsonRetentionConfig{
		MaxSize:     new("null"),
		MaxDuration: nil,
		RotateSize:  nil,
		RotateDur:   nil,
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
		MaxSize:   new("xyz"),
		RotateDur: new("1h"),
	}
	_, err := parseRetentionConfig(jc)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestParseRetentionConfigInvalidDuration(t *testing.T) {
	jc := jsonRetentionConfig{
		RotateDur: new("xyz"),
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
			rc, err := parseRetentionConfig(jsonRetentionConfig{RotateDur: new(tc.value)})
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
	cacheDir := withTestCacheDir(t)
	root := journal.Root("local")
	want := filepath.Join(cacheDir, "log", "traps", "local")
	if root != want {
		t.Fatalf("expected %q, got %q", want, root)
	}
}

func TestJournalRootPrefersNetdataLogDirEnv(t *testing.T) {
	logDir := filepath.Join(t.TempDir(), "env-log")
	withNetdataLogDir(t, logDir)

	root := journal.Root("local")
	want := filepath.Join(logDir, "traps", "local")
	if root != want {
		t.Fatalf("expected %q, got %q", want, root)
	}
}

func TestValidateNetdataLogRootRequiresExistingDirectory(t *testing.T) {
	root := filepath.Join(t.TempDir(), "missing")
	withNetdataLogDir(t, root)

	err := journal.ValidateLogRoot()
	if err == nil {
		t.Fatal("expected missing Netdata log root error")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("expected missing root error, got %v", err)
	}
	if _, statErr := os.Stat(root); !os.IsNotExist(statErr) {
		t.Fatalf("Netdata log root was created unexpectedly: %v", statErr)
	}
}

func TestValidateNetdataLogRootRejectsFile(t *testing.T) {
	root := filepath.Join(t.TempDir(), "log")
	if err := os.WriteFile(root, []byte("not a directory"), 0640); err != nil {
		t.Fatalf("create test file: %v", err)
	}
	withNetdataLogDir(t, root)

	err := journal.ValidateLogRoot()
	if err == nil {
		t.Fatal("expected non-directory Netdata log root error")
	}
	if !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("expected non-directory root error, got %v", err)
	}
}
