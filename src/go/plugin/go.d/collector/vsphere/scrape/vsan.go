// SPDX-License-Identifier: GPL-3.0-or-later

package scrape

import (
	"errors"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	rs "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/resources"

	"github.com/vmware/govmomi/vim25/soap"
	"github.com/vmware/govmomi/vim25/types"
	vsantypes "github.com/vmware/govmomi/vsan/types"
)

const (
	defaultVSANPerfInterval = 300
	maxVSANWarningKeys      = 64
)

const (
	vsanQueryClusterPrefix = "cluster-domclient:"
	vsanQueryHostPrefix    = "host-domclient:"
	vsanQueryVMPrefix      = "virtual-machine:"
)

type VSANMetrics struct {
	Clusters map[string]VSANEntityMetrics
	Hosts    map[string]VSANEntityMetrics
	VMs      map[string]VSANEntityMetrics
	Space    map[string]VSANSpaceUsage
	Health   map[string]string
}

type VSANEntityMetrics map[string]float64

type VSANSpaceUsage struct {
	Total int64
	Free  int64
}

type vsanMetricSpec struct {
	name string
	rate bool
}

var (
	vsanClusterMetricSpecs = map[string]vsanMetricSpec{
		"iopsRead":        {name: "read_operations"},
		"iopsWrite":       {name: "write_operations"},
		"throughputRead":  {name: "read_throughput", rate: true},
		"throughputWrite": {name: "write_throughput", rate: true},
		"latencyAvgRead":  {name: "read_latency"},
		"latencyAvgWrite": {name: "write_latency"},
		"congestion":      {name: "congestions", rate: true},
	}
	vsanHostMetricSpecs = map[string]vsanMetricSpec{
		"iopsRead":           {name: "read_operations"},
		"iopsWrite":          {name: "write_operations"},
		"throughputRead":     {name: "read_throughput", rate: true},
		"throughputWrite":    {name: "write_throughput", rate: true},
		"latencyAvgRead":     {name: "read_latency"},
		"latencyAvgWrite":    {name: "write_latency"},
		"congestion":         {name: "congestions", rate: true},
		"clientCacheHitRate": {name: "cache_hit_rate"},
	}
	vsanVMMetricSpecs = map[string]vsanMetricSpec{
		"iopsRead":        {name: "read_operations"},
		"iopsWrite":       {name: "write_operations"},
		"throughputRead":  {name: "read_throughput", rate: true},
		"throughputWrite": {name: "write_throughput", rate: true},
		"latencyRead":     {name: "read_latency"},
		"latencyWrite":    {name: "write_latency"},
	}
)

func (s *Scraper) ScrapeVSAN(clusters rs.Clusters, hosts rs.Hosts, vms rs.VMs) *VSANMetrics {
	out := &VSANMetrics{
		Clusters: make(map[string]VSANEntityMetrics),
		Hosts:    make(map[string]VSANEntityMetrics),
		VMs:      make(map[string]VSANEntityMetrics),
		Space:    make(map[string]VSANSpaceUsage),
		Health:   make(map[string]string),
	}

	vsanClusters := sortedVSANClusters(clusters)
	if len(vsanClusters) == 0 {
		return out
	}

	clusterByUUID := clusterIDByVSANUUID(clusters)
	hostByUUID := hostIDByVSANNodeUUID(hosts)
	vmByUUID := vmIDByInstanceUUID(vms)

	for _, cluster := range vsanClusters {
		s.scrapeVSANClusterSummary(out, cluster)
		s.scrapeVSANPerf(out.Clusters, cluster.Ref, vsanClusterQueryIDs(cluster), vsanClusterMetricSpecs, clusterByUUID)
		s.scrapeVSANPerf(out.Hosts, cluster.Ref, vsanHostQueryIDs(cluster, hosts), vsanHostMetricSpecs, hostByUUID)
		s.scrapeVSANPerf(out.VMs, cluster.Ref, vsanVMQueryIDs(cluster, vms), vsanVMMetricSpecs, vmByUUID)
	}

	return out
}

func (s *Scraper) scrapeVSANClusterSummary(out *VSANMetrics, cluster *rs.Cluster) {
	space, err := s.VSANSpaceUsage(cluster.Ref)
	if err != nil {
		s.warnVSANOnce("space:"+vsanFaultName(err), "failed to query vSAN space usage for cluster %s: %v", cluster.ID, err)
	} else if space != nil {
		out.Space[cluster.ID] = VSANSpaceUsage{
			Total: space.TotalCapacityB,
			Free:  space.FreeCapacityB,
		}
	}

	health, err := s.VSANHealth(cluster.Ref)
	if err != nil {
		s.warnVSANOnce("health:"+vsanFaultName(err), "failed to query vSAN health for cluster %s: %v", cluster.ID, err)
		return
	}
	out.Health[cluster.ID] = health
}

func (s *Scraper) scrapeVSANPerf(dst map[string]VSANEntityMetrics, cluster types.ManagedObjectReference, queries []string, specs map[string]vsanMetricSpec, idByUUID map[string]string) {
	if len(queries) == 0 {
		return
	}
	now := time.Now()
	start := now.Add(-defaultVSANPerfInterval * time.Second)
	qspecs := make([]vsantypes.VsanPerfQuerySpec, 0, len(queries))
	labels := sortedVSANMetricLabels(specs)
	for _, query := range queries {
		qspecs = append(qspecs, vsantypes.VsanPerfQuerySpec{
			EntityRefId: query,
			StartTime:   &start,
			EndTime:     &now,
			Labels:      labels,
		})
	}
	raw, err := s.VSANPerfMetrics(cluster, qspecs)
	if err != nil {
		s.warnVSANOnce("perf:"+queries[0]+":"+vsanFaultName(err), "failed to query %d vSAN performance entity refs for cluster %s: %v", len(queries), cluster.Value, err)
		return
	}

	values, err := parseVSANEntityMetrics(raw, specs)
	if err != nil {
		s.warnVSANOnce("parse:"+queries[0], "failed to parse vSAN performance metrics for cluster %s: %v", cluster.Value, err)
		return
	}

	for uuid, metrics := range values {
		id := idByUUID[uuid]
		if id == "" || len(metrics) == 0 {
			continue
		}
		dst[id] = metrics
	}
}

func parseVSANEntityMetrics(raw []vsantypes.VsanPerfEntityMetricCSV, specs map[string]vsanMetricSpec) (map[string]VSANEntityMetrics, error) {
	out := make(map[string]VSANEntityMetrics)
	for _, entity := range raw {
		uuid, ok := vsanEntityUUID(entity.EntityRefId)
		if !ok {
			continue
		}
		values := make(VSANEntityMetrics)
		sampleIndex := latestVSANSampleIndex(entity)
		for _, series := range entity.Value {
			spec, ok := specs[series.MetricId.Label]
			if !ok {
				continue
			}
			value, ok := latestVSANValue(series.Values, sampleIndex)
			if !ok {
				continue
			}
			if spec.rate {
				interval := series.MetricId.MetricsCollectInterval
				if interval == 0 {
					interval = defaultVSANPerfInterval
				}
				value /= float64(interval)
			}
			values[spec.name] = value
		}
		if len(values) > 0 {
			out[uuid] = values
		}
	}
	return out, nil
}

func latestVSANSampleIndex(entity vsantypes.VsanPerfEntityMetricCSV) int {
	sampleCount := len(csvParts(entity.SampleInfo))
	if sampleCount == 0 {
		return -1
	}
	latest := -1
	for _, series := range entity.Value {
		parts := csvParts(series.Values)
		limit := len(parts) - 1
		if sampleCount > 0 && limit >= sampleCount {
			limit = sampleCount - 1
		}
		for i := limit; i >= 0; i-- {
			if strings.TrimSpace(parts[i]) == "" {
				continue
			}
			if i > latest {
				latest = i
			}
			break
		}
	}
	return latest
}

func latestVSANValue(csv string, sampleIndex int) (float64, bool) {
	parts := csvParts(csv)
	if sampleIndex >= 0 {
		if sampleIndex >= len(parts) {
			return 0, false
		}
		part := strings.TrimSpace(parts[sampleIndex])
		if part == "" {
			return 0, false
		}
		v, err := strconv.ParseFloat(part, 64)
		return v, err == nil
	}

	return latestNonEmptyVSANValue(parts)
}

func latestNonEmptyVSANValue(parts []string) (float64, bool) {
	if len(parts) == 0 {
		return 0, false
	}
	for i := len(parts) - 1; i >= 0; i-- {
		part := strings.TrimSpace(parts[i])
		if part == "" {
			continue
		}
		v, err := strconv.ParseFloat(part, 64)
		return v, err == nil
	}
	return 0, false
}

func csvParts(csv string) []string {
	if csv == "" {
		return nil
	}
	return strings.Split(csv, ",")
}

func vsanEntityUUID(refID string) (string, bool) {
	_, uuid, ok := strings.Cut(refID, ":")
	return uuid, ok && uuid != ""
}

func sortedVSANMetricLabels(specs map[string]vsanMetricSpec) []string {
	labels := make([]string, 0, len(specs))
	for label := range specs {
		labels = append(labels, label)
	}
	sort.Strings(labels)
	return labels
}

func sortedVSANClusters(clusters rs.Clusters) []*rs.Cluster {
	out := make([]*rs.Cluster, 0, len(clusters))
	for _, cluster := range clusters {
		if cluster.VSANEnabled {
			out = append(out, cluster)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func vsanClusterQueryIDs(cluster *rs.Cluster) []string {
	if cluster == nil || cluster.VSANUUID == "" {
		return nil
	}
	return []string{vsanQueryClusterPrefix + cluster.VSANUUID}
}

func vsanHostQueryIDs(cluster *rs.Cluster, hosts rs.Hosts) []string {
	if cluster == nil {
		return nil
	}
	var out []string
	for _, host := range hosts {
		if host.Hier.Cluster.ID == cluster.ID && host.VSANNodeUUID != "" {
			out = append(out, vsanQueryHostPrefix+host.VSANNodeUUID)
		}
	}
	sort.Strings(out)
	return out
}

func vsanVMQueryIDs(cluster *rs.Cluster, vms rs.VMs) []string {
	if cluster == nil {
		return nil
	}
	var out []string
	for _, vm := range vms {
		if vm.Hier.Cluster.ID == cluster.ID && vm.InstanceUUID != "" {
			out = append(out, vsanQueryVMPrefix+vm.InstanceUUID)
		}
	}
	sort.Strings(out)
	return out
}

func clusterIDByVSANUUID(clusters rs.Clusters) map[string]string {
	out := make(map[string]string, len(clusters))
	for _, cluster := range clusters {
		if cluster.VSANUUID != "" {
			out[cluster.VSANUUID] = cluster.ID
		}
	}
	return out
}

func hostIDByVSANNodeUUID(hosts rs.Hosts) map[string]string {
	out := make(map[string]string, len(hosts))
	for _, host := range hosts {
		if host.VSANNodeUUID != "" {
			out[host.VSANNodeUUID] = host.ID
		}
	}
	return out
}

func vmIDByInstanceUUID(vms rs.VMs) map[string]string {
	out := make(map[string]string, len(vms))
	for _, vm := range vms {
		if vm.InstanceUUID != "" {
			out[vm.InstanceUUID] = vm.ID
		}
	}
	return out
}

func (s *Scraper) warnVSANOnce(key, format string, args ...any) {
	s.vsanWarningsLock.Lock()
	defer s.vsanWarningsLock.Unlock()

	if s.vsanWarnings == nil {
		s.vsanWarnings = make(map[string]bool)
	}
	if s.vsanWarnings[key] {
		return
	}
	if len(s.vsanWarnings) >= maxVSANWarningKeys {
		return
	}
	s.vsanWarnings[key] = true
	s.Warningf(format, args...)
}

func vsanFaultName(err error) string {
	for e := err; e != nil; e = errors.Unwrap(e) {
		if !soap.IsSoapFault(e) {
			continue
		}
		fault := soap.ToSoapFault(e)
		if fault.Detail.Fault != nil {
			t := reflect.TypeOf(fault.Detail.Fault)
			if t.Kind() == reflect.Ptr {
				t = t.Elem()
			}
			return t.Name()
		}
		if fault.String != "" {
			return fault.String
		}
	}
	return "unknown"
}
