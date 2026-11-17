// SPDX-License-Identifier: GPL-3.0-or-later

package journalstore

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A container that mounts the host root under the host prefix needs no local
// machine id, and the journal carries the host's id even when it has one.
func TestOpenResolvesMachineIDUnderHostPrefix(t *testing.T) {
	const machineID = "0123456789abcdef0123456789abcdef"
	prefix := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(prefix, "etc"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(prefix, "etc", "machine-id"), []byte(machineID+"\n"), 0644))
	root := t.TempDir()

	s, err := Open(context.Background(), root, prefix)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, s.Close()) })
	appendEntry(t, s)

	assert.FileExists(t, filepath.Join(root, machineID, "dem.journal"))
}
