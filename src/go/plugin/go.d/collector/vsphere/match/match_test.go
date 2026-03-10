// SPDX-License-Identifier: GPL-3.0-or-later

package match

import (
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/resources"

	"github.com/stretchr/testify/assert"
)

var (
	trueHostDC  = hostDCMatcher{matcher.TRUE()}
	falseHostDC = hostDCMatcher{matcher.FALSE()}
	trueVMDC    = vmDCMatcher{matcher.TRUE()}
	falseVMDC   = vmDCMatcher{matcher.FALSE()}
	trueDSDC    = dsDCMatcher{matcher.TRUE()}
	falseDSDC   = dsDCMatcher{matcher.FALSE()}
	trueClDC    = clusterDCMatcher{matcher.TRUE()}
	falseClDC   = clusterDCMatcher{matcher.FALSE()}
)

func TestOrHostMatcher_Match(t *testing.T) {
	tests := map[string]struct {
		expected bool
		lhs      HostMatcher
		rhs      HostMatcher
	}{
		"true, true":   {expected: true, lhs: trueHostDC, rhs: trueHostDC},
		"true, false":  {expected: true, lhs: trueHostDC, rhs: falseHostDC},
		"false, true":  {expected: true, lhs: falseHostDC, rhs: trueHostDC},
		"false, false": {expected: false, lhs: falseHostDC, rhs: falseHostDC},
	}

	var host resources.Host
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			m := newOrHostMatcher(test.lhs, test.rhs)
			assert.Equal(t, test.expected, m.Match(&host))
		})
	}
}

func TestAndHostMatcher_Match(t *testing.T) {
	tests := map[string]struct {
		expected bool
		lhs      HostMatcher
		rhs      HostMatcher
	}{
		"true, true":   {expected: true, lhs: trueHostDC, rhs: trueHostDC},
		"true, false":  {expected: false, lhs: trueHostDC, rhs: falseHostDC},
		"false, true":  {expected: false, lhs: falseHostDC, rhs: trueHostDC},
		"false, false": {expected: false, lhs: falseHostDC, rhs: falseHostDC},
	}

	var host resources.Host
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			m := newAndHostMatcher(test.lhs, test.rhs)
			assert.Equal(t, test.expected, m.Match(&host))
		})
	}
}

func TestOrVMMatcher_Match(t *testing.T) {
	tests := map[string]struct {
		expected bool
		lhs      VMMatcher
		rhs      VMMatcher
	}{
		"true, true":   {expected: true, lhs: trueVMDC, rhs: trueVMDC},
		"true, false":  {expected: true, lhs: trueVMDC, rhs: falseVMDC},
		"false, true":  {expected: true, lhs: falseVMDC, rhs: trueVMDC},
		"false, false": {expected: false, lhs: falseVMDC, rhs: falseVMDC},
	}

	var vm resources.VM
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			m := newOrVMMatcher(test.lhs, test.rhs)
			assert.Equal(t, test.expected, m.Match(&vm))
		})
	}
}

func TestAndVMMatcher_Match(t *testing.T) {
	tests := map[string]struct {
		expected bool
		lhs      VMMatcher
		rhs      VMMatcher
	}{
		"true, true":   {expected: true, lhs: trueVMDC, rhs: trueVMDC},
		"true, false":  {expected: false, lhs: trueVMDC, rhs: falseVMDC},
		"false, true":  {expected: false, lhs: falseVMDC, rhs: trueVMDC},
		"false, false": {expected: false, lhs: falseVMDC, rhs: falseVMDC},
	}

	var vm resources.VM
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			m := newAndVMMatcher(test.lhs, test.rhs)
			assert.Equal(t, test.expected, m.Match(&vm))
		})
	}
}

func TestHostIncludes_Parse(t *testing.T) {
	tests := map[string]struct {
		valid    bool
		expected HostMatcher
	}{
		"":        {valid: false},
		"*/C1/H1": {valid: false},
		"/":       {valid: true, expected: falseHostDC},
		"/*":      {valid: true, expected: trueHostDC},
		"/!*":     {valid: true, expected: falseHostDC},
		"/!*/":    {valid: true, expected: falseHostDC},
		"/!*/ ": {
			valid: true,
			expected: andHostMatcher{
				lhs: falseHostDC,
				rhs: hostClusterMatcher{matcher.FALSE()},
			},
		},
		"/DC1* DC2* !*/Cluster*": {
			valid: true,
			expected: andHostMatcher{
				lhs: hostDCMatcher{mustSP("DC1* DC2* !*")},
				rhs: hostClusterMatcher{mustSP("Cluster*")},
			},
		},
		"/*/*/HOST1*": {
			valid: true,
			expected: andHostMatcher{
				lhs: andHostMatcher{
					lhs: trueHostDC,
					rhs: hostClusterMatcher{matcher.TRUE()},
				},
				rhs: hostHostMatcher{mustSP("HOST1*")},
			},
		},
		"/*/*/HOST1*/*/*": {
			valid: true,
			expected: andHostMatcher{
				lhs: andHostMatcher{
					lhs: trueHostDC,
					rhs: hostClusterMatcher{matcher.TRUE()},
				},
				rhs: hostHostMatcher{mustSP("HOST1*")},
			},
		},
		"[/DC1*,/DC2*]": {
			valid: true,
			expected: orHostMatcher{
				lhs: hostDCMatcher{mustSP("DC1*")},
				rhs: hostDCMatcher{mustSP("DC2*")},
			},
		},
		"[/DC1*,/DC2*,/DC3*/Cluster1*/H*]": {
			valid: true,
			expected: orHostMatcher{
				lhs: orHostMatcher{
					lhs: hostDCMatcher{mustSP("DC1*")},
					rhs: hostDCMatcher{mustSP("DC2*")},
				},
				rhs: andHostMatcher{
					lhs: andHostMatcher{
						lhs: hostDCMatcher{mustSP("DC3*")},
						rhs: hostClusterMatcher{mustSP("Cluster1*")},
					},
					rhs: hostHostMatcher{mustSP("H*")},
				},
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
				assert.Equal(t, test.expected, m)
			}
		})
	}
}

func TestVMIncludes_Parse(t *testing.T) {
	tests := map[string]struct {
		valid    bool
		includes []string
		expected VMMatcher
	}{
		"":           {valid: false},
		"*/C1/H1/V1": {valid: false},
		"/*":         {valid: true, expected: trueVMDC},
		"/!*":        {valid: true, expected: falseVMDC},
		"/!*/":       {valid: true, expected: falseVMDC},
		"/!*/ ": {
			valid: true,
			expected: andVMMatcher{
				lhs: falseVMDC,
				rhs: vmClusterMatcher{matcher.FALSE()},
			},
		},
		"/DC1* DC2* !*/Cluster*": {
			valid: true,
			expected: andVMMatcher{
				lhs: vmDCMatcher{mustSP("DC1* DC2* !*")},
				rhs: vmClusterMatcher{mustSP("Cluster*")},
			},
		},
		"/*/*/HOST1": {
			valid: true,
			expected: andVMMatcher{
				lhs: andVMMatcher{
					lhs: trueVMDC,
					rhs: vmClusterMatcher{matcher.TRUE()},
				},
				rhs: vmHostMatcher{mustSP("HOST1")},
			},
		},
		"/*/*/HOST1*/*/*": {
			valid: true,
			expected: andVMMatcher{
				lhs: andVMMatcher{
					lhs: andVMMatcher{
						lhs: trueVMDC,
						rhs: vmClusterMatcher{matcher.TRUE()},
					},
					rhs: vmHostMatcher{mustSP("HOST1*")},
				},
				rhs: vmVMMatcher{matcher.TRUE()},
			},
		},
		"[/DC1*,/DC2*]": {
			valid: true,
			expected: orVMMatcher{
				lhs: vmDCMatcher{mustSP("DC1*")},
				rhs: vmDCMatcher{mustSP("DC2*")},
			},
		},
		"[/DC1*,/DC2*,/DC3*/Cluster1*/H*/VM*]": {
			valid: true,
			expected: orVMMatcher{
				lhs: orVMMatcher{
					lhs: vmDCMatcher{mustSP("DC1*")},
					rhs: vmDCMatcher{mustSP("DC2*")},
				},
				rhs: andVMMatcher{
					lhs: andVMMatcher{
						lhs: andVMMatcher{
							lhs: vmDCMatcher{mustSP("DC3*")},
							rhs: vmClusterMatcher{mustSP("Cluster1*")},
						},
						rhs: vmHostMatcher{mustSP("H*")},
					},
					rhs: vmVMMatcher{mustSP("VM*")},
				},
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
				assert.Equal(t, test.expected, m)
			}
		})
	}
}

func TestOrDSMatcher_Match(t *testing.T) {
	tests := map[string]struct {
		expected bool
		lhs      DatastoreMatcher
		rhs      DatastoreMatcher
	}{
		"true, true":   {expected: true, lhs: trueDSDC, rhs: trueDSDC},
		"true, false":  {expected: true, lhs: trueDSDC, rhs: falseDSDC},
		"false, true":  {expected: true, lhs: falseDSDC, rhs: trueDSDC},
		"false, false": {expected: false, lhs: falseDSDC, rhs: falseDSDC},
	}

	var ds resources.Datastore
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			m := orDSMatcher{lhs: test.lhs, rhs: test.rhs}
			assert.Equal(t, test.expected, m.Match(&ds))
		})
	}
}

func TestAndDSMatcher_Match(t *testing.T) {
	tests := map[string]struct {
		expected bool
		lhs      DatastoreMatcher
		rhs      DatastoreMatcher
	}{
		"true, true":   {expected: true, lhs: trueDSDC, rhs: trueDSDC},
		"true, false":  {expected: false, lhs: trueDSDC, rhs: falseDSDC},
		"false, true":  {expected: false, lhs: falseDSDC, rhs: trueDSDC},
		"false, false": {expected: false, lhs: falseDSDC, rhs: falseDSDC},
	}

	var ds resources.Datastore
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			m := andDSMatcher{lhs: test.lhs, rhs: test.rhs}
			assert.Equal(t, test.expected, m.Match(&ds))
		})
	}
}

func TestDatastoreIncludes_Parse(t *testing.T) {
	tests := map[string]struct {
		valid    bool
		expected DatastoreMatcher
	}{
		"":      {valid: false},
		"*/DS1": {valid: false},
		"/":     {valid: true, expected: falseDSDC},
		"/*":    {valid: true, expected: trueDSDC},
		"/!*":   {valid: true, expected: falseDSDC},
		"/!*/":  {valid: true, expected: falseDSDC},
		"/!*/ ": {
			valid: true,
			expected: andDSMatcher{
				lhs: falseDSDC,
				rhs: dsDSMatcher{matcher.FALSE()},
			},
		},
		"/DC1*/DS*": {
			valid: true,
			expected: andDSMatcher{
				lhs: dsDCMatcher{mustSP("DC1*")},
				rhs: dsDSMatcher{mustSP("DS*")},
			},
		},
		"/*/*/extra": {
			valid: true,
			expected: andDSMatcher{
				lhs: trueDSDC,
				rhs: dsDSMatcher{matcher.TRUE()},
			},
		},
		"[/DC1*,/DC2*]": {
			valid: true,
			expected: orDSMatcher{
				lhs: dsDCMatcher{mustSP("DC1*")},
				rhs: dsDCMatcher{mustSP("DC2*")},
			},
		},
		"[/DC1*,/DC2*,/DC3*/DS*]": {
			valid: true,
			expected: orDSMatcher{
				lhs: orDSMatcher{
					lhs: dsDCMatcher{mustSP("DC1*")},
					rhs: dsDCMatcher{mustSP("DC2*")},
				},
				rhs: andDSMatcher{
					lhs: dsDCMatcher{mustSP("DC3*")},
					rhs: dsDSMatcher{mustSP("DS*")},
				},
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
				assert.Equal(t, test.expected, m)
			}
		})
	}
}

func TestOrClusterMatcher_Match(t *testing.T) {
	tests := map[string]struct {
		expected bool
		lhs      ClusterMatcher
		rhs      ClusterMatcher
	}{
		"true, true":   {expected: true, lhs: trueClDC, rhs: trueClDC},
		"true, false":  {expected: true, lhs: trueClDC, rhs: falseClDC},
		"false, true":  {expected: true, lhs: falseClDC, rhs: trueClDC},
		"false, false": {expected: false, lhs: falseClDC, rhs: falseClDC},
	}

	var cl resources.Cluster
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			m := orClusterMatcher{lhs: test.lhs, rhs: test.rhs}
			assert.Equal(t, test.expected, m.Match(&cl))
		})
	}
}

func TestAndClusterMatcher_Match(t *testing.T) {
	tests := map[string]struct {
		expected bool
		lhs      ClusterMatcher
		rhs      ClusterMatcher
	}{
		"true, true":   {expected: true, lhs: trueClDC, rhs: trueClDC},
		"true, false":  {expected: false, lhs: trueClDC, rhs: falseClDC},
		"false, true":  {expected: false, lhs: falseClDC, rhs: trueClDC},
		"false, false": {expected: false, lhs: falseClDC, rhs: falseClDC},
	}

	var cl resources.Cluster
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			m := andClusterMatcher{lhs: test.lhs, rhs: test.rhs}
			assert.Equal(t, test.expected, m.Match(&cl))
		})
	}
}

func TestClusterIncludes_Parse(t *testing.T) {
	tests := map[string]struct {
		valid    bool
		expected ClusterMatcher
	}{
		"":      {valid: false},
		"*/C1":  {valid: false},
		"/":     {valid: true, expected: falseClDC},
		"/*":    {valid: true, expected: trueClDC},
		"/!*":   {valid: true, expected: falseClDC},
		"/!*/":  {valid: true, expected: falseClDC},
		"/!*/ ": {
			valid: true,
			expected: andClusterMatcher{
				lhs: falseClDC,
				rhs: clusterNameMatcher{matcher.FALSE()},
			},
		},
		"/DC1*/Cluster*": {
			valid: true,
			expected: andClusterMatcher{
				lhs: clusterDCMatcher{mustSP("DC1*")},
				rhs: clusterNameMatcher{mustSP("Cluster*")},
			},
		},
		"/*/*/extra": {
			valid: true,
			expected: andClusterMatcher{
				lhs: trueClDC,
				rhs: clusterNameMatcher{matcher.TRUE()},
			},
		},
		"[/DC1*,/DC2*]": {
			valid: true,
			expected: orClusterMatcher{
				lhs: clusterDCMatcher{mustSP("DC1*")},
				rhs: clusterDCMatcher{mustSP("DC2*")},
			},
		},
		"[/DC1*,/DC2*,/DC3*/Cluster*]": {
			valid: true,
			expected: orClusterMatcher{
				lhs: orClusterMatcher{
					lhs: clusterDCMatcher{mustSP("DC1*")},
					rhs: clusterDCMatcher{mustSP("DC2*")},
				},
				rhs: andClusterMatcher{
					lhs: clusterDCMatcher{mustSP("DC3*")},
					rhs: clusterNameMatcher{mustSP("Cluster*")},
				},
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
				assert.Equal(t, test.expected, m)
			}
		})
	}
}

func prepareIncludes(include string) []string {
	trimmed := strings.Trim(include, "[]")
	return strings.Split(trimmed, ",")
}

func mustSP(expr string) matcher.Matcher {
	return matcher.Must(matcher.NewSimplePatternsMatcher(expr))
}
