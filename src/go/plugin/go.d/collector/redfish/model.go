// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/stmcginnis/gofish"
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
		ClientContextQuery   *bool `json:"ClientContextQuery"`
		ExpandQuery          struct {
			ExpandAll bool `json:"ExpandAll"`
			Levels    bool `json:"Levels"`
			Links     bool `json:"Links"`
			MaxLevels uint `json:"MaxLevels"`
			NoLinks   bool `json:"NoLinks"`
		} `json:"ExpandQuery"`
	} `json:"ProtocolFeaturesSupported"`

	Typed    *gofish.Service  `json:"-"`
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
	Raw      []byte
	Response responseMetadata
}

type collectionProgress struct {
	NextURL            string
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
	ODataType        string        `json:"@odata.type"`
	ID               string        `json:"Id"`
	Name             string        `json:"Name"`
	Description      string        `json:"Description"`
	Status           genericStatus `json:"Status"`
	PowerState       string        `json:"PowerState"`
	FailurePredicted *bool         `json:"FailurePredicted"`
	Manufacturer     string        `json:"Manufacturer"`
	Model            string        `json:"Model"`
	PartNumber       string        `json:"PartNumber"`
	SerialNumber     string        `json:"SerialNumber"`
	AssetTag         string        `json:"AssetTag"`
	SKU              string        `json:"SKU"`
}

type inventoryHost struct {
	uri  string
	key  string
	name string
}

func (c *protocolClient) inventoryResourceRow(
	node *graphNode,
	observedAt time.Time,
	host inventoryHost,
) map[string]any {
	kind, key, uri := node.Kind, node.Key, node.URI
	id, name, schemaType := node.Doc.ID, node.Doc.Name, node.Doc.ODataType
	status, powerState := node.Doc.Status, node.Doc.PowerState
	failurePredicted := node.Doc.FailurePredicted
	conditions := conditionCountsFrom(status.Conditions)
	complete := node.Complete
	hostURI, hostKey, hostName := host.uri, host.key, host.name
	severity, rank := resourceSeverity(status, failurePredicted, conditions, complete)
	if name == "" {
		name = id
	}
	if hostName == "" {
		hostName = hostURI
	}
	rowKey := stableKey("netdata:redfish:inventory-row:v1", "resource\x00"+key, 64)
	return map[string]any{
		"row_key":                  rowKey,
		"sort_key":                 fmt.Sprintf("%d\x00%s\x00%s\x00%s", rank, kind, strings.ToLower(name), key),
		"row_type":                 "resource",
		"severity":                 severity,
		"severity_rank":            rank,
		"observed_at":              observedAt.UnixMilli(),
		"membership_complete":      complete,
		"acquisition_state":        "readable",
		"endpoint_key":             stableKey("netdata:redfish:endpoint:v1", mustOrigin(c.config.URL), endpointKeyHexChars),
		"host_uri":                 hostURI,
		"host_key":                 hostKey,
		"host_name":                hostName,
		"resource_kind":            kind,
		"resource_key":             key,
		"id":                       emptyToNil(id),
		"name":                     emptyToNil(name),
		"resource_uri":             uri,
		"identity_quality":         "addressable",
		"health":                   emptyToNil(status.Health),
		"health_rollup":            emptyToNil(status.HealthRollup),
		"state":                    emptyToNil(status.State),
		"power_state":              emptyToNil(powerState),
		"failure_predicted":        failurePredicted,
		"condition_ok_count":       conditions.OK,
		"condition_warning_count":  conditions.Warning,
		"condition_critical_count": conditions.Critical,
		"condition_unknown_count":  conditions.Unknown,
		"component_family":         kind,
		"detail_gate":              "open",
		"detail_component_count":   1,
		"detail_component_cap":     dereferenceInt(c.config.Charts.MaxDetailedComponentsPerFamily),
		"source_schema_type":       emptyToNil(schemaType),
		"source_uris":              uri,
	}
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

func resourceSeverity(
	status genericStatus,
	failurePredicted *bool,
	conditions conditionCounts,
	complete bool,
) (string, int) {
	if conditions.Critical > 0 {
		return "critical", 0
	}
	if conditions.Warning > 0 {
		return "warning", 1
	}
	for _, value := range []string{status.Health, status.HealthRollup} {
		switch strings.ToLower(value) {
		case "critical":
			return "critical", 0
		case "warning":
			return "warning", 1
		}
	}
	if failurePredicted != nil && *failurePredicted {
		return "critical", 0
	}
	if !complete {
		return "warning", 1
	}
	return "normal", 2
}

func (c *protocolClient) selectedSystemState(resources []baseResource, complete bool) string {
	if c.config.SystemURI == "" {
		return ""
	}
	selected, _ := normalizeConfiguredResourceURI(c.root, c.config.SystemURI)
	for _, resource := range resources {
		if resource.Kind == "system" && resource.URI == selected {
			switch resource.AcquisitionState {
			case "readable":
				return "present"
			case "unreadable":
				return "unreadable"
			default:
				return "unknown"
			}
		}
	}
	if complete {
		return "absent"
	}
	return "unreadable"
}

func mustOrigin(raw string) string {
	_, origin, err := normalizeServiceRoot(raw)
	if err != nil {
		return ""
	}
	return origin
}

func emptyToNil(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func dereferenceInt(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}
