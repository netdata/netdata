// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"sort"
	"strings"
	"sync"
)

type relationshipMode string

const (
	relationshipComponents = "component"
	relationshipEnrichment = "enrichment"
	relationshipLegacy     = "legacy"
)

type graphRelationship struct {
	ParentKind string
	Path       string
	ChildKind  string
	Family     string
	Mode       relationshipMode
	Embedded   bool
	Source     string
}

type graphCollectionRequest struct {
	parent             *graphNode
	relationship       graphRelationship
	members            []collectionMember
	membershipComplete bool
	pageErr            error
}

// Supported source links. These describe acquisition, not inferred ownership.
var graphRelationships = []graphRelationship{
	{ParentKind: "service", Path: "Storage", ChildKind: "storage", Family: "storage", Mode: "traversal"},
	{ParentKind: "service", Path: "UpdateService", ChildKind: "update_service", Family: "firmware", Mode: "traversal"},
	{ParentKind: "update_service", Path: "FirmwareInventory", ChildKind: "firmware", Family: "firmware", Mode: "traversal"},
	{ParentKind: "update_service", Path: "SoftwareInventory", ChildKind: "software", Family: "firmware", Mode: "traversal"},
	{ParentKind: "system", Path: "Processors", ChildKind: "processor", Family: "compute", Mode: "component"},
	{ParentKind: "system", Path: "Memory", ChildKind: "memory", Family: "memory", Mode: "component"},
	{ParentKind: "system", Path: "Storage", ChildKind: "storage", Family: "storage", Mode: "component"},
	{ParentKind: "system", Path: "EthernetInterfaces", ChildKind: "ethernet_interface", Family: "network", Mode: "component"},
	{ParentKind: "system", Path: "NetworkInterfaces", ChildKind: "network_interface", Family: "network", Mode: "component"},
	{ParentKind: "system", Path: "PCIeDevices", ChildKind: "pcie_device", Family: "pcie", Mode: "component"},
	{ParentKind: "system", Path: "PCIeFunctions", ChildKind: "pcie_function", Family: "pcie", Mode: "component"},
	{ParentKind: "system", Path: "Redundancy", ChildKind: "redundancy", Family: "base", Mode: "component", Embedded: true, Source: "embedded_excerpt"},
	{ParentKind: "system", Path: "Links.OffloadedNetworkDeviceFunctions", ChildKind: "network_device_function", Family: "network", Mode: "association"},
	{ParentKind: "chassis", Path: "Drives", ChildKind: "drive", Family: "storage", Mode: "component"},
	{ParentKind: "chassis", Path: "Memory", ChildKind: "memory", Family: "memory", Mode: "component"},
	{ParentKind: "chassis", Path: "NetworkAdapters", ChildKind: "network_adapter", Family: "network", Mode: "component"},
	{ParentKind: "chassis", Path: "PCIeDevices", ChildKind: "pcie_device", Family: "pcie", Mode: "component"},
	{ParentKind: "chassis", Path: "Sensors", ChildKind: "sensor", Family: "sensors", Mode: "component"},
	{ParentKind: "chassis", Path: "Controls", ChildKind: "control", Family: "thermal", Mode: "component"},
	{ParentKind: "chassis", Path: "LeakDetectors", ChildKind: "leak_detector", Family: "thermal", Mode: "component"},
	{ParentKind: "chassis", Path: "ThermalSubsystem", ChildKind: "thermal_subsystem", Family: "thermal", Mode: "component"},
	{ParentKind: "chassis", Path: "PowerSubsystem", ChildKind: "power_subsystem", Family: "power", Mode: "component"},
	{ParentKind: "chassis", Path: "Processors", ChildKind: "processor", Family: "compute", Mode: "component"},
	{ParentKind: "chassis", Path: "Links.Storage", ChildKind: "storage", Family: "storage", Mode: "association"},
	{ParentKind: "chassis", Path: "Thermal", ChildKind: "legacy_thermal", Family: "thermal", Mode: "legacy"},
	{ParentKind: "chassis", Path: "Power", ChildKind: "legacy_power", Family: "power", Mode: "legacy"},
	{ParentKind: "chassis", Path: "EnvironmentMetrics", ChildKind: "environment_metrics", Family: "thermal", Mode: "enrichment"},
	{ParentKind: "chassis", Path: "Assembly", ChildKind: "assembly_document", Family: "firmware", Mode: "enrichment"},
	{ParentKind: "manager", Path: "EthernetInterfaces", ChildKind: "ethernet_interface", Family: "network", Mode: "component"},
	{ParentKind: "manager", Path: "DedicatedNetworkPorts", ChildKind: "network_port", Family: "network", Mode: "component"},
	{ParentKind: "manager", Path: "SharedNetworkPorts", ChildKind: "network_port", Family: "network", Mode: "component"},
	{ParentKind: "manager", Path: "Redundancy", ChildKind: "redundancy", Family: "base", Mode: "component", Embedded: true, Source: "embedded_excerpt"},
	{ParentKind: "processor", Path: "Ports", ChildKind: "port", Family: "network", Mode: "component"},
	{ParentKind: "processor", Path: "Metrics", ChildKind: "processor_metrics", Family: "compute", Mode: "enrichment"},
	{ParentKind: "processor", Path: "EnvironmentMetrics", ChildKind: "environment_metrics", Family: "thermal", Mode: "enrichment"},
	{ParentKind: "processor", Path: "Assembly", ChildKind: "assembly_document", Family: "firmware", Mode: "enrichment"},
	{ParentKind: "memory", Path: "Metrics", ChildKind: "memory_metrics", Family: "memory", Mode: "enrichment"},
	{ParentKind: "memory", Path: "EnvironmentMetrics", ChildKind: "environment_metrics", Family: "thermal", Mode: "enrichment"},
	{ParentKind: "memory", Path: "Assembly", ChildKind: "assembly_document", Family: "firmware", Mode: "enrichment"},
	{ParentKind: "storage", Path: "Controllers", ChildKind: "storage_controller", Family: "storage", Mode: "component"},
	{ParentKind: "storage", Path: "Drives", ChildKind: "drive", Family: "storage", Mode: "component"},
	{ParentKind: "storage", Path: "Volumes", ChildKind: "volume", Family: "storage", Mode: "component"},
	{ParentKind: "storage", Path: "Redundancy", ChildKind: "redundancy", Family: "storage", Mode: "component", Embedded: true, Source: "embedded_excerpt"},
	{ParentKind: "storage", Path: "Metrics", ChildKind: "storage_metrics", Family: "storage", Mode: "enrichment"},
	{ParentKind: "storage_controller", Path: "Ports", ChildKind: "port", Family: "network", Mode: "component"},
	{ParentKind: "storage_controller", Path: "Metrics", ChildKind: "storage_controller_metrics", Family: "storage", Mode: "enrichment"},
	{ParentKind: "storage_controller", Path: "EnvironmentMetrics", ChildKind: "environment_metrics", Family: "thermal", Mode: "enrichment"},
	{ParentKind: "storage_controller", Path: "Assembly", ChildKind: "assembly_document", Family: "firmware", Mode: "enrichment"},
	{ParentKind: "drive", Path: "Metrics", ChildKind: "drive_metrics", Family: "storage", Mode: "enrichment"},
	{ParentKind: "drive", Path: "EnvironmentMetrics", ChildKind: "environment_metrics", Family: "thermal", Mode: "enrichment"},
	{ParentKind: "drive", Path: "Assembly", ChildKind: "assembly_document", Family: "firmware", Mode: "enrichment"},
	{ParentKind: "volume", Path: "Metrics", ChildKind: "volume_metrics", Family: "storage", Mode: "enrichment"},
	{ParentKind: "network_adapter", Path: "NetworkDeviceFunctions", ChildKind: "network_device_function", Family: "network", Mode: "component"},
	{ParentKind: "network_adapter", Path: "NetworkPorts", ChildKind: "network_port", Family: "network", Mode: "component"},
	{ParentKind: "network_adapter", Path: "Ports", ChildKind: "port", Family: "network", Mode: "component"},
	{ParentKind: "network_adapter", Path: "Metrics", ChildKind: "network_adapter_metrics", Family: "network", Mode: "enrichment"},
	{ParentKind: "network_adapter", Path: "EnvironmentMetrics", ChildKind: "environment_metrics", Family: "thermal", Mode: "enrichment"},
	{ParentKind: "network_adapter", Path: "Assembly", ChildKind: "assembly_document", Family: "firmware", Mode: "enrichment"},
	{ParentKind: "network_interface", Path: "NetworkDeviceFunctions", ChildKind: "network_device_function", Family: "network", Mode: "component"},
	{ParentKind: "network_interface", Path: "NetworkPorts", ChildKind: "network_port", Family: "network", Mode: "component"},
	{ParentKind: "network_interface", Path: "Ports", ChildKind: "port", Family: "network", Mode: "component"},
	{ParentKind: "network_device_function", Path: "Metrics", ChildKind: "network_device_function_metrics", Family: "network", Mode: "enrichment"},
	{ParentKind: "port", Path: "Metrics", ChildKind: "port_metrics", Family: "network", Mode: "enrichment"},
	{ParentKind: "port", Path: "EnvironmentMetrics", ChildKind: "environment_metrics", Family: "thermal", Mode: "enrichment"},
	{ParentKind: "pcie_device", Path: "PCIeFunctions", ChildKind: "pcie_function", Family: "pcie", Mode: "component"},
	{ParentKind: "pcie_device", Path: "EnvironmentMetrics", ChildKind: "environment_metrics", Family: "thermal", Mode: "enrichment"},
	{ParentKind: "pcie_device", Path: "Assembly", ChildKind: "assembly_document", Family: "firmware", Mode: "enrichment"},
	{ParentKind: "thermal_subsystem", Path: "CoolantConnectors", ChildKind: "coolant_connector", Family: "thermal", Mode: "component"},
	{ParentKind: "thermal_subsystem", Path: "Fans", ChildKind: "fan", Family: "thermal", Mode: "component"},
	{ParentKind: "thermal_subsystem", Path: "Filters", ChildKind: "filter", Family: "thermal", Mode: "component"},
	{ParentKind: "thermal_subsystem", Path: "Heaters", ChildKind: "heater", Family: "thermal", Mode: "component"},
	{ParentKind: "thermal_subsystem", Path: "Pumps", ChildKind: "pump", Family: "thermal", Mode: "component"},
	{ParentKind: "thermal_subsystem", Path: "LeakDetection", ChildKind: "leak_detection", Family: "thermal", Mode: "component"},
	{ParentKind: "thermal_subsystem", Path: "CoolantConnectorRedundancy", ChildKind: "redundancy", Family: "thermal", Mode: "component", Embedded: true, Source: "embedded_excerpt"},
	{ParentKind: "thermal_subsystem", Path: "FanRedundancy", ChildKind: "redundancy", Family: "thermal", Mode: "component", Embedded: true, Source: "embedded_excerpt"},
	{ParentKind: "thermal_subsystem", Path: "ThermalMetrics", ChildKind: "thermal_metrics", Family: "thermal", Mode: "enrichment"},
	{ParentKind: "leak_detection", Path: "LeakDetectorGroups", ChildKind: "leak_detector_group", Family: "thermal", Mode: "component", Embedded: true, Source: "embedded_excerpt"},
	{ParentKind: "leak_detection", Path: "LeakDetectors", ChildKind: "leak_detector", Family: "thermal", Mode: "component"},
	{ParentKind: "leak_detector_group", Path: "Detectors", ChildKind: "leak_detector", Family: "thermal", Mode: "component", Embedded: true, Source: "embedded_excerpt"},
	{ParentKind: "power_subsystem", Path: "PowerSupplies", ChildKind: "power_supply", Family: "power", Mode: "component"},
	{ParentKind: "power_subsystem", Path: "Batteries", ChildKind: "battery", Family: "power", Mode: "component"},
	{ParentKind: "power_subsystem", Path: "PowerSupplyRedundancy", ChildKind: "redundancy", Family: "power", Mode: "component", Embedded: true, Source: "embedded_excerpt"},
	{ParentKind: "power_supply", Path: "Metrics", ChildKind: "power_supply_metrics", Family: "power", Mode: "enrichment"},
	{ParentKind: "power_supply", Path: "Assembly", ChildKind: "assembly_document", Family: "firmware", Mode: "enrichment"},
	{ParentKind: "battery", Path: "Metrics", ChildKind: "battery_metrics", Family: "power", Mode: "enrichment"},
	{ParentKind: "battery", Path: "Assembly", ChildKind: "assembly_document", Family: "firmware", Mode: "enrichment"},
	{ParentKind: "sensor", Path: "SensorGroup", ChildKind: "redundancy", Family: "sensors", Mode: "component", Embedded: true, Source: "embedded_excerpt"},
	{ParentKind: "heater", Path: "Metrics", ChildKind: "heater_metrics", Family: "thermal", Mode: "enrichment"},
	{ParentKind: "system", Path: "ProcessorSummary.Metrics", ChildKind: "processor_summary_metrics", Family: "compute", Mode: "enrichment"},
	{ParentKind: "system", Path: "MemorySummary.Metrics", ChildKind: "memory_summary_metrics", Family: "memory", Mode: "enrichment"},
}

type graphNode struct {
	Kind             string
	URI              string
	Locator          string
	Key              string
	Data             map[string]any
	Enrichment       map[string]map[string]any
	Doc              genericResource
	AcquisitionState string
	ErrorClass       string
	Complete         bool
	IdentityQuality  string
	SourcePath       string
	SourceModel      string
	Response         responseMetadata
	SensorExcerpts   []sensorExcerptSource

	Parents map[string]*graphNode
}

type sensorExcerptSource struct {
	Path  string
	Type  string
	Units string
	Data  map[string]any
}

type resourceGraph struct {
	Nodes       []*graphNode
	ByIdentity  map[string]*graphNode
	ByKey       map[string]*graphNode
	ByURI       map[string]*graphNode
	KeySources  map[string]string
	Slices      []graphSlice
	Complete    bool
	Diagnostics []string
	diagnostics boundedDiagnosticAccumulator
	register    func([]identityBinding) error
}

func (g *resourceGraph) addDiagnostic(value string) {
	if g == nil {
		return
	}
	g.diagnostics.Add(value)
	g.Diagnostics = g.diagnostics.values
}

func (g *resourceGraph) finalDiagnostics() []string {
	if g == nil {
		return nil
	}
	return g.diagnostics.Values()
}

type graphSlice struct {
	ParentKey string
	Path      string
	ChildKind string
	Complete  bool
}

type graphMembershipSnapshot struct {
	ParentKey    string
	Relationship graphRelationship
	Members      []*graphNode
}

type graphFetchBroker struct {
	mu      sync.Mutex
	entries map[string]*graphFetchEntry
}

type graphFetchEntry struct {
	done chan struct{}
	node *graphNode
	err  error
}

type graphFetchBrokerContextKey struct{}

func withGraphFetchBroker(ctx context.Context) context.Context {
	if _, ok := ctx.Value(graphFetchBrokerContextKey{}).(*graphFetchBroker); ok {
		return ctx
	}
	return context.WithValue(ctx, graphFetchBrokerContextKey{}, &graphFetchBroker{
		entries: make(map[string]*graphFetchEntry),
	})
}

func graphFetchBrokerFrom(ctx context.Context) *graphFetchBroker {
	broker, _ := ctx.Value(graphFetchBrokerContextKey{}).(*graphFetchBroker)
	return broker
}

func (b *graphFetchBroker) fetch(
	ctx context.Context,
	key string,
	fetch func() (*graphNode, error),
) (*graphNode, error) {
	if b == nil {
		return fetch()
	}
	b.mu.Lock()
	if entry := b.entries[key]; entry != nil {
		b.mu.Unlock()
		select {
		case <-entry.done:
			return cloneFetchedGraphNode(entry.node), entry.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	entry := &graphFetchEntry{done: make(chan struct{})}
	b.entries[key] = entry
	b.mu.Unlock()

	entry.node, entry.err = fetch()
	b.mu.Lock()
	close(entry.done)
	b.mu.Unlock()
	return cloneFetchedGraphNode(entry.node), entry.err
}

func (g *resourceGraph) emittedNodes() []*graphNode {
	out := make([]*graphNode, 0, len(g.Nodes))
	for _, node := range g.Nodes {
		switch node.Kind {
		case "update_service":
			continue
		}
		out = append(out, node)
	}
	return out
}

func (c *protocolClient) collectResourceGraph(ctx context.Context, root *serviceRootDocument, base []baseResource, stats *wireStats) (graph *resourceGraph, resultErr error) {
	ctx = withGraphFetchBroker(ctx)
	graph = &resourceGraph{Complete: true, register: c.identities.register}
	defer func() {
		if resultErr != nil {
			graph.Complete = false
		}
		if err := c.finalizeGraphMembership(graph); err != nil {
			graph.Complete = false
			resultErr = errors.Join(resultErr, err)
		}
		graph.addResponseDiagnostics()
	}()
	service := &graphNode{Kind: "service", URI: "/redfish/v1/", Locator: "/redfish/v1/", Data: serviceRootMap(root), AcquisitionState: "readable", Complete: true, IdentityQuality: "addressable", SourceModel: "resource", Response: root.Response, Parents: make(map[string]*graphNode)}
	service.Key = resourceKey(c.origin, service.Kind, service.Locator)
	if err := graph.add(service); err != nil {
		return graph, err
	}
	queue := []*graphNode{service}
	for _, item := range base {
		node := &graphNode{Kind: item.Kind, URI: item.URI, Locator: item.URI, Data: item.Data, Doc: item.Doc, AcquisitionState: item.AcquisitionState, ErrorClass: item.ErrorClass, Complete: item.MembershipComplete, IdentityQuality: "addressable", SourceModel: "resource", Response: item.Response, Parents: map[string]*graphNode{service.Key: service}}
		node.Key = resourceKey(c.origin, node.Kind, node.Locator)
		if err := graph.add(node); err != nil {
			return graph, err
		}
		queue = append(queue, node)
	}
	visited := make(map[*graphNode]string)
	for pos := 0; pos < len(queue); pos++ {
		if err := ctx.Err(); err != nil {
			return graph, err
		}
		parent := queue[pos]
		readable := parent.AcquisitionState == "readable" && parent.Data != nil
		if !readable {
			graph.Complete = false
			continue // Restore unavailable descendants after all successful paths are walked.
		}
		if quality, ok := visited[parent]; ok && quality == parent.IdentityQuality {
			continue
		}
		// A later addressable representation may expose links absent from an excerpt.
		visited[parent] = parent.IdentityQuality
		for _, rel := range relationshipsFor(parent.Kind) {
			if !c.familyEnabled(rel.Family) {
				continue
			}
			if err := ctx.Err(); err != nil {
				return graph, err
			}
			var children []*graphNode
			var enrichments map[string]map[string]any
			complete := false
			var err error
			children, enrichments, complete, err = c.acquireRelationship(ctx, parent, rel, stats)
			if rel.Mode != relationshipEnrichment {
				children = c.reconcileGraphMembership(parent, rel, children, complete)
				graph.recordMembership(parent, rel, complete)
			}
			if len(enrichments) > 0 {
				if parent.Enrichment == nil {
					parent.Enrichment = make(map[string]map[string]any)
				}
				maps.Copy(parent.Enrichment, enrichments)
			}
			if rel.Mode == relationshipEnrichment {
				if addErr := c.addEmbeddedEnrichmentComponents(ctx, graph, parent, rel, enrichments, complete && err == nil, &queue); addErr != nil {
					return graph, errors.Join(err, addErr)
				}
			}
			for _, child := range children {
				if addErr := graph.addChild(parent, child, &queue); addErr != nil {
					return graph, addErr
				}
			}
			if err != nil {
				graph.addDiagnostic(fmt.Sprintf("%s %s: %v", parent.Kind, rel.Path, err))
			}
			if !complete || err != nil {
				graph.Complete = false
			}
		}
	}
	return graph, nil
}

func (g *resourceGraph) addChild(parent, child *graphNode, queue *[]*graphNode) error {
	if existing := g.ByIdentity[child.Kind+"\x00"+child.Locator]; existing != nil {
		if mergeEquivalentGraphNode(existing, child) {
			*queue = append(*queue, existing)
		}
		existing.Parents[parent.Key] = parent
		return nil
	}
	child.Parents[parent.Key] = parent
	if err := g.add(child); err != nil {
		return err
	}
	*queue = append(*queue, child)
	return nil
}

func (g *resourceGraph) recordMembership(parent *graphNode, rel graphRelationship, complete bool) {
	g.Slices = append(g.Slices, graphSlice{ParentKey: parent.Key, Path: rel.Path, ChildKind: rel.ChildKind, Complete: complete})
}

func graphMembershipKey(parentKey string, rel graphRelationship) string {
	return parentKey + "\x00" + rel.Path + "\x00" + rel.ChildKind
}

func (c *protocolClient) finalizeGraphMembership(graph *resourceGraph) error {
	if graph == nil {
		return nil
	}
	observed := make(map[string]bool, len(graph.Slices))
	for _, slice := range graph.Slices {
		observed[slice.ParentKey+"\x00"+slice.Path+"\x00"+slice.ChildKind] = true
	}
	c.graphMu.Lock()
	defer c.graphMu.Unlock()
	if graph.Complete {
		for key := range c.graphMembership {
			if !observed[key] {
				delete(c.graphMembership, key)
			}
		}
		return nil
	}
	// Unvisited branches retain identities only, so a failed read cannot replay measurements.
	pending := make(map[string][]graphMembershipSnapshot)
	for key, snapshot := range c.graphMembership {
		if !observed[key] {
			pending[snapshot.ParentKey] = append(pending[snapshot.ParentKey], snapshot)
		}
	}
	queue := append([]*graphNode(nil), graph.Nodes...)
	for pos := 0; pos < len(queue); pos++ {
		parent := queue[pos]
		snapshots := pending[parent.Key]
		delete(pending, parent.Key)
		for _, snapshot := range snapshots {
			children := cloneGraphNodesStatic(snapshot.Members, true)
			for _, child := range children {
				if err := graph.addChild(parent, child, &queue); err != nil {
					return err
				}
			}
			graph.recordMembership(parent, snapshot.Relationship, false)
		}
	}
	return nil
}

func (c *protocolClient) reconcileGraphMembership(parent *graphNode, rel graphRelationship, current []*graphNode, complete bool) []*graphNode {
	key := graphMembershipKey(parent.Key, rel)
	c.graphMu.Lock()
	defer c.graphMu.Unlock()
	if c.graphMembership == nil {
		c.graphMembership = make(map[string]graphMembershipSnapshot)
	}
	if !complete {
		seen := make(map[string]*graphNode, len(current))
		for _, node := range current {
			seen[node.Key] = node
		}
		for _, retained := range cloneGraphNodesStatic(c.graphMembership[key].Members, true) {
			if node := seen[retained.Key]; node != nil {
				if node.AcquisitionState != "readable" {
					node.Doc = retained.Doc
				}
			} else {
				current = append(current, retained)
			}
		}
	}
	if len(current) == 0 {
		delete(c.graphMembership, key)
	} else {
		c.graphMembership[key] = graphMembershipSnapshot{ParentKey: parent.Key, Relationship: rel, Members: cloneGraphNodesStatic(current, false)}
	}
	return current
}

func cloneGraphNodesStatic(nodes []*graphNode, unknown bool) []*graphNode {
	result := make([]*graphNode, 0, len(nodes))
	for _, source := range nodes {
		if source == nil {
			continue
		}
		node := *source
		node.Data = nil
		node.Enrichment = nil
		node.Doc.Status = genericStatus{}
		node.Doc.PowerState = ""
		node.Doc.FailurePredicted = nil
		node.Parents = make(map[string]*graphNode)
		node.SensorExcerpts = nil
		node.Response = responseMetadata{}
		if unknown {
			node.AcquisitionState = "unknown"
			node.ErrorClass = "protocol"
			node.Complete = false
		}
		result = append(result, &node)
	}
	return result
}

// Fetched JSON documents are immutable during a collection cycle.
// Sharing them preserves request coalescing without duplicating the largest
// part of every cached graph node.
func cloneFetchedGraphNode(source *graphNode) *graphNode {
	return cloneGraphNode(source)
}

func cloneGraphNode(source *graphNode) *graphNode {
	if source == nil {
		return nil
	}
	node := *source
	node.Doc.Status.Conditions = append([]genericCondition(nil), source.Doc.Status.Conditions...)
	node.Enrichment = make(map[string]map[string]any, len(source.Enrichment))
	for key, value := range source.Enrichment {
		node.Enrichment[key] = cloneJSONMap(value)
	}
	node.Parents = make(map[string]*graphNode)
	node.SensorExcerpts = cloneSensorExcerptSources(source.SensorExcerpts)
	return &node
}

func (g *resourceGraph) add(node *graphNode) error {
	if node == nil {
		return errors.New("cannot add a nil Redfish graph node")
	}
	identity := node.Kind + "\x00" + node.Locator
	if g.ByIdentity[identity] != nil {
		return errors.New("duplicate Redfish graph resource identity")
	}
	preimage := identity
	if existing, ok := g.KeySources[node.Key]; ok && existing != preimage {
		return fmt.Errorf("%w: resource key collision", errIdentityIntegrity)
	}
	if g.register != nil {
		if err := g.register([]identityBinding{{
			Domain: "resource", Key: node.Key, Preimage: preimage,
		}}); err != nil {
			return err
		}
	}
	g.Nodes = append(g.Nodes, node)
	if g.ByIdentity == nil {
		g.ByIdentity = make(map[string]*graphNode)
	}
	if g.ByKey == nil {
		g.ByKey = make(map[string]*graphNode)
	}
	if g.ByURI == nil {
		g.ByURI = make(map[string]*graphNode)
	}
	if g.KeySources == nil {
		g.KeySources = make(map[string]string)
	}
	g.ByIdentity[identity] = node
	g.ByKey[node.Key] = node
	if node.URI != "" {
		uriKey := node.Kind + "\x00" + node.URI
		if _, exists := g.ByURI[uriKey]; !exists {
			g.ByURI[uriKey] = node
		}
	}
	g.KeySources[node.Key] = preimage
	return nil
}

func relationshipsFor(kind string) []graphRelationship {
	var result []graphRelationship
	for _, rel := range graphRelationships {
		if rel.ParentKind == kind {
			result = append(result, rel)
		}
	}
	return result
}

func (c *protocolClient) familyEnabled(family string) bool {
	if family == "" || family == "base" {
		return true
	}
	return c.families == nil || c.families[family]
}

func (c *protocolClient) acquireRelationship(
	ctx context.Context,
	parent *graphNode,
	rel graphRelationship,
	stats *wireStats,
) ([]*graphNode, map[string]map[string]any, bool, error) {
	value, ok := jsonPath(parent.Data, rel.Path)
	if !ok || value == nil {
		return nil, nil, true, nil
	}

	if rel.Embedded {
		nodes, complete, err := c.acquireEmbeddedValues(ctx, parent, rel, value)
		return nodes, nil, complete, err
	}

	items, complete, err := c.acquireLinkedValues(ctx, parent, rel, value, stats)
	if rel.Mode == relationshipEnrichment {
		result := make(map[string]map[string]any)
		for i, item := range items {
			identity := firstNonEmpty(item.Locator, item.URI, fmt.Sprintf("position:%d", i))
			result[rel.ChildKind+":"+identity] = item.Data
			parent.Response = mergeResponseMetadata(parent.Response, item.Response)
		}
		return nil, result, complete, err
	}
	if rel.Mode == relationshipLegacy {
		var children []*graphNode
		legacyComplete := true
		var legacyFailures boundedErrorAccumulator
		for _, item := range items {
			nodes, ok, itemErr := c.legacyComponents(ctx, parent, rel, item.Data)
			children = append(children, nodes...)
			legacyComplete = legacyComplete && ok
			legacyFailures.Add(itemErr)
		}
		legacyFailures.Add(err)
		return children, nil, complete && legacyComplete, legacyFailures.Err()
	}
	return items, nil, complete, err
}

func (c *protocolClient) acquireEmbeddedValues(
	ctx context.Context,
	parent *graphNode,
	rel graphRelationship,
	value any,
) ([]*graphNode, bool, error) {
	var values []any
	singleton := false
	switch typed := value.(type) {
	case map[string]any:
		values = []any{typed}
		singleton = true
	case []any:
		values = typed
	default:
		return nil, false, fmt.Errorf("embedded value has unexpected type %T", value)
	}

	result := make([]*graphNode, 0, len(values))
	positions := make([]int, 0, len(values))
	complete := true
	var failures boundedErrorAccumulator
	for index, value := range values {
		obj, ok := value.(map[string]any)
		if !ok {
			complete = false
			failures.Add(fmt.Errorf("embedded member %d is not an object", index))
			continue
		}
		position := index
		if singleton {
			position = -1
		}
		node, err := c.embeddedNode(parent, rel, obj, position)
		if err != nil {
			complete = false
			failures.Add(fmt.Errorf("embedded member %d: %w", index, err))
		}
		result = append(result, node)
		positions = append(positions, position)
	}
	c.makeDuplicateEmbeddedIDsPositional(parent, rel, result, positions)
	return result, complete, failures.Err()
}

func (c *protocolClient) acquireLinkedValues(
	ctx context.Context,
	parent *graphNode,
	rel graphRelationship,
	value any,
	stats *wireStats,
) ([]*graphNode, bool, error) {
	switch typed := value.(type) {
	case map[string]any:
		if rawURI, ok := stringValue(typed["@odata.id"]); ok {
			return c.acquireLinkedURI(ctx, parent, rel, rawURI, stats)
		}
		return nil, false, errors.New("link object has no usable @odata.id")
	case []any:
		result := make([]*graphNode, 0, len(typed))
		complete := true
		var failures boundedErrorAccumulator
		for index, value := range typed {
			obj, ok := value.(map[string]any)
			if !ok {
				complete = false
				failures.Add(fmt.Errorf("array member %d is not an object", index))
				continue
			}
			if rawURI, ok := stringValue(obj["@odata.id"]); ok {
				children, ok, err := c.acquireLinkedURI(ctx, parent, rel, rawURI, stats)
				result = append(result, children...)
				complete = complete && ok
				failures.Add(err)
				continue
			}
			complete = false
			failures.Add(fmt.Errorf("array link member %d has no usable @odata.id", index))
		}
		return result, complete, failures.Err()
	default:
		return nil, false, fmt.Errorf("link has unexpected type %T", value)
	}
}

func (c *protocolClient) acquireLinkedURI(
	ctx context.Context,
	parent *graphNode,
	rel graphRelationship,
	rawURI string,
	stats *wireStats,
) ([]*graphNode, bool, error) {
	target, err := c.resolveURI(c.root, rawURI, false)
	if err != nil {
		return nil, false, err
	}
	collectionIdentity := canonicalResourceURI(target)
	if c.isKnownCollection(collectionIdentity) {
		members, complete, pageErr := c.fetchCollectionMembersAt(
			ctx,
			target,
			nil,
			rel.ChildKind,
			stats,
		)
		return c.fetchGraphCollectionMembers(
			ctx,
			graphCollectionRequest{
				parent:             parent,
				relationship:       rel,
				members:            members,
				membershipComplete: complete,
				pageErr:            pageErr,
			},
			stats,
		)
	}
	response, err := c.do(
		ctx,
		protocolRequest{method: http.MethodGet, target: target, auth: c.currentAuth(true)},
		stats,
		true,
		http.StatusOK,
	)
	if err != nil {
		return nil, false, err
	}
	var data map[string]any
	if err := decodeJSON(response, &data); err != nil {
		response.finish(err)
		return nil, false, err
	}
	if members, present := data["Members"]; present {
		c.markKnownCollection(collectionIdentity)
		collectionMembers, complete, pageErr := c.fetchCollectionMemberPages(
			ctx,
			target,
			response,
			stats,
			rel.ChildKind,
			false,
		)
		_ = members
		return c.fetchGraphCollectionMembers(
			ctx,
			graphCollectionRequest{
				parent:             parent,
				relationship:       rel,
				members:            collectionMembers,
				membershipComplete: complete,
				pageErr:            pageErr,
			},
			stats,
		)
	}
	node, err := c.graphNodeFromResponse(rel.ChildKind, response, rel.Source)
	if err != nil {
		return nil, false, err
	}
	return []*graphNode{node}, true, nil
}

func (c *protocolClient) fetchGraphCollectionMembers(
	ctx context.Context,
	request graphCollectionRequest,
	stats *wireStats,
) ([]*graphNode, bool, error) {
	members := request.members
	result := make([]*graphNode, 0, len(members))
	fetched := make([]*graphNode, len(members))
	fetchErrors := make([]error, len(members))
	localStats := make([]*wireStats, len(members))
	attempted := make([]bool, len(members))
	jobs := make(chan int)
	workers := min(max(c.config.MaxConcurrentRequests, 1), len(members))
	var wait sync.WaitGroup
	for range workers {
		wait.Go(func() {
			for index := range jobs {
				itemStats := &wireStats{failures: make(map[string]int)}
				node, fetchErr := c.fetchGraphCollectionMember(
					ctx,
					request.relationship,
					request.parent,
					members[index],
					itemStats,
				)
				fetched[index] = node
				fetchErrors[index] = fetchErr
				localStats[index] = itemStats
			}
		})
	}
	dispatchComplete := true
dispatch:
	for index := range members {
		select {
		case jobs <- index:
			attempted[index] = true
		case <-ctx.Done():
			dispatchComplete = false
			break dispatch
		}
	}
	close(jobs)
	wait.Wait()
	joined := request.pageErr
	if !dispatchComplete {
		joined = errors.Join(joined, context.Cause(ctx))
	}
	memberFailures := 0
	var firstMemberFailure error
	for index, member := range members {
		stats.merge(localStats[index])
		node, err := fetched[index], fetchErrors[index]
		if node == nil && err == nil {
			err = context.Cause(ctx)
			if err == nil {
				err = context.Canceled
			}
		}
		if err != nil {
			memberFailures++
			if firstMemberFailure == nil {
				firstMemberFailure = err
			}
			if attempted[index] {
				node = c.unreadableGraphNode(request.relationship.ChildKind, member.Ref.ODataID, err)
			} else {
				node = c.unknownGraphNode(request.relationship.ChildKind, member.Ref.ODataID, err)
			}
		}
		result = append(result, node)
	}
	if memberFailures > 0 {
		joined = errors.Join(joined, fmt.Errorf(
			"%d %s collection members were not readable; first failure: %w",
			memberFailures,
			request.relationship.ChildKind,
			firstMemberFailure,
		))
	}
	// A complete collection gives authoritative membership even when one
	// member representation is temporarily unreadable. The unreadable node
	// preserves that member and carries the per-resource failure.
	return result, request.membershipComplete && request.pageErr == nil, joined
}

func (c *protocolClient) fetchGraphCollectionMember(
	ctx context.Context,
	rel graphRelationship,
	parent *graphNode,
	member collectionMember,
	stats *wireStats,
) (*graphNode, error) {
	if member.Data == nil || len(member.Raw) == 0 {
		return c.fetchGraphNode(
			ctx,
			rel.ChildKind,
			member.Ref.ODataID,
			rel,
			parent,
			stats,
		)
	}
	key := rel.ChildKind + "\x00" + member.Ref.ODataID
	return graphFetchBrokerFrom(ctx).fetch(ctx, key, func() (*graphNode, error) {
		model := rel.Source
		if model == "" {
			model = "resource"
		}
		node := &graphNode{
			Kind:             rel.ChildKind,
			URI:              member.Ref.ODataID,
			Locator:          member.Ref.ODataID,
			Data:             cloneJSONMap(member.Data),
			Doc:              genericResourceFromMap(member.Data),
			AcquisitionState: "readable",
			Complete:         true,
			IdentityQuality:  "addressable",
			SourceModel:      model,
			Response:         member.Response,
			Parents:          make(map[string]*graphNode),
		}
		node.Key = resourceKey(c.origin, rel.ChildKind, member.Ref.ODataID)
		return node, nil
	})
}

func (c *protocolClient) unreadableGraphNode(kind, uri string, err error) *graphNode {
	node := &graphNode{
		Kind:             kind,
		URI:              uri,
		Locator:          uri,
		AcquisitionState: "unreadable",
		ErrorClass:       classifyError(err),
		Complete:         true,
		IdentityQuality:  "addressable",
		SourceModel:      "resource",
		Parents:          make(map[string]*graphNode),
	}
	node.Key = resourceKey(c.origin, kind, uri)
	return node
}

func (c *protocolClient) unknownGraphNode(kind, uri string, err error) *graphNode {
	node := c.unreadableGraphNode(kind, uri, err)
	node.AcquisitionState = "unknown"
	node.Complete = false
	return node
}

func (c *protocolClient) fetchGraphNode(
	ctx context.Context,
	kind, uri string,
	rel graphRelationship,
	parent *graphNode,
	stats *wireStats,
) (*graphNode, error) {
	target, err := c.resolveURI(c.root, uri, false)
	if err != nil {
		return nil, err
	}
	key := kind + "\x00" + canonicalResourceURI(target)
	return graphFetchBrokerFrom(ctx).fetch(ctx, key, func() (*graphNode, error) {
		response, err := c.do(
			ctx,
			protocolRequest{method: http.MethodGet, target: target, auth: c.currentAuth(true)},
			stats,
			true,
			http.StatusOK,
		)
		if err != nil {
			return nil, err
		}
		return c.graphNodeFromResponse(kind, response, rel.Source)
	})
}

func (c *protocolClient) graphNodeFromResponse(
	kind string,
	response *responseData,
	source string,
) (*graphNode, error) {
	var data map[string]any
	if err := decodeJSON(response, &data); err != nil {
		response.finish(err)
		return nil, err
	}
	if err := validateRequiredResourceProperties(kind, data); err != nil {
		response.finish(err)
		return nil, err
	}
	uri := canonicalResourceURI(response.url)
	id, ok := stringValue(data["@odata.id"])
	if !ok {
		err := errors.New("resource has no usable @odata.id")
		response.finish(err)
		return nil, err
	}
	resolved, err := c.resolveURI(response.url, id, false)
	if err != nil || !sameResourceIdentity(canonicalResourceURI(resolved), uri) {
		err = errors.New("resource identity does not match final response URI")
		response.finish(err)
		return nil, err
	}
	doc := genericResourceFromMap(data)
	model := source
	if model == "" {
		model = "resource"
	}
	node := &graphNode{
		Kind:             kind,
		URI:              uri,
		Locator:          uri,
		Data:             data,
		Doc:              doc,
		AcquisitionState: "readable",
		Complete:         true,
		IdentityQuality:  "addressable",
		SourceModel:      model,
		Response:         metadataForResponse(response),
		Parents:          make(map[string]*graphNode),
	}
	node.Key = resourceKey(c.origin, kind, uri)
	response.finish(nil)
	return node, nil
}

func (c *protocolClient) embeddedNode(
	parent *graphNode,
	rel graphRelationship,
	data map[string]any,
	index int,
) (*graphNode, error) {
	id, _ := stringValue(data["MemberId"])
	if id == "" {
		id, _ = stringValue(data["Id"])
	}
	if index < 0 {
		id = ""
	}
	locator := ""
	quality := "embedded"
	var provenanceErr error
	if rawURI, ok := stringValue(data["DataSourceUri"]); ok {
		base := c.root
		if containerURI := containingResourceURI(parent); containerURI != "" {
			if target, err := c.resolveURI(c.root, containerURI, false); err == nil {
				base = target
			}
		}
		if target, err := resolveRedfishURI(c.origin, base, rawURI, uriProvenance); err == nil {
			locator = canonicalProvenanceURI(target)
			quality = "data_source_uri"
		} else {
			provenanceErr = fmt.Errorf("invalid DataSourceUri: %w", err)
		}
	}
	if locator == "" {
		locator = embeddedLocator(embeddedIdentityContainer(parent), rel.Path, id, index)
	}
	if index >= 0 && id == "" && quality != "data_source_uri" {
		quality = "positional"
	}
	uri := ""
	if quality == "data_source_uri" {
		uri = locator
	}
	doc := genericResourceFromMap(data)
	if doc.Name == "" {
		doc.Name, _ = stringValue(data["GroupName"])
	}
	if doc.Name == "" {
		doc.Name, _ = stringValue(data["DeviceName"])
	}
	node := &graphNode{
		Kind:             rel.ChildKind,
		URI:              uri,
		Locator:          locator,
		Data:             cloneJSONMap(data),
		Doc:              doc,
		AcquisitionState: "readable",
		Complete:         parent.Complete,
		IdentityQuality:  quality,
		SourcePath:       rel.Path,
		SourceModel:      "embedded_excerpt",
		Response:         parent.Response,
		Parents:          map[string]*graphNode{parent.Key: parent},
	}
	node.Key = resourceKey(c.origin, node.Kind, locator)
	return node, provenanceErr
}

func embeddedIdentityContainer(parent *graphNode) string {
	if parent == nil {
		return "unknown"
	}
	return firstNonEmpty(parent.Locator, parent.URI, parent.Key)
}

func containingResourceURI(node *graphNode) string {
	if node == nil {
		return ""
	}
	frontier := []*graphNode{node}
	seen := make(map[string]struct{})
	for len(frontier) > 0 {
		sort.Slice(frontier, func(i, j int) bool { return frontier[i].Key < frontier[j].Key })
		next := make([]*graphNode, 0)
		for _, current := range frontier {
			if current == nil {
				continue
			}
			identity := firstNonEmpty(current.Key, current.Kind+"\x00"+current.Locator)
			if _, ok := seen[identity]; ok {
				continue
			}
			seen[identity] = struct{}{}
			if current.URI != "" &&
				(current.IdentityQuality == "addressable" || !strings.Contains(current.URI, "#")) {
				return current.URI
			}
			parents := make([]*graphNode, 0, len(current.Parents))
			for _, parent := range current.Parents {
				parents = append(parents, parent)
			}
			next = append(next, parents...)
		}
		frontier = next
	}
	return ""
}

func mergeEquivalentGraphNode(existing, candidate *graphNode) bool {
	if existing == nil || candidate == nil {
		return false
	}
	promoted := candidate.AcquisitionState == "readable" && candidate.Data != nil &&
		(existing.AcquisitionState != "readable" || existing.Data == nil ||
			(candidate.IdentityQuality == "addressable" && existing.IdentityQuality != "addressable"))
	if promoted {
		existing.URI = candidate.URI
		existing.Data = candidate.Data
		existing.Doc = candidate.Doc
		existing.AcquisitionState = candidate.AcquisitionState
		existing.ErrorClass = candidate.ErrorClass
		existing.Complete = candidate.Complete
		existing.IdentityQuality = candidate.IdentityQuality
		existing.SourceModel = candidate.SourceModel
		existing.SourcePath = candidate.SourcePath
		existing.Response = candidate.Response
	}

	existing.SensorExcerpts = mergeSensorExcerptSources(
		existing.SensorExcerpts,
		candidate.SensorExcerpts,
	)
	existing.Response = mergeResponseMetadata(existing.Response, candidate.Response)
	return promoted
}

func cloneSensorExcerptSources(values []sensorExcerptSource) []sensorExcerptSource {
	result := make([]sensorExcerptSource, len(values))
	for index, value := range values {
		result[index] = value
		result[index].Data = cloneJSONMap(value.Data)
	}
	return result
}

func mergeSensorExcerptSources(sets ...[]sensorExcerptSource) []sensorExcerptSource {
	byKey := make(map[string]sensorExcerptSource)
	for _, values := range sets {
		for _, value := range values {
			key := value.Path + "\x00" + value.Type + "\x00" + value.Units
			if _, exists := byKey[key]; exists {
				continue
			}
			value.Data = cloneJSONMap(value.Data)
			byKey[key] = value
		}
	}
	keys := make([]string, 0, len(byKey))
	for key := range byKey {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]sensorExcerptSource, 0, len(keys))
	for _, key := range keys {
		result = append(result, byKey[key])
	}
	return result
}

func (g *resourceGraph) addResponseDiagnostics() {
	seen := make(map[string]struct{})
	for _, node := range g.Nodes {
		if node == nil || node.AcquisitionState != "readable" {
			continue
		}
		for header, state := range map[string]string{
			"Content-Type":  node.Response.ContentTypeState,
			"OData-Version": node.Response.ODataVersionState,
		} {
			if state == "" || state == "valid" {
				continue
			}
			key := header + "\x00" + state
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			g.addDiagnostic(fmt.Sprintf("Redfish compatibility: response %s header is %s", header, state))
		}
	}
}

func mergeResponseMetadata(left, right responseMetadata) responseMetadata {
	if left.ContentTypeState == "" {
		return right
	}
	if right.ContentTypeState == "" {
		return left
	}
	left.ContentTypeState = worstResponseState(left.ContentTypeState, right.ContentTypeState)
	left.ODataVersionState = worstResponseState(left.ODataVersionState, right.ODataVersionState)
	if left.StartedAt.IsZero() || (!right.StartedAt.IsZero() && right.StartedAt.Before(left.StartedAt)) {
		left.StartedAt = right.StartedAt
	}
	if right.FinishedAt.After(left.FinishedAt) {
		left.FinishedAt = right.FinishedAt
	}
	return left
}

func worstResponseState(left, right string) string {
	rank := map[string]int{"": 0, "valid": 1, "missing": 2, "invalid": 3}
	if rank[right] > rank[left] {
		return right
	}
	return left
}

func nonEmptyStrings(values ...string) []string {
	var result []string
	for _, value := range values {
		if value != "" {
			result = append(result, value)
		}
	}
	return result
}

func embeddedLocator(container, path, id string, index int) string {
	if id != "" {
		return fmt.Sprintf("embedded:%s:%s:%s", container, path, id)
	}
	if index >= 0 {
		return fmt.Sprintf("embedded:%s:%s:position:%d", container, path, index)
	}
	return fmt.Sprintf("embedded:%s:%s:singleton", container, path)
}

func (c *protocolClient) legacyComponents(
	ctx context.Context,
	parent *graphNode,
	rel graphRelationship,
	data map[string]any,
) ([]*graphNode, bool, error) {
	var relationships []graphRelationship
	switch rel.ChildKind {
	case "legacy_thermal":
		relationships = []graphRelationship{
			{Path: "Temperatures", ChildKind: "sensor", Source: "deprecated_thermal"},
			{Path: "Fans", ChildKind: "fan", Source: "deprecated_thermal"},
			{Path: "Redundancy", ChildKind: "redundancy", Source: "deprecated_thermal"},
		}
	case "legacy_power":
		relationships = []graphRelationship{
			{Path: "PowerControl", ChildKind: "sensor", Source: "deprecated_power"},
			{Path: "Voltages", ChildKind: "sensor", Source: "deprecated_power"},
			{Path: "PowerSupplies", ChildKind: "power_supply", Source: "deprecated_power"},
			{Path: "Redundancy", ChildKind: "redundancy", Source: "deprecated_power"},
		}
	}
	var result []*graphNode
	complete := true
	var failures boundedErrorAccumulator
	for _, childRel := range relationships {
		value, ok := jsonPath(data, childRel.Path)
		if !ok {
			continue
		}
		array, ok := value.([]any)
		if !ok {
			complete = false
			failures.Add(fmt.Errorf("%s is not an array", childRel.Path))
			continue
		}
		var nodes []*graphNode
		var positions []int
		for index, raw := range array {
			obj, ok := raw.(map[string]any)
			if !ok {
				complete = false
				failures.Add(fmt.Errorf("%s member %d is not an object", childRel.Path, index))
				continue
			}
			node, err := c.embeddedNode(parent, childRel, obj, index)
			if err != nil {
				complete = false
				failures.Add(fmt.Errorf("%s member %d: %w", childRel.Path, index, err))
			}
			node.SourceModel = childRel.Source
			nodes = append(nodes, node)
			positions = append(positions, index)
		}
		c.makeDuplicateEmbeddedIDsPositional(parent, childRel, nodes, positions)
		result = append(result, nodes...)
	}
	return result, complete, failures.Err()
}

type sensorExcerptArraySpec struct {
	Path          string
	Type          string
	Units         string
	ScalarMembers bool
}

var sensorExcerptArraySpecs = map[string][]sensorExcerptArraySpec{
	"power_supply_metrics": {
		{Path: "RailPowerWatts", Type: "Power", Units: "W"},
		{Path: "RailCurrentAmps", Type: "Current", Units: "A"},
		{Path: "RailVoltage", Type: "Voltage", Units: "V"},
		{Path: "FanSpeedsPercent", Type: "Percent", Units: "%"},
	},
	"battery_metrics": {
		{Path: "OutputCurrentAmps", Type: "Current", Units: "A"},
		{Path: "OutputVoltages", Type: "Voltage", Units: "V"},
		{Path: "CellVoltages", Type: "Voltage", Units: "V"},
	},
	"environment_metrics": {
		{Path: "FanSpeedsPercent", Type: "Percent", Units: "%"},
	},
	"thermal_metrics": {
		{Path: "TemperatureReadingsCelsius", Type: "Temperature", Units: "Cel"},
	},
	"heater_metrics": {
		{Path: "TemperatureReadingsCelsius", Type: "Temperature", Units: "Cel"},
	},
	"drive_metrics": {
		{
			Path: "NVMeSMART.TemperatureSensorsCelsius", Type: "Temperature", Units: "Cel",
			ScalarMembers: true,
		},
	},
	"storage_controller_metrics": {
		{
			Path: "NVMeSMART.TemperatureSensorsCelsius", Type: "Temperature", Units: "Cel",
			ScalarMembers: true,
		},
	},
}

func (c *protocolClient) addEmbeddedEnrichmentComponents(
	ctx context.Context,
	graph *resourceGraph,
	parent *graphNode,
	rel graphRelationship,
	enrichments map[string]map[string]any,
	complete bool,
	queue *[]*graphNode,
) error {
	enrichmentKeys := make([]string, 0, len(enrichments))
	for key := range enrichments {
		enrichmentKeys = append(enrichmentKeys, key)
	}
	sort.Strings(enrichmentKeys)

	switch rel.ChildKind {
	case "processor_metrics", "assembly_document":
		path, kind, family := "CoreMetrics", "processor_core", "compute"
		if rel.ChildKind == "assembly_document" {
			path, kind, family = "Assemblies", "assembly", "firmware"
		}
		current := make([]*graphNode, 0)
		sliceComplete := complete
		for _, key := range enrichmentKeys {
			data := enrichments[key]
			identityParent := embeddedEnrichmentParent(parent, key)
			nodes, ok := c.embeddedArrayNodes(identityParent, path, kind, data)
			current = append(current, nodes...)
			sliceComplete = sliceComplete && ok
		}
		childRel := graphRelationship{
			ParentKind: parent.Kind, Path: rel.Path + "." + path, ChildKind: kind,
			Family: family, Mode: relationshipComponents, Source: "embedded_excerpt",
		}
		if err := c.addEmbeddedComponentSlice(
			graph, parent, childRel, current, sliceComplete, queue,
		); err != nil {
			return err
		}
	}

	for _, spec := range sensorExcerptArraySpecs[rel.ChildKind] {
		current := make([]*graphNode, 0)
		sliceComplete := complete
		for _, key := range enrichmentKeys {
			identityParent := embeddedEnrichmentParent(parent, key)
			nodes, ok, err := c.sensorExcerptArrayNodes(
				ctx,
				graph,
				identityParent,
				rel.Path+"."+spec.Path,
				spec,
				enrichments[key],
			)
			current = append(current, nodes...)
			sliceComplete = sliceComplete && ok
			if err != nil {
				graph.addDiagnostic(err.Error())
			}
		}
		childRel := graphRelationship{
			ParentKind: parent.Kind,
			Path:       rel.Path + "." + spec.Path,
			ChildKind:  "sensor",
			Family:     rel.Family,
			Mode:       relationshipComponents,
			Source:     "embedded_sensor_excerpt",
		}
		if err := c.addEmbeddedComponentSlice(
			graph, parent, childRel, current, sliceComplete, queue,
		); err != nil {
			return err
		}
	}
	return nil
}

func embeddedEnrichmentParent(parent *graphNode, source string) *graphNode {
	copy := *parent
	copy.Locator = embeddedLocator(embeddedIdentityContainer(parent), "enrichment", source, -1)
	return &copy
}

func (c *protocolClient) addEmbeddedComponentSlice(
	graph *resourceGraph,
	parent *graphNode,
	rel graphRelationship,
	current []*graphNode,
	complete bool,
	queue *[]*graphNode,
) error {
	unique := make([]*graphNode, 0, len(current))
	seen := make(map[string]struct{}, len(current))
	for _, node := range current {
		identity := node.Kind + "\x00" + node.Locator
		if _, exists := seen[identity]; exists {
			complete = false
			graph.addDiagnostic(fmt.Sprintf("%s contains duplicate embedded component identity", rel.Path))
			continue
		}
		seen[identity] = struct{}{}
		unique = append(unique, node)
	}
	if !complete {
		graph.Complete = false
	}
	current = c.reconcileGraphMembership(parent, rel, unique, complete)

	for _, node := range current {
		if err := graph.addChild(parent, node, queue); err != nil {
			return err
		}
	}
	graph.recordMembership(parent, rel, complete)
	return nil
}

func (c *protocolClient) sensorExcerptArrayNodes(
	ctx context.Context,
	graph *resourceGraph,
	parent *graphNode,
	sourcePath string,
	spec sensorExcerptArraySpec,
	data map[string]any,
) ([]*graphNode, bool, error) {
	raw, exists := jsonPath(data, spec.Path)
	if !exists {
		return nil, true, nil
	}
	array, ok := raw.([]any)
	if !ok {
		return nil, false, fmt.Errorf("%s is not a SensorExcerpt array", sourcePath)
	}
	rel := graphRelationship{
		Path: sourcePath, ChildKind: "sensor", Source: "embedded_sensor_excerpt",
	}
	result := make([]*graphNode, 0, len(array))
	positions := make([]int, 0, len(array))
	complete := true
	var failures boundedErrorAccumulator
	for index, item := range array {
		object, ok := item.(map[string]any)
		if !ok && spec.ScalarMembers && item != nil {
			object = map[string]any{"Reading": item}
			ok = true
		}
		if !ok {
			complete = false
			failures.Add(fmt.Errorf("%s member %d is not an object", sourcePath, index))
			continue
		}
		node, err := c.embeddedNode(parent, rel, object, index)
		if err != nil {
			complete = false
			failures.Add(fmt.Errorf("%s member %d: %w", sourcePath, index, err))
		}
		node.SourceModel = "embedded_sensor_excerpt"
		node.SensorExcerpts = []sensorExcerptSource{{
			Path: sourcePath, Type: spec.Type, Units: spec.Units, Data: cloneJSONMap(object),
		}}
		if node.Doc.Name == "" {
			node.Doc.Name = firstNonEmpty(
				stringAt(object, "MemberId"),
				stringAt(object, "Id"),
				fmt.Sprintf("%s member %d", spec.Path, index),
			)
		}
		if node.IdentityQuality == "data_source_uri" {
			target, resolveErr := resolveRedfishURI(c.origin, c.root, node.Locator, uriProvenance)
			if resolveErr == nil {
				resourceURI := canonicalResourceURI(target)
				if graph.findURI("sensor", resourceURI) != nil {
					node.URI = resourceURI
					node.Locator = resourceURI
					node.Key = resourceKey(c.origin, node.Kind, resourceURI)
				}
			}
		}
		result = append(result, node)
		positions = append(positions, index)
	}
	c.makeDuplicateEmbeddedIDsPositional(parent, rel, result, positions)
	return result, complete, failures.Err()
}

func (c *protocolClient) embeddedArrayNodes(
	parent *graphNode,
	path, kind string,
	data map[string]any,
) ([]*graphNode, bool) {
	raw, ok := jsonPath(data, path)
	if !ok {
		return nil, true
	}
	array, ok := raw.([]any)
	if !ok {
		return nil, false
	}
	rel := graphRelationship{Path: path, ChildKind: kind}
	result := make([]*graphNode, 0, len(array))
	positions := make([]int, 0, len(array))
	complete := true
	for index, item := range array {
		obj, ok := item.(map[string]any)
		if !ok {
			complete = false
			continue
		}
		node, err := c.embeddedNode(parent, rel, obj, index)
		if err != nil {
			complete = false
		}
		result = append(result, node)
		positions = append(positions, index)
	}
	c.makeDuplicateEmbeddedIDsPositional(parent, rel, result, positions)
	return result, complete
}

func (c *protocolClient) makeDuplicateEmbeddedIDsPositional(
	parent *graphNode,
	rel graphRelationship,
	nodes []*graphNode,
	positions []int,
) {
	counts := make(map[string]int, len(nodes))
	for _, node := range nodes {
		if node != nil && node.IdentityQuality == "embedded" {
			counts[node.Locator]++
		}
	}
	for i, node := range nodes {
		if node == nil || node.IdentityQuality != "embedded" || counts[node.Locator] < 2 {
			continue
		}
		position := i
		if i < len(positions) {
			position = positions[i]
		}
		node.IdentityQuality = "positional"
		node.Locator = embeddedLocator(embeddedIdentityContainer(parent), rel.Path, "", position)
		node.Key = resourceKey(c.origin, node.Kind, node.Locator)
	}
}

func (g *resourceGraph) findURI(kind, uri string) *graphNode {
	g.ensureLookupIndexes()
	return g.ByURI[kind+"\x00"+uri]
}

func (g *resourceGraph) ensureLookupIndexes() {
	if len(g.ByKey) == len(g.Nodes) && g.ByURI != nil {
		return
	}
	g.ByKey = make(map[string]*graphNode, len(g.Nodes))
	g.ByURI = make(map[string]*graphNode, len(g.Nodes))
	for _, node := range g.Nodes {
		g.ByKey[node.Key] = node
		if node.URI != "" {
			uriKey := node.Kind + "\x00" + node.URI
			if _, exists := g.ByURI[uriKey]; !exists {
				g.ByURI[uriKey] = node
			}
		}
	}
}

func serviceRootMap(root *serviceRootDocument) map[string]any {
	if root != nil && root.Raw != nil {
		return cloneJSONMap(root.Raw)
	}
	data := map[string]any{
		"@odata.id":      root.ODataID,
		"@odata.type":    root.ODataType,
		"Id":             root.ID,
		"Name":           root.Name,
		"RedfishVersion": root.RedfishVersion,
		"UUID":           root.UUID,
		"Vendor":         root.Vendor,
		"Product":        root.Product,
	}
	if root.UpdateService.ODataID != "" {
		data["UpdateService"] = map[string]any{"@odata.id": root.UpdateService.ODataID}
	}
	if root.Storage.ODataID != "" {
		data["Storage"] = map[string]any{"@odata.id": root.Storage.ODataID}
	}
	return data
}

func genericResourceFromMap(data map[string]any) genericResource {
	var result genericResource
	raw, err := json.Marshal(data)
	if err == nil {
		_ = json.Unmarshal(raw, &result)
	}
	return result
}

func cloneJSONMap(src map[string]any) map[string]any {
	if src == nil {
		return nil
	}
	out := make(map[string]any, len(src))
	for key, value := range src {
		out[key] = cloneJSONValue(value)
	}
	return out
}

func cloneJSONValue(value any) any {
	switch value := value.(type) {
	case map[string]any:
		return cloneJSONMap(value)
	case []any:
		out := make([]any, len(value))
		for i := range value {
			out[i] = cloneJSONValue(value[i])
		}
		return out
	default:
		return value
	}
}

func jsonPath(data map[string]any, path string) (any, bool) {
	var current any = data
	for segment := range strings.SplitSeq(path, ".") {
		object, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = object[segment]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

func stringValue(value any) (string, bool) {
	result, ok := value.(string)
	result = strings.TrimSpace(result)
	return result, ok && result != ""
}

func boundedDiagnostic(value string) string {
	const max = 1024
	if len(value) <= max {
		return value
	}
	return value[:max]
}
