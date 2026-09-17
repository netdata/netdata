// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
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
	rel := graphRelationship{
		Path:      "Processors",
		ChildKind: "processor",
	}

	nodes, complete, err := client.acquireLinkedValues(
		context.Background(),
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
		assert.Equal(t, embeddedLocator(parent.URI, "Fans", "", index), node.Locator)
	}
	assert.NotEqual(t, nodes[0].Locator, nodes[1].Locator)
}

func TestDuplicateEmbeddedComponentMakesGraphAndSliceIncomplete(t *testing.T) {
	t.Parallel()

	client := &protocolClient{
		graphMembership: make(map[string]graphMembershipSnapshot),
	}
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
	}
	component := func(key string) *graphNode {
		return &graphNode{
			Kind:             "fan",
			Key:              key,
			Locator:          parent.URI + "#/Fans/member:duplicate",
			AcquisitionState: "readable",
			Parents:          make(map[string]*graphNode),
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
	assert.Len(t, queue, 1)
	require.Len(t, graph.Diagnostics, 1)
	assert.Contains(t, graph.Diagnostics[0], "duplicate embedded component identity")
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
		graphRelationship{
			Path:      "Sensor",
			ChildKind: "sensor",
		},
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
	rel := graphRelationship{
		Path:      "Sensor",
		ChildKind: "sensor",
	}

	first, err := client.embeddedNode(parent, rel, map[string]any{"Id": "first"}, -1)
	require.NoError(t, err)
	second, err := client.embeddedNode(parent, rel, map[string]any{"Id": "second"}, -1)
	require.NoError(t, err)

	assert.Equal(t, first.Locator, second.Locator)
	assert.Equal(t, embeddedLocator(parent.URI, "Sensor", "", -1), first.Locator)
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
			graphRelationship{
				Path:      "Members",
				ChildKind: "sensor",
			},
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
		graphRelationship{
			Path:      "Members",
			ChildKind: "sensor",
		},
		map[string]any{"MemberId": "provenance", "DataSourceUri": "#/Reading"},
		0,
	)
	require.NoError(t, err)

	assert.Equal(t, "/redfish/v1/Chassis/Fixture-1#/Reading", provenance.Locator)
}

func TestPartialGraphRestoresUnvisitedMembershipRecursively(t *testing.T) {
	t.Parallel()

	client := &protocolClient{
		graphMembership: make(map[string]graphMembershipSnapshot),
	}
	graph := &resourceGraph{
		ByIdentity: make(map[string]*graphNode),
		KeySources: make(map[string]string),
		Complete:   false,
	}
	service := &graphNode{
		Kind:             "service",
		URI:              "/redfish/v1/",
		Locator:          "/redfish/v1/",
		Key:              "service",
		AcquisitionState: "readable",
		IdentityQuality:  "addressable",
		Parents:          make(map[string]*graphNode),
	}
	system := &graphNode{
		Kind:             "system",
		URI:              "/redfish/v1/Systems/1",
		Locator:          "/redfish/v1/Systems/1",
		Key:              "system",
		AcquisitionState: "readable",
		IdentityQuality:  "addressable",
		Parents:          map[string]*graphNode{service.Key: service},
	}
	require.NoError(t, graph.add(service))
	require.NoError(t, graph.add(system))

	processorRel := graphRelationship{
		ParentKind: "system",
		Path:       "Processors",
		ChildKind:  "processor",
		Mode:       relationshipComponents,
	}
	sensorRel := graphRelationship{
		ParentKind: "processor",
		Path:       "Sensors",
		ChildKind:  "sensor",
		Mode:       relationshipComponents,
	}
	processor := &graphNode{
		Kind:             "processor",
		URI:              "/redfish/v1/Systems/1/Processors/1",
		Locator:          "/redfish/v1/Systems/1/Processors/1",
		Key:              "processor",
		AcquisitionState: "readable",
		IdentityQuality:  "addressable",
		Parents:          make(map[string]*graphNode),
	}
	sensor := &graphNode{
		Kind:             "sensor",
		URI:              "/redfish/v1/Systems/1/Processors/1/Sensors/1",
		Locator:          "/redfish/v1/Systems/1/Processors/1/Sensors/1",
		Key:              "sensor",
		AcquisitionState: "readable",
		IdentityQuality:  "addressable",
		Parents:          make(map[string]*graphNode),
	}
	client.graphMembership[graphMembershipKey(system.Key, processorRel)] = graphMembershipSnapshot{
		ParentKey:    system.Key,
		Relationship: processorRel,
		Members:      snapshotGraphMembers([]*graphNode{processor}),
	}
	client.graphMembership[graphMembershipKey(processor.Key, sensorRel)] = graphMembershipSnapshot{
		ParentKey:    processor.Key,
		Relationship: sensorRel,
		Members:      snapshotGraphMembers([]*graphNode{sensor}),
	}

	require.NoError(t, client.finalizeGraphMembership(graph, true))
	restoredProcessor := graph.findKey(processor.Key)
	restoredSensor := graph.findKey(sensor.Key)
	require.NotNil(t, restoredProcessor)
	require.NotNil(t, restoredSensor)
	assert.Equal(t, "unknown", restoredProcessor.AcquisitionState)
	assert.Equal(t, "unknown", restoredSensor.AcquisitionState)
	assert.Contains(t, restoredProcessor.Parents, system.Key)
	assert.Contains(t, restoredSensor.Parents, processor.Key)
	require.Len(t, graph.Slices, 2)
}

func TestPartialGraphRestoresReverseOrderedRetainedChainInOneWorklistPass(t *testing.T) {
	const depth = 2048
	client := &protocolClient{
		graphMembership: make(map[string]graphMembershipSnapshot),
	}
	graph := &resourceGraph{
		ByIdentity: make(map[string]*graphNode),
		KeySources: make(map[string]string),
		Complete:   false,
	}
	root := graphTestNode("service", "root", "/redfish/v1/", nil)
	require.NoError(t, graph.add(root))

	parentKey := root.Key
	for index := range depth {
		key := fmt.Sprintf("node-%04d", index)
		child := graphTestNode("sensor", key, fmt.Sprintf("/redfish/v1/Sensors/%d", index), nil)
		relationship := graphRelationship{
			ParentKind: "sensor",
			Path:       "Children",
			ChildKind:  "sensor",
			Mode:       relationshipComponents,
		}
		client.graphMembership[fmt.Sprintf("%04d", depth-index)] = graphMembershipSnapshot{
			ParentKey:    parentKey,
			Relationship: relationship,
			Members:      snapshotGraphMembers([]*graphNode{child}),
		}
		parentKey = child.Key
	}

	require.NoError(t, client.finalizeGraphMembership(graph, true))
	require.Len(t, graph.Nodes, depth+1)
	require.NotNil(t, graph.findKey(parentKey))
}

func TestPartialGraphRetainedCycleWithoutRootTerminatesWithoutRestoration(t *testing.T) {
	client := &protocolClient{
		graphMembership: make(map[string]graphMembershipSnapshot),
	}
	graph := &resourceGraph{
		ByIdentity: make(map[string]*graphNode),
		KeySources: make(map[string]string),
		Complete:   false,
	}
	root := graphTestNode("service", "root", "/redfish/v1/", nil)
	require.NoError(t, graph.add(root))

	relationship := graphRelationship{
		Path:      "Children",
		ChildKind: "sensor",
		Mode:      relationshipComponents,
	}
	first := graphTestNode("sensor", "first", "/redfish/v1/Sensors/first", nil)
	second := graphTestNode("sensor", "second", "/redfish/v1/Sensors/second", nil)
	client.graphMembership["first"] = graphMembershipSnapshot{
		ParentKey:    second.Key,
		Relationship: relationship,
		Members:      snapshotGraphMembers([]*graphNode{first}),
	}
	client.graphMembership["second"] = graphMembershipSnapshot{
		ParentKey:    first.Key,
		Relationship: relationship,
		Members:      snapshotGraphMembers([]*graphNode{second}),
	}

	require.NoError(t, client.finalizeGraphMembership(graph, true))
	require.Len(t, graph.Nodes, 1)
	require.Empty(t, graph.Slices)
}

func TestCompleteGraphPrunesUnseenRetainedMembership(t *testing.T) {
	client := &protocolClient{
		graphMembership: map[string]graphMembershipSnapshot{
			"stale": {
				ParentKey: "old-parent",
				Relationship: graphRelationship{
					ParentKind: "system",
					Path:       "Processors",
					ChildKind:  "processor",
				},
			},
		},
	}
	graph := &resourceGraph{
		Complete: true,
	}
	require.NoError(t, client.finalizeGraphMembership(graph, true))
	assert.Empty(t, client.graphMembership)
}

func TestEarlyGraphIntegrityFailureStillFinalizesRetainedState(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	client := newTestProtocolClient(t, testConfig(server.URL, "none"))
	serviceKey := resourceKey(client.origin, "service", "/redfish/v1/")
	rel := graphRelationship{
		ParentKind: "service",
		Path:       "Retained",
		ChildKind:  "fan",
		Family:     "thermal",
		Mode:       relationshipComponents,
	}
	retained := &graphNode{
		Kind:             "fan",
		URI:              "/redfish/v1/Chassis/1/Fans/retained",
		Locator:          "/redfish/v1/Chassis/1/Fans/retained",
		Key:              "retained",
		IdentityQuality:  "addressable",
		AcquisitionState: "readable",
		Parents:          make(map[string]*graphNode),
	}
	client.graphMembership[graphMembershipKey(serviceKey, rel)] = graphMembershipSnapshot{
		ParentKey:    serviceKey,
		Relationship: rel,
		Members:      snapshotGraphMembers([]*graphNode{retained}),
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
	}
	graph, err := client.collectResourceGraph(context.Background(), root, []baseResource{{
		Kind: "system", URI: baseURI, AcquisitionState: "readable",
	}}, nil)
	require.ErrorIs(t, err, errIdentityIntegrity)
	require.False(t, graph.Complete)
	restored := graph.findKey(retained.Key)
	require.NotNil(t, restored)
	assert.Equal(t, "unknown", restored.AcquisitionState)
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
			}, "resource")
			require.Error(t, err)
		})
	}
}

func graphTestNode(kind, key, uri string, links map[string]any) *graphNode {
	if links == nil {
		links = make(map[string]any)
	}
	return &graphNode{
		Kind:    kind,
		Key:     key,
		URI:     uri,
		Locator: uri,
		Data:    map[string]any{"Links": links},
		Parents: make(map[string]*graphNode),
	}
}

func TestGraphFetchBrokerCoalescesCanonicalResource(t *testing.T) {
	t.Parallel()

	var requests atomic.Int64
	server := newResourceTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		writeJSON(w, map[string]any{
			"@odata.id":   r.URL.Path,
			"@odata.type": "#Fan.v1_0_0.Fan",
			"Id":          "1",
			"Name":        "Fan",
		})
	}))
	defer server.Close()
	client := newTestResourceClient(t, testConfig(server.URL, "none"))
	ctx := withGraphFetchBroker(context.Background())
	rel := graphRelationship{
		ChildKind: "fan",
		Source:    "resource",
	}

	first, err := client.fetchGraphNode(
		ctx,
		"fan",
		"/redfish/v1/Chassis/1/Fans/1",
		rel,
		nil,
	)
	require.NoError(t, err)
	second, err := client.fetchGraphNode(
		ctx,
		"fan",
		server.URL+"/redfish/v1/Chassis/1/Fans/1",
		rel,
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

	server := newResourceTestServer(http.NotFoundHandler())
	defer server.Close()
	client := newTestResourceClient(t, testConfig(server.URL, "none"))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cancel()
	members := []collectionMember{
		{Ref: redfishLink{
			ODataID: "/redfish/v1/Chassis/1/Fans/1",
		}},
		{Ref: redfishLink{
			ODataID: "/redfish/v1/Chassis/1/Fans/2",
		}},
	}

	nodes, membershipComplete, err := client.fetchGraphCollectionMembers(
		ctx,
		graphCollectionRequest{
			relationship: graphRelationship{
				ChildKind: "fan",
				Source:    "resource",
			},
			members:            members,
			membershipComplete: true,
		},
		nil,
	)
	require.Error(t, err)
	assert.True(t, membershipComplete)
	assert.Len(t, nodes, 2)
}

func (g *resourceGraph) findKey(key string) *graphNode {
	for _, node := range g.Nodes {
		if node.Key == key {
			return node
		}
	}
	return nil
}

func TestEmbeddedLocatorSeparatesModesAndTupleBoundaries(t *testing.T) {
	type location struct {
		container, path, id string
		index               int
	}
	for _, test := range []struct {
		name        string
		left, right location
	}{
		{"id versus position", location{"/redfish/v1/C", "Members", "position:0", 1}, location{"/redfish/v1/C", "Members", "", 0}},
		{"id versus singleton", location{"/redfish/v1/C", "Members", "singleton", 0}, location{"/redfish/v1/C", "Members", "", -1}},
		{"container boundary", location{"/redfish/v1/C:Members", "Members", "one", 0}, location{"/redfish/v1/C", "Members", "Members:one", 0}},
		{"path boundary", location{"/redfish/v1/C", "Members:Sub", "one", 0}, location{"/redfish/v1/C", "Members", "Sub:one", 0}},
	} {
		t.Run(test.name, func(t *testing.T) {
			left, right := test.left, test.right
			assert.NotEqual(
				t,
				embeddedLocator(left.container, left.path, left.id, left.index),
				embeddedLocator(right.container, right.path, right.id, right.index),
			)
		})
	}
}
