// SPDX-License-Identifier: GPL-3.0-or-later

package sdext

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRegistry_DockerInclusion(t *testing.T) {
	withDocker := Registry(true)
	assert.Contains(t, withDocker.Types(), discovererHTTP)
	assert.Contains(t, withDocker.Types(), discovererDocker)

	withoutDocker := Registry(false)
	assert.Contains(t, withoutDocker.Types(), discovererHTTP)
	assert.NotContains(t, withoutDocker.Types(), discovererDocker)
}
