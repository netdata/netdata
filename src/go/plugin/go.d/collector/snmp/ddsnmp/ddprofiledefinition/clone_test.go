// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package ddprofiledefinition

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type cloneMe struct {
	label *string
	ps    []int
}

func item(label string, ps ...int) *cloneMe {
	return &cloneMe{
		label: &label,
		ps:    ps,
	}
}

func (c *cloneMe) Clone() *cloneMe {
	c2 := &cloneMe{
		ps: slices.Clone(c.ps),
	}
	if c.label != nil {
		var tmp = *c.label
		c2.label = &tmp
	}
	return c2
}

func TestCloneSlice(t *testing.T) {
	tests := map[string]struct {
		input        []*cloneMe
		wantOriginal []*cloneMe
		wantClone    []*cloneMe
		mutate       func([]*cloneMe) []*cloneMe
	}{
		"nil":   {},
		"empty": {input: []*cloneMe{}, wantOriginal: []*cloneMe{}, wantClone: []*cloneMe{}},
		"nested mutation": {
			input:        []*cloneMe{item("a", 1, 2, 3, 4), item("b", 1, 2)},
			wantOriginal: []*cloneMe{item("a", 1, 2, 3, 4), item("b", 1, 2)},
			wantClone:    []*cloneMe{item("aaa", 1, 2, 3, 4), item("bbb", 10, 20), item("ccc", 100, 200)},
			mutate: func(cloned []*cloneMe) []*cloneMe {
				*cloned[0].label = "aaa"
				cloned[1] = item("bbb", 10, 20)
				return append(cloned, item("ccc", 100, 200))
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			cloned := cloneSlice(tc.input)
			require.Equal(t, tc.wantOriginal, cloned)
			if tc.mutate != nil {
				cloned = tc.mutate(cloned)
			}
			assert.Equal(t, tc.wantOriginal, tc.input)
			assert.Equal(t, tc.wantClone, cloned)
		})
	}
}

func TestCloneMap(t *testing.T) {
	tests := map[string]struct {
		input        map[string]*cloneMe
		wantOriginal map[string]*cloneMe
		wantClone    map[string]*cloneMe
		mutate       func(map[string]*cloneMe)
	}{
		"nil":   {},
		"empty": {input: map[string]*cloneMe{}, wantOriginal: map[string]*cloneMe{}, wantClone: map[string]*cloneMe{}},
		"nested mutation": {
			input:        map[string]*cloneMe{"Item A": item("a", 1, 2, 3, 4), "Item B": item("b", 1, 2)},
			wantOriginal: map[string]*cloneMe{"Item A": item("a", 1, 2, 3, 4), "Item B": item("b", 1, 2)},
			wantClone: map[string]*cloneMe{
				"Item A": item("a", 100, 2, 3, 4),
				"Item B": item("bbb", 10, 20),
				"Item C": item("ccc", 100, 200),
			},
			mutate: func(cloned map[string]*cloneMe) {
				cloned["Item A"].ps[0] = 100
				cloned["Item B"] = item("bbb", 10, 20)
				cloned["Item C"] = item("ccc", 100, 200)
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			cloned := cloneMap(tc.input)
			require.Equal(t, tc.wantOriginal, cloned)
			if tc.mutate != nil {
				tc.mutate(cloned)
			}
			assert.Equal(t, tc.wantOriginal, tc.input)
			assert.Equal(t, tc.wantClone, cloned)
		})
	}
}

func TestCloneNamedContainers(t *testing.T) {
	type customSlice []*cloneMe
	type customMap map[string]*cloneMe
	tests := map[string]struct {
		clone func() any
		want  any
	}{
		"slice": {
			clone: func() any { return cloneSlice(customSlice{item("a", 1, 2, 3, 4), item("b", 1, 2)}) },
			want:  customSlice{item("a", 1, 2, 3, 4), item("b", 1, 2)},
		},
		"map": {
			clone: func() any {
				return cloneMap(customMap{
					"Item A": item("a", 1, 2, 3, 4),
					"Item B": item("b", 1, 2),
				})
			},
			want: customMap{
				"Item A": item("a", 1, 2, 3, 4),
				"Item B": item("b", 1, 2),
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) { assert.Equal(t, tc.want, tc.clone()) })
	}
}
