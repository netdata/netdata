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

type (
	VMIncludes   []string
	HostIncludes []string
)

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
