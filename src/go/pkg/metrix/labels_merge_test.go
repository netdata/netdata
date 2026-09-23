// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import (
	"maps"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAppendCanonicalLabelMatchesCanonicalizeLabels(t *testing.T) {
	tests := map[string]struct {
		base  map[string]string
		extra Label
	}{
		"empty base": {
			extra: Label{Key: "m", Value: "value"},
		},
		"insert at beginning": {
			base:  map[string]string{"m": "middle", "z": "last"},
			extra: Label{Key: "a", Value: "first"},
		},
		"insert in middle": {
			base:  map[string]string{"a": "first", "z": "last"},
			extra: Label{Key: "m", Value: "middle"},
		},
		"insert at end": {
			base:  map[string]string{"a": "first", "m": "middle"},
			extra: Label{Key: "z", Value: "last"},
		},
		"replace duplicate like map assignment": {
			base:  map[string]string{"a": "first", "m": "old", "z": "last"},
			extra: Label{Key: "m", Value: "new"},
		},
		"empty value": {
			base:  map[string]string{"a": "first"},
			extra: Label{Key: "m", Value: ""},
		},
		"packed delimiter in value": {
			base:  map[string]string{"a": "first"},
			extra: Label{Key: "m", Value: "before\xffafter"},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			base, _, err := canonicalizeLabels(tc.base)
			require.NoError(t, err)
			before := append([]Label(nil), base...)

			prefix := []Label{{Key: "prior", Value: "kept"}}
			var key strings.Builder
			key.WriteString("prior-key")
			appended, ok := appendCanonicalLabel(prefix, &key, base, tc.extra)
			require.True(t, ok)
			require.Equal(t, prefix, appended[:len(prefix)], "append changed the existing destination")
			gotLabels := appended[len(prefix):]
			gotKey := strings.TrimPrefix(key.String(), "prior-key")

			reference := make(map[string]string, len(tc.base)+1)
			maps.Copy(reference, tc.base)
			reference[tc.extra.Key] = tc.extra.Value
			wantLabels, wantKey, err := canonicalizeLabels(reference)
			require.NoError(t, err)

			require.Equal(t, wantLabels, gotLabels)
			require.Equal(t, wantKey, gotKey)
			require.Equal(t, before, base, "append mutated canonical source labels")
		})
	}
}

func TestAppendCanonicalLabelRejectsEmptyKey(t *testing.T) {
	var key strings.Builder
	labels, ok := appendCanonicalLabel(nil, &key, []Label{{Key: "a", Value: "value"}}, Label{
		Value: "invalid",
	})
	require.False(t, ok)
	require.Nil(t, labels)
	require.Empty(t, key.String())
}
