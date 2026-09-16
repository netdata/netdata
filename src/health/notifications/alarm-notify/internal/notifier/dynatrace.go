// SPDX-License-Identifier: GPL-3.0-or-later

package notifier

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

type dynatraceEvent struct {
	EventType      string            `json:"eventType"`
	Title          string            `json:"title"`
	EntitySelector string            `json:"entitySelector"`
	Properties     map[string]string `json:"properties"`
}

func renderDynatrace(dst Destination, event Event) (dynatraceEvent, error) {
	eventType, source := dst.EventType, dst.Source
	if eventType == "" {
		eventType = "CUSTOM_INFO"
	}
	if source == "" {
		source = "Netdata Alarm"
	}
	message := dynatraceEvent{
		EventType: eventType, Title: event.Node + " " + event.Status + ": " + event.Summary,
		EntitySelector: dst.EntitySelector,
		Properties: map[string]string{
			"dt.event.source": source, "dt.event.description": notificationPlainText(event, true),
			"netdata.incident_id": event.IncidentID, "netdata.timestamp": event.Timestamp.Format(time.RFC3339Nano),
		},
	}
	for _, value := range message.Properties {
		if utf8.RuneCountInString(value) > 4096 {
			return dynatraceEvent{}, errors.New("dynatrace event property exceeds 4096 characters")
		}
	}
	return message, nil
}

func sendDynatrace(ctx context.Context, dst Destination, event Event, timeout time.Duration) error {
	if err := dst.resolveMonitoring(ctx); err != nil {
		return err
	}
	message, err := renderDynatrace(dst, event)
	if err != nil {
		return err
	}
	client := notificationHTTPClient(timeout)
	defer client.CloseIdleConnections()
	response, err := postNotificationJSON(
		ctx,
		client,
		"dynatrace",
		strings.TrimRight(dst.APIURL, "/")+"/api/v2/events/ingest",
		http.Header{"Authorization": {"Api-Token " + dst.APIToken}},
		message,
	)
	if err != nil {
		return err
	}
	return readDynatraceResponse(response)
}

func readDynatraceResponse(response *http.Response) error {
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		return fmt.Errorf("dynatrace returned HTTP %d", response.StatusCode)
	}
	var result struct {
		ReportCount int `json:"reportCount"`
		Results     []struct {
			Status        string `json:"status"`
			CorrelationID string `json:"correlationId"`
		} `json:"eventIngestResults"`
	}
	if err := decodeNotificationResponse("dynatrace", response.Body, &result); err != nil {
		return err
	}
	if result.ReportCount <= 0 || result.ReportCount != len(result.Results) {
		return errors.New("dynatrace did not acknowledge any complete event result set")
	}
	for _, event := range result.Results {
		if event.Status != "OK" || strings.TrimSpace(event.CorrelationID) == "" {
			return errors.New("dynatrace did not acknowledge every event")
		}
	}
	return nil
}
