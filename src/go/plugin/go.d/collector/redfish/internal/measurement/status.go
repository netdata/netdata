// SPDX-License-Identifier: GPL-3.0-or-later

package measurement

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/internal/identity"
)

type Status struct {
	Health       string      `json:"Health"`
	HealthRollup string      `json:"HealthRollup"`
	State        string      `json:"State"`
	Conditions   []Condition `json:"Conditions"`
}

type Condition struct {
	Message           string          `json:"Message"`
	MessageID         string          `json:"MessageId"`
	MessageArgs       []string        `json:"MessageArgs"`
	Severity          json.RawMessage `json:"Severity"`
	Timestamp         string          `json:"Timestamp"`
	OriginOfCondition Link            `json:"OriginOfCondition"`
}

type Document struct {
	ODataID          string `json:"@odata.id"`
	ID               string `json:"Id"`
	Name             string `json:"Name"`
	Status           Status `json:"Status"`
	PowerState       string `json:"PowerState"`
	FailurePredicted *bool  `json:"FailurePredicted"`
}

type conditionCounts struct {
	OK       int
	Warning  int
	Critical int
	Unknown  int
}

func conditionCountsFrom(conditions []Condition) conditionCounts {
	var counts conditionCounts
	seen := make(map[string]struct{}, len(conditions))
	for _, condition := range conditions {
		severity, present := normalizedConditionSeverity(condition.Severity)
		if !present {
			continue
		}
		args, _ := json.Marshal(condition.MessageArgs)
		key := identity.Tuple(
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
