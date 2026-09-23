// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"bytes"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChartIDCollisionWarning(t *testing.T) {
	const warning = "chartengine: dropped "
	documentSet := func(t *testing.T) *TemplateSet {
		spec := charttpl.Spec{
			Version: charttpl.VersionV1,
		}
		for _, entry := range collisionEntries([]string{"first", "second"}, "first", "second") {
			spec.Groups = append(spec.Groups, entry.Groups[0])
		}
		data, err := spec.MarshalTemplate()
		require.NoError(t, err)
		set, err := NewTemplateSetYAML([]byte(data))
		require.NoError(t, err)
		return set
	}

	tests := map[string]struct {
		set  func(t *testing.T) *TemplateSet
		want string
	}{
		"native entries": {
			set: func(t *testing.T) *TemplateSet {
				return testTemplateSet(t, collisionEntries([]string{"a", "b"}, "a", "b")...)
			},
			want: "dropped 1 series routes to charts owned by another template (chart 'shared' owned by entry 'a' chart g0.c0, rejected entry 'b' chart g0.c0)",
		},
		"yaml document": {
			set:  documentSet,
			want: "dropped 1 series routes to charts owned by another template (chart 'shared' owned by template g0.c0, rejected template g1.c0)",
		},
		"no collision": {
			set: func(t *testing.T) *TemplateSet {
				return testTemplateSet(t, collisionEntries([]string{"a", "b"}, "a")...)
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var logs bytes.Buffer
			e, err := New(WithRuntimeStore(nil), WithLogger(logger.NewWithWriter(&logs)))
			require.NoError(t, err)
			store := metrix.NewCollectorStore()
			set := tc.set(t)
			// The second cycle collides again against the materialized owner; the
			// warning stays rate limited.
			for range 2 {
				attempt := templateAttempt(t, e, store, set, map[string]float64{"requests": 1})
				require.NoError(t, attempt.Commit())
			}

			if tc.want == "" {
				assert.NotContains(t, logs.String(), warning)
				return
			}
			assert.Equal(t, 1, strings.Count(logs.String(), warning))
			assert.Contains(t, logs.String(), tc.want)
		})
	}
}
