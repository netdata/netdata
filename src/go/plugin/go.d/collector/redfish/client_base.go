// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"context"
	"errors"
	"fmt"
	"net/http"
)

func (c *protocolClient) fetchServiceRoot(
	ctx context.Context,
	authenticated bool,
	stats *wireStats,
) (document *serviceRootDocument, err error) {
	response, err := c.do(
		ctx,
		protocolRequest{
			method: http.MethodGet,
			target: c.root,
			auth:   c.currentAuth(authenticated),
		},
		stats,
		true,
		http.StatusOK,
	)
	if err != nil {
		return nil, err
	}
	defer func() { response.finish(err) }()
	var root serviceRootDocument
	if err := decodeJSON(response, &root); err != nil {
		return nil, fmt.Errorf("decode ServiceRoot: %w", err)
	}
	// The typed envelope owns protocol features; the raw document retains source fields for metrics.
	if err := decodeJSON(response, &root.Raw); err != nil {
		return nil, fmt.Errorf("decode raw ServiceRoot: %w", err)
	}
	if err := c.validateResourceIdentity("service", root.Raw, response.url); err != nil {
		return nil, fmt.Errorf("validate ServiceRoot: %w", err)
	}
	if !validRedfishVersion(root.RedfishVersion) {
		err := errors.New("ServiceRoot has no valid RedfishVersion")
		return nil, err
	}
	root.Response = metadataForResponse(response)
	if root.ProtocolFeaturesSupported.MultipleHTTPRequests != nil &&
		!*root.ProtocolFeaturesSupported.MultipleHTTPRequests {
		c.setRequestLimit(1)
	} else {
		c.setRequestLimit(c.config.MaxConcurrentRequests)
	}
	c.setExpansionValue(expansionValue(&root))
	return &root, nil
}

type baseResource struct {
	Kind             string
	URI              string
	Doc              genericResource
	Data             map[string]any
	Response         responseMetadata
	AcquisitionState string
}

func (c *protocolClient) fetchBaseResources(
	ctx context.Context,
	root *serviceRootDocument,
	stats *wireStats,
) ([]baseResource, bool, error) {
	collections := []struct {
		kind string
		link redfishLink
	}{
		{"system", root.Systems},
		{"chassis", root.Chassis},
		{"manager", root.Managers},
	}
	var resources []baseResource
	complete := true
	var joined error
	for _, collection := range collections {
		if collection.link.ODataID == "" {
			c.replaceBaseMembership(collection.kind, nil)
			continue
		}
		members, ok, err := c.fetchCollectionMembers(ctx, collection.link.ODataID, collection.kind, stats)
		if err != nil {
			complete = false
			joined = errors.Join(joined, fmt.Errorf("%s collection: %w", collection.kind, err))
			current, memberErr := c.fetchBaseMembers(
				ctx,
				collection.kind,
				members,
				stats,
			)
			joined = errors.Join(joined, memberErr)
			for _, resource := range c.incompleteBaseMembership(collection.kind, current) {
				resources = append(resources, resource)
			}
			continue
		}
		complete = complete && ok
		current, memberErr := c.fetchBaseMembers(
			ctx,
			collection.kind,
			members,
			stats,
		)
		if memberErr != nil {
			complete = false
			joined = errors.Join(joined, memberErr)
		}
		c.replaceBaseMembership(collection.kind, current)
		for _, resource := range current {
			resources = append(resources, resource)
		}
	}
	return resources, complete, joined
}

func (c *protocolClient) fetchBaseMembers(
	ctx context.Context,
	kind string,
	members []collectionMember,
	stats *wireStats,
) ([]baseResource, error) {
	current := make([]baseResource, len(members))
	var failures boundedErrorAccumulator
	completed := true
	for index := range members {
		if err := contextError(ctx); err != nil {
			failures.Add(err)
			completed = false
			break
		}
		member := members[index]
		resource, err := c.fetchBaseMember(ctx, kind, member, stats)
		if err != nil {
			failures.Add(fmt.Errorf("%s resource: %w", kind, err))
			resource = c.unreadableBaseResource(kind, member.Ref.ODataID)
			if isCallerContextError(err) {
				completed = false
			}
		}
		current[index] = resource
		if !completed {
			break
		}
	}
	if !completed {
		for index := range current {
			if current[index].URI != "" {
				continue
			}
			resource := c.unreadableBaseResource(
				kind,
				members[index].Ref.ODataID,
			)
			resource.AcquisitionState = "unknown"
			current[index] = resource
		}
	}
	return current, failures.Err()
}

func (c *protocolClient) fetchBaseMember(
	ctx context.Context,
	kind string,
	member collectionMember,
	stats *wireStats,
) (baseResource, error) {
	if member.Data == nil {
		return c.fetchBaseResource(ctx, kind, member.Ref, stats)
	}
	target, err := c.resolveURI(c.root, member.Ref.ODataID, false)
	if err != nil {
		return baseResource{}, err
	}
	doc, err := c.validateResourceData(kind, member.Data, target)
	if err != nil {
		return baseResource{}, err
	}
	return baseResource{
		Kind:             kind,
		URI:              member.Ref.ODataID,
		Doc:              doc,
		Data:             cloneJSONMap(member.Data),
		Response:         member.Response,
		AcquisitionState: "readable",
	}, nil
}

func (c *protocolClient) fetchBaseResource(
	ctx context.Context,
	kind string,
	ref redfishLink,
	stats *wireStats,
) (resource baseResource, err error) {
	target, err := c.resolveURI(c.root, ref.ODataID, false)
	if err != nil {
		return baseResource{}, err
	}
	response, err := c.do(
		ctx,
		protocolRequest{
			method: http.MethodGet,
			target: target,
			auth:   c.currentAuth(true),
		},
		stats,
		true,
		http.StatusOK,
	)
	if err != nil {
		return baseResource{}, err
	}
	defer func() { response.finish(err) }()
	var data map[string]any
	if err = decodeJSON(response, &data); err != nil {
		return baseResource{}, err
	}
	doc, err := c.validateResourceData(kind, data, response.url)
	if err != nil {
		return baseResource{}, err
	}
	return baseResource{
		Kind:             kind,
		URI:              canonicalResourceURI(response.url),
		Doc:              doc,
		Data:             data,
		Response:         metadataForResponse(response),
		AcquisitionState: "readable",
	}, nil
}

func (c *protocolClient) replaceBaseMembership(kind string, resources []baseResource) {
	c.baseMu.Lock()
	c.baseMembership[kind] = snapshotBaseResources(resources)
	c.baseMu.Unlock()
}

func (c *protocolClient) incompleteBaseMembership(kind string, current []baseResource) []baseResource {
	c.baseMu.Lock()
	defer c.baseMu.Unlock()
	resources := append([]baseResource(nil), current...)
	present := make(map[string]struct{}, len(current))
	for _, resource := range current {
		present[resource.URI] = struct{}{}
	}
	for _, retained := range c.baseMembership[kind] {
		if _, ok := present[retained.URI]; !ok {
			resources = append(resources, baseResource{
				Kind:             retained.Kind,
				URI:              retained.URI,
				Doc:              retained.Display.document(),
				AcquisitionState: "unknown",
			})
		}
	}
	c.baseMembership[kind] = snapshotBaseResources(resources)
	return resources
}

func (c *protocolClient) unreadableBaseResource(kind, uri string) baseResource {
	resource := baseResource{
		Kind:             kind,
		URI:              uri,
		AcquisitionState: "unreadable",
	}
	c.baseMu.Lock()
	defer c.baseMu.Unlock()
	for _, previous := range c.baseMembership[kind] {
		if previous.URI == uri {
			resource.Doc = previous.Display.document()
			break
		}
	}
	return resource
}

// Retain display identity explicitly so no measurement can survive a failed read.
type baseResourceIdentity struct {
	Kind    string
	URI     string
	Display resourceDisplayIdentity
}

func snapshotBaseResources(resources []baseResource) []baseResourceIdentity {
	result := make([]baseResourceIdentity, len(resources))
	for i, resource := range resources {
		result[i] = baseResourceIdentity{
			Kind:    resource.Kind,
			URI:     resource.URI,
			Display: snapshotResourceDisplay(resource.Doc),
		}
	}
	return result
}
