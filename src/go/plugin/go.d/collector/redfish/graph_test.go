// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"slices"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGraphRelationshipsRejectObjectsWithoutLinkIdentity(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	client := newTestProtocolClient(t, testConfig(server.URL, "none"))
	parent := fixtureParent()
	rel := graphRelationship{Path: "Processors", ChildKind: "processor"}

	nodes, complete, err := client.acquireLinkedValues(
		context.Background(),
		parent,
		rel,
		map[string]any{},
		nil,
	)
	require.ErrorContains(t, err, "no usable @odata.id")
	assert.False(t, complete)
	assert.Empty(t, nodes)
}

func TestDuplicateEmbeddedIDsMakeEveryMemberPositional(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	client := newTestProtocolClient(t, testConfig(server.URL, "none"))
	parent := fixtureParent()
	nodes, complete := client.embeddedArrayNodes(parent, "Fans", "fan", map[string]any{
		"Fans": []any{
			map[string]any{"MemberId": "duplicate", "Name": "first"},
			map[string]any{"MemberId": "duplicate", "Name": "second"},
		},
	})
	require.True(t, complete)
	require.Len(t, nodes, 2)
	for index, node := range nodes {
		assert.Equal(t, "positional", node.IdentityQuality)
		require.NotNil(t, node.SourcePosition)
		assert.Equal(t, index, *node.SourcePosition)
		assert.Contains(t, node.Locator, "position:")
	}
	assert.NotEqual(t, nodes[0].Locator, nodes[1].Locator)
}

func TestDuplicateEmbeddedComponentMakesGraphAndSliceIncomplete(t *testing.T) {
	t.Parallel()

	client := &protocolClient{graphMembership: make(map[string]graphMembershipSnapshot)}
	graph := &resourceGraph{
		ByIdentity: make(map[string]*graphNode),
		KeySources: make(map[string]string),
		Complete:   true,
	}
	parent := fixtureParent()
	rel := graphRelationship{
		ParentKind: parent.Kind,
		Path:       "Fans",
		ChildKind:  "fan",
		Mode:       relationshipComponents,
		Source:     "embedded",
		RollupRank: 0,
	}
	component := func(key string) *graphNode {
		return &graphNode{
			Kind:             "fan",
			Key:              key,
			Locator:          parent.URI + "#/Fans/member:duplicate",
			AcquisitionState: "readable",
			Parents:          make(map[string]*graphNode),
			RollupParents:    make(map[string]*graphNode),
			SystemOwners:     make(map[string]*graphNode),
		}
	}
	var queue []*graphNode

	require.NoError(t, client.addEmbeddedComponentSlice(
		graph,
		parent,
		rel,
		[]*graphNode{component("fan-1"), component("fan-2")},
		true,
		&queue,
	))
	require.False(t, graph.Complete)
	require.Len(t, graph.Slices, 1)
	assert.False(t, graph.Slices[0].Complete)
	assert.Len(t, graph.Slices[0].Members, 1)
	assert.Len(t, queue, 1)
	require.Len(t, graph.Diagnostics, 1)
	assert.Contains(t, graph.Diagnostics[0], "duplicate embedded component identity")
}

func TestGraphMembershipRetentionIsBoundedWithoutDroppingCurrentCollection(t *testing.T) {
	client := &protocolClient{}
	parent := fixtureParent()
	rel := graphRelationship{
		ParentKind: parent.Kind,
		Path:       "Fans",
		ChildKind:  "fan",
		Mode:       relationshipComponents,
		RollupRank: 0,
	}
	component := func(key string) *graphNode {
		return &graphNode{
			Kind:             "fan",
			Key:              key,
			Locator:          "/redfish/v1/Fans/" + key,
			AcquisitionState: "readable",
			Parents:          make(map[string]*graphNode),
			RollupParents:    make(map[string]*graphNode),
			SystemOwners:     make(map[string]*graphNode),
		}
	}
	budget := retainedStateBudget{entries: 1, members: 1}

	current, retained := client.reconcileGraphMembershipWithinBudget(
		parent, rel, []*graphNode{component("first")}, true, budget,
	)
	require.True(t, retained)
	require.Len(t, current, 1)

	otherParent := fixtureParent()
	otherParent.Key = "other-parent"
	current, retained = client.reconcileGraphMembershipWithinBudget(
		otherParent, rel, []*graphNode{component("second")}, true, budget,
	)
	assert.False(t, retained)
	require.Len(t, current, 1)
	assert.Equal(t, "second", current[0].Key)
	assert.Len(t, client.graphMembership, 1)

	current, retained = client.reconcileGraphMembershipWithinBudget(
		parent, rel, []*graphNode{component("replacement"), component("overflow")}, true, budget,
	)
	assert.False(t, retained)
	require.Len(t, current, 2)
	assert.Empty(t, client.graphMembership)
}

func TestEmbeddedProvenancePreservesValidatedJSONPointer(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	client := newTestProtocolClient(t, testConfig(server.URL, "none"))
	parent := fixtureParent()
	nodes, complete := client.embeddedArrayNodes(parent, "Fans", "fan", map[string]any{
		"Fans": []any{
			map[string]any{
				"MemberId":      "1",
				"DataSourceUri": "/redfish/v1/Chassis/1/Sensors/1#/Reading",
			},
		},
	})
	require.True(t, complete)
	require.Len(t, nodes, 1)
	assert.Equal(t, "data_source_uri", nodes[0].IdentityQuality)
	assert.Equal(t, "/redfish/v1/Chassis/1/Sensors/1#/Reading", nodes[0].Locator)

	_, complete = client.embeddedArrayNodes(parent, "Fans", "fan", map[string]any{
		"Fans": []any{
			map[string]any{
				"MemberId":      "1",
				"DataSourceUri": "/redfish/v1/Chassis/1/Sensors/1#invalid",
			},
		},
	})
	assert.False(t, complete)
}

func TestEmbeddedProvenanceResolvesFragmentAgainstContainingResource(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	client := newTestProtocolClient(t, testConfig(server.URL, "none"))
	parent := fixtureParent()
	node, err := client.embeddedNode(
		parent,
		graphRelationship{Path: "Sensor", ChildKind: "sensor"},
		map[string]any{
			"Id":            "vendor-id-that-must-not-drive-a-singleton",
			"DataSourceUri": "#/Reading",
		},
		-1,
	)
	require.NoError(t, err)
	assert.Equal(
		t,
		"/redfish/v1/Chassis/Fixture-1#/Reading",
		node.Locator,
	)
}

func TestEmbeddedSingletonLocatorDoesNotDependOnOptionalID(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	client := newTestProtocolClient(t, testConfig(server.URL, "none"))
	parent := fixtureParent()
	rel := graphRelationship{Path: "Sensor", ChildKind: "sensor"}

	first, err := client.embeddedNode(parent, rel, map[string]any{"Id": "first"}, -1)
	require.NoError(t, err)
	second, err := client.embeddedNode(parent, rel, map[string]any{"Id": "second"}, -1)
	require.NoError(t, err)

	assert.Equal(t, first.Locator, second.Locator)
	assert.Contains(t, first.Locator, ":singleton")
}

func TestNestedEmbeddedIdentityUsesImmediateContainerAndResourceProvenance(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	client := newTestProtocolClient(t, testConfig(server.URL, "none"))
	container := fixtureParent()
	groups, complete := client.embeddedArrayNodes(container, "Groups", "sensor_group", map[string]any{
		"Groups": []any{
			map[string]any{"MemberId": "group-a", "Name": "A"},
			map[string]any{"MemberId": "group-b", "Name": "B"},
		},
	})
	require.True(t, complete)
	require.Len(t, groups, 2)

	var children []*graphNode
	for _, group := range groups {
		node, err := client.embeddedNode(
			group,
			graphRelationship{Path: "Members", ChildKind: "sensor"},
			map[string]any{"MemberId": "same"},
			0,
		)
		require.NoError(t, err)
		children = append(children, node)
	}
	require.NotEqual(t, groups[0].Locator, groups[1].Locator)
	require.NotEqual(t, children[0].Key, children[1].Key)
	provenance, err := client.embeddedNode(
		groups[0],
		graphRelationship{Path: "Members", ChildKind: "sensor"},
		map[string]any{"MemberId": "provenance", "DataSourceUri": "#/Reading"},
		0,
	)
	require.NoError(t, err)
	assert.Equal(t, "/redfish/v1/Chassis/Fixture-1", provenance.SourceContainer)
	assert.Equal(t, "/redfish/v1/Chassis/Fixture-1#/Reading", provenance.Locator)
}

func TestLinkedRelationshipArraysConsumeTheSharedMemberBudget(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	client := newTestProtocolClient(t, testConfig(server.URL, "none"))
	ctx := withOperationBudget(context.Background())
	require.NoError(t, consumeCollectionMemberBudget(ctx, maxCollectionMembers))

	_, complete, err := client.acquireLinkedValues(
		ctx,
		fixtureParent(),
		graphRelationship{Path: "Links", ChildKind: "fan"},
		[]any{map[string]any{"@odata.id": "/redfish/v1/Chassis/1/Fans/1"}},
		nil,
	)
	require.ErrorContains(t, err, "collection member work")
	assert.False(t, complete)
}

func TestPartialGraphRestoresUnvisitedMembershipRecursively(t *testing.T) {
	t.Parallel()

	client := &protocolClient{
		graphMembership: make(map[string]graphMembershipSnapshot),
		logicalOwners:   make(map[string]logicalPlacementSnapshot),
		systemOwners:    make(map[string][]string),
	}
	graph := &resourceGraph{
		ByIdentity: make(map[string]*graphNode),
		KeySources: make(map[string]string),
		Complete:   false,
	}
	service := &graphNode{
		Kind: "service", URI: "/redfish/v1/", Locator: "/redfish/v1/", Key: "service",
		AcquisitionState: "readable", IdentityQuality: "addressable",
		Parents: make(map[string]*graphNode), RollupParents: make(map[string]*graphNode),
		SystemOwners: make(map[string]*graphNode),
	}
	system := &graphNode{
		Kind: "system", URI: "/redfish/v1/Systems/1", Locator: "/redfish/v1/Systems/1", Key: "system",
		AcquisitionState: "readable", IdentityQuality: "addressable",
		Parents: map[string]*graphNode{service.Key: service}, RollupParents: make(map[string]*graphNode),
		SystemOwners: make(map[string]*graphNode),
	}
	system.SystemOwners[system.Key] = system
	require.NoError(t, graph.add(service))
	require.NoError(t, graph.add(system))

	processorRel := graphRelationship{
		ParentKind: "system", Path: "Processors", ChildKind: "processor",
		Mode: relationshipComponents, RollupRank: 0,
	}
	sensorRel := graphRelationship{
		ParentKind: "processor", Path: "Sensors", ChildKind: "sensor",
		Mode: relationshipComponents, RollupRank: 0,
	}
	processor := &graphNode{
		Kind: "processor", URI: "/redfish/v1/Systems/1/Processors/1",
		Locator: "/redfish/v1/Systems/1/Processors/1", Key: "processor",
		AcquisitionState: "readable", IdentityQuality: "addressable",
		Parents: make(map[string]*graphNode), RollupParents: make(map[string]*graphNode),
		SystemOwners: make(map[string]*graphNode),
	}
	sensor := &graphNode{
		Kind: "sensor", URI: "/redfish/v1/Systems/1/Processors/1/Sensors/1",
		Locator: "/redfish/v1/Systems/1/Processors/1/Sensors/1", Key: "sensor",
		AcquisitionState: "readable", IdentityQuality: "addressable",
		Parents: make(map[string]*graphNode), RollupParents: make(map[string]*graphNode),
		SystemOwners: make(map[string]*graphNode),
	}
	client.graphMembership[graphMembershipKey(system.Key, processorRel)] = graphMembershipSnapshot{
		ParentKey: system.Key, Relationship: processorRel, Members: []*graphNode{processor},
	}
	client.graphMembership[graphMembershipKey(processor.Key, sensorRel)] = graphMembershipSnapshot{
		ParentKey: processor.Key, Relationship: sensorRel, Members: []*graphNode{sensor},
	}

	require.NoError(t, client.finalizeGraphMembership(graph))
	restoredProcessor := graph.findKey(processor.Key)
	restoredSensor := graph.findKey(sensor.Key)
	require.NotNil(t, restoredProcessor)
	require.NotNil(t, restoredSensor)
	assert.Equal(t, "unknown", restoredProcessor.AcquisitionState)
	assert.Equal(t, "unknown", restoredSensor.AcquisitionState)
	assert.Contains(t, restoredProcessor.Parents, system.Key)
	assert.Contains(t, restoredSensor.Parents, processor.Key)
	require.Len(t, graph.Slices, 2)
	for _, slice := range graph.Slices {
		assert.False(t, slice.Complete)
	}
}

func TestPartialGraphRestoresReverseOrderedRetainedChainInOneWorklistPass(t *testing.T) {
	const depth = 2048
	client := &protocolClient{graphMembership: make(map[string]graphMembershipSnapshot)}
	graph := &resourceGraph{
		ByIdentity: make(map[string]*graphNode),
		KeySources: make(map[string]string),
		Complete:   false,
	}
	root := placementTestNode("service", "root", "/redfish/v1/", nil)
	require.NoError(t, graph.add(root))

	parentKey := root.Key
	for index := range depth {
		key := fmt.Sprintf("node-%04d", index)
		child := placementTestNode("sensor", key, fmt.Sprintf("/redfish/v1/Sensors/%d", index), nil)
		relationship := graphRelationship{
			ParentKind: "sensor", Path: "Children", ChildKind: "sensor",
			Mode: relationshipComponents, RollupRank: 0,
		}
		client.graphMembership[fmt.Sprintf("%04d", depth-index)] = graphMembershipSnapshot{
			ParentKey: parentKey, Relationship: relationship, Members: []*graphNode{child},
		}
		parentKey = child.Key
	}

	require.NoError(t, client.finalizeGraphMembership(graph))
	require.Len(t, graph.Nodes, depth+1)
	require.NotNil(t, graph.findKey(parentKey))
}

func TestPartialGraphRetainedCycleWithoutRootTerminatesWithoutRestoration(t *testing.T) {
	client := &protocolClient{graphMembership: make(map[string]graphMembershipSnapshot)}
	graph := &resourceGraph{
		ByIdentity: make(map[string]*graphNode),
		KeySources: make(map[string]string),
		Complete:   false,
	}
	root := placementTestNode("service", "root", "/redfish/v1/", nil)
	require.NoError(t, graph.add(root))

	relationship := graphRelationship{Path: "Children", ChildKind: "sensor", Mode: relationshipComponents}
	first := placementTestNode("sensor", "first", "/redfish/v1/Sensors/first", nil)
	second := placementTestNode("sensor", "second", "/redfish/v1/Sensors/second", nil)
	client.graphMembership["first"] = graphMembershipSnapshot{
		ParentKey: second.Key, Relationship: relationship, Members: []*graphNode{first},
	}
	client.graphMembership["second"] = graphMembershipSnapshot{
		ParentKey: first.Key, Relationship: relationship, Members: []*graphNode{second},
	}

	require.NoError(t, client.finalizeGraphMembership(graph))
	require.Len(t, graph.Nodes, 1)
	require.Empty(t, graph.Slices)
}

func TestCompleteGraphPrunesUnseenRetainedMembership(t *testing.T) {
	client := &protocolClient{
		graphMembership: map[string]graphMembershipSnapshot{
			"stale": {
				ParentKey: "old-parent",
				Relationship: graphRelationship{
					ParentKind: "system", Path: "Processors", ChildKind: "processor",
				},
			},
		},
	}
	graph := &resourceGraph{Complete: true}
	require.NoError(t, client.finalizeGraphMembership(graph))
	assert.Empty(t, client.graphMembership)
}

func TestEarlyGraphIntegrityFailureStillFinalizesRetainedStateAndDiagnostics(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	client := newTestProtocolClient(t, testConfig(server.URL, "none"))
	serviceKey := resourceKey(client.origin, "service", "/redfish/v1/")
	rel := graphRelationship{
		ParentKind: "service", Path: "Retained", ChildKind: "fan",
		Family: "thermal", Mode: relationshipComponents, RollupRank: 0,
	}
	retained := &graphNode{
		Kind: "fan", URI: "/redfish/v1/Chassis/1/Fans/retained",
		Locator: "/redfish/v1/Chassis/1/Fans/retained", Key: "retained",
		IdentityQuality: "addressable", AcquisitionState: "readable",
		Parents: make(map[string]*graphNode), RollupParents: make(map[string]*graphNode),
		SystemOwners: make(map[string]*graphNode),
	}
	client.graphMembership[graphMembershipKey(serviceKey, rel)] = graphMembershipSnapshot{
		ParentKey: serviceKey, Relationship: rel, Members: []*graphNode{retained},
	}

	baseURI := "/redfish/v1/Systems/1"
	baseKey := resourceKey(client.origin, "system", baseURI)
	require.NoError(t, client.identities.register([]identityBinding{{
		Domain: "resource", Key: baseKey, Preimage: "different\x00resource",
	}}))
	root := &serviceRootDocument{
		Raw: map[string]any{
			"@odata.id": "/redfish/v1/", "@odata.type": "#ServiceRoot.v1_19_0.ServiceRoot",
			"Id": "RootService", "Name": "Root Service", "RedfishVersion": "1.19.0",
		},
		Response: responseMetadata{ContentTypeState: "missing", ODataVersionState: "missing"},
	}
	graph, err := client.collectResourceGraph(context.Background(), root, []baseResource{{
		Kind: "system", URI: baseURI, AcquisitionState: "readable", MembershipComplete: true,
	}}, nil)
	require.ErrorIs(t, err, errIdentityIntegrity)
	require.False(t, graph.Complete)
	restored := graph.findKey(retained.Key)
	require.NotNil(t, restored)
	assert.Equal(t, "unknown", restored.AcquisitionState)
	assert.Contains(t, graph.Diagnostics, "Redfish compatibility: response Content-Type header is missing")
	assert.Contains(t, graph.Diagnostics, "Redfish compatibility: response OData-Version header is missing")
}

func TestGraphFrontierRotationPreventsLaterParentStarvation(t *testing.T) {
	var mu sync.Mutex
	var hits []string
	var cancel context.CancelFunc
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits = append(hits, r.URL.Path)
		stop := cancel
		mu.Unlock()
		writeJSON(w, map[string]any{
			"@odata.id":           r.URL.Path,
			"@odata.type":         "#ProcessorCollection.ProcessorCollection",
			"Members@odata.count": 0,
			"Members":             []any{},
		})
		if stop != nil {
			stop()
		}
	}))
	defer server.Close()
	client := newTestProtocolClient(t, testConfig(server.URL, "none"))
	root := &serviceRootDocument{Raw: map[string]any{
		"@odata.id": "/redfish/v1/", "@odata.type": "#ServiceRoot.v1_19_0.ServiceRoot",
		"Id": "RootService", "Name": "Root Service", "RedfishVersion": "1.19.0",
	}}
	base := []baseResource{
		{
			Kind: "system", URI: "/redfish/v1/Systems/A", AcquisitionState: "readable", MembershipComplete: true,
			Data: map[string]any{"Processors": map[string]any{"@odata.id": "/redfish/v1/Systems/A/Processors"}},
		},
		{
			Kind: "system", URI: "/redfish/v1/Systems/B", AcquisitionState: "readable", MembershipComplete: true,
			Data: map[string]any{"Processors": map[string]any{"@odata.id": "/redfish/v1/Systems/B/Processors"}},
		},
	}

	for range 2 {
		ctx, stop := context.WithCancel(withOperationBudget(context.Background()))
		mu.Lock()
		cancel = stop
		mu.Unlock()
		_, _ = client.collectResourceGraph(ctx, root, base, nil)
		stop()
	}
	mu.Lock()
	defer mu.Unlock()
	require.Len(t, hits, 2)
	assert.NotEqual(t, hits[0], hits[1], "the second collection must start from the other same-depth parent")
}

func TestGraphResourceRequiresExactTypeAndFinalIdentity(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	client := newTestProtocolClient(t, testConfig(server.URL, "none"))
	target, err := url.Parse(server.URL + "/redfish/v1/Chassis/1/Fans/1")
	require.NoError(t, err)

	for name, body := range map[string]string{
		"missing identity": `{"@odata.type":"#Fan.v1_0_0.Fan"}`,
		"wrong identity":   `{"@odata.id":"/redfish/v1/Chassis/1/Fans/2","@odata.type":"#Fan.v1_0_0.Fan"}`,
		"wrong type":       `{"@odata.id":"/redfish/v1/Chassis/1/Fans/1","@odata.type":"#Sensor.v1_0_0.Sensor"}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := client.graphNodeFromResponse("fan", &responseData{
				url:  target,
				body: []byte(body),
			}, "typed_resource")
			require.Error(t, err)
		})
	}
}

func TestPartialTopologyRestoresCompleteSystemPlacementAndSuppressesNewComponents(t *testing.T) {
	t.Parallel()

	client := &protocolClient{
		config:        Config{NodeMode: "system_vnodes"},
		logicalOwners: make(map[string]logicalPlacementSnapshot),
		systemOwners:  make(map[string][]string),
	}
	service := &graphNode{
		Kind: "service", Key: "service", URI: "/redfish/v1/",
		Parents: make(map[string]*graphNode), RollupParents: make(map[string]*graphNode),
		SystemOwners: make(map[string]*graphNode),
	}
	system := &graphNode{
		Kind: "system", Key: "system", URI: "/redfish/v1/Systems/1",
		Parents: map[string]*graphNode{service.Key: service}, RollupParents: make(map[string]*graphNode),
		SystemOwners: make(map[string]*graphNode),
	}
	system.SystemOwners[system.Key] = system
	sensor := &graphNode{
		Kind: "sensor", Key: "sensor", URI: "/redfish/v1/Sensors/1",
		Parents:       map[string]*graphNode{system.Key: system},
		RollupParents: map[string]*graphNode{system.Key: system},
		SystemOwners:  make(map[string]*graphNode),
	}
	complete := &resourceGraph{Nodes: []*graphNode{service, system, sensor}, Complete: true}
	client.resolveGraphPlacement(complete)
	require.True(t, sensor.PlacementComplete)
	require.Contains(t, sensor.SystemOwners, system.Key)

	partialService := cloneCurrentGraphNode(service)
	partialSystem := cloneCurrentGraphNode(system)
	partialSystem.SystemOwners[partialSystem.Key] = partialSystem
	partialSensor := cloneCurrentGraphNode(sensor)
	partialSensor.Parents[partialService.Key] = partialService
	partialSensor.RollupParents[partialService.Key] = partialService
	newSensor := &graphNode{
		Kind: "sensor", Key: "new-sensor", URI: "/redfish/v1/Sensors/2",
		Parents:       map[string]*graphNode{partialService.Key: partialService},
		RollupParents: map[string]*graphNode{partialService.Key: partialService},
		SystemOwners:  make(map[string]*graphNode),
	}
	partial := &resourceGraph{
		Nodes:    []*graphNode{partialService, partialSystem, partialSensor, newSensor},
		Complete: false,
	}
	client.resolveGraphPlacement(partial)

	require.True(t, partialSensor.PlacementComplete)
	require.Contains(t, partialSensor.SystemOwners, partialSystem.Key)
	require.True(t, client.metricPlacementReady(partialSensor))
	require.False(t, newSensor.PlacementComplete)
	require.False(t, client.metricPlacementReady(newSensor))
}

func TestManagerForChassisPlacementIsIndependentOfGraphOrder(t *testing.T) {
	t.Parallel()

	for _, managerFirst := range []bool{false, true} {
		name := "chassis-first"
		if managerFirst {
			name = "manager-first"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			system := &graphNode{
				Kind: "system", Key: "system", URI: "/redfish/v1/Systems/1",
				SystemOwners: make(map[string]*graphNode),
			}
			system.SystemOwners[system.Key] = system
			chassis := &graphNode{
				Kind: "chassis", Key: "chassis", URI: "/redfish/v1/Chassis/1",
				Data: map[string]any{"Links": map[string]any{
					"ComputerSystems": []any{map[string]any{"@odata.id": system.URI}},
				}},
				SystemOwners: make(map[string]*graphNode),
			}
			manager := &graphNode{
				Kind: "manager", Key: "manager", URI: "/redfish/v1/Managers/1",
				Data: map[string]any{"Links": map[string]any{
					"ManagerForChassis": []any{map[string]any{"@odata.id": chassis.URI}},
				}},
				SystemOwners: make(map[string]*graphNode),
			}
			nodes := []*graphNode{system, chassis, manager}
			if managerFirst {
				nodes = []*graphNode{manager, system, chassis}
			}

			root, err := url.Parse("https://bmc.example.test/redfish/v1/")
			require.NoError(t, err)
			client := &protocolClient{root: root, origin: "https://bmc.example.test"}
			client.resolveGraphPlacement(&resourceGraph{Nodes: nodes, Complete: true})

			require.Contains(t, manager.SystemOwners, system.Key)
		})
	}
}

func TestNestedChassisPlacementPropagatesTowardContainers(t *testing.T) {
	t.Parallel()

	root, err := url.Parse("https://bmc.example.test/redfish/v1/")
	require.NoError(t, err)
	client := &protocolClient{root: root, origin: "https://bmc.example.test"}

	systemA := placementTestNode("system", "system-a", "/redfish/v1/Systems/A", nil)
	systemA.SystemOwners[systemA.Key] = systemA
	systemB := placementTestNode("system", "system-b", "/redfish/v1/Systems/B", nil)
	systemB.SystemOwners[systemB.Key] = systemB
	childA := placementTestNode("chassis", "child-a", "/redfish/v1/Chassis/A", map[string]any{
		"ComputerSystems": []any{map[string]any{"@odata.id": systemA.URI}},
		"ContainedBy":     map[string]any{"@odata.id": "/redfish/v1/Chassis/Middle"},
	})
	childB := placementTestNode("chassis", "child-b", "/redfish/v1/Chassis/B", map[string]any{
		"ComputerSystems": []any{map[string]any{"@odata.id": systemB.URI}},
	})
	middle := placementTestNode("chassis", "middle", "/redfish/v1/Chassis/Middle", map[string]any{
		"ContainedBy": map[string]any{"@odata.id": "/redfish/v1/Chassis/Enclosure"},
	})
	enclosure := placementTestNode("chassis", "enclosure", "/redfish/v1/Chassis/Enclosure", map[string]any{
		"Contains": []any{map[string]any{"@odata.id": childB.URI}},
	})
	manager := placementTestNode("manager", "manager", "/redfish/v1/Managers/1", nil)
	enclosure.Data["Links"].(map[string]any)["ManagedBy"] = []any{
		map[string]any{"@odata.id": manager.URI},
	}

	graph := &resourceGraph{Nodes: []*graphNode{
		manager, enclosure, middle, childB, childA, systemB, systemA,
	}, Complete: true}
	client.resolveGraphPlacement(graph)

	require.Equal(t, map[string]*graphNode{systemA.Key: systemA}, childA.SystemOwners)
	require.Equal(t, map[string]*graphNode{systemB.Key: systemB}, childB.SystemOwners)
	require.Contains(t, middle.SystemOwners, systemA.Key)
	require.Contains(t, enclosure.SystemOwners, systemA.Key)
	require.Contains(t, enclosure.SystemOwners, systemB.Key)
	require.Len(t, enclosure.SystemOwners, 2)
	require.Contains(t, manager.SystemOwners, systemA.Key)
	require.Contains(t, manager.SystemOwners, systemB.Key)
}

func TestChassisPlacementHandlesDeepContainmentAndCycle(t *testing.T) {
	t.Parallel()

	root, err := url.Parse("https://bmc.example.test/redfish/v1/")
	require.NoError(t, err)
	client := &protocolClient{root: root, origin: "https://bmc.example.test"}
	system := placementTestNode("system", "system", "/redfish/v1/Systems/1", nil)
	system.SystemOwners[system.Key] = system
	child := placementTestNode("chassis", "chassis-0", "/redfish/v1/Chassis/0", map[string]any{
		"ComputerSystems": []any{map[string]any{"@odata.id": system.URI}},
	})
	nodes := []*graphNode{system, child}
	for index := 1; index <= 1_000; index++ {
		parent := placementTestNode(
			"chassis",
			fmt.Sprintf("chassis-%d", index),
			fmt.Sprintf("/redfish/v1/Chassis/%d", index),
			map[string]any{"Contains": []any{map[string]any{"@odata.id": child.URI}}},
		)
		nodes = append(nodes, parent)
		child = parent
	}
	child.Data["Links"].(map[string]any)["ContainedBy"] = map[string]any{"@odata.id": "/redfish/v1/Chassis/999"}
	slices.Reverse(nodes)

	client.resolveGraphPlacement(&resourceGraph{Nodes: nodes, Complete: true})
	require.Contains(t, child.SystemOwners, system.Key)
}

func TestPlacementResolverBoundsOwnerBindings(t *testing.T) {
	t.Parallel()

	system := placementTestNode("system", "system", "/redfish/v1/Systems/1", nil)
	system.SystemOwners[system.Key] = system
	first := placementTestNode("chassis", "first", "/redfish/v1/Chassis/1", nil)
	second := placementTestNode("chassis", "second", "/redfish/v1/Chassis/2", nil)
	resolver := newPlacementResolver(&resourceGraph{Nodes: []*graphNode{system, first, second}})
	resolver.maxBindings = 2
	require.True(t, resolver.addOwner(first, system))
	require.False(t, resolver.addOwner(second, system))
	require.True(t, resolver.overflow)
	require.Empty(t, second.SystemOwners)
}

func placementTestNode(kind, key, uri string, links map[string]any) *graphNode {
	if links == nil {
		links = make(map[string]any)
	}
	return &graphNode{
		Kind: kind, Key: key, URI: uri, Locator: uri,
		Data:    map[string]any{"Links": links},
		Parents: make(map[string]*graphNode), RollupParents: make(map[string]*graphNode),
		SystemOwners: make(map[string]*graphNode),
	}
}

func TestGraphFetchBrokerCoalescesCanonicalResource(t *testing.T) {
	t.Parallel()

	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		writeJSON(w, map[string]any{
			"@odata.id":   r.URL.Path,
			"@odata.type": "#Fan.v1_0_0.Fan",
			"Id":          "1",
			"Name":        "Fan",
		})
	}))
	defer server.Close()
	client := newTestProtocolClient(t, testConfig(server.URL, "none"))
	ctx := withGraphFetchBroker(withOperationBudget(context.Background()))
	rel := graphRelationship{ChildKind: "fan", Source: "typed_resource"}

	first, err := client.fetchGraphNode(
		ctx,
		"fan",
		"/redfish/v1/Chassis/1/Fans/1",
		rel,
		fixtureParent(),
		nil,
	)
	require.NoError(t, err)
	second, err := client.fetchGraphNode(
		ctx,
		"fan",
		server.URL+"/redfish/v1/Chassis/1/Fans/1",
		rel,
		fixtureParent(),
		nil,
	)
	require.NoError(t, err)
	assert.Equal(t, int64(1), requests.Load())
	assert.NotSame(t, first, second)
	assert.Equal(t, reflect.ValueOf(first.Data).Pointer(), reflect.ValueOf(second.Data).Pointer())
	first.Parents["first-parent"] = fixtureParent()
	assert.NotContains(t, second.Parents, "first-parent")
	assert.Equal(t, first.Key, second.Key)
}

func TestCompleteCollectionMembershipSurvivesMemberAcquisitionFailure(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	client := newTestProtocolClient(t, testConfig(server.URL, "none"))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cancel()
	members := []collectionMember{
		{Ref: redfishLink{ODataID: "/redfish/v1/Chassis/1/Fans/1"}},
		{Ref: redfishLink{ODataID: "/redfish/v1/Chassis/1/Fans/2"}},
	}

	nodes, membershipComplete, err := client.fetchGraphCollectionMembers(
		ctx,
		graphCollectionRequest{
			cursorKey:          "/redfish/v1/Chassis/1/Fans",
			parent:             fixtureParent(),
			relationship:       graphRelationship{ChildKind: "fan", Source: "typed_resource"},
			members:            members,
			membershipComplete: true,
		},
		nil,
	)
	require.Error(t, err)
	assert.True(t, membershipComplete)
	assert.Len(t, nodes, 2)
}
