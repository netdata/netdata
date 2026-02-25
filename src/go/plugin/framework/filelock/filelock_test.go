// SPDX-License-Identifier: GPL-3.0-or-later

package filelock

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	assert.NotNil(t, New(""))
}

func TestLocker_Lock(t *testing.T) {
	tests := map[string]func(t *testing.T, dir string){
		"register a lock": func(t *testing.T, dir string) {
			reg := New(dir)

			ok, err := reg.Lock("name")
			assert.True(t, ok)
			assert.NoError(t, err)
		},
		"register the same lock twice": func(t *testing.T, dir string) {
			reg := New(dir)

			ok, err := reg.Lock("name")
			require.True(t, ok)
			require.NoError(t, err)

			ok, err = reg.Lock("name")
			assert.True(t, ok)
			assert.NoError(t, err)
		},
		"failed to register locked by other process lock": func(t *testing.T, dir string) {
			reg1 := New(dir)
			reg2 := New(dir)

			ok, err := reg1.Lock("name")
			require.True(t, ok)
			require.NoError(t, err)

			ok, err = reg2.Lock("name")
			assert.False(t, ok)
			assert.NoError(t, err)
		},
		"failed to register because a directory doesnt exist": func(t *testing.T, dir string) {
			reg := New(dir + dir)

			ok, err := reg.Lock("name")
			assert.False(t, ok)
			assert.Error(t, err)
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			dir, err := os.MkdirTemp(os.TempDir(), "netdata-go-test-file-lock-registry")
			require.NoError(t, err)
			defer func() { require.NoError(t, os.RemoveAll(dir)) }()

			test(t, dir)
		})
	}
}

func TestLocker_Unlock(t *testing.T) {
	tests := map[string]func(t *testing.T, dir string){
		"unregister a lock": func(t *testing.T, dir string) {
			reg := New(dir)

			ok, err := reg.Lock("name")
			require.True(t, ok)
			require.NoError(t, err)
			reg.Unlock("name")

			assert.False(t, reg.isLocked("name"))
		},
		"unregister not registered lock": func(t *testing.T, dir string) {
			reg := New(dir)

			reg.Unlock("name")

			assert.False(t, reg.isLocked("name"))
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			dir, err := os.MkdirTemp(os.TempDir(), "netdata-go-test-file-lock-registry")
			require.NoError(t, err)
			defer func() { require.NoError(t, os.RemoveAll(dir)) }()

			test(t, dir)
		})
	}
}

func TestLocker_UnlockAll(t *testing.T) {
	tests := map[string]func(t *testing.T, dir string){
		"unlock all": func(t *testing.T, dir string) {
			reg := New(dir)

			ok, err := reg.Lock("name1")
			require.True(t, ok)
			require.NoError(t, err)

			ok, err = reg.Lock("name2")
			require.True(t, ok)
			require.NoError(t, err)

			reg.UnlockAll()

			assert.False(t, reg.isLocked("name1"))
			assert.False(t, reg.isLocked("name2"))
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			dir, err := os.MkdirTemp(os.TempDir(), "netdata-go-test-file-lock-registry")
			require.NoError(t, err)
			defer func() { require.NoError(t, os.RemoveAll(dir)) }()

			test(t, dir)
		})
	}
}
