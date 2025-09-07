// SPDX-License-Identifier: GPL-3.0-or-later

package file

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/confgroup"

	"github.com/goccy/go-yaml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type (
	discoverySim struct {
		discovery      *Discovery
		beforeRun      func()
		afterRun       func()
		expectedGroups []*confgroup.Group
	}
)

func (sim discoverySim) run(t *testing.T) {
	t.Helper()
	require.NotNil(t, sim.discovery)

	if sim.beforeRun != nil {
		sim.beforeRun()
	}

	in, out := make(chan []*confgroup.Group), make(chan []*confgroup.Group)
	go sim.collectGroups(t, in, out)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	go sim.discovery.Run(ctx, in)
	time.Sleep(time.Millisecond * 250)

	if sim.afterRun != nil {
		sim.afterRun()
	}

	actual := <-out

	sortGroups(actual)
	sortGroups(sim.expectedGroups)

	assert.Equal(t, sim.expectedGroups, actual)
}

func (sim discoverySim) collectGroups(t *testing.T, in, out chan []*confgroup.Group) {
	timeout := time.Second * 5
	var groups []*confgroup.Group
loop:
	for {
		select {
		case updates := <-in:
			if groups = append(groups, updates...); len(groups) >= len(sim.expectedGroups) {
				break loop
			}
		case <-time.After(timeout):
			t.Logf("discovery %s timed out after %s, got %d groups, expected %d, some events are skipped",
				sim.discovery.discoverers, timeout, len(groups), len(sim.expectedGroups))
			break loop
		}
	}
	out <- groups
}

type tmpDir struct {
	dir string
	t   *testing.T
}

func newTmpDir(t *testing.T, pattern string) *tmpDir {
	pattern = "netdata-go-test-discovery-file-" + pattern
	dir, err := os.MkdirTemp(os.TempDir(), pattern)
	require.NoError(t, err)
	return &tmpDir{dir: dir, t: t}
}

func (d *tmpDir) cleanup() {
	assert.NoError(d.t, os.RemoveAll(d.dir))
}

func (d *tmpDir) join(filename string) string {
	return filepath.Join(d.dir, filename)
}

func (d *tmpDir) createFile(pattern string) string {
	f, err := os.CreateTemp(d.dir, pattern)
	require.NoError(d.t, err)
	_ = f.Close()
	return f.Name()
}

func (d *tmpDir) removeFile(filename string) {
	err := os.Remove(filename)
	require.NoError(d.t, err)
}

func (d *tmpDir) renameFile(origFilename, newFilename string) {
	err := os.Rename(origFilename, newFilename)
	require.NoError(d.t, err)
}

func (d *tmpDir) writeYAML(filename string, in any) {
	bs, err := yaml.Marshal(in)
	require.NoError(d.t, err)
	err = os.WriteFile(filename, bs, 0644)
	require.NoError(d.t, err)
}

func (d *tmpDir) writeString(filename, data string) {
	err := os.WriteFile(filename, []byte(data), 0644)
	require.NoError(d.t, err)
}

func sortGroups(groups []*confgroup.Group) {
	if len(groups) == 0 {
		return
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].Source < groups[j].Source })
}
