// SPDX-License-Identifier: GPL-3.0-or-later

package vsphere

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	rs "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/resources"
)

const (
	defaultMaxVMDisks      = 1024
	vmDiskCapacityContext  = "vsphere.vm_disk_capacity"
	vmDiskCapacityDim      = "capacity"
	vmDiskCapacityMetric   = "vm_disk_capacity_capacity"
	vmDiskKeyLabel         = "disk_key"
	vmDiskDisplayNameLabel = "disk"
)

func optionalMetricNames() []string {
	names := []string{
		vmDiskCapacityMetric,
	}
	names = append(names, datastoreClusterOptionalMetricNames()...)
	names = append(names, vmDiskPerformanceOptionalMetricNames()...)
	names = append(names, vmNICPerformanceOptionalMetricNames()...)
	names = append(names, hostNICPerformanceOptionalMetricNames()...)
	names = append(names, hostDiskPerformanceOptionalMetricNames()...)
	names = append(names, hostStorageAdapterPerformanceOptionalMetricNames()...)
	names = append(names, hostStoragePathPerformanceOptionalMetricNames()...)
	names = append(names, hostCPUInstancePerformanceOptionalMetricNames()...)
	names = append(names, powerMetricsOptionalMetricNames()...)
	names = append(names, vsanOptionalMetricNames()...)
	return names
}

func newSimplePatternsMatcher(name string, patterns []string) (matcher.Matcher, error) {
	var terms simplePatternListMatcher
	hasPositive := false
	for _, pattern := range patterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		term := simplePatternListTerm{positive: true}
		if strings.HasPrefix(pattern, "!") {
			term.positive = false
			pattern = strings.TrimSpace(strings.TrimPrefix(pattern, "!"))
		}
		if pattern == "" {
			return nil, fmt.Errorf("%s has invalid empty negative pattern", name)
		}
		hasPositive = hasPositive || term.positive
		m, err := matcher.NewGlobMatcher(pattern)
		if err != nil {
			return nil, fmt.Errorf("%s has invalid pattern: %w", name, err)
		}
		term.matcher = m
		terms = append(terms, term)
	}
	if len(terms) == 0 {
		return matcher.FALSE(), nil
	}
	if !hasPositive {
		return nil, fmt.Errorf("%s must include at least one positive pattern", name)
	}
	return terms, nil
}

type simplePatternListTerm struct {
	matcher  matcher.Matcher
	positive bool
}

type simplePatternListMatcher []simplePatternListTerm

func (m simplePatternListMatcher) Match(b []byte) bool {
	return m.MatchString(string(b))
}

func (m simplePatternListMatcher) MatchString(line string) bool {
	for _, term := range m {
		if term.matcher.MatchString(line) {
			return term.positive
		}
	}
	return false
}

func (c *Collector) writeOptionalMetrics(meter metrix.SnapshotMeter) {
	c.writeDatastoreClusterMetrics(meter)
	c.writeVMDiskMetrics(meter)
	c.writeVMDiskPerformanceMetrics(meter)
	c.writeVMNICPerformanceMetrics(meter)
	c.writeHostNICPerformanceMetrics(meter)
	c.writeHostDiskPerformanceMetrics(meter)
	c.writeHostStorageAdapterPerformanceMetrics(meter)
	c.writeHostStoragePathPerformanceMetrics(meter)
	c.writeHostCPUInstancePerformanceMetrics(meter)
	c.writePowerMetrics(meter)
	c.writeVSANMetrics(meter)
}

func (c *Collector) writeVMDiskMetrics(meter metrix.SnapshotMeter) {
	if !c.CollectVMDisks || c.resources == nil || c.MaxVMDisks < 1 {
		return
	}

	m := c.vmDiskMatcher
	if m == nil {
		m = matcher.TRUE()
	}

	gaugeName := v2MetricName(vmDiskCapacityContext, vmDiskCapacityDim)
	count := 0
	for _, vm := range sortedVMs(c.resources.VMs) {
		for _, disk := range sortedVMDiskCopy(vm.Disks) {
			if !vmDiskMatches(m, disk) {
				continue
			}
			if count >= c.MaxVMDisks {
				return
			}
			count++

			scope := c.resourceHostScope(vm.ID)
			writeMeter := meter
			scoped := !scope.IsDefault()
			if scoped {
				writeMeter = meter.WithHostScope(scope)
			}
			if gauge := c.mx.gauge(writeMeter, gaugeName, scoped); gauge != nil {
				gauge.Observe(metrix.SampleValue(disk.CapacityBytes), writeMeter.LabelSet(c.vmDiskLabels(vm, disk)...))
			}
		}
	}
}

func (c *Collector) vmDiskLabels(vm *rs.VM, disk rs.VMDisk) []metrix.Label {
	labels := []metrix.Label{
		{Key: "id", Value: vm.ID},
		{Key: "datacenter", Value: vm.Hier.DC.Name},
		{Key: "cluster", Value: vm.Hier.Cluster.Name},
		{Key: "host", Value: vm.Hier.Host.Name},
		{Key: "vm", Value: vm.Name},
		{Key: vmDiskDisplayNameLabel, Value: diskDisplayName(disk)},
		{Key: vmDiskKeyLabel, Value: vmDiskKey(disk)},
	}
	labels = append(labels, c.resourceEnrichmentLabels(vm.ID)...)
	return labels
}

func vmDiskMatches(m matcher.Matcher, disk rs.VMDisk) bool {
	label := diskDisplayName(disk)
	key := vmDiskKey(disk)
	return m.MatchString(label) || m.MatchString(key) || m.MatchString("key:"+key)
}

func diskDisplayName(disk rs.VMDisk) string {
	if strings.TrimSpace(disk.Label) != "" {
		return disk.Label
	}
	return "disk-" + vmDiskKey(disk)
}

func vmDiskKey(disk rs.VMDisk) string {
	return strconv.FormatInt(int64(disk.Key), 10)
}

func sortedVMs(vms rs.VMs) []*rs.VM {
	out := make([]*rs.VM, 0, len(vms))
	for _, vm := range vms {
		out = append(out, vm)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	return out
}

func sortedVMDiskCopy(disks []rs.VMDisk) []rs.VMDisk {
	out := append([]rs.VMDisk(nil), disks...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Key != out[j].Key {
			return out[i].Key < out[j].Key
		}
		return out[i].Label < out[j].Label
	})
	return out
}
