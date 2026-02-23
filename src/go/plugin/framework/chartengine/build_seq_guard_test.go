// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestObserveBuildSuccessSeqTransitions(t *testing.T) {
	e, err := New()
	require.NoError(t, err)

	obs := e.observeBuildSuccessSeq(10)
	assert.Equal(t, buildSeqTransitionNone, obs.transition)
	assert.Equal(t, uint64(0), obs.previous)

	obs = e.observeBuildSuccessSeq(10)
	assert.Equal(t, buildSeqTransitionBroken, obs.transition)
	assert.Equal(t, uint64(10), obs.previous)

	obs = e.observeBuildSuccessSeq(10)
	assert.Equal(t, buildSeqTransitionNone, obs.transition)
	assert.Equal(t, uint64(10), obs.previous)

	obs = e.observeBuildSuccessSeq(9)
	assert.Equal(t, buildSeqTransitionNone, obs.transition)
	assert.Equal(t, uint64(10), obs.previous)

	obs = e.observeBuildSuccessSeq(11)
	assert.Equal(t, buildSeqTransitionRecovered, obs.transition)
	assert.Equal(t, uint64(10), obs.previous)

	obs = e.observeBuildSuccessSeq(12)
	assert.Equal(t, buildSeqTransitionNone, obs.transition)
	assert.Equal(t, uint64(11), obs.previous)
}
