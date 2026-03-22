//go:build !windows

// SPDX-License-Identifier: GPL-3.0-or-later

package ndexec

import (
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveDirectKillFallback(t *testing.T) {
	groupKillErr := errors.New("group kill failed")

	tests := map[string]struct {
		killErr   error
		assertErr func(*testing.T, error)
	}{
		"successful direct kill becomes success": {
			assertErr: func(t *testing.T, err error) {
				t.Helper()
				assert.NoError(t, err)
			},
		},
		"process already done preserves os.ErrProcessDone semantics": {
			killErr: os.ErrProcessDone,
			assertErr: func(t *testing.T, err error) {
				t.Helper()
				assert.ErrorIs(t, err, os.ErrProcessDone)
			},
		},
		"direct kill failure is returned": {
			killErr: groupKillErr,
			assertErr: func(t *testing.T, err error) {
				t.Helper()
				assert.ErrorIs(t, err, groupKillErr)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			tc.assertErr(t, resolveDirectKillFallback(tc.killErr))
		})
	}
}
