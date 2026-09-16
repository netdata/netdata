// SPDX-License-Identifier: GPL-3.0-or-later

package tlscfg

import (
	"context"
	"encoding/pem"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/credentialfile/testutil"
	"github.com/netdata/netdata/go/plugins/pkg/safefile"
	"github.com/stretchr/testify/require"
)

func TestTLSReaderRequired(t *testing.T) {
	_, err := newTLSConfig(context.Background(), TLSConfig{TLSCA: "synthetic-ca"}, nil)
	require.ErrorIs(t, err, ErrTLSFile)
	require.ErrorIs(t, err, safefile.ErrFile)
	require.NotErrorIs(t, err, fs.ErrNotExist)
}

func TestTLSParserDoesNotDisclosePEMLabels(t *testing.T) {
	const secret = "SYNTHETIC PRIVATE FILE CONTENT"
	path := filepath.Join(t.TempDir(), "input.pem")
	require.NoError(
		t,
		os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: secret, Bytes: []byte("synthetic")}), 0600),
	)
	_, err := newTLSConfig(context.Background(), TLSConfig{TLSCert: path, TLSKey: path}, testutil.New().Read)
	require.ErrorIs(t, err, ErrTLSFile)
	require.NotContains(t, err.Error(), secret)
}
