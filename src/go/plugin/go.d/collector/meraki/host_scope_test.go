// SPDX-License-Identifier: GPL-3.0-or-later

package meraki

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeviceHostIdentity(t *testing.T) {
	first := deviceHostScope("org", device{
		serial:    "S1",
		name:      "Old",
		networkID: "N1",
	})
	second := deviceHostScope("org", device{
		serial:    "S1",
		name:      "New",
		networkID: "N2",
	})
	assert.Equal(t, "1acd32e0-81c6-5eaa-a7b9-7c99ec62bfe9", first.GUID)
	assert.Equal(t, first.GUID, second.GUID)
	assert.Equal(t, first.GUID, first.ScopeKey)
	assert.NotEqual(t, first.GUID, deviceHostScope("other", device{
		serial: "S1",
	}).GUID)
	assert.NotEqual(
		t,
		deviceHostScope("a", device{
			serial: "b:c",
		}).GUID,
		deviceHostScope("a:b", device{
			serial: "c",
		}).GUID,
	)
	badName := deviceHostScope("org", device{
		serial: "S1",
		name:   "invalid\nname",
	})
	_, err := uuid.Parse(badName.GUID)
	require.NoError(t, err)
	assert.Equal(t, "meraki-"+first.GUID, badName.Hostname)
}
