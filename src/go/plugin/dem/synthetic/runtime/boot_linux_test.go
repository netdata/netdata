// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestKernelBootProvenance(t *testing.T) {
	for name, fixture := range map[string]struct {
		mount string
		valid bool
	}{
		"normal":           {"20 1 0:1 / /proc rw - proc proc rw", true},
		"readonly-sys":     {"20 1 0:1 /sys /proc/sys ro - proc proc rw", true},
		"container-file":   {"20 1 0:2 /boot-id /proc/sys/kernel/random/boot_id ro - tmpfs tmpfs rw", false},
		"random-uuid-bind": {"20 1 0:1 /sys/kernel/random/uuid /proc/sys/kernel/random/boot_id ro - proc proc rw", false},
		"wrong-id":         {"21 1 0:1 / /proc rw - proc proc rw", false},
	} {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, fixture.valid, kernelBootMount(strings.NewReader(fixture.mount+"\n"), 20, "/proc/sys/kernel/random/boot_id"))
		})
	}
	actual := hostBoot()
	if actual.Verified {
		require.True(t, validBootID(actual.ID))
	}
	s, now := testStore(t)
	m := importFixture(t, s, "boot")
	s.ledger.boot = bootIdentity{ID: "11111111-1111-1111-1111-111111111111", Verified: true}
	use, err := s.beginUse(m)
	require.NoError(t, err)
	require.NoError(t, use.Finish(false))
	*now = now.Add(100 * 24 * time.Hour)
	s.ledger.boot = bootIdentity{ID: "22222222-2222-2222-2222-222222222222", Verified: false}
	require.NoError(t, s.Sweep())
	require.FileExists(t, filepath.Join(s.path, "generations", m.Archive.SHA256, "prepared.json"))
	s.ledger.boot.Verified = true
	require.NoError(t, s.Sweep())
	*now = now.Add(8 * 24 * time.Hour)
	require.NoError(t, s.Sweep())
	_, err = s.beginUse(m)
	require.ErrorIs(t, err, ErrUnavailable)
}
