// SPDX-License-Identifier: GPL-3.0-or-later

package vnoderegistry

import (
	"fmt"
	"sync"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegistryScenarios(t *testing.T) {
	cases := map[string]struct {
		run func(t *testing.T)
	}{
		"updates metadata and tracks owners": {
			run: func(t *testing.T) {
				reg := New()
				first := netdataapi.HostInfo{
					GUID:     "node-guid",
					Hostname: "node-a",
					Labels:   map[string]string{"_hostname": "node-a", "region": "eu"},
				}
				same := netdataapi.HostInfo{
					GUID:     "node-guid",
					Hostname: "node-a",
					Labels:   map[string]string{"_hostname": "node-a", "region": "eu"},
				}
				conflict := netdataapi.HostInfo{
					GUID:     "node-guid",
					Hostname: "node-b",
					Labels:   map[string]string{"_hostname": "node-b", "region": "us"},
				}

				got, err := reg.Register("job-a", first)
				require.NoError(t, err)
				assert.True(t, got.NeedDefine)
				assert.True(t, got.OwnerAdded)
				assert.False(t, got.MetadataUpdated)
				assert.Equal(t, first, got.Info)

				got, err = reg.Register("job-b", same)
				require.NoError(t, err)
				assert.False(t, got.NeedDefine)
				assert.True(t, got.OwnerAdded)
				assert.False(t, got.MetadataUpdated)
				assert.Equal(t, first, got.Info)

				got, err = reg.Register("job-c", conflict)
				require.NoError(t, err)
				assert.True(t, got.NeedDefine)
				assert.True(t, got.OwnerAdded)
				assert.True(t, got.MetadataUpdated)
				assert.True(t, got.UpdateFirstSeen)
				assert.Equal(t, conflict, got.Info)
				assert.Equal(t, first, got.Previous)

				got, err = reg.Register("job-c", conflict)
				require.NoError(t, err)
				assert.False(t, got.NeedDefine)
				assert.False(t, got.OwnerAdded)
				assert.False(t, got.MetadataUpdated)
				assert.False(t, got.UpdateFirstSeen)

				assert.Equal(t, []Owner{"job-a", "job-b", "job-c"}, reg.Owners("node-guid"))
				assert.False(t, reg.Release("job-a", "node-guid"))
				assert.Equal(t, 1, reg.Len())
				assert.Equal(t, []Owner{"job-b", "job-c"}, reg.Owners("node-guid"))
				assert.False(t, reg.Release("job-b", "node-guid"))
				assert.True(t, reg.Release("job-c", "node-guid"))
				assert.Equal(t, 0, reg.Len())
			},
		},
		"normalizes metadata before compare": {
			run: func(t *testing.T) {
				reg := New()
				first := netdataapi.HostInfo{
					GUID:     "node-guid",
					Hostname: "node-a",
				}
				sameWireInfo := netdataapi.HostInfo{
					GUID:     " node-guid ",
					Hostname: " node-a ",
					Labels: map[string]string{
						"_hostname": "node-a",
					},
				}

				got, err := reg.Register("job-a", first)
				require.NoError(t, err)
				assert.True(t, got.NeedDefine)

				got, err = reg.Register("job-b", sameWireInfo)
				require.NoError(t, err)
				assert.False(t, got.NeedDefine)
				assert.Equal(t, map[string]string{"_hostname": "node-a"}, got.Info.Labels)
			},
		},
		"rollback restores metadata update": {
			run: func(t *testing.T) {
				reg := New()
				first := netdataapi.HostInfo{GUID: "node-guid", Hostname: "node-a"}
				next := netdataapi.HostInfo{GUID: "node-guid", Hostname: "node-b"}

				_, err := reg.Register("job-a", first)
				require.NoError(t, err)
				got, err := reg.Register("job-b", next)
				require.NoError(t, err)
				require.True(t, got.MetadataUpdated)
				require.True(t, got.OwnerAdded)

				reg.Rollback("job-b", got)

				info, ok := reg.Lookup("node-guid")
				require.True(t, ok)
				assert.Equal(t, "node-a", info.Hostname)
				assert.Equal(t, []Owner{"job-a"}, reg.Owners("node-guid"))
			},
		},
		"rollback does not undo newer metadata update": {
			run: func(t *testing.T) {
				reg := New()
				_, err := reg.Register("job-a", netdataapi.HostInfo{GUID: "node-guid", Hostname: "node-a"})
				require.NoError(t, err)
				rollback, err := reg.Register("job-b", netdataapi.HostInfo{GUID: "node-guid", Hostname: "node-b"})
				require.NoError(t, err)
				_, err = reg.Register("job-c", netdataapi.HostInfo{GUID: "node-guid", Hostname: "node-c"})
				require.NoError(t, err)

				reg.Rollback("job-b", rollback)

				info, ok := reg.Lookup("node-guid")
				require.True(t, ok)
				assert.Equal(t, "node-c", info.Hostname)
				assert.Equal(t, []Owner{"job-a", "job-c"}, reg.Owners("node-guid"))
			},
		},
		"update warnings are per state and bounded": {
			run: func(t *testing.T) {
				reg := New()
				_, err := reg.Register("job", netdataapi.HostInfo{GUID: "node-guid", Hostname: "node-a"})
				require.NoError(t, err)

				got, err := reg.Register("job", netdataapi.HostInfo{GUID: "node-guid", Hostname: "node-b"})
				require.NoError(t, err)
				assert.True(t, got.UpdateFirstSeen)

				got, err = reg.Register("job", netdataapi.HostInfo{GUID: "node-guid", Hostname: "node-a"})
				require.NoError(t, err)
				assert.True(t, got.UpdateFirstSeen)

				got, err = reg.Register("job", netdataapi.HostInfo{GUID: "node-guid", Hostname: "node-b"})
				require.NoError(t, err)
				assert.False(t, got.UpdateFirstSeen)

				for i := range maxReportedMetadataStatesPerGUID + 1 {
					_, err = reg.Register("job", netdataapi.HostInfo{
						GUID:     "node-guid",
						Hostname: fmt.Sprintf("node-%d", i),
					})
					require.NoError(t, err)
				}
				reg.mu.Lock()
				defer reg.mu.Unlock()
				require.Len(t, reg.entries["node-guid"].reportedOrder, maxReportedMetadataStatesPerGUID)
				require.Len(t, reg.entries["node-guid"].reportedStates, maxReportedMetadataStatesPerGUID)
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, tc.run)
	}
}

func TestRegistryValidation(t *testing.T) {
	cases := map[string]struct {
		owner   Owner
		info    netdataapi.HostInfo
		wantErr string
	}{
		"missing owner": {
			info:    netdataapi.HostInfo{GUID: "guid", Hostname: "host"},
			wantErr: "owner is required",
		},
		"missing guid": {
			owner:   "job",
			info:    netdataapi.HostInfo{Hostname: "host"},
			wantErr: "host guid is required",
		},
		"missing hostname": {
			owner:   "job",
			info:    netdataapi.HostInfo{GUID: "guid"},
			wantErr: "host hostname is required",
		},
		"unsafe hostname": {
			owner:   "job",
			info:    netdataapi.HostInfo{GUID: "guid", Hostname: "host\nname"},
			wantErr: "unsupported characters",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			reg := New()
			_, err := reg.Register(tc.owner, tc.info)
			require.ErrorContains(t, err, tc.wantErr)
		})
	}
}

func TestRegistryConcurrentScenarios(t *testing.T) {
	cases := map[string]struct {
		run func(t *testing.T)
	}{
		"registration": {
			run: func(t *testing.T) {
				reg := New()
				first := netdataapi.HostInfo{
					GUID:     "node-guid",
					Hostname: "node",
				}

				const owners = 64
				var wg sync.WaitGroup
				errs := make(chan error, owners)
				for i := range owners {
					wg.Go(func() {
						info := first
						if i%2 == 1 {
							info.Hostname = "node-conflict"
						}
						_, err := reg.Register(Owner(fmt.Sprintf("job-%d", i)), info)
						errs <- err
					})
				}
				wg.Wait()
				close(errs)
				for err := range errs {
					require.NoError(t, err)
				}

				info, ok := reg.Lookup("node-guid")
				require.True(t, ok)
				require.NotEmpty(t, info.Hostname)
				require.Len(t, reg.Owners("node-guid"), owners)
			},
		},
		"register and release": {
			run: func(t *testing.T) {
				reg := New()
				const owners = 64

				var wg sync.WaitGroup
				errs := make(chan error, owners)
				for i := range owners {
					wg.Go(func() {
						owner := Owner(fmt.Sprintf("job-%d", i))
						_, err := reg.Register(owner, netdataapi.HostInfo{
							GUID:     "node-guid",
							Hostname: "node",
						})
						errs <- err
						reg.Release(owner, "node-guid")
					})
				}
				wg.Wait()
				close(errs)
				for err := range errs {
					require.NoError(t, err)
				}

				assert.Empty(t, reg.Owners("node-guid"))
				assert.Equal(t, 0, reg.Len())
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, tc.run)
	}
}
