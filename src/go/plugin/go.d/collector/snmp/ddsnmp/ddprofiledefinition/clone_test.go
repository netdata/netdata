// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package ddprofiledefinition

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
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
	items := []*cloneMe{
		item("a", 1, 2, 3, 4),
		item("b", 1, 2),
	}
	itemsCopy := cloneSlice(items)
	*itemsCopy[0].label = "aaa"
	itemsCopy[1] = item("bbb", 10, 20)
	itemsCopy = append(itemsCopy, item("ccc", 100, 200))
	// items is unchanged
	assert.Equal(t, []*cloneMe{
		item("a", 1, 2, 3, 4),
		item("b", 1, 2),
	}, items)
	assert.Equal(t, []*cloneMe{
		item("aaa", 1, 2, 3, 4),
		item("bbb", 10, 20),
		item("ccc", 100, 200),
	}, itemsCopy)
}

func TestCloneMap(t *testing.T) {
	m := map[string]*cloneMe{
		"Item A": item("a", 1, 2, 3, 4),
		"Item B": item("b", 1, 2),
	}
	mCopy := cloneMap(m)
	mCopy["Item A"].ps[0] = 100
	mCopy["Item B"] = item("bbb", 10, 20)
	mCopy["Item C"] = item("ccc", 100, 200)
	assert.Equal(t, map[string]*cloneMe{
		"Item A": item("a", 1, 2, 3, 4),
		"Item B": item("b", 1, 2),
	}, m)
	assert.Equal(t, map[string]*cloneMe{
		"Item A": item("a", 100, 2, 3, 4),
		"Item B": item("bbb", 10, 20),
		"Item C": item("ccc", 100, 200),
	}, mCopy)
}

func TestCustomSliceClone(t *testing.T) {
	type customSlice []*cloneMe

	items := customSlice{
		item("a", 1, 2, 3, 4),
		item("b", 1, 2),
	}
	itemsCopy := cloneSlice(items)

	assert.IsType(t, customSlice{}, itemsCopy)
	assert.Equal(t, items, itemsCopy)
}

func TestCustomMapClone(t *testing.T) {
	type customMap map[string]*cloneMe

	m := customMap{
		"Item A": item("a", 1, 2, 3, 4),
		"Item B": item("b", 1, 2),
	}
	mCopy := cloneMap(m)
	assert.IsType(t, customMap{}, mCopy)
	assert.Equal(t, m, mCopy)
}
