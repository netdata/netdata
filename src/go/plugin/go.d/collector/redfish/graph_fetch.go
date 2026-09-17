// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

type graphCollectionRequest struct {
	relationship       graphRelationship
	members            []collectionMember
	membershipComplete bool
	pageErr            error
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
			return cloneGraphNode(entry.node), entry.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	entry := &graphFetchEntry{
		done: make(chan struct{}),
	}
	b.entries[key] = entry
	b.mu.Unlock()

	entry.node, entry.err = fetch()
	b.mu.Lock()
	close(entry.done)
	b.mu.Unlock()
	return cloneGraphNode(entry.node), entry.err
}

// Fetched JSON documents are immutable during a collection cycle.
// Sharing them preserves request coalescing without duplicating the largest
// part of every cached graph node.
func cloneGraphNode(source *graphNode) *graphNode {
	if source == nil {
		return nil
	}
	node := *source
	node.Doc.Status.Conditions = append([]genericCondition(nil), source.Doc.Status.Conditions...)
	node.Enrichment = make(map[string]enrichmentResource, len(source.Enrichment))
	for key, value := range source.Enrichment {
		value.Data = cloneJSONMap(value.Data)
		node.Enrichment[key] = value
	}
	node.Parents = make(map[string]*graphNode)
	node.SensorExcerpts = cloneSensorExcerptSources(source.SensorExcerpts)
	return &node
}

func (c *protocolClient) acquireLinkedValues(
	ctx context.Context,
	rel graphRelationship,
	value any,
	stats *wireStats,
) ([]*graphNode, bool, error) {
	switch typed := value.(type) {
	case map[string]any:
		if rawURI, ok := stringValue(typed["@odata.id"]); ok {
			return c.acquireLinkedURI(ctx, rel, rawURI, stats)
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
				children, ok, err := c.acquireLinkedURI(ctx, rel, rawURI, stats)
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
	rel graphRelationship,
	rawURI string,
	stats *wireStats,
) ([]*graphNode, bool, error) {
	target, err := c.resolveURI(c.root, rawURI, false)
	if err != nil {
		return nil, false, err
	}
	response, err := c.get(ctx, target, stats)
	if err != nil {
		return nil, false, err
	}
	var data map[string]any
	if err := decodeJSON(response, &data); err != nil {
		response.finish(err)
		return nil, false, err
	}
	if _, present := data["Members"]; present {
		collectionMembers, complete, pageErr := c.fetchCollectionMemberPages(
			ctx,
			target,
			response,
			stats,
		)
		return c.fetchGraphCollectionMembers(
			ctx,
			graphCollectionRequest{
				relationship:       rel,
				members:            collectionMembers,
				membershipComplete: complete,
				pageErr:            pageErr,
			},
			stats,
		)
	}
	node, err := c.graphNodeFromData(rel.ChildKind, response, data, rel.Source)
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
	workers := min(max(c.requestLimit, 1), len(members))
	var wait sync.WaitGroup
	for range workers {
		wait.Go(func() {
			for index := range jobs {
				itemStats := &wireStats{
					failures: make(map[string]int),
				}
				node, fetchErr := c.fetchGraphNode(
					ctx,
					request.relationship.ChildKind,
					members[index].Ref.ODataID,
					request.relationship,
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
				node = c.unavailableGraphNode(request.relationship.ChildKind, member.Ref.ODataID, "unreadable")
			} else {
				node = c.unavailableGraphNode(request.relationship.ChildKind, member.Ref.ODataID, "unknown")
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

func (c *protocolClient) unavailableGraphNode(kind, uri, state string) *graphNode {
	node := &graphNode{
		Kind:             kind,
		URI:              uri,
		Locator:          uri,
		AcquisitionState: state,
		IdentityQuality:  "addressable",
		SourceModel:      "resource",
		Parents:          make(map[string]*graphNode),
	}
	node.Key = resourceKey(c.origin, kind, uri)
	return node
}

func (c *protocolClient) fetchGraphNode(
	ctx context.Context,
	kind, uri string,
	rel graphRelationship,
	stats *wireStats,
) (*graphNode, error) {
	target, err := c.resolveURI(c.root, uri, false)
	if err != nil {
		return nil, err
	}
	key := kind + "\x00" + canonicalResourceURI(target)
	return graphFetchBrokerFrom(ctx).fetch(ctx, key, func() (*graphNode, error) {
		response, err := c.get(ctx, target, stats)
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
	return c.graphNodeFromData(kind, response, data, source)
}

func (c *protocolClient) graphNodeFromData(
	kind string,
	response *responseData,
	data map[string]any,
	source string,
) (*graphNode, error) {
	doc, err := c.validateResourceData(kind, data, response.url)
	var decodeErr *resourceDecodeError
	if err != nil && !errors.As(err, &decodeErr) {
		response.finish(err)
		return nil, err
	}
	// Descendants retain valid optional fields when another typed field is malformed.
	uri := canonicalResourceURI(response.url)
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
		IdentityQuality:  "addressable",
		SourceModel:      model,
		Response:         metadataForResponse(response),
		Parents:          make(map[string]*graphNode),
	}
	node.Key = resourceKey(c.origin, kind, uri)
	response.finish(nil)
	return node, nil
}
