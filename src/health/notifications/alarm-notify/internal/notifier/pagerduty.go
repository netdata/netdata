// SPDX-License-Identifier: GPL-3.0-or-later

package notifier

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"time"
	"unicode/utf8"

	notifyevent "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/event"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/httpclient"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/secret"
)

const (
	pagerDutyDefaultAPI   = "https://events.pagerduty.com"
	pagerDutyV1Path       = "/generic/2010-04-15/create_event.json"
	pagerDutyV2Path       = "/v2/enqueue"
	pagerDutyRequestLimit = 512 * 1024
)

func (dst Destination) pagerDutyVersion() int64 {
	if dst.APIVersion == nil {
		return 1
	}
	return int64(*dst.APIVersion)
}

func (dst Destination) validatePagerDuty() error {
	allowed := Destination{
		Type:           dst.Type,
		IntegrationKey: dst.IntegrationKey,
		APIURL:         dst.APIURL,
		APIVersion:     dst.APIVersion,
	}
	if !reflect.DeepEqual(dst, allowed) {
		return errors.New("pagerduty destination contains fields for another provider")
	}
	if version := dst.pagerDutyVersion(); version != 1 && version != 2 {
		return errors.New("pagerduty api_version must be 1 or 2")
	}
	for _, field := range []struct{ name, value string }{{"api_url", dst.APIURL}, {"integration_key", dst.IntegrationKey}} {
		reference, err := secret.IsReference(field.value)
		if err != nil {
			return fmt.Errorf("pagerduty %s: %w", field.name, err)
		}
		if !reference {
			if err := validatePagerDutyField(dst.pagerDutyVersion(), field.name, field.value); err != nil {
				return err
			}
		}
	}
	return nil
}

func validatePagerDutyField(version int64, name, value string) error {
	if name == "api_url" {
		for _, host := range []string{"events.pagerduty.com", "events.eu.pagerduty.com"} {
			if err := validateAPIBase(value, "pagerduty", host); err != nil {
				return err
			}
		}
		return nil
	}
	if err := validateToken(value, "pagerduty", name); err != nil {
		return err
	}
	if len(value) != 32 {
		return errors.New("pagerduty integration_key must be 32 characters")
	}
	if version == 1 {
		if _, err := hex.DecodeString(value); err != nil {
			return errors.New("pagerduty v1 integration_key must be hexadecimal")
		}
	}
	return nil
}

type pagerDutyV1Event struct {
	ServiceKey  string            `json:"service_key"`
	EventType   string            `json:"event_type"`
	IncidentKey string            `json:"incident_key"`
	Description string            `json:"description"`
	Details     notifyevent.Event `json:"details"`
	Client      string            `json:"client,omitempty"`
	ClientURL   string            `json:"client_url,omitempty"`
}

type pagerDutyV2Event struct {
	RoutingKey  string           `json:"routing_key"`
	EventAction string           `json:"event_action"`
	DedupKey    string           `json:"dedup_key"`
	Payload     pagerDutyPayload `json:"payload"`
	Links       []pagerDutyLink  `json:"links,omitempty"`
}

type pagerDutyPayload struct {
	Summary       string            `json:"summary"`
	Source        string            `json:"source"`
	Severity      string            `json:"severity"`
	Timestamp     time.Time         `json:"timestamp"`
	Class         string            `json:"class,omitempty"`
	CustomDetails notifyevent.Event `json:"custom_details"`
}

type pagerDutyLink struct {
	Href string `json:"href"`
	Text string `json:"text"`
}

func pagerDutyIncidentKey(id string) string {
	// Keep opaque IDs distinct without truncation, within both APIs' 255-character bound.
	return fmt.Sprintf("%x", sha256.Sum256([]byte(id)))
}

func renderPagerDuty(dst Destination, event notifyevent.Event) any {
	action, severity := "trigger", "warning"
	if event.Status == "CRITICAL" {
		severity = "critical"
	}
	if event.Status == "CLEAR" {
		action, severity = "resolve", "info"
	}
	title := event.Node + " " + event.Status + ": " + event.Summary
	if utf8.RuneCountInString(title) > 1024 {
		title = string([]rune(title)[:1021]) + "..."
	}
	key := pagerDutyIncidentKey(event.IncidentID)
	if dst.pagerDutyVersion() == 1 {
		message := pagerDutyV1Event{
			ServiceKey:  dst.IntegrationKey,
			EventType:   action,
			IncidentKey: key,
			Description: title,
			Details:     event,
		}
		if action == "trigger" {
			message.Client, message.ClientURL = "Netdata", event.URL
		}
		return message
	}
	message := pagerDutyV2Event{
		RoutingKey: dst.IntegrationKey, EventAction: action, DedupKey: key,
		Payload: pagerDutyPayload{
			Summary:       title,
			Source:        event.Node,
			Severity:      severity,
			Timestamp:     event.Timestamp,
			Class:         event.Chart,
			CustomDetails: event,
		},
	}
	// Retain payload fields on resolve for integrations whose routing rules inspect them.
	if event.URL != "" {
		message.Links = []pagerDutyLink{{Href: event.URL, Text: "View alert"}}
	}
	return message
}

func sendPagerDuty(ctx context.Context, dst Destination, event notifyevent.Event, timeout time.Duration) error {
	version := dst.pagerDutyVersion()
	for _, field := range []struct {
		name  string
		value *string
	}{{"api_url", &dst.APIURL}, {"integration_key", &dst.IntegrationKey}} {
		value, err := secret.Resolve(ctx, *field.value)
		if err != nil {
			return fmt.Errorf("pagerduty %s: %w", field.name, err)
		}
		if err := validatePagerDutyField(version, field.name, value); err != nil {
			return err
		}
		*field.value = value
	}
	data, err := json.Marshal(renderPagerDuty(dst, event))
	if err != nil {
		return errors.New("could not encode pagerduty notification")
	}
	if len(data) > pagerDutyRequestLimit {
		return errors.New("pagerduty payload exceeds the 512 KiB limit")
	}
	base := dst.APIURL
	if base == "" {
		base = pagerDutyDefaultAPI
	}
	path := pagerDutyV1Path
	if version == 2 {
		path = pagerDutyV2Path
	}
	client := httpclient.New(timeout)
	defer client.CloseIdleConnections()
	response, err := httpclient.Post(ctx, client, "pagerduty", strings.TrimRight(base, "/")+path, "application/json",
		http.Header{"Accept": {"application/json"}}, bytes.NewReader(data))
	if err != nil {
		return err
	}
	return readPagerDutyResponse(response, version, pagerDutyIncidentKey(event.IncidentID))
}

func readPagerDutyResponse(response *http.Response, version int64, key string) error {
	defer response.Body.Close()
	status := http.StatusOK
	if version == 2 {
		status = http.StatusAccepted
	}
	if response.StatusCode != status {
		return fmt.Errorf("pagerduty returned HTTP %d", response.StatusCode)
	}
	var result struct {
		Status      string `json:"status"`
		IncidentKey string `json:"incident_key"`
		DedupKey    string `json:"dedup_key"`
	}
	if err := httpclient.DecodeResponse("pagerduty", response.Body, &result); err != nil {
		return err
	}
	ackKey := result.IncidentKey
	if version == 2 {
		ackKey = result.DedupKey
	}
	if result.Status != "success" || ackKey != key {
		return errors.New("invalid pagerduty acknowledgment")
	}
	return nil
}
