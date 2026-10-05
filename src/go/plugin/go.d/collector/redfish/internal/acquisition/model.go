// SPDX-License-Identifier: GPL-3.0-or-later

package acquisition

import (
	"encoding/json"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/internal/measurement"
)

type redfishLink struct {
	ODataID string `json:"@odata.id"`
}

type serviceRootDocument struct {
	Raw      map[string]any
	Response measurement.ResponseTiming
}

type collectionMember struct{ Ref redfishLink }

// resourceDecodeError marks malformed optional typed properties. Base resources
// reject it; descendant and embedded adapters retain the partial document.
type resourceDecodeError struct{ cause error }

func (err *resourceDecodeError) Error() string { return err.cause.Error() }
func (err *resourceDecodeError) Unwrap() error { return err.cause }

// decodeGenericResource projects only the generic envelope before conversion.
// encoding/json retains its case-insensitive, null, and partial-decode behavior;
// unrelated source fields and potentially large OEM payloads are not serialized.
func decodeGenericResource(data map[string]any) (measurement.Document, error) {
	envelope := make(map[string]any, 6)
	for key, value := range data {
		for _, property := range []string{"@odata.id", "Id", "Name", "Status", "PowerState", "FailurePredicted"} {
			if strings.EqualFold(key, property) {
				envelope[key] = value
				break
			}
		}
	}
	var doc measurement.Document
	raw, err := json.Marshal(envelope)
	if err == nil {
		err = json.Unmarshal(raw, &doc)
	}
	if err != nil {
		return doc, &resourceDecodeError{
			cause: err,
		}
	}
	return doc, nil
}
