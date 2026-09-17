// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"errors"
	"net/url"
	"strings"
)

// resourceDisplayIdentity is the only document data retained across failed reads.
// Health, measurements, and response timestamps always belong to the current cycle.
type resourceDisplayIdentity struct{ ODataID, ID, Name string }

func snapshotResourceDisplay(doc genericResource) resourceDisplayIdentity {
	return resourceDisplayIdentity{
		ODataID: doc.ODataID,
		ID:      doc.ID,
		Name:    doc.Name,
	}
}

func (identity resourceDisplayIdentity) document() genericResource {
	return genericResource{
		ODataID: identity.ODataID,
		ID:      identity.ID,
		Name:    identity.Name,
	}
}

// validateResourceData owns addressable resource contracts independent of how
// the representation was acquired. The caller owns response accounting.
func (c *protocolClient) validateResourceData(
	kind string,
	data map[string]any,
	responseURL *url.URL,
) (genericResource, error) {
	if err := c.validateResourceIdentity(kind, data, responseURL); err != nil {
		return genericResource{}, err
	}
	return decodeGenericResource(data)
}

func (c *protocolClient) validateResourceIdentity(kind string, data map[string]any, responseURL *url.URL) error {
	rawType, _ := stringValue(data["@odata.type"])
	if err := validateResourceSchemaType(kind, rawType); err != nil {
		return err
	}
	id, ok := stringValue(data["@odata.id"])
	if !ok {
		return errors.New("resource has no usable @odata.id")
	}
	resolved, err := c.resolveURI(responseURL, id, false)
	if err != nil || !sameResourceIdentity(canonicalResourceURI(resolved), canonicalResourceURI(responseURL)) {
		return errors.New("resource identity does not match final response URI")
	}
	return nil
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
