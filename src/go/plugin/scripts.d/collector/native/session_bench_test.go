// SPDX-License-Identifier: GPL-3.0-or-later

package native

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
	"github.com/stretchr/testify/require"
)

// Measures the real serialized collection transport and parser with one sample.
// Cost is O(frame bytes + samples), independent of retained metric series.
func BenchmarkPersistentCollection(b *testing.B) {
	if runtime.GOOS == "windows" {
		b.Skip("benchmark peer uses Bash")
	}
	dir := b.TempDir()
	shim := filepath.Join(dir, "nd-run")
	require.NoError(b, os.WriteFile(shim, []byte("#!/bin/sh\nexec \"$@\"\n"), 0755))
	b.Cleanup(ndexec.SetRunnerPathsForTests(shim, ""))
	script := filepath.Join(dir, "peer.sh")
	require.NoError(b, os.WriteFile(script, []byte(`#!/bin/bash
set -eu
printf '%s\n' '{"version":"v1","ready":true}'
pattern='^\{"id":"([1-9][0-9]*)","method":"collect"\}$'
while IFS= read -r line; do
 [[ $line =~ $pattern ]] || exit 2
 printf '{"id":"%s","result":{"version":"v1","metrics":[{"name":"depth","value":42}],"checks":[]}}\n' "${BASH_REMATCH[1]}"
done
`), 0755))
	manifest := filepath.Join(dir, "manifest.yaml")
	require.NoError(
		b,
		os.WriteFile(
			manifest,
			[]byte(
				"version: v1\nmode: persistent\ncommand: [./peer.sh]\nmetrics: [{name: depth, type: gauge, unit: jobs}]\n",
			),
			0644,
		),
	)
	c := New()
	c.Manifest = manifest
	c.validateExecutable = func(path string) (string, error) { return path, nil }
	require.NoError(b, c.Init(context.Background()))
	ctx, cancel := context.WithCancel(context.Background())
	ready := make(chan struct{})
	done := make(chan error, 1)
	go func() { done <- c.Run(ctx, func() { close(ready) }) }()
	b.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			b.Error("peer did not stop")
		}
	})
	select {
	case <-ready:
	case err := <-done:
		b.Fatalf("peer startup: %v", err)
	case <-time.After(3 * time.Second):
		b.Fatal("peer startup timed out")
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := c.collectPersistent(context.Background())
		if err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
}
