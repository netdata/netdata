// SPDX-License-Identifier: GPL-3.0-or-later

package native

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSelfContainedExamples(t *testing.T) {
	setupRunner(t)
	examples, err := filepath.Abs("../../development")
	require.NoError(t, err)
	for _, language := range []string{"bash", "python", "go"} {
		t.Run(language, func(t *testing.T) {
			source := filepath.Join(examples, "self-contained-"+language, "collect.sh")
			switch language {
			case "python":
				if _, err := exec.LookPath("python3"); err != nil {
					t.Skip("example requires python3")
				}
				source = filepath.Join(examples, "self-contained-python", "collect.py")
			case "go":
				goBin, err := exec.LookPath("go")
				if err != nil {
					t.Skip("binary example requires Go compiler")
				}
				source = filepath.Join(t.TempDir(), "example")
				ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
				defer cancel()
				out, err := exec.CommandContext(ctx, goBin, "build", "-o", source, filepath.Join(examples, "self-contained-go")).
					CombinedOutput()
				require.NoError(t, err, "%s", out)
			}
			data, err := os.ReadFile(source)
			require.NoError(t, err)
			for _, mode := range []string{modeOneshot, modePersistent} {
				t.Run(mode, func(t *testing.T) {
					// Deploy only one file; the Go source's embedded YAML is not present.
					dir := t.TempDir()
					program := filepath.Join(dir, "package")
					require.NoError(t, os.WriteFile(program, data, 0755))
					command := []string{program}
					if mode == modePersistent {
						command = append(command, "--persistent")
					}
					inventory := executableInventory(t, command)
					registry, err := loadPackages(
						context.Background(),
						inventory,
						collectorapi.Registry{},
						testPackagePath,
					)
					require.NoError(t, err)
					entries, err := os.ReadDir(dir)
					require.NoError(t, err)
					assert.Len(t, entries, 1, "description must not extract sidecars")
					a := startNativeAgent(t, registry, dir)
					a.call(t, "add", "config scripts.d:collector:native-fixture add example", `{"update_every":1}`, 202)
					a.call(t, "enable", "config scripts.d:collector:native-fixture:example enable", "", 202)
					require.Eventually(
						t,
						func() bool { return strings.Contains(a.out.String(), "status running") },
						5*time.Second,
						10*time.Millisecond,
					)
					if language != "bash" {
						info := a.call(t, "info", "native-fixture:items info __job:example", "", 200)
						validateFunctionUI(t, info)
						result := a.call(t, "data", "native-fixture:items __job:example", "", 200)
						validateFunctionUI(t, result)
						assert.Contains(t, result, "[[17]]")
					}
					require.Eventually(
						t,
						func() bool { return strings.Contains(a.out.String(), " = 17") },
						3*time.Second,
						10*time.Millisecond,
					)
					assert.Contains(t, a.out.String(), "selfcontained_"+language+".depth")
				})
			}
		})
	}
}
