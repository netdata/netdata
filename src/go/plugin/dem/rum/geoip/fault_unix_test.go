// SPDX-License-Identifier: GPL-3.0-or-later
//go:build unix

package geoip

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

// Run genuine mapped faults in a subprocess: a regression must fail the test,
// rather than terminate the whole package's test process.
func TestMappedFaultSubprocess(t *testing.T) {
	if phase := os.Getenv("DEM_GEOIP_FAULT_TEST"); phase != "" {
		path := filepath.Join(t.TempDir(), "active.mmdb")
		data, err := os.ReadFile(fixture)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(path, data, 0600))
		prior := os.Getenv("DEM_GEOIP_PRIOR_FAULT") == "true"
		previous := debug.SetPanicOnFault(prior)
		defer debug.SetPanicOnFault(previous)
		if phase == "construction" {
			f, err := openFile(path)
			require.NoError(t, err)
			region, err := mapFile(f, len(data))
			require.NoError(t, err)
			require.NoError(t, f.Close())
			raw := region.data
			require.NoError(t, os.Truncate(path, 0))
			db, err := decodeDatabase(region)
			require.ErrorIs(t, err, errMappedFault)
			require.Nil(t, db)
			assert.Error(t, unix.Munmap(raw), "failed construction must already have released the mapping")
		} else {
			stock := filepath.Join(t.TempDir(), "stock.mmdb")
			require.NoError(t, os.WriteFile(stock, data, 0600))
			r := New("", Paths{
				Cache: path,
				Stock: stock,
			})
			defer r.Close()
			require.NoError(t, r.Refresh())
			raw := r.current.db.mapping.data
			require.NoError(t, os.Truncate(path, 0))
			c, city, lat, lon, ok := r.Lookup("81.2.69.142")
			assert.Empty(t, c)
			assert.Empty(t, city)
			assert.Zero(t, lat)
			assert.Zero(t, lon)
			assert.False(t, ok)
			assert.Equal(t, "unavailable", r.Status().State)
			assert.Equal(t, uint64(1), r.Status().LookupErrors)
			select {
			case <-r.Faults():
			default:
				t.Fatal("missing refresh wakeup")
			}
			// Another lookup must skip the tainted mapping.
			r.Lookup("81.2.69.142")
			assert.Equal(t, uint64(1), r.Status().LookupErrors)
			require.Error(t, r.Refresh())
			assert.Equal(t, "stock", r.Status().Source)
			c, _, _, _, _ = r.Lookup("81.2.69.142")
			assert.Equal(t, "GB", c)
			assert.Error(t, unix.Munmap(raw), "faulted source must have been released after fallback")
		}
		assert.Equal(t, prior, debug.SetPanicOnFault(prior), "guard must restore the caller's setting")
		return
	}
	executable, err := os.Executable()
	require.NoError(t, err)
	for _, phase := range []string{"construction", "lookup"} {
		for _, prior := range []string{"false", "true"} {
			t.Run(phase+"/"+prior, func(t *testing.T) {
				cmd := exec.Command(executable, "-test.run=^TestMappedFaultSubprocess$", "-test.timeout=20s")
				cmd.Env = append(os.Environ(), "DEM_GEOIP_FAULT_TEST="+phase, "DEM_GEOIP_PRIOR_FAULT="+prior)
				output, err := cmd.CombinedOutput()
				require.NoError(t, err, "%s", output)
			})
		}
	}
}

func TestFaultGuardRethrowsUnrelatedPanics(t *testing.T) {
	for _, prior := range []bool{false, true} {
		previous := debug.SetPanicOnFault(prior)
		region := &mapping{
			data: []byte{1},
		}
		assert.PanicsWithValue(
			t,
			"programming error",
			func() { _ = region.read(func() error { panic("programming error") }) },
		)
		assert.Panics(t, func() { _ = region.read(func() error { var nilPointer *byte; _ = *nilPointer; return nil }) })
		assert.Equal(t, prior, debug.SetPanicOnFault(previous))
	}
}

func TestFIFOIsRejectedWithoutBlocking(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fifo")
	require.NoError(t, unix.Mkfifo(path, 0600))
	r := New(path, Paths{})
	defer r.Close()
	require.Error(t, r.Refresh())
	assert.Equal(t, "unavailable", r.Status().State)
}
