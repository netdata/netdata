// SPDX-License-Identifier: GPL-3.0-or-later

package acquisition

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/internal/identity"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/internal/measurement"
)

func (c *Client) acquireEmbeddedValues(
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

	return c.decodeEmbeddedMembers(parent, rel, values, singleton, false, "embedded", nil)
}

// decodeEmbeddedMembers preserves original array positions and valid siblings.
// Adaptation of legacy or SensorExcerpt metadata stays with its source owner.
func (c *Client) decodeEmbeddedMembers(
	parent *graphNode,
	rel graphRelationship,
	values []any,
	singleton, scalarMembers bool,
	label string,
	decorate func(*graphNode, int),
) ([]*graphNode, bool, error) {
	result := make([]*graphNode, 0, len(values))
	positions := make([]int, 0, len(values))
	complete := true
	var failures boundedErrorAccumulator
	for index, value := range values {
		obj, ok := value.(map[string]any)
		if !ok && scalarMembers && value != nil {
			obj, ok = map[string]any{"Reading": value}, true
		}
		if !ok {
			complete = false
			failures.Add(fmt.Errorf("%s member %d is not an object", label, index))
			continue
		}
		position := index
		if singleton {
			position = -1
		}
		node, err := c.embeddedNode(parent, rel, obj, position)
		if err != nil {
			complete = false
			failures.Add(fmt.Errorf("%s member %d: %w", label, index, err))
		}
		if decorate != nil {
			decorate(node, index)
		}
		result = append(result, node)
		positions = append(positions, position)
	}
	c.makeDuplicateEmbeddedIDsPositional(parent, rel, result, positions)
	return result, complete, failures.Err()
}

func (c *Client) embeddedNode(
	parent *graphNode,
	rel graphRelationship,
	data map[string]any,
	index int,
) (*graphNode, error) {
	id, _ := measurement.Properties(data).Text("MemberId")
	if id == "" {
		id, _ = measurement.Properties(data).Text("Id")
	}
	if index < 0 {
		id = ""
	}
	locator := ""
	quality := "embedded"
	var provenanceErr error
	if rawURI, ok := measurement.Properties(data).Text("DataSourceUri"); ok {
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
	doc, _ := decodeGenericResource(data) // Embedded objects retain valid fields from partial documents.
	if doc.Name == "" {
		doc.Name, _ = measurement.Properties(data).Text("GroupName")
	}
	if doc.Name == "" {
		doc.Name, _ = measurement.Properties(data).Text("DeviceName")
	}
	node := &graphNode{
		Kind:             rel.ChildKind,
		URI:              uri,
		Locator:          locator,
		Data:             cloneJSONMap(data),
		Doc:              doc,
		AcquisitionState: "readable",
		IdentityQuality:  quality,
		SourcePath:       rel.Path,
		SourceModel:      "embedded_excerpt",
		Response:         parent.Response,
		Parents:          map[string]*graphNode{parent.Key: parent},
	}
	node.Key = identity.ResourceKey(c.origin, node.Kind, locator)
	if quality == "data_source_uri" {
		// Reading adapters also consume provenance. Resolve it once against the
		// containing document before the node is attached to its ownership parent.
		node.Data["DataSourceUri"] = locator
	}
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

func embeddedLocator(container, path, id string, index int) string {
	if id != "" {
		return "embedded:" + identity.Tuple(container, path, "id", id)
	}
	if index >= 0 {
		return "embedded:" + identity.Tuple(container, path, "position", strconv.Itoa(index))
	}
	return "embedded:" + identity.Tuple(container, path, "singleton")
}

func (c *Client) legacyComponents(
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
		value, ok := measurement.Properties(data).Lookup(childRel.Path)
		if !ok {
			continue
		}
		array, ok := value.([]any)
		if !ok {
			complete = false
			failures.Add(fmt.Errorf("%s is not an array", childRel.Path))
			continue
		}
		nodes, valid, err := c.decodeEmbeddedMembers(parent, childRel, array, false, false, childRel.Path, nil)
		complete = complete && valid
		failures.Add(err)
		for _, node := range nodes {
			node.SourceModel = childRel.Source
		}

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

func (c *Client) addEmbeddedEnrichmentComponents(
	graph *resourceGraph,
	parent *graphNode,
	rel graphRelationship,
	enrichments map[string]measurement.Enrichment,
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
			item := enrichments[key]
			identityParent := embeddedEnrichmentParent(parent, key, item.URI)
			nodes, ok := c.embeddedArrayNodes(identityParent, path, kind, item.Data)
			current = append(current, nodes...)
			sliceComplete = sliceComplete && ok
		}
		childRel := graphRelationship{
			ParentKind: parent.Kind,
			Path:       rel.Path + "." + path,
			ChildKind:  kind,
			Family:     family,
			Mode:       relationshipComponents,
			Source:     "embedded_excerpt",
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
			item := enrichments[key]
			identityParent := embeddedEnrichmentParent(parent, key, item.URI)
			nodes, ok, err := c.sensorExcerptArrayNodes(
				identityParent,
				rel.Path+"."+spec.Path,
				spec,
				item.Data,
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

func embeddedEnrichmentParent(parent *graphNode, source, uri string) *graphNode {
	copy := *parent
	copy.Locator = embeddedLocator(embeddedIdentityContainer(parent), "enrichment", source, -1)
	if uri != "" {
		copy.URI = uri
		copy.IdentityQuality = "addressable"
	}
	return &copy
}

func (c *Client) addEmbeddedComponentSlice(
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
	return c.applyComponentSlice(graph, parent, rel, unique, complete, queue)
}

func (c *Client) sensorExcerptArrayNodes(
	parent *graphNode,
	sourcePath string,
	spec sensorExcerptArraySpec,
	data map[string]any,
) ([]*graphNode, bool, error) {
	raw, exists := measurement.Properties(data).Lookup(spec.Path)
	if !exists {
		return nil, true, nil
	}
	array, ok := raw.([]any)
	if !ok {
		return nil, false, fmt.Errorf("%s is not a SensorExcerpt array", sourcePath)
	}
	rel := graphRelationship{
		Path:      sourcePath,
		ChildKind: "sensor",
		Source:    "embedded_sensor_excerpt",
	}
	return c.decodeEmbeddedMembers(
		parent,
		rel,
		array,
		false,
		spec.ScalarMembers,
		sourcePath,
		func(node *graphNode, index int) {
			node.SourceModel = "embedded_sensor_excerpt"
			node.SensorExcerpts = []measurement.SensorExcerpt{
				{Path: sourcePath, Type: spec.Type, Units: spec.Units, Data: cloneJSONMap(node.Data)},
			}
			if node.Doc.Name == "" {
				node.Doc.Name = firstNonEmpty(
					stringAt(node.Data, "MemberId"),
					stringAt(node.Data, "Id"),
					fmt.Sprintf("%s member %d", spec.Path, index),
				)
			}
		},
	)
}

func (c *Client) embeddedArrayNodes(
	parent *graphNode,
	path, kind string,
	data map[string]any,
) ([]*graphNode, bool) {
	raw, ok := measurement.Properties(data).Lookup(path)
	if !ok {
		return nil, true
	}
	array, ok := raw.([]any)
	if !ok {
		return nil, false
	}
	rel := graphRelationship{
		Path:      path,
		ChildKind: kind,
	}
	// These legacy array adapters report incompleteness without per-member diagnostics.
	nodes, complete, _ := c.decodeEmbeddedMembers(parent, rel, array, false, false, path, nil)
	return nodes, complete
}

func (c *Client) makeDuplicateEmbeddedIDsPositional(
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
		node.Key = identity.ResourceKey(c.origin, node.Kind, node.Locator)
	}
}
