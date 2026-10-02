// SPDX-License-Identifier: GPL-3.0-or-later

package acquisition

import (
	"errors"
	"net/url"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/internal/measurement"
)

// resourceDisplayIdentity is the only document data retained across failed reads.
// Health, measurements, and response timestamps always belong to the current cycle.
type resourceDisplayIdentity struct{ ODataID, ID, Name string }

func snapshotResourceDisplay(doc measurement.Document) resourceDisplayIdentity {
	return resourceDisplayIdentity{
		ODataID: doc.ODataID,
		ID:      doc.ID,
		Name:    doc.Name,
	}
}

func (identity resourceDisplayIdentity) document() measurement.Document {
	return measurement.Document{
		ODataID: identity.ODataID,
		ID:      identity.ID,
		Name:    identity.Name,
	}
}

// validateResourceData owns addressable resource contracts independent of how
// the representation was acquired. The caller owns response accounting.
func (c *Client) validateResourceData(
	kind string,
	data map[string]any,
	responseURL *url.URL,
) (measurement.Document, error) {
	if err := c.validateResourceIdentity(kind, data, responseURL); err != nil {
		return measurement.Document{}, err
	}
	return decodeGenericResource(data)
}

func (c *Client) validateResourceIdentity(kind string, data map[string]any, responseURL *url.URL) error {
	rawType, _ := measurement.Properties(data).Text("@odata.type")
	if err := validateResourceSchemaType(kind, rawType); err != nil {
		return err
	}
	id, ok := measurement.Properties(data).Text("@odata.id")
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
