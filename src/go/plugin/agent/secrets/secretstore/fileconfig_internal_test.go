// SPDX-License-Identifier: GPL-3.0-or-later

package secretstore

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/stretchr/testify/require"
)

func TestValidateFileConfigAcceptsWindowsSourcePath(t *testing.T) {
	config := Config{
		"name":            "main",
		"kind":            string(KindVault),
		"__source__":      `file=C:\Program Files\Netdata\etc\netdata\ss\vault.conf`,
		"__source_type__": confgroup.TypeUser,
	}

	require.NoError(t, validateFileConfig(config))
}

func TestValidateFileConfigRefusesUnsafeSource(t *testing.T) {
	config := Config{
		"name":            "main",
		"kind":            string(KindVault),
		"__source__":      `file=C:\`,
		"__source_type__": confgroup.TypeUser,
	}

	require.Error(t, validateFileConfig(config))
}
