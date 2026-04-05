// SPDX-License-Identifier: GPL-3.0-or-later

package file

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TODO: tech dept
func TestNewDiscovery(t *testing.T) {

}

// TODO: tech dept
func TestDiscovery_Run(t *testing.T) {

}

func prepareDiscovery(t *testing.T, cfg Config) *Discovery {
	d, err := NewDiscovery(cfg)
	require.NoError(t, err)
	return d
}
