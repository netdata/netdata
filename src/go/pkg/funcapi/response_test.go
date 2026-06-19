// SPDX-License-Identifier: GPL-3.0-or-later

package funcapi

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMethodConfig_WithPresentationReturnsUpdatedCopy(t *testing.T) {
	cfg := MethodConfig{ID: "topology"}

	updated := cfg.WithPresentation(map[string]any{"mode": "graph"})

	assert.Nil(t, cfg.Presentation())
	assert.Equal(t, map[string]any{"mode": "graph"}, updated.Presentation())
}
