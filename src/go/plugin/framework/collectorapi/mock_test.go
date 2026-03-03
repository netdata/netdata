// SPDX-License-Identifier: GPL-3.0-or-later

package collectorapi

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMockModule_Init(t *testing.T) {
	m := &MockCollectorV1{}
	ctx := context.Background()

	assert.NoError(t, m.Init(ctx))
	m.InitFunc = func(context.Context) error { return nil }
	assert.NoError(t, m.Init(ctx))
}

func TestMockModule_Check(t *testing.T) {
	m := &MockCollectorV1{}
	ctx := context.Background()

	assert.NoError(t, m.Check(ctx))
	m.CheckFunc = func(context.Context) error { return nil }
	assert.NoError(t, m.Check(ctx))
}

func TestMockModule_Charts(t *testing.T) {
	m := &MockCollectorV1{}
	c := &Charts{}

	assert.Nil(t, m.Charts())
	m.ChartsFunc = func() *Charts { return c }
	assert.True(t, c == m.Charts())
}

func TestMockModule_Collect(t *testing.T) {
	m := &MockCollectorV1{}
	d := map[string]int64{
		"1": 1,
	}
	ctx := context.Background()

	assert.Nil(t, m.Collect(ctx))
	m.CollectFunc = func(ctx context.Context) map[string]int64 { return d }
	assert.Equal(t, d, m.Collect(ctx))
}

func TestMockModule_Cleanup(t *testing.T) {
	m := &MockCollectorV1{}
	ctx := context.Background()

	require.False(t, m.CleanupDone)

	m.Cleanup(ctx)
	assert.True(t, m.CleanupDone)
}
