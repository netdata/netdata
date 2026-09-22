// SPDX-License-Identifier: GPL-3.0-or-later

package acquisition

import (
	"context"
	"errors"
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/internal/measurement"
)

func (c *Client) fetchServiceRoot(
	ctx context.Context,
	stats *wireStats,
) (document *serviceRootDocument, err error) {
	response, err := c.get(ctx, c.root, stats)
	if err != nil {
		return nil, err
	}
	defer func() { response.finish(err) }()
	root, err := c.decodeServiceRoot(response)
	if err != nil {
		return nil, err
	}
	c.requestLimit = c.config.MaxConcurrentRequests
	if multiple, ok := measurement.Properties(root.Raw).Lookup("ProtocolFeaturesSupported.MultipleHTTPRequests"); ok &&
		multiple == false {
		c.requestLimit = 1
	}
	return root, nil
}

func (c *Client) decodeServiceRoot(response *responseData) (*serviceRootDocument, error) {
	var root serviceRootDocument
	if err := decodeJSON(response, &root.Raw); err != nil {
		return nil, err
	}
	if err := c.validateResourceIdentity("service", root.Raw, response.url); err != nil {
		return nil, err
	}
	if stringAt(root.Raw, "RedfishVersion") == "" {
		return nil, errors.New("ServiceRoot has no RedfishVersion")
	}
	root.Response = metadataForResponse(response)
	return &root, nil
}

type baseResource struct {
	Kind             string
	URI              string
	Doc              measurement.Document
	Data             map[string]any
	Response         measurement.ResponseTiming
	AcquisitionState string
}

func (c *Client) fetchBaseResources(
	ctx context.Context,
	root *serviceRootDocument,
	stats *wireStats,
) ([]baseResource, bool, error) {
	collections := []struct {
		kind     string
		property string
	}{
		{"system", "Systems"},
		{"chassis", "Chassis"},
		{"manager", "Managers"},
	}
	var resources []baseResource
	complete := true
	var joined error
	for _, collection := range collections {
		if root.Raw[collection.property] == nil {
			c.replaceBaseMembership(collection.kind, nil)
			continue
		}
		var members []collectionMember
		var ok bool
		var err error
		if ref := linkAt(root.Raw, collection.property); ref != "" {
			members, ok, err = c.fetchCollectionMembers(ctx, ref, stats)
		} else {
			err = fmt.Errorf("%s link has no usable @odata.id", collection.property)
		}
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

func (c *Client) fetchBaseMembers(
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
		resource, err := c.fetchBaseResource(ctx, kind, member.Ref, stats)
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

func (c *Client) fetchBaseResource(
	ctx context.Context,
	kind string,
	ref redfishLink,
	stats *wireStats,
) (resource baseResource, err error) {
	target, err := c.resolveURI(c.root, ref.ODataID, false)
	if err != nil {
		return baseResource{}, err
	}
	response, err := c.get(ctx, target, stats)
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

func (c *Client) replaceBaseMembership(kind string, resources []baseResource) {
	c.baseMu.Lock()
	c.baseMembership[kind] = snapshotBaseResources(resources)
	c.baseMu.Unlock()
}

func (c *Client) incompleteBaseMembership(kind string, current []baseResource) []baseResource {
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

func (c *Client) unreadableBaseResource(kind, uri string) baseResource {
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
