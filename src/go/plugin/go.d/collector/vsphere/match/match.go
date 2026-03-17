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

type (
	hostDCMatcher      struct{ m matcher.Matcher }
	hostClusterMatcher struct{ m matcher.Matcher }
	hostHostMatcher    struct{ m matcher.Matcher }
	vmDCMatcher        struct{ m matcher.Matcher }
	vmClusterMatcher   struct{ m matcher.Matcher }
	vmHostMatcher      struct{ m matcher.Matcher }
	vmVMMatcher        struct{ m matcher.Matcher }
	orHostMatcher      struct{ lhs, rhs HostMatcher }
	orVMMatcher        struct{ lhs, rhs VMMatcher }
	andHostMatcher     struct{ lhs, rhs HostMatcher }
	andVMMatcher       struct{ lhs, rhs VMMatcher }
)

func (m hostDCMatcher) Match(host *rs.Host) bool      { return m.m.MatchString(host.Hier.DC.Name) }
func (m hostClusterMatcher) Match(host *rs.Host) bool { return m.m.MatchString(host.Hier.Cluster.Name) }
func (m hostHostMatcher) Match(host *rs.Host) bool    { return m.m.MatchString(host.Name) }
func (m vmDCMatcher) Match(vm *rs.VM) bool            { return m.m.MatchString(vm.Hier.DC.Name) }
func (m vmClusterMatcher) Match(vm *rs.VM) bool       { return m.m.MatchString(vm.Hier.Cluster.Name) }
func (m vmHostMatcher) Match(vm *rs.VM) bool          { return m.m.MatchString(vm.Hier.Host.Name) }
func (m vmVMMatcher) Match(vm *rs.VM) bool            { return m.m.MatchString(vm.Name) }
func (m orHostMatcher) Match(host *rs.Host) bool      { return m.lhs.Match(host) || m.rhs.Match(host) }
func (m orVMMatcher) Match(vm *rs.VM) bool            { return m.lhs.Match(vm) || m.rhs.Match(vm) }
func (m andHostMatcher) Match(host *rs.Host) bool     { return m.lhs.Match(host) && m.rhs.Match(host) }
func (m andVMMatcher) Match(vm *rs.VM) bool           { return m.lhs.Match(vm) && m.rhs.Match(vm) }

func newAndHostMatcher(lhs, rhs HostMatcher, others ...HostMatcher) andHostMatcher {
	m := andHostMatcher{lhs: lhs, rhs: rhs}
	switch len(others) {
	case 0:
		return m
	default:
		return newAndHostMatcher(m, others[0], others[1:]...)
	}
}

func newAndVMMatcher(lhs, rhs VMMatcher, others ...VMMatcher) andVMMatcher {
	m := andVMMatcher{lhs: lhs, rhs: rhs}
	switch len(others) {
	case 0:
		return m
	default:
		return newAndVMMatcher(m, others[0], others[1:]...)
	}
}

func newOrHostMatcher(lhs, rhs HostMatcher, others ...HostMatcher) orHostMatcher {
	m := orHostMatcher{lhs: lhs, rhs: rhs}
	switch len(others) {
	case 0:
		return m
	default:
		return newOrHostMatcher(m, others[0], others[1:]...)
	}
}

func newOrVMMatcher(lhs, rhs VMMatcher, others ...VMMatcher) orVMMatcher {
	m := orVMMatcher{lhs: lhs, rhs: rhs}
	switch len(others) {
	case 0:
		return m
	default:
		return newOrVMMatcher(m, others[0], others[1:]...)
	}
}

func newOrDSMatcher(lhs, rhs DatastoreMatcher, others ...DatastoreMatcher) orDSMatcher {
	m := orDSMatcher{lhs: lhs, rhs: rhs}
	switch len(others) {
	case 0:
		return m
	default:
		return newOrDSMatcher(m, others[0], others[1:]...)
	}
}

type (
	VMIncludes        []string
	HostIncludes      []string
	DatastoreIncludes []string
	ClusterIncludes   []string
)

type (
	dsDCMatcher  struct{ m matcher.Matcher }
	dsDSMatcher  struct{ m matcher.Matcher }
	orDSMatcher  struct{ lhs, rhs DatastoreMatcher }
	andDSMatcher struct{ lhs, rhs DatastoreMatcher }
)

func (m dsDCMatcher) Match(ds *rs.Datastore) bool  { return m.m.MatchString(ds.Hier.DC.Name) }
func (m dsDSMatcher) Match(ds *rs.Datastore) bool  { return m.m.MatchString(ds.Name) }
func (m orDSMatcher) Match(ds *rs.Datastore) bool  { return m.lhs.Match(ds) || m.rhs.Match(ds) }
func (m andDSMatcher) Match(ds *rs.Datastore) bool { return m.lhs.Match(ds) && m.rhs.Match(ds) }

type (
	clusterDCMatcher   struct{ m matcher.Matcher }
	clusterNameMatcher struct{ m matcher.Matcher }
	orClusterMatcher   struct{ lhs, rhs ClusterMatcher }
	andClusterMatcher  struct{ lhs, rhs ClusterMatcher }
)

func (m clusterDCMatcher) Match(c *rs.Cluster) bool   { return m.m.MatchString(c.Hier.DC.Name) }
func (m clusterNameMatcher) Match(c *rs.Cluster) bool { return m.m.MatchString(c.Name) }
func (m orClusterMatcher) Match(c *rs.Cluster) bool   { return m.lhs.Match(c) || m.rhs.Match(c) }
func (m andClusterMatcher) Match(c *rs.Cluster) bool  { return m.lhs.Match(c) && m.rhs.Match(c) }

func newOrClusterMatcher(lhs, rhs ClusterMatcher, others ...ClusterMatcher) orClusterMatcher {
	m := orClusterMatcher{lhs: lhs, rhs: rhs}
	switch len(others) {
	case 0:
		return m
	default:
		return newOrClusterMatcher(m, others[0], others[1:]...)
	}
}

func (vi VMIncludes) Parse() (VMMatcher, error) {
	var ms []VMMatcher
	for _, v := range vi {
		m, err := parseVMInclude(v)
		if err != nil {
			return nil, err
		}
		if m == nil {
			continue
		}
		ms = append(ms, m)
	}

	switch len(ms) {
	case 0:
		return nil, nil
	case 1:
		return ms[0], nil
	default:
		return newOrVMMatcher(ms[0], ms[1], ms[2:]...), nil
	}
}

func (hi HostIncludes) Parse() (HostMatcher, error) {
	var ms []HostMatcher
	for _, v := range hi {
		m, err := parseHostInclude(v)
		if err != nil {
			return nil, err
		}
		if m == nil {
			continue
		}
		ms = append(ms, m)
	}

	switch len(ms) {
	case 0:
		return nil, nil
	case 1:
		return ms[0], nil
	default:
		return newOrHostMatcher(ms[0], ms[1], ms[2:]...), nil
	}
}

func (di DatastoreIncludes) Parse() (DatastoreMatcher, error) {
	var ms []DatastoreMatcher
	for _, v := range di {
		m, err := parseDatastoreInclude(v)
		if err != nil {
			return nil, err
		}
		if m == nil {
			continue
		}
		ms = append(ms, m)
	}

	switch len(ms) {
	case 0:
		return nil, nil
	case 1:
		return ms[0], nil
	default:
		return newOrDSMatcher(ms[0], ms[1], ms[2:]...), nil
	}
}

const (
	datacenterIdx = iota
	clusterIdx
	hostIdx
	vmIdx
)

func cleanInclude(include string) string {
	return strings.Trim(include, "/")
}

func parseHostInclude(include string) (HostMatcher, error) {
	if !isIncludeFormatValid(include) {
		return nil, fmt.Errorf("bad include format: %s", include)
	}

	include = cleanInclude(include)
	parts := strings.Split(include, "/") // /dc/clusterIdx/hostIdx
	var ms []HostMatcher

	for i, v := range parts {
		m, err := parseSubInclude(v)
		if err != nil {
			return nil, err
		}
		switch i {
		case datacenterIdx:
			ms = append(ms, hostDCMatcher{m})
		case clusterIdx:
			ms = append(ms, hostClusterMatcher{m})
		case hostIdx:
			ms = append(ms, hostHostMatcher{m})
		default:
		}
	}

	switch len(ms) {
	case 0:
		return nil, nil
	case 1:
		return ms[0], nil
	default:
		return newAndHostMatcher(ms[0], ms[1], ms[2:]...), nil
	}
}

func parseVMInclude(include string) (VMMatcher, error) {
	if !isIncludeFormatValid(include) {
		return nil, fmt.Errorf("bad include format: %s", include)
	}

	include = cleanInclude(include)
	parts := strings.Split(include, "/") // /dc/clusterIdx/hostIdx/vmIdx
	var ms []VMMatcher

	for i, v := range parts {
		m, err := parseSubInclude(v)
		if err != nil {
			return nil, err
		}
		switch i {
		case datacenterIdx:
			ms = append(ms, vmDCMatcher{m})
		case clusterIdx:
			ms = append(ms, vmClusterMatcher{m})
		case hostIdx:
			ms = append(ms, vmHostMatcher{m})
		case vmIdx:
			ms = append(ms, vmVMMatcher{m})
		}
	}

	switch len(ms) {
	case 0:
		return nil, nil
	case 1:
		return ms[0], nil
	default:
		return newAndVMMatcher(ms[0], ms[1], ms[2:]...), nil
	}
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
	var ms []ClusterMatcher
	for _, v := range ci {
		m, err := parseClusterInclude(v)
		if err != nil {
			return nil, err
		}
		if m == nil {
			continue
		}
		ms = append(ms, m)
	}

	switch len(ms) {
	case 0:
		return nil, nil
	case 1:
		return ms[0], nil
	default:
		return newOrClusterMatcher(ms[0], ms[1], ms[2:]...), nil
	}
}

const (
	clDCIdx = iota
	clClusterIdx
)

func parseClusterInclude(include string) (ClusterMatcher, error) {
	if !isIncludeFormatValid(include) {
		return nil, fmt.Errorf("bad include format: %s", include)
	}

	include = cleanInclude(include)
	parts := strings.Split(include, "/") // /dc/cluster
	var ms []ClusterMatcher

	for i, v := range parts {
		m, err := parseSubInclude(v)
		if err != nil {
			return nil, err
		}
		switch i {
		case clDCIdx:
			ms = append(ms, clusterDCMatcher{m})
		case clClusterIdx:
			ms = append(ms, clusterNameMatcher{m})
		default:
		}
	}

	switch len(ms) {
	case 0:
		return nil, nil
	case 1:
		return ms[0], nil
	default:
		return andClusterMatcher{lhs: ms[0], rhs: ms[1]}, nil
	}
}

const (
	dsDatacenterIdx = iota
	dsDatastoreIdx
)

func parseDatastoreInclude(include string) (DatastoreMatcher, error) {
	if !isIncludeFormatValid(include) {
		return nil, fmt.Errorf("bad include format: %s", include)
	}

	include = cleanInclude(include)
	parts := strings.Split(include, "/") // /dc/datastore
	var ms []DatastoreMatcher

	for i, v := range parts {
		m, err := parseSubInclude(v)
		if err != nil {
			return nil, err
		}
		switch i {
		case dsDatacenterIdx:
			ms = append(ms, dsDCMatcher{m})
		case dsDatastoreIdx:
			ms = append(ms, dsDSMatcher{m})
		default:
		}
	}

	switch len(ms) {
	case 0:
		return nil, nil
	case 1:
		return ms[0], nil
	default:
		return andDSMatcher{lhs: ms[0], rhs: ms[1]}, nil
	}
}
