// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"bytes"
	"encoding/json"
	"strings"
)

type redfishLink struct {
	ODataID string `json:"@odata.id"`
}

type serviceRootDocument struct {
	ODataID        string `json:"@odata.id"`
	ODataType      string `json:"@odata.type"`
	ID             string `json:"Id"`
	Name           string `json:"Name"`
	RedfishVersion string `json:"RedfishVersion"`
	UUID           string `json:"UUID"`
	Vendor         string `json:"Vendor"`
	Product        string `json:"Product"`

	Systems        redfishLink `json:"Systems"`
	Chassis        redfishLink `json:"Chassis"`
	Managers       redfishLink `json:"Managers"`
	Storage        redfishLink `json:"Storage"`
	SessionService redfishLink `json:"SessionService"`
	UpdateService  redfishLink `json:"UpdateService"`
	Links          struct {
		Sessions redfishLink `json:"Sessions"`
	} `json:"Links"`
	ProtocolFeaturesSupported struct {
		MultipleHTTPRequests *bool `json:"MultipleHTTPRequests"`
		ExpandQuery          struct {
			ExpandAll bool `json:"ExpandAll"`
			Levels    bool `json:"Levels"`
			Links     bool `json:"Links"`
			MaxLevels uint `json:"MaxLevels"`
			NoLinks   bool `json:"NoLinks"`
		} `json:"ExpandQuery"`
	} `json:"ProtocolFeaturesSupported"`

	Raw      map[string]any   `json:"-"`
	Response responseMetadata `json:"-"`
}

type collectionPage struct {
	ODataID   string          `json:"@odata.id"`
	ODataType string          `json:"@odata.type"`
	Count     *int            `json:"Members@odata.count"`
	Members   json.RawMessage `json:"Members"`
	NextLink  string          `json:"Members@odata.nextLink"`
}

type collectionMember struct {
	Ref      redfishLink
	Data     map[string]any
	Response responseMetadata
}

type collectionProgress struct {
	CollectionIdentity string
	ExpectedCount      int
	Members            []collectionMember
	SeenPages          map[string]struct{}
	SeenMembers        map[string]struct{}
	InvalidMembers     int
	FirstMemberError   string
}

type genericStatus struct {
	Health       string             `json:"Health"`
	HealthRollup string             `json:"HealthRollup"`
	State        string             `json:"State"`
	Conditions   []genericCondition `json:"Conditions"`
}

type genericCondition struct {
	Message           string          `json:"Message"`
	MessageID         string          `json:"MessageId"`
	MessageArgs       []string        `json:"MessageArgs"`
	Severity          json.RawMessage `json:"Severity"`
	Timestamp         string          `json:"Timestamp"`
	OriginOfCondition redfishLink     `json:"OriginOfCondition"`
}

type genericResource struct {
	ODataID          string        `json:"@odata.id"`
	ID               string        `json:"Id"`
	Name             string        `json:"Name"`
	Status           genericStatus `json:"Status"`
	PowerState       string        `json:"PowerState"`
	FailurePredicted *bool         `json:"FailurePredicted"`
}

type conditionCounts struct {
	OK       int
	Warning  int
	Critical int
	Unknown  int
}

func conditionCountsFrom(conditions []genericCondition) conditionCounts {
	var counts conditionCounts
	seen := make(map[string]struct{}, len(conditions))
	for _, condition := range conditions {
		severity, present := normalizedConditionSeverity(condition.Severity)
		if !present {
			continue
		}
		args, _ := json.Marshal(condition.MessageArgs)
		key := structuralTuple(
			condition.MessageID,
			string(args),
			condition.OriginOfCondition.ODataID,
			condition.Timestamp,
			string(bytes.TrimSpace(condition.Severity)),
		)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		switch severity {
		case "ok":
			counts.OK++
		case "warning":
			counts.Warning++
		case "critical":
			counts.Critical++
		default:
			counts.Unknown++
		}
	}
	return counts
}

func normalizedConditionSeverity(raw json.RawMessage) (string, bool) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return "", false
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "unknown", true
	}
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "ok":
		return "ok", true
	case "warning":
		return "warning", true
	case "critical":
		return "critical", true
	default:
		return "unknown", true
	}
}

func dereferenceInt(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

// resourceDecodeError marks malformed optional typed properties. Base resources
// reject it; descendant and embedded adapters retain the partial document.
type resourceDecodeError struct{ cause error }

func (err *resourceDecodeError) Error() string { return err.cause.Error() }
func (err *resourceDecodeError) Unwrap() error { return err.cause }

// decodeGenericResource projects only the generic envelope before conversion.
// encoding/json retains its case-insensitive, null, and partial-decode behavior;
// unrelated source fields and potentially large OEM payloads are not serialized.
func decodeGenericResource(data map[string]any) (genericResource, error) {
	envelope := make(map[string]any, 6)
	for key, value := range data {
		for _, property := range []string{"@odata.id", "Id", "Name", "Status", "PowerState", "FailurePredicted"} {
			if strings.EqualFold(key, property) {
				envelope[key] = value
				break
			}
		}
	}
	var doc genericResource
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
