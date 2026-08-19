// SPDX-License-Identifier: GPL-3.0-or-later

package graph

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestActorHandleAllocatorSeparatesGenerationsAndContinuesHighWater(t *testing.T) {
	first := NewActorHandleAllocator()
	firstOne := first.Next()
	_ = first.Next()
	firstThree := first.Next()

	second := NewActorHandleAllocator()
	secondOne := second.Next()
	require.False(t, firstOne == secondOne)

	var continued ActorHandleAllocator
	require.NoError(t, continued.Include(firstOne))
	require.NoError(t, continued.Include(firstThree))
	require.Error(t, continued.Include(secondOne))
	require.Error(t, continued.Include(ActorHandle{}))
	require.Equal(t, first.Next(), continued.Next())
}
