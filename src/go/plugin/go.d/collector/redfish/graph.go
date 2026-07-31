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

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/internal/registry"
)

const (
	maxGraphResources            = 100_000
	maxPlacementEdges            = maxGraphResources
	maxPlacementOwnerBindings    = maxGraphResources
	maxPlacementPropagationSteps = maxGraphResources * 16
)

type relationshipMode = registry.RelationshipMode

const (
	relationshipComponents  = registry.RelationshipComponent
	relationshipEnrichment  = registry.RelationshipEnrichment
	relationshipTraversal   = registry.RelationshipTraversal
	relationshipAssociation = registry.RelationshipAssociation
	relationshipLegacy      = registry.RelationshipLegacy
)

type graphRelationship struct {
	ParentKind string
	Path       string
	ChildKind  string
	Family     string
	Mode       relationshipMode
	Embedded   bool
	Source     string
	RollupRank int
}

type graphCollectionRequest struct {
	cursorKey          string
	parent             *graphNode
	relationship       graphRelationship
	members            []collectionMember
	membershipComplete bool
	pageErr            error
}

func graphRelationshipsFromRegistry() []graphRelationship {
	result := make([]graphRelationship, 0, len(standardRegistry.Relationships))
	for _, source := range standardRegistry.Relationships {
		sourceModel := source.SourceModel
		if source.Embedded && sourceModel == "" {
			sourceModel = "embedded_excerpt"
		}
		result = append(result, graphRelationship{
			ParentKind: string(source.Parent),
			Path:       source.Path,
			ChildKind:  string(source.Child),
			Family:     source.Family,
			Mode:       source.Mode,
			Embedded:   source.Embedded,
			Source:     sourceModel,
			RollupRank: source.RollupRank,
		})
	}
	return result
}

var graphRelationships = graphRelationshipsFromRegistry()

type graphNode struct {
	Kind             string
	URI              string
	Locator          string
	Key              string
	Data             map[string]any
	Typed            any
	Enrichment       map[string]map[string]any
	Doc              genericResource
	AcquisitionState string
	ErrorClass       string
	Complete         bool
	IdentityQuality  string
	SourceContainer  string
	SourcePath       string
	SourcePosition   *int
	SourceModel      string
	SourceURIs       []string
	SourceModels     []string
	Response         responseMetadata
	SensorExcerpts   []sensorExcerptSource

	Parents           map[string]*graphNode
	RollupParents     map[string]*graphNode
	RollupOwner       *graphNode
	LogicalOwner      *graphNode
	LogicalReason     string
	LogicalCandidates []string
	SystemOwners      map[string]*graphNode
	PlacementComplete bool
	TraversalDepth    int
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
	ByAnyURI    map[string]*graphNode
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
	Family    string
	Source    string
	Mode      relationshipMode
	Complete  bool
	Members   []string
}

type graphMembershipSnapshot struct {
	ParentKey    string
	Relationship graphRelationship
	Members      []*graphNode
}

type logicalPlacementSnapshot struct {
	OwnerKey   string
	Candidates []string
	Reason     string
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

func (c *protocolClient) collectResourceGraph(
	ctx context.Context,
	root *serviceRootDocument,
	base []baseResource,
	stats *wireStats,
) (graph *resourceGraph, resultErr error) {
	ctx = withGraphFetchBroker(ctx)
	graph = &resourceGraph{
		ByIdentity: make(map[string]*graphNode),
		ByKey:      make(map[string]*graphNode),
		ByURI:      make(map[string]*graphNode),
		ByAnyURI:   make(map[string]*graphNode),
		KeySources: make(map[string]string),
		Complete:   true,
		register:   c.identities.register,
	}
	defer func() {
		if resultErr != nil {
			graph.Complete = false
		}
		if err := c.finalizeGraphMembership(graph); err != nil {
			graph.Complete = false
			resultErr = errors.Join(resultErr, err)
		}
		c.resolveGraphPlacement(graph)
		graph.addResponseDiagnostics()
	}()
	service := &graphNode{
		Kind:             "service",
		URI:              "/redfish/v1/",
		Locator:          "/redfish/v1/",
		Data:             serviceRootMap(root),
		AcquisitionState: "readable",
		Complete:         true,
		IdentityQuality:  "addressable",
		SourceModel:      "typed_resource",
		SourceURIs:       []string{"/redfish/v1/"},
		SourceModels:     []string{"typed_resource"},
		Response:         root.Response,
		Parents:          make(map[string]*graphNode),
		RollupParents:    make(map[string]*graphNode),
		SystemOwners:     make(map[string]*graphNode),
	}
	service.Key = resourceKey(c.origin, service.Kind, service.Locator)
	if err := graph.add(service); err != nil {
		return graph, err
	}

	queue := make([]*graphNode, 0, len(base)+1)
	queue = append(queue, service)
	for _, item := range base {
		node := &graphNode{
			Kind:             item.Kind,
			URI:              item.URI,
			Locator:          item.URI,
			Data:             cloneJSONMap(item.Data),
			Typed:            item.Typed,
			Doc:              item.Doc,
			AcquisitionState: item.AcquisitionState,
			ErrorClass:       item.ErrorClass,
			Complete:         item.MembershipComplete,
			IdentityQuality:  "addressable",
			SourceModel:      "typed_resource",
			SourceURIs:       nonEmptyStrings(item.URI),
			SourceModels:     []string{"typed_resource"},
			Response:         item.Response,
			Parents:          map[string]*graphNode{service.Key: service},
			RollupParents:    make(map[string]*graphNode),
			SystemOwners:     make(map[string]*graphNode),
			TraversalDepth:   1,
		}
		node.Key = resourceKey(c.origin, node.Kind, node.Locator)
		if item.Kind == "system" {
			node.SystemOwners[node.Key] = node
		}
		if err := graph.add(node); err != nil {
			graph.Complete = false
			return graph, err
		}
		queue = append(queue, node)
	}

	// Root base links were already acquired above. UpdateService and a
	// root-level Storage collection are the only additional root edges.
	var traversalErr error
traversal:
	for pos := 0; pos < len(queue); {
		depth := queue[pos].TraversalDepth
		end := pos
		for end < len(queue) && queue[end].TraversalDepth == depth {
			end++
		}
		frontier := append([]*graphNode(nil), queue[pos:end]...)
		sort.Slice(frontier, func(i, j int) bool { return frontier[i].Key < frontier[j].Key })
		frontierKey := fmt.Sprintf("graph-frontier-depth:%d", depth)
		frontierOrder := c.fairnessOrder(frontierKey, len(frontier))
		frontierAttempted := 0
		for _, frontierIndex := range frontierOrder {
			if err := ctx.Err(); err != nil {
				graph.Complete = false
				traversalErr = errors.Join(traversalErr, err)
				c.advanceFairnessAfterWork(frontierKey, len(frontier), frontierAttempted)
				break traversal
			}
			parent := frontier[frontierIndex]
			frontierAttempted++
			if parent.AcquisitionState != "readable" || parent.Data == nil {
				graph.Complete = false
				graph.addDiagnostic(fmt.Sprintf(
					"%s %s is %s (%s)",
					parent.Kind, parent.URI, parent.AcquisitionState, parent.ErrorClass,
				))
				for _, rel := range relationshipsFor(parent.Kind) {
					if rel.Mode == relationshipEnrichment || !c.familyEnabled(rel.Family) {
						continue
					}
					children, _ := c.reconcileGraphMembership(parent, rel, nil, false)
					slice := graphSlice{
						ParentKey: parent.Key,
						Path:      rel.Path,
						ChildKind: rel.ChildKind,
						Family:    rel.Family,
						Source:    rel.Source,
						Mode:      rel.Mode,
						Complete:  false,
						Members:   make([]string, 0, len(children)),
					}
					for _, child := range children {
						slice.Members = append(slice.Members, child.Key)
						existing := graph.ByIdentity[child.Kind+"\x00"+child.Locator]
						if existing != nil {
							mergeEquivalentGraphNode(existing, child)
							existing.Parents[parent.Key] = parent
							if rel.establishesRollupOwnership() {
								existing.RollupParents[parent.Key] = parent
							}
							continue
						}
						child.Parents[parent.Key] = parent
						child.TraversalDepth = parent.TraversalDepth + 1
						if rel.establishesRollupOwnership() {
							child.RollupParents[parent.Key] = parent
						}
						if err := graph.add(child); err != nil {
							graph.Complete = false
							return graph, err
						}
						queue = append(queue, child)
					}
					graph.Slices = append(graph.Slices, slice)
				}
				continue
			}
			relationships := relationshipsFor(parent.Kind)
			relationshipOrder := c.fairnessOrder(
				"graph-relationships\x00"+parent.Key,
				len(relationships),
			)
			relationshipsAttempted := 0
			for _, relationshipIndex := range relationshipOrder {
				if err := ctx.Err(); err != nil {
					graph.Complete = false
					traversalErr = errors.Join(traversalErr, err)
					c.advanceFairnessCursor(
						"graph-relationships\x00"+parent.Key,
						len(relationships),
						relationshipsAttempted,
					)
					c.advanceFairnessAfterWork(frontierKey, len(frontier), frontierAttempted)
					break traversal
				}
				rel := relationships[relationshipIndex]
				if !c.familyEnabled(rel.Family) {
					continue
				}
				relationshipsAttempted++
				children, enrichments, complete, err := c.acquireRelationship(ctx, parent, rel, stats)
				if rel.Mode != relationshipEnrichment {
					var retained bool
					children, retained = c.reconcileGraphMembership(parent, rel, children, complete)
					if !retained {
						graph.addDiagnostic("Redfish graph continuity state exceeded its internal retention budget")
					}
					slice := graphSlice{
						ParentKey: parent.Key,
						Path:      rel.Path,
						ChildKind: rel.ChildKind,
						Family:    rel.Family,
						Source:    rel.Source,
						Mode:      rel.Mode,
						Complete:  complete,
						Members:   make([]string, 0, len(children)),
					}
					for _, child := range children {
						slice.Members = append(slice.Members, child.Key)
					}
					graph.Slices = append(graph.Slices, slice)
				}
				if len(enrichments) > 0 {
					if parent.Enrichment == nil {
						parent.Enrichment = make(map[string]map[string]any)
					}
					maps.Copy(parent.Enrichment, enrichments)
				}
				if rel.Mode == relationshipEnrichment {
					if addErr := c.addEmbeddedEnrichmentComponents(
						ctx, graph, parent, rel, enrichments, complete && err == nil, &queue,
					); addErr != nil {
						graph.Complete = false
						return graph, errors.Join(err, addErr)
					}
				}
				for _, child := range children {
					existing := graph.ByIdentity[child.Kind+"\x00"+child.Locator]
					if existing != nil {
						mergeEquivalentGraphNode(existing, child)
						existing.Parents[parent.Key] = parent
						if rel.establishesRollupOwnership() {
							existing.RollupParents[parent.Key] = parent
						}
						continue
					}
					child.Parents[parent.Key] = parent
					child.TraversalDepth = parent.TraversalDepth + 1
					if rel.establishesRollupOwnership() {
						child.RollupParents[parent.Key] = parent
					}
					if err := graph.add(child); err != nil {
						graph.Complete = false
						return graph, err
					}
					queue = append(queue, child)
				}
				if err != nil {
					graph.Complete = false
					graph.addDiagnostic(fmt.Sprintf("%s %s: %v", parent.Kind, rel.Path, err))
				}
				if !complete {
					graph.Complete = false
				}
				if err != nil && classifyError(err) == "limit" {
					traversalErr = errors.Join(traversalErr, err)
					c.advanceFairnessCursor(
						"graph-relationships\x00"+parent.Key,
						len(relationships),
						relationshipsAttempted,
					)
					c.advanceFairnessAfterWork(frontierKey, len(frontier), frontierAttempted)
					break traversal
				}
			}
			c.advanceFairnessCursor(
				"graph-relationships\x00"+parent.Key,
				len(relationships),
				relationshipsAttempted,
			)
		}
		c.advanceFairnessAfterWork(frontierKey, len(frontier), frontierAttempted)
		pos = end
	}

	return graph, traversalErr
}

func (c *protocolClient) advanceFairnessAfterWork(key string, count, attempted int) {
	step := attempted
	if attempted == count {
		step = 1
	}
	c.advanceFairnessCursor(key, count, step)
}

func graphMembershipKey(parentKey string, rel graphRelationship) string {
	return parentKey + "\x00" + rel.Path + "\x00" + rel.ChildKind
}

func (c *protocolClient) finalizeGraphMembership(graph *resourceGraph) error {
	if graph == nil {
		return nil
	}
	observed := make(map[string]struct{}, len(graph.Slices))
	for _, slice := range graph.Slices {
		observed[slice.ParentKey+"\x00"+slice.Path+"\x00"+slice.ChildKind] = struct{}{}
	}
	if graph.Complete {
		c.graphMu.Lock()
		c.ensureGraphMembershipUsageLocked()
		for key := range c.graphMembership {
			if _, ok := observed[key]; !ok {
				c.graphMembershipSize -= len(c.graphMembership[key].Members)
				delete(c.graphMembership, key)
			}
		}
		indices := make([]int, len(graph.Slices))
		for index := range graph.Slices {
			indices[index] = index
		}
		sort.Slice(indices, func(i, j int) bool {
			left, right := graph.Slices[indices[i]], graph.Slices[indices[j]]
			return graphMembershipKeyFromSlice(left) < graphMembershipKeyFromSlice(right)
		})
		retentionComplete := true
		for _, index := range indices {
			slice := graph.Slices[index]
			key := graphMembershipKeyFromSlice(slice)
			if len(slice.Members) == 0 {
				continue
			}
			if _, exists := c.graphMembership[key]; exists {
				continue
			}
			if !retainedStateFits(
				len(c.graphMembership),
				c.graphMembershipSize,
				1,
				len(slice.Members),
				graphMembershipRetentionBudget,
			) {
				retentionComplete = false
				continue
			}
			parent := graph.findKey(slice.ParentKey)
			if parent == nil {
				retentionComplete = false
				continue
			}
			members := make([]*graphNode, 0, len(slice.Members))
			for _, memberKey := range slice.Members {
				member := graph.findKey(memberKey)
				if member == nil {
					retentionComplete = false
					continue
				}
				members = append(members, member)
			}
			if len(members) != len(slice.Members) {
				continue
			}
			c.graphMembership[key] = graphMembershipSnapshot{
				ParentKey:    parent.Key,
				Relationship: relationshipFromGraphSlice(parent, slice),
				Members:      cloneGraphNodesStatic(members, false),
			}
			c.graphMembershipSize += len(members)
		}
		c.graphMu.Unlock()
		if !retentionComplete {
			graph.addDiagnostic("Redfish graph continuity state exceeded its internal retention budget")
		}
		return nil
	}

	c.graphMu.Lock()
	pending := make(map[string]graphMembershipSnapshot, len(c.graphMembership))
	for key, snapshot := range c.graphMembership {
		if _, ok := observed[key]; ok {
			continue
		}
		pending[key] = graphMembershipSnapshot{
			ParentKey:    snapshot.ParentKey,
			Relationship: snapshot.Relationship,
			Members:      cloneGraphNodesStatic(snapshot.Members, true),
		}
	}
	c.graphMu.Unlock()

	keys := make([]string, 0, len(pending))
	waiting := make(map[string][]string)
	for key, snapshot := range pending {
		keys = append(keys, key)
		waiting[snapshot.ParentKey] = append(waiting[snapshot.ParentKey], key)
	}
	sort.Strings(keys)
	for parentKey := range waiting {
		sort.Strings(waiting[parentKey])
	}

	queued := make(map[string]struct{}, len(pending))
	ready := make([]string, 0, len(pending))
	enqueue := func(key string) {
		if _, exists := queued[key]; exists {
			return
		}
		queued[key] = struct{}{}
		ready = append(ready, key)
	}
	enqueueChildren := func(parentKey string) {
		for _, key := range waiting[parentKey] {
			enqueue(key)
		}
	}
	for _, key := range keys {
		if graph.findKey(pending[key].ParentKey) != nil {
			enqueue(key)
		}
	}

	var failures boundedErrorAccumulator
	for position := 0; position < len(ready); position++ {
		snapshot := pending[ready[position]]
		parent := graph.findKey(snapshot.ParentKey)
		if parent == nil {
			continue
		}
		rel := snapshot.Relationship
		slice := graphSlice{
			ParentKey: parent.Key,
			Path:      rel.Path,
			ChildKind: rel.ChildKind,
			Family:    rel.Family,
			Source:    rel.Source,
			Mode:      rel.Mode,
			Complete:  false,
			Members:   make([]string, 0, len(snapshot.Members)),
		}
		for _, child := range snapshot.Members {
			child.Parents[parent.Key] = parent
			child.TraversalDepth = parent.TraversalDepth + 1
			if rel.establishesRollupOwnership() {
				child.RollupParents[parent.Key] = parent
			}
			if existing := graph.ByIdentity[child.Kind+"\x00"+child.Locator]; existing != nil {
				mergeEquivalentGraphNode(existing, child)
				existing.Parents[parent.Key] = parent
				if rel.establishesRollupOwnership() {
					existing.RollupParents[parent.Key] = parent
				}
				slice.Members = append(slice.Members, existing.Key)
				enqueueChildren(existing.Key)
				continue
			}
			if err := graph.add(child); err != nil {
				failures.Add(fmt.Errorf("restore retained graph slice %s: %w", rel.Path, err))
				continue
			}
			slice.Members = append(slice.Members, child.Key)
			enqueueChildren(child.Key)
		}
		graph.Slices = append(graph.Slices, slice)
	}
	return failures.Err()
}

func graphMembershipKeyFromSlice(slice graphSlice) string {
	return graphMembershipKey(slice.ParentKey, graphRelationship{
		Path:      slice.Path,
		ChildKind: slice.ChildKind,
	})
}

func relationshipFromGraphSlice(parent *graphNode, slice graphSlice) graphRelationship {
	for _, candidate := range relationshipsFor(parent.Kind) {
		if candidate.Path == slice.Path &&
			candidate.ChildKind == slice.ChildKind &&
			candidate.Mode == slice.Mode &&
			candidate.Source == slice.Source {
			return candidate
		}
	}
	return graphRelationship{
		ParentKind: parent.Kind,
		Path:       slice.Path,
		ChildKind:  slice.ChildKind,
		Family:     slice.Family,
		Mode:       slice.Mode,
		Source:     slice.Source,
		RollupRank: 0,
	}
}

func (g *resourceGraph) detailEvidence(
	nodes []*graphNode,
	readings map[string][]normalizedReading,
) map[string]bool {
	nodeByKey := make(map[string]*graphNode, len(nodes))
	result := make(map[string]bool)
	incompleteOwnerKinds := make(map[string]map[string]struct{})
	for _, node := range nodes {
		nodeByKey[node.Key] = node
		if !isSubordinate(node.Kind) || node.LogicalOwner == nil {
			continue
		}
		result[node.LogicalOwner.Key+"\x00"+componentFamily(node, readings[node.Key])] = true
	}
	for _, slice := range g.Slices {
		if slice.Complete {
			continue
		}
		parent := nodeByKey[slice.ParentKey]
		if parent != nil {
			owner := parent.LogicalOwner
			if owner == nil && !isSubordinate(parent.Kind) {
				owner = parent
			}
			if owner != nil {
				kinds := incompleteOwnerKinds[owner.Key]
				if kinds == nil {
					kinds = make(map[string]struct{})
					incompleteOwnerKinds[owner.Key] = kinds
				}
				kinds[slice.ChildKind] = struct{}{}
				if slice.ChildKind == "sensor" || slice.ModeledFamilyIsVariable() {
					kinds["*"] = struct{}{}
				}
			}
		}
		for _, memberKey := range slice.Members {
			node := nodeByKey[memberKey]
			if node == nil || !isSubordinate(node.Kind) || node.LogicalOwner == nil {
				continue
			}
			result[node.LogicalOwner.Key+"\x00"+componentFamily(node, readings[node.Key])] = false
		}
	}
	for key := range result {
		ownerKey, family, ok := strings.Cut(key, "\x00")
		if !ok {
			continue
		}
		kinds := incompleteOwnerKinds[ownerKey]
		if _, all := kinds["*"]; all {
			result[key] = false
			continue
		}
		if _, exact := kinds[family]; exact {
			result[key] = false
		}
		if strings.HasPrefix(family, "sensor.") {
			if _, sensor := kinds["sensor"]; sensor {
				result[key] = false
			}
		}
	}
	return result
}

func (s graphSlice) ModeledFamilyIsVariable() bool {
	return s.Mode == relationshipLegacy
}

func (c *protocolClient) reconcileGraphMembership(
	parent *graphNode,
	rel graphRelationship,
	current []*graphNode,
	complete bool,
) ([]*graphNode, bool) {
	return c.reconcileGraphMembershipWithinBudget(
		parent,
		rel,
		current,
		complete,
		graphMembershipRetentionBudget,
	)
}

func (c *protocolClient) reconcileGraphMembershipWithinBudget(
	parent *graphNode,
	rel graphRelationship,
	current []*graphNode,
	complete bool,
	budget retainedStateBudget,
) ([]*graphNode, bool) {
	sliceKey := graphMembershipKey(parent.Key, rel)
	c.graphMu.Lock()
	defer c.graphMu.Unlock()
	c.ensureGraphMembershipUsageLocked()
	if complete {
		existing, exists := c.graphMembership[sliceKey]
		baseMembers := c.graphMembershipSize - len(existing.Members)
		baseEntries := len(c.graphMembership)
		if exists {
			baseEntries--
		}
		if len(current) == 0 {
			if exists {
				delete(c.graphMembership, sliceKey)
				c.graphMembershipSize = baseMembers
			}
			return current, true
		}
		if !retainedStateFits(baseEntries, baseMembers, 1, len(current), budget) {
			if exists {
				delete(c.graphMembership, sliceKey)
				c.graphMembershipSize = baseMembers
			}
			return current, false
		}
		c.graphMembership[sliceKey] = graphMembershipSnapshot{
			ParentKey:    parent.Key,
			Relationship: rel,
			Members:      cloneGraphNodesStatic(current, false),
		}
		c.graphMembershipSize = baseMembers + len(current)
		return current, true
	}
	snapshot := c.graphMembership[sliceKey]
	byIdentity := make(map[string]struct{}, len(current))
	previous := make(map[string]*graphNode, len(snapshot.Members))
	for _, node := range snapshot.Members {
		previous[node.Kind+"\x00"+node.Locator] = node
	}
	for _, node := range current {
		identity := node.Kind + "\x00" + node.Locator
		byIdentity[identity] = struct{}{}
		if node.AcquisitionState != "readable" {
			if retained := previous[identity]; retained != nil {
				node.Doc = retained.Doc
			}
		}
	}
	for _, retained := range cloneGraphNodesStatic(snapshot.Members, true) {
		if _, exists := byIdentity[retained.Kind+"\x00"+retained.Locator]; exists {
			continue
		}
		retained.Parents = map[string]*graphNode{parent.Key: parent}
		if rel.establishesRollupOwnership() {
			retained.RollupParents = map[string]*graphNode{parent.Key: parent}
		}
		current = append(current, retained)
	}
	return current, true
}

func (c *protocolClient) ensureGraphMembershipUsageLocked() {
	if c.graphMembership == nil {
		c.graphMembership = make(map[string]graphMembershipSnapshot)
		c.graphMembershipSize = 0
		c.graphMembershipCounted = true
		return
	}
	if c.graphMembershipCounted {
		return
	}
	c.graphMembershipSize = 0
	for _, snapshot := range c.graphMembership {
		c.graphMembershipSize += len(snapshot.Members)
	}
	c.graphMembershipCounted = true
}

func cloneGraphNodesStatic(nodes []*graphNode, unknown bool) []*graphNode {
	result := make([]*graphNode, 0, len(nodes))
	for _, source := range nodes {
		if source == nil {
			continue
		}
		node := *source
		node.Data = nil
		node.Typed = nil
		node.Enrichment = nil
		node.Doc.Status = genericStatus{}
		node.Doc.PowerState = ""
		node.Doc.FailurePredicted = nil
		node.Parents = make(map[string]*graphNode)
		node.RollupParents = make(map[string]*graphNode)
		node.SystemOwners = make(map[string]*graphNode)
		node.RollupOwner = nil
		node.LogicalOwner = nil
		node.LogicalCandidates = append([]string(nil), source.LogicalCandidates...)
		node.SourceURIs = append([]string(nil), source.SourceURIs...)
		node.SourceModels = append([]string(nil), source.SourceModels...)
		node.SensorExcerpts = cloneSensorExcerptSources(source.SensorExcerpts)
		if source.SourcePosition != nil {
			position := *source.SourcePosition
			node.SourcePosition = &position
		}
		if unknown {
			node.AcquisitionState = "unknown"
			node.ErrorClass = "protocol"
			node.Complete = false
		}
		result = append(result, &node)
	}
	return result
}

func cloneCurrentGraphNode(source *graphNode) *graphNode {
	return cloneGraphNode(source, true)
}

// Fetched JSON and typed models are immutable during a collection cycle.
// Sharing them preserves request coalescing without duplicating the largest
// part of every cached graph node.
func cloneFetchedGraphNode(source *graphNode) *graphNode {
	return cloneGraphNode(source, false)
}

func cloneGraphNode(source *graphNode, cloneData bool) *graphNode {
	if source == nil {
		return nil
	}
	node := *source
	if cloneData {
		node.Data = cloneJSONMap(source.Data)
	}
	node.Doc.Status.Conditions = append([]genericCondition(nil), source.Doc.Status.Conditions...)
	node.Enrichment = make(map[string]map[string]any, len(source.Enrichment))
	for key, value := range source.Enrichment {
		node.Enrichment[key] = cloneJSONMap(value)
	}
	node.Parents = make(map[string]*graphNode)
	node.RollupParents = make(map[string]*graphNode)
	node.SystemOwners = make(map[string]*graphNode)
	node.RollupOwner = nil
	node.LogicalOwner = nil
	node.LogicalCandidates = append([]string(nil), source.LogicalCandidates...)
	node.SourceURIs = append([]string(nil), source.SourceURIs...)
	node.SourceModels = append([]string(nil), source.SourceModels...)
	node.SensorExcerpts = cloneSensorExcerptSources(source.SensorExcerpts)
	if source.SourcePosition != nil {
		position := *source.SourcePosition
		node.SourcePosition = &position
	}
	return &node
}

func (r graphRelationship) establishesRollupOwnership() bool {
	return (r.Mode == relationshipComponents || r.Mode == relationshipLegacy) && r.RollupRank >= 0
}

func (g *resourceGraph) add(node *graphNode) error {
	if node == nil {
		return errors.New("cannot add a nil Redfish graph node")
	}
	if len(g.Nodes) >= maxGraphResources {
		return errors.New("Redfish resource graph exceeds the internal resource limit")
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
	if g.ByAnyURI == nil {
		g.ByAnyURI = make(map[string]*graphNode)
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
		if _, exists := g.ByAnyURI[node.URI]; !exists {
			g.ByAnyURI[node.URI] = node
		}
	}
	if node.Locator != "" {
		if _, exists := g.ByAnyURI[node.Locator]; !exists {
			g.ByAnyURI[node.Locator] = node
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
			parent.SourceURIs = mergeStringSets(parent.SourceURIs, item.SourceURIs)
			parent.SourceModels = mergeStringSets(parent.SourceModels, item.SourceModels)
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
	if err := consumeCollectionMemberBudget(ctx, len(values)); err != nil {
		return nil, false, err
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
		if err := consumeCollectionMemberBudget(ctx, len(typed)); err != nil {
			return nil, false, err
		}
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
				cursorKey:          collectionIdentity,
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
			"ordinary\x00"+collectionIdentity,
			rel.ChildKind,
			false,
		)
		_ = members
		return c.fetchGraphCollectionMembers(
			ctx,
			graphCollectionRequest{
				cursorKey:          collectionIdentity,
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
	order := c.collectionMemberOrder("graph\x00"+request.cursorKey, len(members))
	attemptedCount := 0
dispatch:
	for _, index := range order {
		select {
		case jobs <- index:
			attempted[index] = true
			attemptedCount++
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
	workComplete := dispatchComplete
	if request.pageErr != nil && classifyError(request.pageErr) == "limit" {
		workComplete = false
	}
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
			if classifyError(err) == "limit" {
				workComplete = false
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
	c.advanceCollectionMemberCursor(
		"graph\x00"+request.cursorKey,
		len(members),
		attemptedCount,
		workComplete,
	)
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
		typed, err := decodeTypedResource(rel.ChildKind, member.Raw)
		if err != nil {
			return nil, err
		}
		model := rel.Source
		if model == "" {
			model = "typed_resource"
		}
		node := &graphNode{
			Kind:             rel.ChildKind,
			URI:              member.Ref.ODataID,
			Locator:          member.Ref.ODataID,
			Data:             cloneJSONMap(member.Data),
			Typed:            typed,
			Doc:              genericResourceFromMap(member.Data),
			AcquisitionState: "readable",
			Complete:         true,
			IdentityQuality:  "addressable",
			SourceModel:      model,
			SourceURIs:       nonEmptyStrings(member.Ref.ODataID),
			SourceModels:     nonEmptyStrings(model),
			Response:         member.Response,
			Parents:          make(map[string]*graphNode),
			RollupParents:    make(map[string]*graphNode),
			SystemOwners:     make(map[string]*graphNode),
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
		SourceModel:      "typed_resource",
		SourceURIs:       nonEmptyStrings(uri),
		SourceModels:     []string{"typed_resource"},
		Parents:          make(map[string]*graphNode),
		RollupParents:    make(map[string]*graphNode),
		SystemOwners:     make(map[string]*graphNode),
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
	typed, err := decodeTypedResource(kind, response.body)
	if err != nil {
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
		model = "typed_resource"
	}
	node := &graphNode{
		Kind:             kind,
		URI:              uri,
		Locator:          uri,
		Data:             data,
		Typed:            typed,
		Doc:              doc,
		AcquisitionState: "readable",
		Complete:         true,
		IdentityQuality:  "addressable",
		SourceModel:      model,
		SourceURIs:       nonEmptyStrings(uri),
		SourceModels:     nonEmptyStrings(model),
		Response:         metadataForResponse(response),
		Parents:          make(map[string]*graphNode),
		RollupParents:    make(map[string]*graphNode),
		SystemOwners:     make(map[string]*graphNode),
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
	var position *int
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
		position = new(index)
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
		SourceContainer:  containingResourceURI(parent),
		SourcePath:       rel.Path,
		SourcePosition:   position,
		SourceModel:      "embedded_excerpt",
		SourceURIs:       mergeStringSets(nonEmptyStrings(containingResourceURI(parent)), nonEmptyStrings(locator)),
		SourceModels:     []string{"embedded_excerpt"},
		Response:         parent.Response,
		Parents:          map[string]*graphNode{parent.Key: parent},
		RollupParents:    map[string]*graphNode{parent.Key: parent},
		SystemOwners:     make(map[string]*graphNode),
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

func mergeEquivalentGraphNode(existing, candidate *graphNode) {
	if existing == nil || candidate == nil {
		return
	}
	if candidate.IdentityQuality == "addressable" && existing.IdentityQuality != "addressable" {
		existing.URI = candidate.URI
		existing.Data = candidate.Data
		existing.Typed = candidate.Typed
		existing.Doc = candidate.Doc
		existing.AcquisitionState = candidate.AcquisitionState
		existing.ErrorClass = candidate.ErrorClass
		existing.Complete = candidate.Complete
		existing.IdentityQuality = candidate.IdentityQuality
		existing.SourceModel = candidate.SourceModel
		existing.Response = candidate.Response
	}
	existing.SourceURIs = mergeStringSets(existing.SourceURIs, candidate.SourceURIs)
	existing.SourceModels = mergeStringSets(existing.SourceModels, candidate.SourceModels)
	existing.SensorExcerpts = mergeSensorExcerptSources(
		existing.SensorExcerpts,
		candidate.SensorExcerpts,
	)
	existing.Response = mergeResponseMetadata(existing.Response, candidate.Response)
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

func mergeStringSets(sets ...[]string) []string {
	unique := make(map[string]struct{})
	for _, values := range sets {
		for _, value := range values {
			if value != "" {
				unique[value] = struct{}{}
			}
		}
	}
	result := make([]string, 0, len(unique))
	for value := range unique {
		result = append(result, value)
	}
	sort.Strings(result)
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
			{Path: "Temperatures", ChildKind: "sensor", Source: "deprecated_thermal", RollupRank: 0},
			{Path: "Fans", ChildKind: "fan", Source: "deprecated_thermal", RollupRank: 0},
			{Path: "Redundancy", ChildKind: "redundancy", Source: "deprecated_thermal", RollupRank: 0},
		}
	case "legacy_power":
		relationships = []graphRelationship{
			{Path: "PowerControl", ChildKind: "sensor", Source: "deprecated_power", RollupRank: 0},
			{Path: "Voltages", ChildKind: "sensor", Source: "deprecated_power", RollupRank: 0},
			{Path: "PowerSupplies", ChildKind: "power_supply", Source: "deprecated_power", RollupRank: 0},
			{Path: "Redundancy", ChildKind: "redundancy", Source: "deprecated_power", RollupRank: 0},
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
		if err := consumeCollectionMemberBudget(ctx, len(array)); err != nil {
			complete = false
			failures.Add(fmt.Errorf("%s: %w", childRel.Path, err))
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
			node.SourceModels = mergeStringSets(node.SourceModels, nonEmptyStrings(childRel.Source))
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
			raw, exists := jsonPath(data, path)
			if exists {
				array, ok := raw.([]any)
				if !ok {
					sliceComplete = false
				} else if err := consumeCollectionMemberBudget(ctx, len(array)); err != nil {
					sliceComplete = false
					graph.addDiagnostic(err.Error())
					continue
				}
			}
			nodes, ok := c.embeddedArrayNodes(identityParent, path, kind, data)
			current = append(current, nodes...)
			sliceComplete = sliceComplete && ok
		}
		childRel := graphRelationship{
			ParentKind: parent.Kind, Path: rel.Path + "." + path, ChildKind: kind,
			Family: family, Mode: relationshipComponents, Source: "embedded_excerpt", RollupRank: 0,
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
			RollupRank: 0,
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
	var retained bool
	current, retained = c.reconcileGraphMembership(parent, rel, unique, complete)
	if !retained {
		graph.addDiagnostic("Redfish graph continuity state exceeded its internal retention budget")
	}
	slice := graphSlice{
		ParentKey: parent.Key, Path: rel.Path, ChildKind: rel.ChildKind, Family: rel.Family,
		Source: rel.Source, Mode: rel.Mode, Complete: complete,
		Members: make([]string, 0, len(current)),
	}
	for _, node := range current {
		slice.Members = append(slice.Members, node.Key)
		node.Parents[parent.Key] = parent
		node.TraversalDepth = parent.TraversalDepth + 1
		if rel.establishesRollupOwnership() {
			node.RollupParents[parent.Key] = parent
		}
		if existing := graph.ByIdentity[node.Kind+"\x00"+node.Locator]; existing != nil {
			mergeEquivalentGraphNode(existing, node)
			existing.Parents[parent.Key] = parent
			existing.RollupParents[parent.Key] = parent
			continue
		}
		if err := graph.add(node); err != nil {
			return err
		}
		*queue = append(*queue, node)
	}
	graph.Slices = append(graph.Slices, slice)
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
	if err := consumeCollectionMemberBudget(ctx, len(array)); err != nil {
		return nil, false, err
	}
	rel := graphRelationship{
		Path: sourcePath, ChildKind: "sensor", Source: "embedded_sensor_excerpt", RollupRank: 0,
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
		node.SourceModels = mergeStringSets(node.SourceModels, []string{"embedded_sensor_excerpt"})
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
					node.SourceURIs = mergeStringSets(node.SourceURIs, []string{node.Locator, resourceURI})
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
	rel := graphRelationship{Path: path, ChildKind: kind, RollupRank: 0}
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
		node.SourcePosition = new(position)
		node.Locator = embeddedLocator(embeddedIdentityContainer(parent), rel.Path, "", position)
		node.Key = resourceKey(c.origin, node.Kind, node.Locator)
		node.SourceURIs = mergeStringSets(nonEmptyStrings(containingResourceURI(parent)), nonEmptyStrings(node.Locator))
	}
}

func (c *protocolClient) resolveGraphPlacement(graph *resourceGraph) {
	placement := newPlacementResolver(graph)
	// Seed the topology from both link directions before propagating it through
	// direct acquisition parents.
	for _, node := range graph.Nodes {
		if node.Kind != "system" {
			continue
		}
		for _, uri := range c.canonicalLinkURIs(node.Data, "Links.Chassis") {
			if chassis := placement.findURI("chassis", uri); chassis != nil {
				placement.addOwner(chassis, node)
			}
		}
		for _, uri := range c.canonicalLinkURIs(node.Data, "Links.ManagedBy") {
			if manager := placement.findURI("manager", uri); manager != nil {
				placement.addOwner(manager, node)
			}
		}
	}
	for _, node := range graph.Nodes {
		if node.Kind != "chassis" {
			continue
		}
		for _, uri := range c.canonicalLinkURIs(node.Data, "Links.ComputerSystems") {
			if system := placement.findURI("system", uri); system != nil {
				placement.addOwner(node, system)
			}
		}
		for _, uri := range c.canonicalLinkURIs(node.Data, "Links.Contains") {
			if contained := placement.findURI("chassis", uri); contained != nil {
				placement.addEdge(contained, node)
			}
		}
		for _, uri := range c.canonicalLinkURIs(node.Data, "Links.ContainedBy") {
			if container := placement.findURI("chassis", uri); container != nil {
				placement.addEdge(node, container)
			}
		}
	}
	placement.propagate()
	placement.resetEdges()
	for _, node := range graph.Nodes {
		if node.Kind != "manager" {
			continue
		}
		for _, uri := range c.canonicalLinkURIs(node.Data, "Links.ManagerForServers") {
			if system := placement.findURI("system", uri); system != nil {
				placement.addOwner(node, system)
			}
		}
		for _, uri := range c.canonicalLinkURIs(node.Data, "Links.ManagerForChassis") {
			if chassis := placement.findURI("chassis", uri); chassis != nil {
				placement.copyOwners(node, chassis)
			}
		}
	}
	for _, node := range graph.Nodes {
		if node.Kind != "chassis" {
			continue
		}
		for _, uri := range c.canonicalLinkURIs(node.Data, "Links.ManagedBy") {
			if manager := placement.findURI("manager", uri); manager != nil {
				placement.copyOwners(manager, node)
			}
		}
	}

	for _, node := range graph.Nodes {
		for _, parent := range node.Parents {
			placement.addEdge(parent, node)
		}
	}
	placement.propagate()
	if placement.overflow {
		graph.Complete = false
		graph.addDiagnostic(
			"Redfish placement exceeded the internal topology safety limit; retaining prior authoritative placement",
		)
		for _, node := range graph.Nodes {
			if node.Kind != "system" {
				node.SystemOwners = make(map[string]*graphNode)
			}
		}
	}

	c.graphMu.Lock()
	if c.systemOwners == nil {
		c.systemOwners = make(map[string][]string)
	}
	if graph.Complete {
		for _, node := range graph.Nodes {
			node.PlacementComplete = true
		}
	} else {
		for _, node := range graph.Nodes {
			prior, exists := c.systemOwners[node.Key]
			if !exists {
				node.PlacementComplete = !isSubordinate(node.Kind)
				continue
			}
			restored := make(map[string]*graphNode, len(prior))
			complete := true
			for _, key := range prior {
				system := graph.findKey(key)
				if system == nil || system.Kind != "system" {
					complete = false
					continue
				}
				restored[key] = system
			}
			node.SystemOwners = restored
			node.PlacementComplete = complete
		}
	}
	c.graphMu.Unlock()

	for _, node := range graph.Nodes {
		node.RollupOwner = chooseRollupOwner(node)
		node.LogicalOwner, node.LogicalCandidates, node.LogicalReason = c.chooseLogicalOwner(node, graph)
	}

	c.graphMu.Lock()
	if c.logicalOwners == nil {
		c.logicalOwners = make(map[string]logicalPlacementSnapshot)
	}
	active := make(map[string]struct{}, len(graph.Nodes))
	for _, node := range graph.Nodes {
		active[node.Key] = struct{}{}
		if !graph.Complete {
			if priorDecision, ok := c.logicalOwners[node.Key]; ok {
				if prior := graph.findKey(priorDecision.OwnerKey); prior != nil {
					node.LogicalOwner = prior
					node.LogicalCandidates = append([]string(nil), priorDecision.Candidates...)
					node.LogicalReason = priorDecision.Reason
				}
			}
			continue
		}
		ownerKeys := make([]string, 0, len(node.SystemOwners))
		for key := range node.SystemOwners {
			ownerKeys = append(ownerKeys, key)
		}
		sort.Strings(ownerKeys)
		c.systemOwners[node.Key] = ownerKeys
		if node.LogicalOwner != nil {
			c.logicalOwners[node.Key] = logicalPlacementSnapshot{
				OwnerKey:   node.LogicalOwner.Key,
				Candidates: append([]string(nil), node.LogicalCandidates...),
				Reason:     node.LogicalReason,
			}
		} else {
			delete(c.logicalOwners, node.Key)
		}
	}
	if graph.Complete {
		for key := range c.logicalOwners {
			if _, exists := active[key]; !exists {
				delete(c.logicalOwners, key)
			}
		}
		for key := range c.systemOwners {
			if _, exists := active[key]; !exists {
				delete(c.systemOwners, key)
			}
		}
	}
	c.graphMu.Unlock()
}

func chooseRollupOwner(node *graphNode) *graphNode {
	if node.Kind == "service" || node.Kind == "system" || node.Kind == "chassis" || node.Kind == "manager" {
		return nil
	}
	parents := make([]*graphNode, 0, len(node.RollupParents))
	for _, parent := range node.RollupParents {
		parents = append(parents, parent)
	}
	if len(parents) == 1 {
		return parents[0]
	}
	if len(parents) == 0 {
		return nil
	}
	sort.Slice(parents, func(i, j int) bool {
		ri := rollupRank(node.Kind, parents[i].Kind)
		rj := rollupRank(node.Kind, parents[j].Kind)
		if ri != rj {
			return ri < rj
		}
		return parents[i].Key < parents[j].Key
	})
	best := rollupRank(node.Kind, parents[0].Kind)
	if len(parents) > 1 && rollupRank(node.Kind, parents[1].Kind) == best {
		return nil
	}
	if best >= 100 {
		return nil
	}
	return parents[0]
}

func rollupRank(child, parent string) int {
	best := 100
	for _, relationship := range graphRelationships {
		if relationship.ChildKind == child &&
			relationship.ParentKind == parent &&
			relationship.establishesRollupOwnership() {
			best = min(best, relationship.RollupRank)
		}
	}
	return best
}

func (c *protocolClient) chooseLogicalOwner(
	node *graphNode,
	graph *resourceGraph,
) (*graphNode, []string, string) {
	structural := structuralLogicalOwner(node, graph)
	candidateMap := make(map[string]*graphNode)
	for _, uri := range c.canonicalLinkURIs(node.Data, "RelatedItem") {
		target := graph.findAnyURI(uri)
		if target == nil {
			continue
		}
		if target.Kind == "system" {
			candidateMap[target.Key] = target
		}
		maps.Copy(candidateMap, target.SystemOwners)
	}
	candidateKeys := make([]string, 0, len(candidateMap))
	for key := range candidateMap {
		candidateKeys = append(candidateKeys, key)
	}
	sort.Strings(candidateKeys)
	if len(candidateKeys) == 1 {
		candidate := candidateMap[candidateKeys[0]]
		compatible := structural == nil || structural.Kind == "service" ||
			len(structural.SystemOwners) != 1
		if structural != nil && structural.Kind == "system" {
			compatible = structural.Key == candidate.Key
		}
		if structural != nil {
			if _, ok := structural.SystemOwners[candidate.Key]; ok {
				compatible = true
			}
		}
		if compatible {
			return candidate, candidateKeys, "related_item_unique"
		}
		return structural, candidateKeys, "related_item_incompatible"
	}
	if len(candidateKeys) > 1 {
		return structural, candidateKeys, "related_item_ambiguous"
	}
	if structural != nil {
		return structural, nil, "structural"
	}
	return nil, nil, "unresolved"
}

func structuralLogicalOwner(node *graphNode, graph *resourceGraph) *graphNode {
	for _, kind := range []string{"chassis", "system", "manager", "service"} {
		candidates := make([]*graphNode, 0)
		seenCandidates := make(map[string]struct{})
		visited := make(map[string]struct{})
		pending := []*graphNode{node}
		for len(pending) > 0 {
			current := pending[len(pending)-1]
			pending = pending[:len(pending)-1]
			if current == nil {
				continue
			}
			if _, ok := visited[current.Key]; ok {
				continue
			}
			visited[current.Key] = struct{}{}
			if current.Kind == kind {
				if _, ok := seenCandidates[current.Key]; !ok {
					seenCandidates[current.Key] = struct{}{}
					candidates = append(candidates, current)
				}
				continue
			}
			for _, parent := range current.RollupParents {
				pending = append(pending, parent)
			}
		}
		if len(candidates) == 1 {
			return candidates[0]
		}
		// Several equally near structural owners are shared ownership, not a
		// license to pick one by traversal order. Continue toward the next
		// broader structural scope.
	}
	return graph.findURI("service", "/redfish/v1/")
}

func (g *resourceGraph) findAnyURI(uri string) *graphNode {
	g.ensureLookupIndexes()
	return g.ByAnyURI[uri]
}

func (g *resourceGraph) findURI(kind, uri string) *graphNode {
	g.ensureLookupIndexes()
	return g.ByURI[kind+"\x00"+uri]
}

func (g *resourceGraph) ensureLookupIndexes() {
	if len(g.ByKey) == len(g.Nodes) && g.ByURI != nil && g.ByAnyURI != nil {
		return
	}
	g.ByKey = make(map[string]*graphNode, len(g.Nodes))
	g.ByURI = make(map[string]*graphNode, len(g.Nodes))
	g.ByAnyURI = make(map[string]*graphNode, len(g.Nodes))
	for _, node := range g.Nodes {
		g.ByKey[node.Key] = node
		if node.URI != "" {
			uriKey := node.Kind + "\x00" + node.URI
			if _, exists := g.ByURI[uriKey]; !exists {
				g.ByURI[uriKey] = node
			}
			if _, exists := g.ByAnyURI[node.URI]; !exists {
				g.ByAnyURI[node.URI] = node
			}
		}
		if node.Locator != "" {
			if _, exists := g.ByAnyURI[node.Locator]; !exists {
				g.ByAnyURI[node.Locator] = node
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

type placementEdge struct {
	from *graphNode
	to   *graphNode
}

type placementBinding struct {
	node   *graphNode
	system *graphNode
}

type placementResolver struct {
	byURI       map[string]*graphNode
	edges       map[*graphNode][]*graphNode
	edgeSet     map[placementEdge]struct{}
	bindings    int
	work        int
	overflow    bool
	maxEdges    int
	maxBindings int
	maxWork     int
}

func newPlacementResolver(graph *resourceGraph) *placementResolver {
	graph.ensureLookupIndexes()
	resolver := &placementResolver{
		byURI:       graph.ByURI,
		edges:       make(map[*graphNode][]*graphNode),
		edgeSet:     make(map[placementEdge]struct{}),
		maxEdges:    maxPlacementEdges,
		maxBindings: maxPlacementOwnerBindings,
		maxWork:     maxPlacementPropagationSteps,
	}
	for _, node := range graph.Nodes {
		resolver.bindings += len(node.SystemOwners)
	}
	if resolver.bindings > resolver.maxBindings {
		resolver.overflow = true
	}
	return resolver
}

func (r *placementResolver) findURI(kind, uri string) *graphNode {
	return r.byURI[kind+"\x00"+uri]
}

func (r *placementResolver) addEdge(from, to *graphNode) {
	if r.overflow || from == nil || to == nil {
		return
	}
	edge := placementEdge{from: from, to: to}
	if _, exists := r.edgeSet[edge]; exists {
		return
	}
	if len(r.edgeSet) >= r.maxEdges {
		r.overflow = true
		return
	}
	r.edgeSet[edge] = struct{}{}
	r.edges[from] = append(r.edges[from], to)
}

func (r *placementResolver) resetEdges() {
	r.edges = make(map[*graphNode][]*graphNode)
	r.edgeSet = make(map[placementEdge]struct{})
}

func (r *placementResolver) addOwner(node, system *graphNode) bool {
	if r.overflow || node == nil || system == nil {
		return false
	}
	if node.SystemOwners == nil {
		node.SystemOwners = make(map[string]*graphNode)
	}
	if _, exists := node.SystemOwners[system.Key]; exists {
		return false
	}
	if r.bindings >= r.maxBindings {
		r.overflow = true
		return false
	}
	node.SystemOwners[system.Key] = system
	r.bindings++
	return true
}

func (r *placementResolver) copyOwners(destination, source *graphNode) {
	if destination == nil || source == nil {
		return
	}
	for _, system := range source.SystemOwners {
		r.addOwner(destination, system)
	}
}

func (r *placementResolver) propagate() {
	if r.overflow {
		return
	}
	queue := make([]placementBinding, 0)
	for source := range r.edges {
		for _, system := range source.SystemOwners {
			queue = append(queue, placementBinding{node: source, system: system})
		}
	}
	for position := 0; position < len(queue) && !r.overflow; position++ {
		binding := queue[position]
		for _, destination := range r.edges[binding.node] {
			if r.work >= r.maxWork {
				r.overflow = true
				break
			}
			r.work++
			if r.addOwner(destination, binding.system) {
				queue = append(queue, placementBinding{
					node: destination, system: binding.system,
				})
			}
		}
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

func linkURIs(data map[string]any, path string) []string {
	value, ok := jsonPath(data, path)
	if !ok {
		return nil
	}
	var result []string
	switch value := value.(type) {
	case map[string]any:
		if uri, ok := stringValue(value["@odata.id"]); ok {
			result = append(result, uri)
		}
	case []any:
		for _, item := range value {
			if object, ok := item.(map[string]any); ok {
				if uri, ok := stringValue(object["@odata.id"]); ok {
					result = append(result, uri)
				}
			}
		}
	}
	return result
}

func (c *protocolClient) canonicalLinkURIs(data map[string]any, path string) []string {
	raw := linkURIs(data, path)
	result := make([]string, 0, len(raw))
	for _, value := range raw {
		target, err := c.resolveURI(c.root, value, false)
		if err != nil {
			continue
		}
		result = append(result, canonicalResourceURI(target))
	}
	return result
}

func boundedDiagnostic(value string) string {
	const max = 1024
	if len(value) <= max {
		return value
	}
	return value[:max]
}
