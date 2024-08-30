// SPDX-License-Identifier: GPL-3.0-or-later

package discovery

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/discovery/file"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewManager(t *testing.T) {
	tests := map[string]struct {
		cfg     Config
		wantErr bool
	}{
		"valid config": {
			cfg: Config{
				Registry: confgroup.Registry{"module1": confgroup.Default{}},
				File:     file.Config{Read: []string{"path"}},
			},
		},
		"invalid config, registry not set": {
			cfg: Config{
				File: file.Config{Read: []string{"path"}},
			},
			wantErr: true,
		},
		"invalid config, discoverers not set": {
			cfg: Config{
				Registry: confgroup.Registry{"module1": confgroup.Default{}},
			},
			wantErr: true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			mgr, err := NewManager(test.cfg)

			if test.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, mgr)
			}
		})
	}
}

func TestManager_Run(t *testing.T) {
	tests := map[string]func() discoverySim{
		"several discoverers, unique groups with delayed collect": func() discoverySim {
			const numGroups, numCfgs = 2, 2
			d1 := prepareMockDiscoverer("test1", numGroups, numCfgs)
			d2 := prepareMockDiscoverer("test2", numGroups, numCfgs)
			mgr := prepareManager(d1, d2)
			expected := combineGroups(d1.groups, d2.groups)

			sim := discoverySim{
				mgr:            mgr,
				collectDelay:   mgr.sendEvery + time.Second,
				expectedGroups: expected,
			}
			return sim
		},
		"several discoverers, unique groups": func() discoverySim {
			const numGroups, numCfgs = 2, 2
			d1 := prepareMockDiscoverer("test1", numGroups, numCfgs)
			d2 := prepareMockDiscoverer("test2", numGroups, numCfgs)
			mgr := prepareManager(d1, d2)
			expected := combineGroups(d1.groups, d2.groups)
			sim := discoverySim{
				mgr:            mgr,
				expectedGroups: expected,
			}
			return sim
		},
		"several discoverers, same groups": func() discoverySim {
			const numGroups, numTargets = 2, 2
			d1 := prepareMockDiscoverer("test1", numGroups, numTargets)
			mgr := prepareManager(d1, d1)
			expected := combineGroups(d1.groups)

			sim := discoverySim{
				mgr:            mgr,
				expectedGroups: expected,
			}
			return sim
		},
		"several discoverers, empty groups": func() discoverySim {
			const numGroups, numCfgs = 1, 0
			d1 := prepareMockDiscoverer("test1", numGroups, numCfgs)
			d2 := prepareMockDiscoverer("test2", numGroups, numCfgs)
			mgr := prepareManager(d1, d2)
			expected := combineGroups(d1.groups, d2.groups)

			sim := discoverySim{
				mgr:            mgr,
				expectedGroups: expected,
			}
			return sim
		},
		"several discoverers, nil groups": func() discoverySim {
			const numGroups, numCfgs = 0, 0
			d1 := prepareMockDiscoverer("test1", numGroups, numCfgs)
			d2 := prepareMockDiscoverer("test2", numGroups, numCfgs)
			mgr := prepareManager(d1, d2)

			sim := discoverySim{
				mgr:            mgr,
				expectedGroups: nil,
			}
			return sim
		},
	}

	for name, sim := range tests {
		t.Run(name, func(t *testing.T) { sim().run(t) })
	}
}

func prepareMockDiscoverer(source string, groups, configs int) mockDiscoverer {
	d := mockDiscoverer{}

	for i := 0; i < groups; i++ {
		group := confgroup.Group{
			Source: fmt.Sprintf("%s_group_%d", source, i+1),
		}
		for j := 0; j < configs; j++ {
			group.Configs = append(group.Configs,
				confgroup.Config{"name": fmt.Sprintf("%s_group_%d_target_%d", source, i+1, j+1)})
		}
		d.groups = append(d.groups, &group)
	}
	return d
}

func prepareManager(discoverers ...discoverer) *Manager {
	mgr := &Manager{
		send:        make(chan struct{}, 1),
		sendEvery:   2 * time.Second,
		discoverers: discoverers,
		cache:       newCache(),
		mux:         &sync.RWMutex{},
	}
	return mgr
}

type mockDiscoverer struct {
	groups []*confgroup.Group
}

func (md mockDiscoverer) Run(ctx context.Context, out chan<- []*confgroup.Group) {
	for {
		select {
		case <-ctx.Done():
			return
		case out <- md.groups:
			return
		}
	}
}

func combineGroups(groups ...[]*confgroup.Group) (combined []*confgroup.Group) {
	for _, set := range groups {
		combined = append(combined, set...)
	}
	return combined
}
