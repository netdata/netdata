// SPDX-License-Identifier: GPL-3.0-or-later

package diagnostic

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testLeaf struct {
	ID   string            `json:"id"`
	Tags map[string]string `json:"tags"`
}

func (v testLeaf) Validate() error { return validateID("test leaf", v.ID) }

func testReaderLimits() ReaderLimits {
	return ReaderLimits{
		MaxStoredBytes:    1 << 20,
		MaxLogicalBytes:   1 << 20,
		MaxMemberBytes:    1 << 18,
		MaxMembers:        128,
		MaxDevices:        32,
		MaxProfiles:       128,
		MaxRows:           4096,
		MaxTags:           4096,
		MaxStringBytes:    1 << 17,
		MaxDNSRecords:     1024,
		MaxOUIRecords:     1024,
		MaxReferenceEdges: 1024,
		MaxNestingDepth:   32,
		MaxJSONTokens:     1 << 15,
		MaxReplayWork:     1 << 20,
	}
}

func TestSeal_DomainSeparatesMemberIdentity(t *testing.T) {
	t.Parallel()

	value := testLeaf{ID: "leaf-a", Tags: map[string]string{"z": "last", "a": "first"}}
	refA, dataA, err := Seal(MemberType{Kind: "semantic_leaf", Schema: "1"}, value)
	require.NoError(t, err)
	refB, dataB, err := Seal(MemberType{Kind: "observation_leaf", Schema: "1"}, value)
	require.NoError(t, err)

	assert.Equal(t, dataA, dataB)
	assert.NotEqual(t, refA.SHA256, refB.SHA256)
	assert.Equal(t, `{"id":"leaf-a","tags":{"a":"first","z":"last"}}`, string(dataA))
	require.NoError(t, VerifyContent(refA, dataA))
}

func TestDecodeCanonical_RejectsNonCanonicalAndHostileJSON(t *testing.T) {
	t.Parallel()

	limits := testReaderLimits()
	tests := map[string]struct {
		data    []byte
		mutate  func(*ReaderLimits)
		wantErr string
	}{
		"duplicate key": {
			data:    []byte(`{"id":"leaf-a","id":"leaf-b","tags":{}}`),
			wantErr: "duplicate JSON object key",
		},
		"unknown key": {
			data:    []byte(`{"extra":true,"id":"leaf-a","tags":{}}`),
			wantErr: "unknown field",
		},
		"noncanonical object order": {
			data:    []byte(`{"tags":{},"id":"leaf-a"}`),
			wantErr: "not canonical",
		},
		"trailing value": {
			data:    []byte(`{"id":"leaf-a","tags":{}} {}`),
			wantErr: "multiple JSON values",
		},
		"invalid utf8": {
			data:    []byte{'{', '"', 'i', 'd', '"', ':', '"', 0xff, '"', '}'},
			wantErr: "valid UTF-8",
		},
		"token budget": {
			data: []byte(`{"id":"leaf-a","tags":{"a":"1","b":"2"}}`),
			mutate: func(l *ReaderLimits) {
				l.MaxJSONTokens = 3
			},
			wantErr: "JSON tokens",
		},
		"string budget": {
			data: []byte(`{"id":"leaf-a","tags":{"a":"123456789"}}`),
			mutate: func(l *ReaderLimits) {
				l.MaxStringBytes = 4
			},
			wantErr: "JSON string bytes",
		},
		"nesting budget": {
			data: []byte(`{"id":"leaf-a","tags":{}}`),
			mutate: func(l *ReaderLimits) {
				l.MaxNestingDepth = 1
			},
			wantErr: "nesting depth",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			limits := limits
			if tc.mutate != nil {
				tc.mutate(&limits)
			}
			var got testLeaf
			err := DecodeCanonical(tc.data, limits, &got)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestDecodeCanonical_AcceptsCanonicalJSON(t *testing.T) {
	t.Parallel()

	want := testLeaf{ID: "leaf-a", Tags: map[string]string{"a": "first", "z": "last"}}
	data, err := CanonicalBytes(want)
	require.NoError(t, err)
	var got testLeaf
	require.NoError(t, DecodeCanonical(data, testReaderLimits(), &got))
	assert.Equal(t, want, got)
}

func FuzzDecodeCanonical(f *testing.F) {
	f.Add([]byte(`{"id":"leaf-a","tags":{"role":"switch"}}`))
	f.Add([]byte(`{"id":"leaf-a","id":"leaf-b","tags":{}}`))
	f.Add(bytes.Repeat([]byte("["), 64))

	f.Fuzz(func(t *testing.T, data []byte) {
		limits := testReaderLimits()
		limits.MaxMemberBytes = 4096
		limits.MaxLogicalBytes = 4096
		var value testLeaf
		err := DecodeCanonical(data, limits, &value)
		if err == nil {
			canonical, marshalErr := CanonicalBytes(value)
			require.NoError(t, marshalErr)
			require.Equal(t, string(canonical), string(data))
			require.False(t, strings.Contains(string(data), "\n"))
		}
	})
}
