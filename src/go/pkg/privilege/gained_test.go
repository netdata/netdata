// SPDX-License-Identifier: GPL-3.0-or-later

package privilege

import (
	"errors"
	"os"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIdentityTransition(t *testing.T) {
	probeErr := errors.New("probe unavailable")
	for name, tc := range map[string]struct {
		ids       [4]int
		probeGain bool
		probeErr  error
		want      bool
		wantProbe bool
	}{
		"setuid":          {ids: [4]int{100, 0, 100, 100}, want: true},
		"setgid":          {ids: [4]int{100, 100, 100, 200}, want: true},
		"root":            {ids: [4]int{}, want: false},
		"ordinary user":   {ids: [4]int{100, 100, 100, 100}, wantProbe: true},
		"capability gain": {ids: [4]int{100, 100, 100, 100}, probeGain: true, want: true, wantProbe: true},
		"probe failure":   {ids: [4]int{100, 100, 100, 100}, probeErr: probeErr, wantProbe: true},
	} {
		t.Run(name, func(t *testing.T) {
			called := false
			got, err := gainedAtExec(tc.ids[0], tc.ids[1], tc.ids[2], tc.ids[3], func() (bool, error) {
				called = true
				return tc.probeGain, tc.probeErr
			})
			assert.Equal(t, tc.want, got)
			assert.Equal(t, tc.wantProbe, called)
			if tc.wantProbe {
				assert.ErrorIs(t, err, tc.probeErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestCurrentProcess(t *testing.T) {
	got, err := GainedAtExec()
	require.NoError(t, err)
	if expected := os.Getenv("PRIVILEGE_TEST_EXPECT_GAIN"); expected != "" {
		want, err := strconv.ParseBool(expected)
		require.NoError(t, err)
		assert.Equal(t, want, got)
	}
	if os.Geteuid() == 0 && os.Getuid() == 0 && os.Getgid() == os.Getegid() {
		assert.False(t, got, "direct root invocation is not a privilege transition")
	}
}
