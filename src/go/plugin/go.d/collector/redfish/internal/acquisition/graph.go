// SPDX-License-Identifier: GPL-3.0-or-later

package acquisition

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"sort"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/internal/identity"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/internal/measurement"
)

type graphNode struct {
	measurement.Resource
	Locator         string
	IdentityQuality string
	Parents         map[string]*graphNode
}

type resourceGraph struct {
	Nodes       []*graphNode
	ByIdentity  map[string]*graphNode
	KeySources  map[string]string
	Slices      []graphSlice
	Complete    bool
	diagnostics Diagnostics
	register    func([]identity.Binding) error
}

func (g *resourceGraph) addDiagnostic(value string) {
	if g == nil {
		return
	}
	g.diagnostics.Add(value)
}

func (c *Client) collectResourceGraph(
	ctx context.Context,
	root *serviceRootDocument,
	base []baseResource,
	stats *wireStats,
) (graph *resourceGraph, resultErr error) {
	ctx = withGraphFetchBroker(ctx)
	graph = &resourceGraph{
		Complete: true,
		register: c.identities.Register,
	}
	defer func() {
		if resultErr != nil {
			graph.Complete = false
		}
		if err := c.finalizeGraphMembership(graph, resultErr == nil); err != nil {
			graph.Complete = false
			resultErr = errors.Join(resultErr, err)
		}
		c.reconcileSensorExcerptIdentities(graph)
	}()
	queue, err := c.seedResourceGraph(graph, root, base)
	if err != nil {
		return graph, err
	}
	type representation struct{ quality, source string }
	visited := make(map[*graphNode]representation)
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
		current := representation{parent.IdentityQuality, parent.SourceModel}
		if previous, ok := visited[parent]; ok && previous == current {
			continue
		}
		// A later addressable representation may expose links absent from an excerpt.
		visited[parent] = current
		for _, rel := range relationshipsFor(parent.Kind) {
			if !c.familyEnabled(rel.Family) {
				continue
			}
			if err := ctx.Err(); err != nil {
				return graph, err
			}
			acquired := c.acquireRelationship(ctx, parent, rel, stats)
			if err := c.applyRelationship(graph, parent, rel, acquired, &queue); err != nil {
				return graph, err
			}
		}
	}
	return graph, nil
}

func (c *Client) applyRelationship(
	graph *resourceGraph,
	parent *graphNode,
	rel graphRelationship,
	acquired acquiredRelationship,
	queue *[]*graphNode,
) error {
	if len(acquired.enrichments) > 0 {
		if parent.Enrichment == nil {
			parent.Enrichment = make(map[string]measurement.Enrichment)
		}
		maps.Copy(parent.Enrichment, acquired.enrichments)
		parent.Response = mergeResponseMetadata(parent.Response, acquired.response)
	}
	if rel.Mode == relationshipEnrichment {
		if err := c.addEmbeddedEnrichmentComponents(graph, parent, rel, acquired.enrichments, acquired.complete && acquired.err == nil, queue); err != nil {
			return errors.Join(acquired.err, err)
		}
	} else if err := c.applyComponentSlice(graph, parent, rel, acquired.children, acquired.complete, queue); err != nil {
		return err
	}
	if acquired.err != nil {
		graph.addDiagnostic(fmt.Sprintf("%s %s: %v", parent.Kind, rel.Path, acquired.err))
	}
	if !acquired.complete || acquired.err != nil {
		graph.Complete = false
	}
	return nil
}

// applyComponentSlice is the sole owner of membership reconciliation, graph
// insertion, scheduling, and recording for both linked and embedded components.
func (c *Client) applyComponentSlice(
	graph *resourceGraph,
	parent *graphNode,
	rel graphRelationship,
	children []*graphNode,
	complete bool,
	queue *[]*graphNode,
) error {
	children = c.reconcileGraphMembership(parent, rel, children, complete)
	graph.recordMembership(parent, rel)
	for _, child := range children {
		if err := graph.addChild(parent, child, queue); err != nil {
			return err
		}
	}
	return nil
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

func (g *resourceGraph) add(node *graphNode) error {
	if node == nil {
		return errors.New("cannot add a nil Redfish graph node")
	}
	resourceIdentity := node.Kind + "\x00" + node.Locator
	if g.ByIdentity[resourceIdentity] != nil {
		return errors.New("duplicate Redfish graph resource identity")
	}
	preimage := resourceIdentity
	if existing, ok := g.KeySources[node.Key]; ok && existing != preimage {
		return fmt.Errorf("%w: resource key collision", identity.ErrIntegrity)
	}
	if g.register != nil {
		if err := g.register([]identity.Binding{{
			Domain: "resource", Key: node.Key, Preimage: preimage,
		}}); err != nil {
			return err
		}
	}
	g.Nodes = append(g.Nodes, node)
	if g.ByIdentity == nil {
		g.ByIdentity = make(map[string]*graphNode)
	}
	if g.KeySources == nil {
		g.KeySources = make(map[string]string)
	}
	g.ByIdentity[resourceIdentity] = node

	g.KeySources[node.Key] = preimage
	return nil
}

type acquiredRelationship struct {
	children    []*graphNode
	enrichments map[string]measurement.Enrichment
	response    measurement.ResponseTiming
	complete    bool
	err         error
}

func (c *Client) acquireRelationship(
	ctx context.Context,
	parent *graphNode,
	rel graphRelationship,
	stats *wireStats,
) acquiredRelationship {
	value, ok := measurement.Properties(parent.Data).Lookup(rel.Path)
	if !ok || value == nil {
		return acquiredRelationship{
			complete: true,
		}
	}

	if rel.Embedded {
		nodes, complete, err := c.acquireEmbeddedValues(parent, rel, value)
		return acquiredRelationship{
			children: nodes,
			complete: complete,
			err:      err,
		}
	}

	items, complete, err := c.acquireLinkedValues(ctx, rel, value, stats)
	if rel.Mode == relationshipEnrichment {
		result := make(map[string]measurement.Enrichment)
		var response measurement.ResponseTiming
		for i, item := range items {
			identity := firstNonEmpty(item.Locator, item.URI, fmt.Sprintf("position:%d", i))
			result[rel.ChildKind+":"+identity] = measurement.Enrichment{
				Data: item.Data,
				URI:  item.URI,
			}
			response = mergeResponseMetadata(response, item.Response)
		}
		return acquiredRelationship{
			enrichments: result,
			response:    response,
			complete:    complete,
			err:         err,
		}
	}
	if rel.Mode == relationshipLegacy {
		var children []*graphNode
		legacyComplete := true
		var legacyFailures boundedErrorAccumulator
		for _, item := range items {
			nodes, ok, itemErr := c.legacyComponents(parent, rel, item.Data)
			children = append(children, nodes...)
			legacyComplete = legacyComplete && ok
			legacyFailures.Add(itemErr)
		}
		legacyFailures.Add(err)
		return acquiredRelationship{
			children: children,
			complete: complete && legacyComplete,
			err:      legacyFailures.Err(),
		}
	}
	return acquiredRelationship{
		children: items,
		complete: complete,
		err:      err,
	}
}

func mergeEquivalentGraphNode(existing, candidate *graphNode) bool {
	if existing == nil || candidate == nil {
		return false
	}
	promoted := candidate.AcquisitionState == "readable" && candidate.Data != nil &&
		(existing.AcquisitionState != "readable" || existing.Data == nil ||
			(candidate.IdentityQuality == "addressable" && existing.IdentityQuality != "addressable") ||
			(candidate.SourceModel == "resource" && existing.SourceModel != "resource"))
	if promoted {
		existing.URI = candidate.URI
		existing.Data = candidate.Data
		existing.Doc = candidate.Doc
		existing.AcquisitionState = candidate.AcquisitionState
		// Identity proof survives an excerpt supplying this cycle's data. SourceModel
		// still selects the adapter and lets a later full resource take precedence.
		if existing.IdentityQuality != "addressable" {
			existing.IdentityQuality = candidate.IdentityQuality
		}
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

func cloneSensorExcerptSources(values []measurement.SensorExcerpt) []measurement.SensorExcerpt {
	result := make([]measurement.SensorExcerpt, len(values))
	for index, value := range values {
		result[index] = value
		result[index].Data = cloneJSONMap(value.Data)
	}
	return result
}

func mergeSensorExcerptSources(sets ...[]measurement.SensorExcerpt) []measurement.SensorExcerpt {
	byKey := make(map[string]measurement.SensorExcerpt)
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
	result := make([]measurement.SensorExcerpt, 0, len(keys))
	for _, key := range keys {
		result = append(result, byKey[key])
	}
	return result
}

func (c *Client) seedResourceGraph(
	graph *resourceGraph,
	root *serviceRootDocument,
	base []baseResource,
) ([]*graphNode, error) {
	service := &graphNode{
		Kind:    "service",
		URI:     "/redfish/v1/",
		Locator: "/redfish/v1/",
		Data:    cloneJSONMap(root.Raw),
		Doc: measurement.Document{
			Name: stringAt(root.Raw, "Name"),
		},
		AcquisitionState: "readable",
		IdentityQuality:  "addressable",
		SourceModel:      "resource",
		Response:         root.Response,
		Parents:          make(map[string]*graphNode),
	}
	service.Key = identity.ResourceKey(c.origin, service.Kind, service.Locator)
	if err := graph.add(service); err != nil {
		return nil, err
	}
	queue := []*graphNode{service}
	for _, item := range base {
		node := &graphNode{
			Kind:             item.Kind,
			URI:              item.URI,
			Locator:          item.URI,
			Data:             item.Data,
			Doc:              item.Doc,
			AcquisitionState: item.AcquisitionState,
			IdentityQuality:  "addressable",
			SourceModel:      "resource",
			Response:         item.Response,
			Parents:          map[string]*graphNode{service.Key: service},
		}
		node.Key = identity.ResourceKey(c.origin, node.Kind, node.Locator)
		if err := graph.add(node); err != nil {
			return nil, err
		}
		queue = append(queue, node)
	}
	return queue, nil
}

func (g *resourceGraph) measurementResources() []*measurement.Resource {
	resources := make([]*measurement.Resource, 0, len(g.Nodes))
	for _, node := range g.Nodes {
		if node.Kind != "update_service" {
			resources = append(resources, &node.Resource)
		}
	}
	return resources
}
