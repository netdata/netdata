// SPDX-License-Identifier: GPL-3.0-or-later

package strmutil

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTruncateText(t *testing.T) {
	tests := map[string]struct {
		input    string
		maxLen   int
		expected string
	}{
		"short text unchanged": {
			input:    "hello",
			maxLen:   100,
			expected: "hello",
		},
		"text exactly at limit": {
			input:    "hello",
			maxLen:   5,
			expected: "hello",
		},
		"text truncated with ellipsis": {
			input:    "hello world",
			maxLen:   8,
			expected: "hello...",
		},
		"empty string": {
			input:    "",
			maxLen:   100,
			expected: "",
		},
		"UTF-8 characters preserved": {
			input:    "日本語テスト",
			maxLen:   13, // 3 chars * 3 bytes each = 9 bytes + "..." = 12 bytes fits in 13
			expected: "日本語...",
		},
		"mixed UTF-8 and ASCII": {
			input:    "hello世界test",
			maxLen:   13, // "hello" (5) + "世" (3) + "界" (3) = 11 bytes + "..." = 14, so truncate to fit
			expected: "hello世...",
		},
		"very small maxLen": {
			input:    "hello",
			maxLen:   4, // only room for 1 char + "..."
			expected: "h...",
		},
		"emoji handling": {
			input:    "test🚀emoji",
			maxLen:   8, // "test" (4) + "..." (3) = 7 bytes fits in 8
			expected: "test...",
		},
		"maxLen zero": {
			input:    "hello",
			maxLen:   0,
			expected: "",
		},
		"maxLen negative": {
			input:    "hello",
			maxLen:   -1,
			expected: "",
		},
		"maxLen one ASCII": {
			input:    "hello",
			maxLen:   1,
			expected: "h",
		},
		"maxLen two ASCII": {
			input:    "hello",
			maxLen:   2,
			expected: "he",
		},
		"maxLen one UTF-8 too small": {
			input:    "日本語", // each char is 3 bytes
			maxLen:   1,     // can't fit any full rune
			expected: "",
		},
		"maxLen two UTF-8 too small": {
			input:    "日本語",
			maxLen:   2, // still can't fit a 3-byte rune
			expected: "",
		},
		"maxLen three with long text": {
			input:    "日本語",
			maxLen:   3, // only room for ellipsis, no content
			expected: "...",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			result := TruncateText(tc.input, tc.maxLen)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestTruncateText_LongQuery(t *testing.T) {
	// Simulate a real SQL query scenario
	longQuery := strings.Repeat("SELECT * FROM table WHERE id = ?; ", 200)
	result := TruncateText(longQuery, 4096)

	assert.LessOrEqual(t, len(result), 4096)
	assert.True(t, strings.HasSuffix(result, "..."))
}
