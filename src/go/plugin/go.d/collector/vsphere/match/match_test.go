// SPDX-License-Identifier: GPL-3.0-or-later

package match

import (
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/resources"

	"github.com/stretchr/testify/assert"
)

func TestHostIncludes_Parse(t *testing.T) {
	tests := map[string]struct {
		valid bool
		cases map[string]struct {
			host *resources.Host
			want bool
		}
	}{
		"":        {valid: false},
		"*/C1/H1": {valid: false},
		"/": {
			valid: true,
			cases: map[string]struct {
				host *resources.Host
				want bool
			}{
				"does not match": {host: testHost("DC1", "Cluster1", "Host1"), want: false},
			},
		},
		"/*": {
			valid: true,
			cases: map[string]struct {
				host *resources.Host
				want bool
			}{
				"matches": {host: testHost("DC1", "Cluster1", "Host1"), want: true},
			},
		},
		"/!*": {
			valid: true,
			cases: map[string]struct {
				host *resources.Host
				want bool
			}{
				"does not match": {host: testHost("DC1", "Cluster1", "Host1"), want: false},
			},
		},
		"/!*/": {
			valid: true,
			cases: map[string]struct {
				host *resources.Host
				want bool
			}{
				"does not match": {host: testHost("DC1", "Cluster1", "Host1"), want: false},
			},
		},
		"/!*/ ": {
			valid: true,
			cases: map[string]struct {
				host *resources.Host
				want bool
			}{
				"does not match": {host: testHost("DC1", "Cluster1", "Host1"), want: false},
			},
		},
		"/DC1* DC2* !*/Cluster*": {
			valid: true,
			cases: map[string]struct {
				host *resources.Host
				want bool
			}{
				"matches first datacenter":    {host: testHost("DC1A", "Cluster1", "Host1"), want: true},
				"matches second datacenter":   {host: testHost("DC2A", "Cluster1", "Host1"), want: true},
				"rejects other datacenter":    {host: testHost("DC3A", "Cluster1", "Host1"), want: false},
				"rejects different cluster":   {host: testHost("DC1A", "Other", "Host1"), want: false},
				"matches any host below path": {host: testHost("DC1A", "Cluster1", "Other"), want: true},
			},
		},
		"/DC1*/Cluster*": {
			valid: true,
			cases: map[string]struct {
				host *resources.Host
				want bool
			}{
				"matches datacenter and cluster":    {host: testHost("DC1A", "Cluster1", "Host1"), want: true},
				"rejects different datacenter":      {host: testHost("DC2A", "Cluster1", "Host1"), want: false},
				"rejects different cluster":         {host: testHost("DC1A", "Other", "Host1"), want: false},
				"matches any host below two levels": {host: testHost("DC1A", "Cluster1", "Other"), want: true},
			},
		},
		"/*/*/HOST1*": {
			valid: true,
			cases: map[string]struct {
				host *resources.Host
				want bool
			}{
				"matches host":           {host: testHost("DC1", "Cluster1", "HOST10"), want: true},
				"rejects different host": {host: testHost("DC1", "Cluster1", "OTHER"), want: false},
			},
		},
		"/*/*/HOST1*/*/*": {
			valid: true,
			cases: map[string]struct {
				host *resources.Host
				want bool
			}{
				"ignores extra segments": {host: testHost("DC1", "Cluster1", "HOST10"), want: true},
			},
		},
		"[/DC1*,/DC2*]": {
			valid: true,
			cases: map[string]struct {
				host *resources.Host
				want bool
			}{
				"matches first include":  {host: testHost("DC1A", "Cluster1", "Host1"), want: true},
				"matches second include": {host: testHost("DC2A", "Cluster1", "Host1"), want: true},
				"rejects other":          {host: testHost("DC3A", "Cluster1", "Host1"), want: false},
			},
		},
		"[/DC1*,/DC2*,/DC3*/Cluster1*/H*]": {
			valid: true,
			cases: map[string]struct {
				host *resources.Host
				want bool
			}{
				"matches first include":     {host: testHost("DC1A", "Other", "Other"), want: true},
				"matches second include":    {host: testHost("DC2A", "Other", "Other"), want: true},
				"matches third include":     {host: testHost("DC3A", "Cluster10", "Host1"), want: true},
				"rejects third bad host":    {host: testHost("DC3A", "Cluster10", "Other"), want: false},
				"rejects third bad cluster": {host: testHost("DC3A", "Other", "Host1"), want: false},
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			includes := prepareIncludes(name)
			m, err := HostIncludes(includes).Parse()

			if !test.valid {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				for caseName, tc := range test.cases {
					t.Run(caseName, func(t *testing.T) {
						assert.Equal(t, tc.want, m.Match(tc.host))
					})
				}
			}
		})
	}
}

func TestVMIncludes_Parse(t *testing.T) {
	tests := map[string]struct {
		valid bool
		cases map[string]struct {
			vm   *resources.VM
			want bool
		}
	}{
		"":           {valid: false},
		"*/C1/H1/V1": {valid: false},
		"/*": {
			valid: true,
			cases: map[string]struct {
				vm   *resources.VM
				want bool
			}{
				"matches": {vm: testVM("DC1", "Cluster1", "Host1", "VM1"), want: true},
			},
		},
		"/!*": {
			valid: true,
			cases: map[string]struct {
				vm   *resources.VM
				want bool
			}{
				"does not match": {vm: testVM("DC1", "Cluster1", "Host1", "VM1"), want: false},
			},
		},
		"/!*/": {
			valid: true,
			cases: map[string]struct {
				vm   *resources.VM
				want bool
			}{
				"does not match": {vm: testVM("DC1", "Cluster1", "Host1", "VM1"), want: false},
			},
		},
		"/!*/ ": {
			valid: true,
			cases: map[string]struct {
				vm   *resources.VM
				want bool
			}{
				"does not match": {vm: testVM("DC1", "Cluster1", "Host1", "VM1"), want: false},
			},
		},
		"/DC1* DC2* !*/Cluster*": {
			valid: true,
			cases: map[string]struct {
				vm   *resources.VM
				want bool
			}{
				"matches first datacenter":  {vm: testVM("DC1A", "Cluster1", "Host1", "VM1"), want: true},
				"matches second datacenter": {vm: testVM("DC2A", "Cluster1", "Host1", "VM1"), want: true},
				"rejects other datacenter":  {vm: testVM("DC3A", "Cluster1", "Host1", "VM1"), want: false},
				"rejects different cluster": {vm: testVM("DC1A", "Other", "Host1", "VM1"), want: false},
				"matches any VM below path": {vm: testVM("DC1A", "Cluster1", "Other", "Other"), want: true},
			},
		},
		"/DC1*/Cluster*": {
			valid: true,
			cases: map[string]struct {
				vm   *resources.VM
				want bool
			}{
				"matches datacenter and cluster":  {vm: testVM("DC1A", "Cluster1", "Host1", "VM1"), want: true},
				"rejects different datacenter":    {vm: testVM("DC2A", "Cluster1", "Host1", "VM1"), want: false},
				"rejects different cluster":       {vm: testVM("DC1A", "Other", "Host1", "VM1"), want: false},
				"matches any VM below two levels": {vm: testVM("DC1A", "Cluster1", "Other", "Other"), want: true},
			},
		},
		"/*/*/HOST1": {
			valid: true,
			cases: map[string]struct {
				vm   *resources.VM
				want bool
			}{
				"matches host":           {vm: testVM("DC1", "Cluster1", "HOST1", "VM1"), want: true},
				"rejects different host": {vm: testVM("DC1", "Cluster1", "HOST2", "VM1"), want: false},
				"matches any VM on host": {vm: testVM("DC1", "Cluster1", "HOST1", "Other"), want: true},
			},
		},
		"/*/*/HOST1*/*/*": {
			valid: true,
			cases: map[string]struct {
				vm   *resources.VM
				want bool
			}{
				"matches host and wildcard VM": {vm: testVM("DC1", "Cluster1", "HOST10", "VM1"), want: true},
				"rejects different host":       {vm: testVM("DC1", "Cluster1", "Other", "VM1"), want: false},
			},
		},
		"[/DC1*,/DC2*]": {
			valid: true,
			cases: map[string]struct {
				vm   *resources.VM
				want bool
			}{
				"matches first include":  {vm: testVM("DC1A", "Cluster1", "Host1", "VM1"), want: true},
				"matches second include": {vm: testVM("DC2A", "Cluster1", "Host1", "VM1"), want: true},
				"rejects other":          {vm: testVM("DC3A", "Cluster1", "Host1", "VM1"), want: false},
			},
		},
		"[/DC1*,/DC2*,/DC3*/Cluster1*/H*/VM*]": {
			valid: true,
			cases: map[string]struct {
				vm   *resources.VM
				want bool
			}{
				"matches first include":  {vm: testVM("DC1A", "Other", "Other", "Other"), want: true},
				"matches second include": {vm: testVM("DC2A", "Other", "Other", "Other"), want: true},
				"matches third include":  {vm: testVM("DC3A", "Cluster10", "Host1", "VM1"), want: true},
				"rejects third bad VM":   {vm: testVM("DC3A", "Cluster10", "Host1", "Other"), want: false},
				"rejects third bad host": {vm: testVM("DC3A", "Cluster10", "Other", "VM1"), want: false},
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			includes := prepareIncludes(name)
			m, err := VMIncludes(includes).Parse()

			if !test.valid {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				for caseName, tc := range test.cases {
					t.Run(caseName, func(t *testing.T) {
						assert.Equal(t, tc.want, m.Match(tc.vm))
					})
				}
			}
		})
	}
}

func TestDatastoreIncludes_Parse(t *testing.T) {
	tests := map[string]struct {
		valid bool
		cases map[string]struct {
			ds   *resources.Datastore
			want bool
		}
	}{
		"":      {valid: false},
		"*/DS1": {valid: false},
		"/": {
			valid: true,
			cases: map[string]struct {
				ds   *resources.Datastore
				want bool
			}{
				"does not match": {ds: testDatastore("DC1", "DS1"), want: false},
			},
		},
		"/*": {
			valid: true,
			cases: map[string]struct {
				ds   *resources.Datastore
				want bool
			}{
				"matches": {ds: testDatastore("DC1", "DS1"), want: true},
			},
		},
		"/!*": {
			valid: true,
			cases: map[string]struct {
				ds   *resources.Datastore
				want bool
			}{
				"does not match": {ds: testDatastore("DC1", "DS1"), want: false},
			},
		},
		"/!*/": {
			valid: true,
			cases: map[string]struct {
				ds   *resources.Datastore
				want bool
			}{
				"does not match": {ds: testDatastore("DC1", "DS1"), want: false},
			},
		},
		"/!*/ ": {
			valid: true,
			cases: map[string]struct {
				ds   *resources.Datastore
				want bool
			}{
				"does not match": {ds: testDatastore("DC1", "DS1"), want: false},
			},
		},
		"/DC1*/DS*": {
			valid: true,
			cases: map[string]struct {
				ds   *resources.Datastore
				want bool
			}{
				"matches datacenter and datastore": {ds: testDatastore("DC1A", "DS1"), want: true},
				"rejects different datacenter":     {ds: testDatastore("DC2A", "DS1"), want: false},
				"rejects different datastore":      {ds: testDatastore("DC1A", "Other"), want: false},
			},
		},
		"/*/*/extra": {
			valid: true,
			cases: map[string]struct {
				ds   *resources.Datastore
				want bool
			}{
				"ignores extra segments": {ds: testDatastore("DC1", "DS1"), want: true},
			},
		},
		"[/DC1*,/DC2*]": {
			valid: true,
			cases: map[string]struct {
				ds   *resources.Datastore
				want bool
			}{
				"matches first include":  {ds: testDatastore("DC1A", "DS1"), want: true},
				"matches second include": {ds: testDatastore("DC2A", "DS1"), want: true},
				"rejects other":          {ds: testDatastore("DC3A", "DS1"), want: false},
			},
		},
		"[/DC1*,/DC2*,/DC3*/DS*]": {
			valid: true,
			cases: map[string]struct {
				ds   *resources.Datastore
				want bool
			}{
				"matches first include":        {ds: testDatastore("DC1A", "Other"), want: true},
				"matches second include":       {ds: testDatastore("DC2A", "Other"), want: true},
				"matches third include":        {ds: testDatastore("DC3A", "DS1"), want: true},
				"rejects third bad datastore":  {ds: testDatastore("DC3A", "Other"), want: false},
				"rejects third bad datacenter": {ds: testDatastore("Other", "DS1"), want: false},
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			includes := prepareIncludes(name)
			m, err := DatastoreIncludes(includes).Parse()

			if !test.valid {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				for caseName, tc := range test.cases {
					t.Run(caseName, func(t *testing.T) {
						assert.Equal(t, tc.want, m.Match(tc.ds))
					})
				}
			}
		})
	}
}

func TestClusterIncludes_Parse(t *testing.T) {
	tests := map[string]struct {
		valid bool
		cases map[string]struct {
			cluster *resources.Cluster
			want    bool
		}
	}{
		"":     {valid: false},
		"*/C1": {valid: false},
		"/": {
			valid: true,
			cases: map[string]struct {
				cluster *resources.Cluster
				want    bool
			}{
				"does not match": {cluster: testCluster("DC1", "Cluster1"), want: false},
			},
		},
		"/*": {
			valid: true,
			cases: map[string]struct {
				cluster *resources.Cluster
				want    bool
			}{
				"matches": {cluster: testCluster("DC1", "Cluster1"), want: true},
			},
		},
		"/!*": {
			valid: true,
			cases: map[string]struct {
				cluster *resources.Cluster
				want    bool
			}{
				"does not match": {cluster: testCluster("DC1", "Cluster1"), want: false},
			},
		},
		"/!*/": {
			valid: true,
			cases: map[string]struct {
				cluster *resources.Cluster
				want    bool
			}{
				"does not match": {cluster: testCluster("DC1", "Cluster1"), want: false},
			},
		},
		"/!*/ ": {
			valid: true,
			cases: map[string]struct {
				cluster *resources.Cluster
				want    bool
			}{
				"does not match": {cluster: testCluster("DC1", "Cluster1"), want: false},
			},
		},
		"/DC1*/Cluster*": {
			valid: true,
			cases: map[string]struct {
				cluster *resources.Cluster
				want    bool
			}{
				"matches datacenter and cluster": {cluster: testCluster("DC1A", "Cluster1"), want: true},
				"rejects different datacenter":   {cluster: testCluster("DC2A", "Cluster1"), want: false},
				"rejects different cluster":      {cluster: testCluster("DC1A", "Other"), want: false},
			},
		},
		"/*/*/extra": {
			valid: true,
			cases: map[string]struct {
				cluster *resources.Cluster
				want    bool
			}{
				"ignores extra segments": {cluster: testCluster("DC1", "Cluster1"), want: true},
			},
		},
		"[/DC1*,/DC2*]": {
			valid: true,
			cases: map[string]struct {
				cluster *resources.Cluster
				want    bool
			}{
				"matches first include":  {cluster: testCluster("DC1A", "Cluster1"), want: true},
				"matches second include": {cluster: testCluster("DC2A", "Cluster1"), want: true},
				"rejects other":          {cluster: testCluster("DC3A", "Cluster1"), want: false},
			},
		},
		"[/DC1*,/DC2*,/DC3*/Cluster*]": {
			valid: true,
			cases: map[string]struct {
				cluster *resources.Cluster
				want    bool
			}{
				"matches first include":        {cluster: testCluster("DC1A", "Other"), want: true},
				"matches second include":       {cluster: testCluster("DC2A", "Other"), want: true},
				"matches third include":        {cluster: testCluster("DC3A", "Cluster1"), want: true},
				"rejects third bad cluster":    {cluster: testCluster("DC3A", "Other"), want: false},
				"rejects third bad datacenter": {cluster: testCluster("Other", "Cluster1"), want: false},
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			includes := prepareIncludes(name)
			m, err := ClusterIncludes(includes).Parse()

			if !test.valid {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				for caseName, tc := range test.cases {
					t.Run(caseName, func(t *testing.T) {
						assert.Equal(t, tc.want, m.Match(tc.cluster))
					})
				}
			}
		})
	}
}

func TestInventoryIncludes_ParseEmptyReturnsNil(t *testing.T) {
	tests := map[string]struct {
		parse func() (any, error)
	}{
		"host":      {parse: func() (any, error) { return HostIncludes{}.Parse() }},
		"VM":        {parse: func() (any, error) { return VMIncludes{}.Parse() }},
		"datastore": {parse: func() (any, error) { return DatastoreIncludes{}.Parse() }},
		"cluster":   {parse: func() (any, error) { return ClusterIncludes{}.Parse() }},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			m, err := tc.parse()

			assert.NoError(t, err)
			assert.Nil(t, m)
		})
	}
}

func TestDatastoreClusterIncludes_Parse(t *testing.T) {
	pod := &resources.StoragePod{
		ID:   "group-p1",
		Name: "Pod1",
		Hier: resources.StoragePodHierarchy{DC: resources.HierarchyValue{Name: "DC1"}},
	}
	tests := map[string]struct {
		includes DatastoreClusterIncludes
		want     bool
		wantErr  bool
	}{
		"invalid pattern": {includes: DatastoreClusterIncludes{"["}, wantErr: true},
		"path match":      {includes: DatastoreClusterIncludes{"/DC1/Pod1"}, want: true},
		"name match":      {includes: DatastoreClusterIncludes{"Pod1"}, want: true},
		"id match":        {includes: DatastoreClusterIncludes{"group-p1"}, want: true},
		"no match":        {includes: DatastoreClusterIncludes{"Pod2"}, want: false},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			m, err := tc.includes.Parse()

			if tc.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tc.want, m.Match(pod))
		})
	}
}

func TestVSANClusterIncludes_Parse(t *testing.T) {
	cluster := &resources.Cluster{
		ID:       "domain-c1",
		Name:     "Cluster1",
		Hier:     resources.ClusterHierarchy{DC: resources.HierarchyValue{Name: "DC1"}},
		VSANUUID: "cluster-uuid",
	}
	tests := map[string]struct {
		includes VSANClusterIncludes
		want     bool
		wantErr  bool
	}{
		"invalid pattern": {includes: VSANClusterIncludes{"["}, wantErr: true},
		"path match":      {includes: VSANClusterIncludes{"/DC1/Cluster1"}, want: true},
		"name match":      {includes: VSANClusterIncludes{"Cluster1"}, want: true},
		"id match":        {includes: VSANClusterIncludes{"domain-c1"}, want: true},
		"uuid match":      {includes: VSANClusterIncludes{"vsan_uuid:cluster-uuid"}, want: true},
		"no match":        {includes: VSANClusterIncludes{"Cluster2"}, want: false},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			m, err := tc.includes.Parse()

			if tc.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tc.want, m.Match(cluster))
		})
	}
}

func TestVSANHostIncludes_Parse(t *testing.T) {
	host := &resources.Host{
		ID:           "host-1",
		Name:         "Host1",
		Hier:         resources.HostHierarchy{DC: resources.HierarchyValue{Name: "DC1"}, Cluster: resources.HierarchyValue{Name: "Cluster1"}},
		VSANNodeUUID: "host-uuid",
	}
	tests := map[string]struct {
		includes VSANHostIncludes
		want     bool
		wantErr  bool
	}{
		"invalid pattern": {includes: VSANHostIncludes{"["}, wantErr: true},
		"path match":      {includes: VSANHostIncludes{"/DC1/Cluster1/Host1"}, want: true},
		"name match":      {includes: VSANHostIncludes{"Host1"}, want: true},
		"id match":        {includes: VSANHostIncludes{"host-1"}, want: true},
		"uuid match":      {includes: VSANHostIncludes{"vsan_node_uuid:host-uuid"}, want: true},
		"no match":        {includes: VSANHostIncludes{"Host2"}, want: false},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			m, err := tc.includes.Parse()

			if tc.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tc.want, m.Match(host))
		})
	}
}

func TestVSANVMIncludes_Parse(t *testing.T) {
	vm := &resources.VM{
		ID:           "vm-1",
		Name:         "VM1",
		Hier:         resources.VMHierarchy{DC: resources.HierarchyValue{Name: "DC1"}, Cluster: resources.HierarchyValue{Name: "Cluster1"}, Host: resources.HierarchyValue{Name: "Host1"}},
		InstanceUUID: "vm-uuid",
	}
	tests := map[string]struct {
		includes VSANVMIncludes
		want     bool
		wantErr  bool
	}{
		"invalid pattern": {includes: VSANVMIncludes{"["}, wantErr: true},
		"path match":      {includes: VSANVMIncludes{"/DC1/Cluster1/Host1/VM1"}, want: true},
		"name match":      {includes: VSANVMIncludes{"VM1"}, want: true},
		"id match":        {includes: VSANVMIncludes{"vm-1"}, want: true},
		"uuid match":      {includes: VSANVMIncludes{"instance_uuid:vm-uuid"}, want: true},
		"no match":        {includes: VSANVMIncludes{"VM2"}, want: false},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			m, err := tc.includes.Parse()

			if tc.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tc.want, m.Match(vm))
		})
	}
}

func testHost(dc, cluster, name string) *resources.Host {
	return &resources.Host{
		Name: name,
		Hier: resources.HostHierarchy{
			DC:      resources.HierarchyValue{Name: dc},
			Cluster: resources.HierarchyValue{Name: cluster},
		},
	}
}

func testVM(dc, cluster, host, name string) *resources.VM {
	return &resources.VM{
		Name: name,
		Hier: resources.VMHierarchy{
			DC:      resources.HierarchyValue{Name: dc},
			Cluster: resources.HierarchyValue{Name: cluster},
			Host:    resources.HierarchyValue{Name: host},
		},
	}
}

func testDatastore(dc, name string) *resources.Datastore {
	return &resources.Datastore{
		Name: name,
		Hier: resources.DatastoreHierarchy{DC: resources.HierarchyValue{Name: dc}},
	}
}

func testCluster(dc, name string) *resources.Cluster {
	return &resources.Cluster{
		Name: name,
		Hier: resources.ClusterHierarchy{DC: resources.HierarchyValue{Name: dc}},
	}
}

func prepareIncludes(include string) []string {
	trimmed := strings.Trim(include, "[]")
	return strings.Split(trimmed, ",")
}
