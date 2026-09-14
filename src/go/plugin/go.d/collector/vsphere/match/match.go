// SPDX-License-Identifier: GPL-3.0-or-later

package match

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	rs "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/resources"
)

type HostMatcher interface {
	Match(*rs.Host) bool
}

type VMMatcher interface {
	Match(*rs.VM) bool
}

type DatastoreMatcher interface {
	Match(*rs.Datastore) bool
}

type ClusterMatcher interface {
	Match(*rs.Cluster) bool
}

type DatastoreClusterMatcher interface {
	Match(*rs.StoragePod) bool
}

type VSANClusterMatcher interface {
	Match(*rs.Cluster) bool
}

type VSANHostMatcher interface {
	Match(*rs.Host) bool
}

type VSANVMMatcher interface {
	Match(*rs.VM) bool
}

type (
	resourceMatcher[T any] interface {
		Match(*T) bool
	}
	fieldMatcher[T any] struct {
		m   matcher.Matcher
		get func(*T) string
	}
	orMatcher[T any]  struct{ lhs, rhs resourceMatcher[T] }
	andMatcher[T any] struct{ lhs, rhs resourceMatcher[T] }
)

func (m fieldMatcher[T]) Match(v *T) bool {
	return m.m.MatchString(m.get(v))
}

func (m orMatcher[T]) Match(v *T) bool {
	return m.lhs.Match(v) || m.rhs.Match(v)
}

func (m andMatcher[T]) Match(v *T) bool {
	return m.lhs.Match(v) && m.rhs.Match(v)
}

func chainAnd[T any](ms []resourceMatcher[T]) resourceMatcher[T] {
	switch len(ms) {
	case 0:
		return nil
	case 1:
		return ms[0]
	}
	m := andMatcher[T]{lhs: ms[0], rhs: ms[1]}
	for _, next := range ms[2:] {
		m = andMatcher[T]{lhs: m, rhs: next}
	}
	return m
}

func chainOr[T any](ms []resourceMatcher[T]) resourceMatcher[T] {
	switch len(ms) {
	case 0:
		return nil
	case 1:
		return ms[0]
	}
	m := orMatcher[T]{lhs: ms[0], rhs: ms[1]}
	for _, next := range ms[2:] {
		m = orMatcher[T]{lhs: m, rhs: next}
	}
	return m
}

type (
	VMIncludes        []string
	HostIncludes      []string
	DatastoreIncludes []string
	ClusterIncludes   []string

	DatastoreClusterIncludes []string
	VSANClusterIncludes      []string
	VSANHostIncludes         []string
	VSANVMIncludes           []string
)

type (
	datastoreClusterMatcher struct{ m matcher.Matcher }
	vsanClusterMatcher      struct{ m matcher.Matcher }
	vsanHostMatcher         struct{ m matcher.Matcher }
	vsanVMMatcher           struct{ m matcher.Matcher }
)

// NewPatternListMatcher parses ordered glob patterns with optional !-prefixed exclusions.
func NewPatternListMatcher(name string, patterns []string) (matcher.Matcher, error) {
	m, err := matcher.NewSimplePatternListMatcher(patterns)
	if err != nil {
		return nil, patternListError(name, err)
	}
	return m, nil
}

func patternListError(name string, err error) error {
	if err.Error() == "invalid empty negative pattern" || strings.HasPrefix(err.Error(), "invalid pattern") {
		return fmt.Errorf("%s has %w", name, err)
	}
	return fmt.Errorf("%s %w", name, err)
}

func (m datastoreClusterMatcher) Match(pod *rs.StoragePod) bool {
	return m.m.MatchString(datastoreClusterPath(pod)) ||
		m.m.MatchString(pod.Name) ||
		m.m.MatchString(pod.ID)
}

func (m vsanClusterMatcher) Match(cluster *rs.Cluster) bool {
	return m.m.MatchString(vsanClusterPath(cluster)) ||
		m.m.MatchString(cluster.Name) ||
		m.m.MatchString(cluster.ID) ||
		m.m.MatchString("vsan_uuid:"+cluster.VSANUUID)
}

func (m vsanHostMatcher) Match(host *rs.Host) bool {
	return m.m.MatchString(vsanHostPath(host)) ||
		m.m.MatchString(host.Name) ||
		m.m.MatchString(host.ID) ||
		m.m.MatchString("vsan_node_uuid:"+host.VSANNodeUUID)
}

func (m vsanVMMatcher) Match(vm *rs.VM) bool {
	return m.m.MatchString(vsanVMPath(vm)) ||
		m.m.MatchString(vm.Name) ||
		m.m.MatchString(vm.ID) ||
		m.m.MatchString("instance_uuid:"+vm.InstanceUUID)
}

func datastoreClusterPath(pod *rs.StoragePod) string {
	if pod.Hier.DC.Name == "" {
		return "/" + pod.Name
	}
	return "/" + pod.Hier.DC.Name + "/" + pod.Name
}

func vsanClusterPath(cluster *rs.Cluster) string {
	if cluster.Hier.DC.Name == "" {
		return "/" + cluster.Name
	}
	return "/" + cluster.Hier.DC.Name + "/" + cluster.Name
}

func vsanHostPath(host *rs.Host) string {
	return "/" + host.Hier.DC.Name + "/" + host.Hier.Cluster.Name + "/" + host.Name
}

func vsanVMPath(vm *rs.VM) string {
	return "/" + vm.Hier.DC.Name + "/" + vm.Hier.Cluster.Name + "/" + vm.Hier.Host.Name + "/" + vm.Name
}

func (dci DatastoreClusterIncludes) Parse() (DatastoreClusterMatcher, error) {
	m, err := NewPatternListMatcher("datastore_cluster_include", []string(dci))
	if err != nil {
		return nil, err
	}
	return datastoreClusterMatcher{m}, nil
}

func (vci VSANClusterIncludes) Parse() (VSANClusterMatcher, error) {
	m, err := NewPatternListMatcher("vsan_cluster_include", []string(vci))
	if err != nil {
		return nil, err
	}
	return vsanClusterMatcher{m}, nil
}

func (vhi VSANHostIncludes) Parse() (VSANHostMatcher, error) {
	m, err := NewPatternListMatcher("vsan_host_include", []string(vhi))
	if err != nil {
		return nil, err
	}
	return vsanHostMatcher{m}, nil
}

func (vvi VSANVMIncludes) Parse() (VSANVMMatcher, error) {
	m, err := NewPatternListMatcher("vsan_vm_include", []string(vvi))
	if err != nil {
		return nil, err
	}
	return vsanVMMatcher{m}, nil
}

func (vi VMIncludes) Parse() (VMMatcher, error) {
	return parseIncludes[rs.VM]("VM include", []string(vi), parseVMInclude)
}

func (hi HostIncludes) Parse() (HostMatcher, error) {
	return parseIncludes[rs.Host]("host include", []string(hi), parseHostInclude)
}

func (di DatastoreIncludes) Parse() (DatastoreMatcher, error) {
	return parseIncludes[rs.Datastore]("datastore include", []string(di), parseDatastoreInclude)
}

func cleanInclude(include string) string {
	return strings.Trim(include, "/")
}

func parseIncludes[T any](name string, includes []string, parse func(string) (resourceMatcher[T], error)) (resourceMatcher[T], error) {
	var ms []resourceMatcher[T]
	for _, v := range includes {
		m, err := parse(v)
		if err != nil {
			return nil, fmt.Errorf("parse %s %q: %w", name, v, err)
		}
		if m == nil {
			continue
		}
		ms = append(ms, m)
	}
	return chainOr(ms), nil
}

func parsePathInclude[T any](include, name, expected string, segments []func(*T) string) (resourceMatcher[T], error) {
	if !isIncludeFormatValid(include) {
		return nil, fmt.Errorf("bad %s include format %q: expected %s", name, include, expected)
	}

	include = cleanInclude(include)
	parts := strings.Split(include, "/")
	ms := make([]resourceMatcher[T], 0, min(len(parts), len(segments)))

	for i, v := range parts {
		m, err := parseSubInclude(v)
		if err != nil {
			return nil, fmt.Errorf("parse %s include segment index=%d value=%q: %w", name, i, v, err)
		}
		if i >= len(segments) {
			continue
		}
		ms = append(ms, fieldMatcher[T]{m: m, get: segments[i]})
	}

	return chainAnd(ms), nil
}

var hostPathSegments = []func(*rs.Host) string{
	func(host *rs.Host) string { return host.Hier.DC.Name },
	func(host *rs.Host) string { return host.Hier.Cluster.Name },
	func(host *rs.Host) string { return host.Name },
}

var vmPathSegments = []func(*rs.VM) string{
	func(vm *rs.VM) string { return vm.Hier.DC.Name },
	func(vm *rs.VM) string { return vm.Hier.Cluster.Name },
	func(vm *rs.VM) string { return vm.Hier.Host.Name },
	func(vm *rs.VM) string { return vm.Name },
}

var clusterPathSegments = []func(*rs.Cluster) string{
	func(cluster *rs.Cluster) string { return cluster.Hier.DC.Name },
	func(cluster *rs.Cluster) string { return cluster.Name },
}

var datastorePathSegments = []func(*rs.Datastore) string{
	func(ds *rs.Datastore) string { return ds.Hier.DC.Name },
	func(ds *rs.Datastore) string { return ds.Name },
}

func parseHostInclude(include string) (resourceMatcher[rs.Host], error) {
	return parsePathInclude(include, "host", "/<datacenter>/<cluster>/<host>", hostPathSegments)
}

func parseVMInclude(include string) (resourceMatcher[rs.VM], error) {
	return parsePathInclude(include, "VM", "/<datacenter>/<cluster>/<host>/<vm>", vmPathSegments)
}

func parseClusterInclude(include string) (resourceMatcher[rs.Cluster], error) {
	return parsePathInclude(include, "cluster", "/<datacenter>/<cluster>", clusterPathSegments)
}

func parseDatastoreInclude(include string) (resourceMatcher[rs.Datastore], error) {
	return parsePathInclude(include, "datastore", "/<datacenter>/<datastore>", datastorePathSegments)
}

func parseSubInclude(sub string) (matcher.Matcher, error) {
	sub = strings.TrimSpace(sub)
	if sub == "" || sub == "!*" {
		return matcher.FALSE(), nil
	}
	if sub == "*" {
		return matcher.TRUE(), nil
	}
	return matcher.NewSimplePatternsMatcher(sub)
}

func isIncludeFormatValid(line string) bool {
	return strings.HasPrefix(line, "/")
}

func (ci ClusterIncludes) Parse() (ClusterMatcher, error) {
	return parseIncludes[rs.Cluster]("cluster include", []string(ci), parseClusterInclude)
}
