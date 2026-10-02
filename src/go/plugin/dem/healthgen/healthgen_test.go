// SPDX-License-Identifier: GPL-3.0-or-later

package healthgen

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSiteLifecycle(t *testing.T) {
	for name, tc := range map[string]struct {
		enabled bool
		debug   bool
	}{
		"enabled":  {enabled: true},
		"disabled": {},
		"debug":    {enabled: true, debug: true},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			owned := filepath.Join(dir, "rum.shop.conf")
			other := filepath.Join(dir, "rum.other.conf")
			require.NoError(t, os.WriteFile(owned, []byte("old output"), 0640))
			require.NoError(t, os.WriteFile(other, []byte("other site"), 0640))
			reloads := make(chan error, 4)
			s, err := NewSite(
				dir,
				config.RumSite{
					Key: "shop",
					Alerts: &config.RumAlerts{
						Enabled: &tc.enabled,
					},
				},
				Options{
					Debug: tc.debug,
					Reload: func(ctx context.Context) error {
						_, deadline := ctx.Deadline()
						if !deadline {
							reloads <- errors.New("reload context has no deadline")
						} else {
							reloads <- ctx.Err()
						}
						return nil
					},
				},
			)
			require.NoError(t, err)
			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan struct{})
			go func() { defer close(done); s.Run(ctx) }()
			t.Cleanup(func() { cancel(); <-done })
			if !tc.debug {
				require.NoError(t, receive(t, reloads))
				if tc.enabled {
					info, err := os.Stat(owned)
					require.NoError(t, err)
					assert.Equal(t, os.FileMode(0640), info.Mode().Perm())
					data, err := os.ReadFile(owned)
					require.NoError(t, err)
					assert.Contains(t, string(data), "on: rum.lcp")
				} else {
					assert.NoFileExists(t, owned)
				}
			}
			cancel()
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("site did not stop")
			}
			if tc.debug {
				data, err := os.ReadFile(owned)
				require.NoError(t, err)
				assert.Equal(t, "old output", string(data))
				assert.Empty(t, reloads)
			} else {
				require.NoError(t, receive(t, reloads)) // detached retirement context is live
				assert.NoFileExists(t, owned)
			}
			data, err := os.ReadFile(other)
			require.NoError(t, err)
			assert.Equal(t, "other site", string(data))
			entries, err := os.ReadDir(dir)
			require.NoError(t, err)
			for _, entry := range entries {
				assert.NotContains(t, entry.Name(), ".rum-")
			}
		})
	}
}

func TestSiteRetriesFailedReloadWithUnchangedFile(t *testing.T) {
	dir := t.TempDir()
	var calls atomic.Int32
	reloaded := make(chan error, 3)
	var firstInfo os.FileInfo
	s, err := NewSite(dir, config.RumSite{
		Key: "shop",
	}, Options{
		Reload: func(context.Context) error {
			call := calls.Add(1)
			if call == 1 {
				var err error
				firstInfo, err = os.Stat(filepath.Join(dir, "rum.shop.conf"))
				reloaded <- err
				return errors.New("unavailable")
			}
			if call == 2 {
				info, err := os.Stat(filepath.Join(dir, "rum.shop.conf"))
				if err == nil && !os.SameFile(firstInfo, info) {
					err = errors.New("unchanged output was rewritten")
				}
				reloaded <- err
			}
			return nil
		},
	})
	require.NoError(t, err)
	s.retry = time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); s.Run(ctx) }()
	t.Cleanup(func() { cancel(); <-done })
	require.NoError(t, receive(t, reloaded))
	require.NoError(t, receive(t, reloaded))
	assert.Equal(t, int32(2), calls.Load())
}

func TestRecoverPreservesUnownedEntries(t *testing.T) {
	for name, debug := range map[string]bool{"normal": false, "debug": true} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			files := []string{
				"rum.shop.conf",
				"rum.a-b_C9.conf",
				"rum..conf",
				"rum.a.b.conf",
				"rum._invalid.conf",
				"other.conf",
				"synthetics.shop.conf",
				".rum-orphan",
			}
			for _, file := range files {
				require.NoError(t, os.WriteFile(filepath.Join(dir, file), []byte(file), 0640))
			}
			require.NoError(t, os.Mkdir(filepath.Join(dir, "rum.directory.conf"), 0750))
			require.NoError(t, os.Symlink(filepath.Join(dir, "other.conf"), filepath.Join(dir, "rum.symlink.conf")))
			require.NoError(t, Recover(dir, debug))
			entries, err := os.ReadDir(dir)
			require.NoError(t, err)
			var names []string
			for _, entry := range entries {
				names = append(names, entry.Name())
			}
			want := []string{
				"rum..conf",
				"rum.a.b.conf",
				"rum._invalid.conf",
				"other.conf",
				"synthetics.shop.conf",
				".rum-orphan",
				"rum.directory.conf",
				"rum.symlink.conf",
			}
			if debug {
				want = append(want, "rum.shop.conf", "rum.a-b_C9.conf")
			}
			assert.ElementsMatch(t, want, names)
		})
	}
	assert.NoError(t, Recover(filepath.Join(t.TempDir(), "absent"), false))
	assert.Error(t, Recover("", false))
	assert.NoError(t, Recover("", true))
}

func TestRecoveryReloadsWithoutFilesAndNeverSweepsActiveJobs(t *testing.T) {
	dir := t.TempDir()
	var calls atomic.Int32
	reloaded := make(chan error, 2)
	r := NewRecovery(dir, Options{
		Reload: func(context.Context) error {
			reloaded <- nil
			if calls.Add(1) == 1 {
				return errors.New("unavailable")
			}
			return nil
		},
	})
	r.retry = time.Millisecond
	require.NoError(t, r.Prepare())
	active := filepath.Join(dir, "rum.active.conf")
	require.NoError(t, os.WriteFile(active, []byte("new generation"), 0640))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); r.Run(ctx) }()
	t.Cleanup(func() { cancel(); <-done })
	require.NoError(t, receive(t, reloaded))
	require.NoError(t, receive(t, reloaded))
	assert.FileExists(t, active)
}

func TestRecoveryDebugDoesNotReload(t *testing.T) {
	r := NewRecovery(
		"",
		Options{
			Debug:  true,
			Reload: func(context.Context) error { t.Error("debug reload"); return nil },
		},
	)
	require.NoError(t, r.Prepare())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r.Run(ctx)
}

func TestRenderSitePolicyAndIdentity(t *testing.T) {
	name := "shop\n\r\u2028secret\\"
	threshold := &config.RumThreshold{
		Warn: .123456,
		Crit: .234567,
	}
	site := config.RumSite{
		Key:  "shop-a",
		Name: name,
		Alerts: &config.RumAlerts{
			CLS: threshold,
		},
	}
	s, err := NewSite(
		t.TempDir(),
		site,
		Options{
			Redact: func(v string) string { return strings.ReplaceAll(v, "secret", "[redacted]") },
		},
	)
	require.NoError(t, err)
	threshold.Warn = 99 // constructor must snapshot policy
	text := string(s.content)
	assert.Equal(t, 4, strings.Count(text, "template:"))
	assert.Equal(t, 4, strings.Count(text, "chart labels: _collect_plugin=dem _collect_job=shop-a"))
	for _, line := range []string{"on: rum.lcp", "on: rum.inp", "on: rum.cls", "on: rum.pageviews", "($this > 2500)", "($this > 4000)", "($this > 200)", "($this > 500)", "($this > 0.123456)", "($this > 0.234567)", "sum -1h unaligned of views", "average -10m unaligned of p75", "($this == nan or $this == inf) ? (nan)"} {
		assert.Contains(t, text, line)
	}
	assert.NotContains(t, text, "secret")
	assert.NotContains(t, text, "\r")
	assert.NotContains(t, text, "\u2028")
	assert.NotContains(t, text, "\\") // health treats a trailing backslash as line continuation
	assert.Contains(t, text, "shop   [redacted]")
	other, err := NewSite(t.TempDir(), config.RumSite{
		Key: "shop_a",
	}, Options{})
	require.NoError(t, err)
	ids := func(content []byte) []string {
		var out []string
		for _, line := range strings.Split(string(content), "\n") {
			if strings.HasPrefix(line, "template:") {
				out = append(out, line)
			}
		}
		return out
	}
	for _, id := range ids(s.content) {
		assert.NotContains(t, ids(other.content), id)
	}
}

func TestInvalidSiteKeyCannotEscapeDirectory(t *testing.T) {
	for _, key := range []string{"", "../a", "a/b", "a.b", "a*", "a b", "_a"} {
		t.Run(key, func(t *testing.T) {
			_, err := NewSite(t.TempDir(), config.RumSite{
				Key: key,
			}, Options{})
			assert.Error(t, err)
		})
	}
}

func TestOversizedMetadataCannotSplitHealthDirectives(t *testing.T) {
	_, err := NewSite(
		t.TempDir(),
		config.RumSite{
			Key:  "shop",
			Name: strings.Repeat("x", 4096) + "template: injected",
		},
		Options{},
	)
	assert.Error(t, err)
}

func receive(t *testing.T, ch <-chan error) error {
	t.Helper()
	select {
	case err := <-ch:
		return err
	case <-time.After(time.Second):
		t.Fatal("reload did not run")
		return nil
	}
}

func TestSiteKeyFitsGeneratedFilename(t *testing.T) {
	_, err := NewSite(t.TempDir(), config.RumSite{
		Key: strings.Repeat("x", 247),
	}, Options{})
	require.ErrorContains(t, err, "246")
	valid, err := NewSite(t.TempDir(), config.RumSite{
		Key: strings.Repeat("x", 246),
	}, Options{})
	require.NoError(t, err)
	require.NoError(t, valid.sync())
	_, err = os.Stat(valid.path)
	require.NoError(t, err)
}
