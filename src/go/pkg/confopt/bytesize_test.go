// SPDX-License-Identifier: GPL-3.0-or-later

package confopt

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func TestParseByteSize(t *testing.T) {
	for input, want := range map[string]ByteSize{
		"0": 0, "1B": 1, "-1": -1, "+1": 1,
		"128 MiB": 128 * MiB, "1.5KiB": 1536, "0.1 KB": 100,
		"1MB": 1000000, "1mb": 1000000, "1MiB": MiB, " 2 gib ": 2 * GiB,
		"1TB": 1000000000000, "1PB": 1000000000000000, "1EB": 1000000000000000000,
		"1TiB": TiB, "1PiB": PiB, "1EiB": EiB, "-1.5KiB": -1536,
		"9007199254740993":    9007199254740993,
		"9223372036854775807": math.MaxInt64, "-9223372036854775808": math.MinInt64,
	} {
		t.Run(input, func(t *testing.T) {
			got, err := ParseByteSize(input)
			require.NoError(t, err)
			assert.Equal(t, want, got)
		})
	}
	for _, input := range []string{
		"", " ", "0.1B", "0.1KiB", "NaN", "Inf", "null", "true", "1e3", "1/2KiB",
		"1MiB junk", "1MiB2KiB", "1 M", "1XB", "1BB", "1 2KB", "1.2.3MB",
		"9223372036854775808", "-9223372036854775809", "8EiB", "10000000000000000000000000GB",
	} {
		t.Run(input, func(t *testing.T) {
			_, err := ParseByteSize(input)
			assert.Error(t, err)
		})
	}
}

func TestByteSizeSerialization(t *testing.T) {
	for _, tc := range []struct {
		value ByteSize
		text  string
	}{
		{0, "0B"}, {1, "1B"}, {-1, "-1B"}, {128 * MiB, "128MiB"}, {1536, "1536B"},
		{9007199254740993, "9007199254740993B"},
		{math.MaxInt64, "9223372036854775807B"}, {math.MinInt64, "-8EiB"},
	} {
		t.Run(tc.text, func(t *testing.T) {
			assert.Equal(t, tc.text, tc.value.String())
			assert.Equal(t, int64(tc.value), tc.value.Bytes())
			for name, codec := range map[string]struct {
				marshal   func(any) ([]byte, error)
				unmarshal func([]byte, any) error
			}{
				"JSON": {json.Marshal, json.Unmarshal},
				"YAML": {yaml.Marshal, yaml.Unmarshal},
			} {
				t.Run(name, func(t *testing.T) {
					data, err := codec.marshal(tc.value)
					require.NoError(t, err)
					var text string
					require.NoError(t, codec.unmarshal(data, &text))
					assert.Equal(t, tc.text, text)
					var got ByteSize
					require.NoError(t, codec.unmarshal(data, &got))
					assert.Equal(t, tc.value, got)
					data, err = codec.marshal(tc.value.Bytes())
					require.NoError(t, err)
					require.NoError(t, codec.unmarshal(data, &got))
					assert.Equal(t, tc.value, got)
				})
			}
		})
	}
}

func TestByteSizeUnmarshal(t *testing.T) {
	for name, unmarshal := range map[string]func([]byte, any) error{"JSON": json.Unmarshal, "YAML": yaml.Unmarshal} {
		t.Run(name, func(t *testing.T) {
			for input, want := range map[string]ByteSize{`"1.5 MiB"`: 1572864, `"0.1KB"`: 100, `1024`: KiB, `1024.0`: KiB} {
				var got ByteSize
				require.NoError(t, unmarshal([]byte(input), &got))
				assert.Equal(t, want, got)
			}
			for _, input := range []string{`"1.5B"`, `"bad"`, `true`, `[]`, `{}`, `9223372036854775808`, `1e3`} {
				got := 2 * MiB
				assert.Error(t, unmarshal([]byte(input), &got), input)
				assert.Equal(t, 2*MiB, got)
			}
			got := MiB
			require.NoError(t, unmarshal([]byte("null"), &got))
			assert.Zero(t, got)
		})
	}
}
