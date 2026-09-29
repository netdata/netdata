// SPDX-License-Identifier: GPL-3.0-or-later

package native

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// Measures the real serialized collection transport and parser with one sample.
// Cost is O(frame bytes + samples), independent of retained metric series.
func BenchmarkPersistentCollection(b *testing.B) {
	setupRunner(b)
	dir := b.TempDir()
	require.NoError(b, os.WriteFile(filepath.Join(dir, "peer.sh"), []byte(`#!/bin/bash
set -eu
printf '%s\n' '{"version":"v1","ready":true}'
pattern='^\{"id":"([1-9][0-9]*)","method":"collect"\}$'
while IFS= read -r line; do
 [[ $line =~ $pattern ]] || exit 2
 printf '{"id":"%s","result":{"version":"v1","metrics":[{"name":"depth","value":42}],"checks":[]}}\n' "${BASH_REMATCH[1]}"
done
`), 0755))
	manifest := filepath.Join(dir, "manifest.yaml")
	require.NoError(b, os.WriteFile(manifest, []byte(
		"version: v1\nmode: persistent\ncommand: [./peer.sh]\nmetrics: [{name: depth, type: gauge, unit: jobs}]\n",
	), 0644))
	c := newManifestCollector(manifest)
	require.NoError(b, c.Init(context.Background()))
	startRuntime(b, c).waitReady(b)
	b.ReportAllocs()
	for b.Loop() {
		if _, err := c.collectPersistent(context.Background()); err != nil {
			b.Fatal(err)
		}
	}
}
