// SPDX-License-Identifier: GPL-3.0-or-later

package promprofiles

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/buildinfo"
	"github.com/netdata/netdata/go/plugins/pkg/executable"
	"github.com/netdata/netdata/go/plugins/pkg/pluginconfig"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigRootSelection(t *testing.T) {
	mode := os.Getenv("NETDATA_TEST_CONFIG_ROOT_SELECTION")
	if mode == "" {
		binary, err := os.Executable()
		require.NoError(t, err)
		for _, mode := range []string{"installed", "development"} {
			t.Run(mode, func(t *testing.T) {
				cmd := exec.Command(binary, "-test.run=^TestConfigRootSelection$")
				cmd.Env = append(os.Environ(), "NETDATA_TEST_CONFIG_ROOT_SELECTION="+mode)
				output, err := cmd.CombinedOutput()
				require.NoError(t, err, "%s", output)
			})
		}
		return
	}
	root := t.TempDir()
	user, stock := filepath.Join(root, "user"), filepath.Join(root, "stock")
	buildinfo.UserConfigDir, buildinfo.StockConfigDir = user, stock
	executable.Name, executable.Directory = "go.d", filepath.Join(root, "bin")
	development := filepath.Join(root, "config", "go.d", "prometheus.profiles", "default")
	require.NoError(t, os.MkdirAll(development, 0755))
	t.Setenv("NETDATA_USER_CONFIG_DIR", user)
	t.Setenv("NETDATA_STOCK_CONFIG_DIR", stock)
	t.Setenv("NETDATA_CYGWIN_BASE_PATH", "")
	t.Setenv("NETDATA_PLUGINS_GOD_WATCH_PATH", "")
	_, err := pluginconfig.Init(pluginconfig.InitInput{RestrictConfig: mode == "installed"})
	require.NoError(t, err)
	if mode == "installed" {
		assert.Equal(t, []DirSpec{{Path: filepath.Join(user, "go.d", profilesDirName)}, {Path: filepath.Join(stock, "go.d", profilesDirName, "default"), IsStock: true}}, defaultDirSpecs())
	} else {
		assert.Equal(t, []DirSpec{{Path: development, IsStock: true}}, defaultDirSpecs())
	}

}
