// SPDX-License-Identifier: GPL-3.0-or-later

package mysql

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitMyCNFErrorsDoNotExposeContent(t *testing.T) {
	const marker = "private_config_marker"
	for name, content := range map[string]string{
		"INI syntax":  marker + "\n",
		"derived DSN": "[client]\ndatabase=%" + marker + "\n",
	} {
		t.Run(name, func(t *testing.T) {
			filename := filepath.Join(t.TempDir(), "my.cnf")
			require.NoError(t, os.WriteFile(filename, []byte(content), 0600))
			c := New()
			c.MyCNF = filename
			err := c.Init(t.Context())
			require.Error(t, err)
			for cause := err; cause != nil; cause = errors.Unwrap(cause) {
				assert.NotContains(t, cause.Error(), marker)
			}
		})
	}
}

func TestMyCNFPreservesFileErrors(t *testing.T) {
	_, err := dsnFromFile(filepath.Join(t.TempDir(), "missing.cnf"))
	require.ErrorIs(t, err, os.ErrNotExist)
}
