// SPDX-License-Identifier: GPL-3.0-or-later

package logs

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadLastLine(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected string
		err      error
	}{
		{"empty", "", "", nil},
		{"empty-ln", "\n", "\n", nil},
		{"one-line", "hello", "hello", nil},
		{"one-line-ln", "hello\n", "hello\n", nil},
		{"multi-line", "hello\nworld", "world", nil},
		{"multi-line-ln", "hello\nworld\n", "world\n", nil},
		{"long-line", "hello hello hello", "", ErrTooLongLine},
		{"long-line-ln", "hello hello hello\n", "", ErrTooLongLine},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			filename := prepareFile(t, test.content)
			defer func() { _ = os.Remove(filename) }()

			line, err := ReadLastLine(filename, 10)

			if test.err != nil {
				require.NotNil(t, err)
				assert.Contains(t, err.Error(), test.err.Error())
			} else {
				assert.Equal(t, test.expected, string(line))
			}
		})
	}
}

func prepareFile(t *testing.T, content string) string {
	t.Helper()
	file, err := os.CreateTemp("", "go-test")
	require.NoError(t, err)
	defer func() { _ = file.Close() }()

	_, _ = file.WriteString(content)
	return file.Name()
}
