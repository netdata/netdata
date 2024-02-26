// SPDX-License-Identifier: GPL-3.0-or-later

package module

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMockModule_Init(t *testing.T) {
	m := &MockModule{}

	assert.NoError(t, m.Init())
	m.InitFunc = func() error { return nil }
	assert.NoError(t, m.Init())
}

func TestMockModule_Check(t *testing.T) {
	m := &MockModule{}

	assert.NoError(t, m.Check())
	m.CheckFunc = func() error { return nil }
	assert.NoError(t, m.Check())
}

func TestMockModule_Charts(t *testing.T) {
	m := &MockModule{}
	c := &Charts{}

	assert.Nil(t, m.Charts())
	m.ChartsFunc = func() *Charts { return c }
	assert.True(t, c == m.Charts())
}

func TestMockModule_Collect(t *testing.T) {
	m := &MockModule{}
	d := map[string]int64{
		"1": 1,
	}

	assert.Nil(t, m.Collect())
	m.CollectFunc = func() map[string]int64 { return d }
	assert.Equal(t, d, m.Collect())
}

func TestMockModule_Cleanup(t *testing.T) {
	m := &MockModule{}
	require.False(t, m.CleanupDone)

	m.Cleanup()
	assert.True(t, m.CleanupDone)
}
