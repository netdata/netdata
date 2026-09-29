// SPDX-License-Identifier: GPL-3.0-or-later

package native

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/joboutput"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func executableInventory(t *testing.T, command []string) string {
	t.Helper()
	data, err := yaml.Marshal(
		map[string]any{"version": "v1", "packages": []any{map[string]any{"name": "fixture", "command": command}}},
	)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "packages.yaml")
	require.NoError(t, os.WriteFile(path, data, 0644))
	return path
}

func testPackagePath(path string) (string, error) { _, err := os.Stat(path); return path, err }

func TestExecutablePackageRegistration(t *testing.T) {
	setupRunner(t)
	// Use the existing real configured peer for operational protocol. Its new
	// describe branch executes before reading configuration and prints inline assets.
	registry, dir := functionFixture(t, modePersistent, false)
	require.NotEmpty(t, registry)
	path := filepath.Join(dir, "peer.py")
	var form map[string]any
	require.NoError(t, json.Unmarshal([]byte(fixtureSchema), &form))
	description := map[string]any{
		"version": "v1", "mode": "persistent",
		"metrics":       []any{map[string]any{"name": "depth", "type": "gauge", "unit": "jobs"}},
		"functions":     []any{map[string]any{"id": "items", "name": "Items", "help": "Show items 😀."}},
		"config_schema": form,
	}
	encoded, err := json.Marshal(description)
	require.NoError(t, err)
	prelude := fmt.Sprintf(`
if sys.argv[-1] == "describe":
    assert sys.stdin.read() == ""
    with (root / "described").open("a") as f: f.write("describe\n")
    print(json.dumps(json.loads(%q)))
    sys.exit(0)
`, string(encoded))
	peer := strings.Replace(
		functionPeer,
		`config = json.loads(sys.stdin.readline())["config"]`,
		prelude+`config = json.loads(sys.stdin.readline())["config"]`,
		1,
	)
	require.NoError(t, os.WriteFile(path, []byte(peer), 0644))
	inventory := executableInventory(t, []string{filepath.Join(dir, "collect.sh")})
	registry, err = loadPackages(context.Background(), inventory, collectorapi.Registry{}, testPackagePath)
	require.NoError(t, err)
	// Editing a former sidecar cannot influence the executable package.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "manifest.yaml"), []byte("invalid"), 0644))
	creator := registry["native-fixture"]
	assert.Equal(t, "Show items 😀.", creator.SharedFunctions()[0].Help)
	for range 2 {
		c := initFunctionCollector(t, registry)
		assert.Equal(t, []string{filepath.Join(dir, "collect.sh")}, c.definition.Command)
	}
	factory, err := joboutput.NewConfigModuleFactory(joboutput.ConfigModuleFactoryConfig{
		Modules: registry,
		Configs: &secrets.ConfigResolver{},
	})
	require.NoError(t, err)
	cfg := confgroup.Config{
		"name":   "test",
		"module": "native-fixture",
		"config": map[string]any{"text": "synthetic"},
	}
	cfg.SetSourceType(confgroup.TypeDyncfg)
	require.NoError(t, factory.Validate(context.Background(), cfg))
	require.NoError(t, factory.Test(context.Background(), cfg))
	assert.Contains(t, creator.JobConfigSchema, "Fixture")
	// Route real requests through Agent/DynCfg, including a replacement job.
	a := startNativeAgent(t, registry, dir)
	a.enable(t, "alpha", 23)
	a.call(t, "items", "native-fixture:items __job:alpha", "", 200)
	a.call(
		t,
		"update",
		"config scripts.d:collector:native-fixture:alpha update",
		`{"update_every":1,"config":{"text":"changed","count":41}}`,
		202,
	)
	require.Eventually(t, func() bool {
		id := fmt.Sprintf("after-%d", time.Now().UnixNano())
		return strings.Contains(a.call(t, id, "native-fixture:items __job:alpha", "", 0), "[[41,")
	}, 5*time.Second, 20*time.Millisecond)
	described, err := os.ReadFile(filepath.Join(dir, "described"))
	require.NoError(t, err)
	assert.Equal(t, "describe\n", string(described))
}

func TestDescribeExecutionBoundaries(t *testing.T) {
	setupRunner(t)
	for _, tc := range []struct{ name, body, want string }{
		{"exit", "printf 'private-output' >&2\nexit 7\n", "status 7"},
		{"oversized", fmt.Sprintf("printf '%%*s' %d x\nsleep 30\n", maxDescriptionBytes+1), "exceeds 64 MiB"},
		{"timeout", "sleep 30\n", "deadline exceeded"},
		{"startup deadline", "exec 1>&-\nsleep 30\n", "deadline exceeded"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := fixtureCollector(t, tc.body)
			ctx := context.Background()
			if tc.name == "timeout" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, 100*time.Millisecond)
				defer cancel()
			}
			data, err := describePackage(ctx, c.definition.Command)
			require.ErrorContains(t, err, tc.want)
			assert.Nil(t, data)
			assert.NotContains(t, err.Error(), "private-output")
		})
	}
	for _, size := range []int{(4 << 20) + 1, maxDescriptionBytes} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			const header = "version: v1\nchecks: [{id: ready, title: Ready}]\n"
			body := fmt.Sprintf(
				"printf '%%s\\n' 'version: v1' 'checks: [{id: ready, title: Ready}]'\nprintf '%%*s' %d ''\n",
				size-len(header),
			)
			c, _ := fixtureCollector(t, body)
			data, err := describePackage(context.Background(), c.definition.Command)
			require.NoError(t, err)
			assert.Len(t, data, size)
			_, _, err = parseDescription(data, c.definition.Command)
			require.NoError(t, err, "large metadata must still compile as a valid package")
		})
	}
	t.Run("canceled before start", func(t *testing.T) {
		c, dir := fixtureCollector(t, "touch \"$(dirname \"$0\")/started\"\n")
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := describePackage(ctx, c.definition.Command)
		require.ErrorIs(t, err, context.Canceled)
		_, err = os.Stat(filepath.Join(dir, "started"))
		require.ErrorIs(t, err, os.ErrNotExist)
	})
}

func TestExecutableInventoryValidation(t *testing.T) {
	setupRunner(t)
	for name, fields := range map[string]string{
		"both sources":     "manifest: /unused\n    command: [/bin/sh]",
		"neither source":   "",
		"empty command":    "command: []",
		"relative command": "command: [relative-script]",
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "packages.yaml")
			require.NoError(
				t,
				os.WriteFile(path, []byte("version: v1\npackages:\n  - name: fixture\n    "+fields+"\n"), 0644),
			)
			base := collectorapi.Registry{}
			_, err := loadPackages(context.Background(), path, base, testPackagePath)
			require.Error(t, err)
			assert.Empty(t, base)
		})
	}
}
