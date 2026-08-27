// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"errors"
	"fmt"
	"net"

	topologyv1 "github.com/netdata/netdata/go/plugins/pkg/topology/v1"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/diagnostic"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyoptions"
	topologyv1renderer "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyv1"
)

func replayTopologyGraphV1(
	manifest diagnostic.ManifestV1,
	source diagnostic.MemberSource,
	limits diagnostic.ReaderLimits,
) (topologyv1.Data, bool, error) {
	registry := diagnostic.NewRegistry()
	capability := diagnostic.GraphCapabilityV1()
	if err := registry.Register(capability, diagnostic.GraphClosureV1()); err != nil {
		return topologyv1.Data{}, false, err
	}
	report, err := registry.ValidateCapability(manifest, source, capability, limits)
	if err != nil {
		return topologyv1.Data{}, false, err
	}
	if !report.Replayable {
		return topologyv1.Data{}, false, fmt.Errorf("graph capability is not replayable: state=%s", report.State)
	}

	rootRef, ok := diagnosticCapabilityRoot(manifest, capability)
	if !ok {
		return topologyv1.Data{}, false, errors.New("graph capability root is missing")
	}
	var root diagnostic.CapabilityRootV1
	if err := diagnostic.DecodeReferenced(source, rootRef, limits, &root); err != nil {
		return topologyv1.Data{}, false, err
	}

	generationSection := diagnosticSection(root, diagnostic.GraphSectionGeneration)
	querySection := diagnosticSection(root, diagnostic.GraphSectionQuery)
	dnsSection := diagnosticSection(root, diagnostic.GraphSectionDNSTrace)
	ouiSection := diagnosticSection(root, diagnostic.GraphSectionOUITrace)

	var generation diagnostic.GenerationV1
	if err := diagnostic.DecodeReferenced(source, generationSection.Members[0], limits, &generation); err != nil {
		return topologyv1.Data{}, false, err
	}
	var query diagnostic.GraphQueryV1
	if err := diagnostic.DecodeReferenced(source, querySection.Members[0], limits, &query); err != nil {
		return topologyv1.Data{}, false, err
	}
	dnsTrace, err := decodeGraphDNSTrace(source, dnsSection, limits)
	if err != nil {
		return topologyv1.Data{}, false, err
	}
	ouiTrace, err := decodeGraphOUITrace(source, ouiSection, limits)
	if err != nil {
		return topologyv1.Data{}, false, err
	}

	work := &replayWorkBudget{limit: limits.MaxReplayWork}
	snapshots := make([]topologymodel.ObservationSnapshot, 0, len(generation.Observations))
	for i, ref := range generation.Observations {
		var observation diagnostic.ObservationV1
		if err := diagnostic.DecodeReferenced(source, ref, limits, &observation); err != nil {
			return topologyv1.Data{}, false, fmt.Errorf("decode graph observation %d: %w", i, err)
		}
		if err := addGraphObservationWork(work, observation); err != nil {
			return topologyv1.Data{}, false, err
		}
		snapshot, err := diagnosticObservationToSnapshot(observation)
		if err != nil {
			return topologyv1.Data{}, false, fmt.Errorf("convert graph observation %d: %w", i, err)
		}
		snapshots = append(snapshots, snapshot)
	}
	if err := work.add(uint64(len(dnsTrace.Records) + len(ouiTrace.Records))); err != nil {
		return topologyv1.Data{}, false, err
	}

	aggregate, aggregateOK := aggregateTopologyObservationSnapshots(snapshots)
	if aggregateOK {
		aggregate.ProducerScopeID = generation.ProducerScopeID
	}
	dnsReplay := graphDNSReplay{records: dnsTrace.Records}
	ouiReplay := graphOUIReplay{records: ouiTrace.Records}
	options := topologyQueryOptionsFromDiagnostic(query.Options)
	options.ResolveDNSName = dnsReplay.lookup
	options.LookupVendorByMAC = ouiReplay.lookup

	data, ok, err := buildSNMPTopologySnapshot(aggregate, options)
	if err != nil {
		return topologyv1.Data{}, false, err
	}
	if err := errors.Join(dnsReplay.err, ouiReplay.err); err != nil {
		return topologyv1.Data{}, false, err
	}
	if dnsReplay.position != len(dnsReplay.records) {
		return topologyv1.Data{}, false, errors.New("graph replay did not consume the complete DNS trace")
	}
	if ouiReplay.position != len(ouiReplay.records) {
		return topologyv1.Data{}, false, errors.New("graph replay did not consume the complete OUI trace")
	}

	if root.State == diagnostic.StateEmpty {
		if ok {
			return topologyv1.Data{}, false, errors.New("empty graph capture replayed as a topology")
		}
		return topologyv1.Data{}, false, nil
	}
	if !ok {
		return topologyv1.Data{}, false, errors.New("successful graph capture replayed without a topology")
	}

	payload, err := topologyv1renderer.Render(data)
	if err != nil {
		return topologyv1.Data{}, false, err
	}
	if err := work.add(uint64(payload.Actors.Rows + payload.Links.Rows)); err != nil {
		return topologyv1.Data{}, false, err
	}
	return payload, true, nil
}

func decodeGraphDNSTrace(
	source diagnostic.MemberSource,
	section diagnostic.SectionInventoryV1,
	limits diagnostic.ReaderLimits,
) (diagnostic.DNSTraceV1, error) {
	if section.State == diagnostic.StateEmpty {
		return diagnostic.DNSTraceV1{}, nil
	}
	var trace diagnostic.DNSTraceV1
	if err := diagnostic.DecodeReferenced(source, section.Members[0], limits, &trace); err != nil {
		return diagnostic.DNSTraceV1{}, err
	}
	return trace, nil
}

func decodeGraphOUITrace(
	source diagnostic.MemberSource,
	section diagnostic.SectionInventoryV1,
	limits diagnostic.ReaderLimits,
) (diagnostic.OUITraceV1, error) {
	if section.State == diagnostic.StateEmpty {
		return diagnostic.OUITraceV1{}, nil
	}
	var trace diagnostic.OUITraceV1
	if err := diagnostic.DecodeReferenced(source, section.Members[0], limits, &trace); err != nil {
		return diagnostic.OUITraceV1{}, err
	}
	return trace, nil
}

type graphDNSReplay struct {
	records  []diagnostic.DNSRecordV1
	position int
	err      error
}

func (r *graphDNSReplay) lookup(ip string) string {
	if r == nil || r.err != nil {
		return ""
	}
	addr, ok := normalizeTopologyReverseDNSCandidateIP(ip)
	if !ok {
		return ""
	}
	if r.position >= len(r.records) {
		r.err = fmt.Errorf("unexpected DNS lookup for %s", addr)
		return ""
	}
	record := r.records[r.position]
	r.position++
	if record.IP != addr.String() {
		r.err = fmt.Errorf("DNS trace lookup %d is for %s, replay requested %s", r.position-1, record.IP, addr)
		return ""
	}
	if record.State == diagnostic.DNSStatePositive {
		return record.Name
	}
	return ""
}

type graphOUIReplay struct {
	records  []diagnostic.OUIRecordV1
	position int
	err      error
}

func (r *graphOUIReplay) lookup(mac string) (vendor string, prefix string) {
	if r == nil || r.err != nil {
		return "", ""
	}
	parsed, err := net.ParseMAC(mac)
	if err != nil {
		r.err = fmt.Errorf("graph replay received invalid OUI lookup %q", mac)
		return "", ""
	}
	mac = parsed.String()
	if r.position >= len(r.records) {
		r.err = fmt.Errorf("unexpected OUI lookup for %s", mac)
		return "", ""
	}
	record := r.records[r.position]
	r.position++
	if record.MAC != mac {
		r.err = fmt.Errorf("OUI trace lookup %d is for %s, replay requested %s", r.position-1, record.MAC, mac)
		return "", ""
	}
	return record.Vendor, record.Prefix
}

func topologyQueryOptionsFromDiagnostic(value diagnostic.GraphQueryOptionsV1) topologyoptions.QueryOptions {
	return topologyoptions.QueryOptions{
		CollapseActorsByIP:     value.CollapseActorsByIP,
		EliminateNonIPInferred: value.EliminateNonIPInferred,
		MapType:                value.MapType,
		InferenceStrategy:      value.InferenceStrategy,
		ManagedDeviceFocus:     value.ManagedDeviceFocus,
		Depth:                  value.Depth,
	}
}

func addGraphObservationWork(work *replayWorkBudget, observation diagnostic.ObservationV1) error {
	if err := work.add(1); err != nil {
		return err
	}
	for _, count := range []int{
		len(observation.L2), len(observation.L3Interfaces), len(observation.OSPFNeighbors), len(observation.BGPPeers),
		len(observation.LocalDevice.ManagementAddresses), len(observation.LocalDevice.Capabilities),
		len(observation.LocalDevice.CapabilitiesSupported), len(observation.LocalDevice.CapabilitiesEnabled),
		len(observation.LocalDevice.Labels), len(observation.LocalDevice.DeviceCharts),
		len(observation.LocalDevice.InterfaceCharts),
	} {
		if err := work.add(uint64(count)); err != nil {
			return err
		}
	}
	for _, chart := range observation.LocalDevice.InterfaceCharts {
		if err := work.add(uint64(len(chart.AvailableMetrics))); err != nil {
			return err
		}
	}
	for _, row := range observation.L2 {
		for _, count := range []int{
			len(row.ManagementAliases), len(row.Interfaces), len(row.BridgePorts), len(row.STPPorts), len(row.FDBEntries),
			len(row.ARPNDEntries), len(row.LLDPRemotes), len(row.CDPRemotes), len(row.Labels),
		} {
			if err := work.add(uint64(count)); err != nil {
				return err
			}
		}
	}
	return nil
}

func diagnosticCapabilityRoot(
	manifest diagnostic.ManifestV1,
	capability diagnostic.CapabilityKey,
) (diagnostic.ContentRef, bool) {
	for _, root := range manifest.Roots {
		if root.CapabilityKey == capability {
			return root.Root, true
		}
	}
	return diagnostic.ContentRef{}, false
}

func diagnosticSection(root diagnostic.CapabilityRootV1, name string) diagnostic.SectionInventoryV1 {
	for _, section := range root.Sections {
		if section.Name == name {
			return section
		}
	}
	return diagnostic.SectionInventoryV1{}
}
