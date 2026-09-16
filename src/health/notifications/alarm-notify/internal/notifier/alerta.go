// SPDX-License-Identifier: GPL-3.0-or-later

package notifier

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/http"
	"strings"
	"time"
)

type alertaEvent struct {
	Resource    string            `json:"resource"`
	Event       string            `json:"event"`
	Environment string            `json:"environment"`
	Severity    string            `json:"severity"`
	Service     []string          `json:"service"`
	Group       string            `json:"group"`
	Value       string            `json:"value"`
	Text        string            `json:"text"`
	Tags        []string          `json:"tags"`
	Attributes  map[string]string `json:"attributes"`
	Origin      string            `json:"origin"`
	Type        string            `json:"type"`
	CreateTime  string            `json:"createTime"`
	RawData     string            `json:"rawData"`
}

func renderAlerta(dst Destination, event Event) (alertaEvent, error) {
	raw, err := json.Marshal(event)
	if err != nil {
		return alertaEvent{}, errors.New("cannot encode alerta event")
	}
	message := alertaEvent{
		Resource: event.Node, Event: event.Alert, Environment: dst.Environment,
		Severity: strings.ToLower(event.Status), Service: []string{"Netdata"}, Group: "Performance",
		Text: notificationPlainText(event, true), Tags: []string{"incident_id:" + event.IncidentID},
		Attributes: map[string]string{"name": event.Alert, "chart": event.Chart, "context": event.Context},
		Origin:     "netdata/" + event.Node, Type: "netdataAlarm",
		CreateTime: event.Timestamp.UTC().Format("2006-01-02T15:04:05.000Z"), RawData: string(raw),
	}
	if event.Status == "CLEAR" {
		message.Severity = "cleared"
	}
	if strings.HasPrefix(event.Chart, "httpcheck") {
		message.Resource = event.Chart
	} else if event.Chart != "" {
		message.Event = event.Chart + "." + event.Alert
	}
	for _, field := range notificationFields(event) {
		if field.name == "Value" {
			message.Value = field.value
		}
	}
	if event.URL != "" {
		message.Attributes["moreInfo"] = `<a href="` + html.EscapeString(event.URL) + `">View Netdata</a>`
	}
	return message, nil
}

func sendAlerta(ctx context.Context, dst Destination, event Event, timeout time.Duration) error {
	if err := dst.resolveMonitoring(ctx); err != nil {
		return err
	}
	message, err := renderAlerta(dst, event)
	if err != nil {
		return err
	}
	headers := make(http.Header)
	if dst.APIKey != "" {
		headers.Set("Authorization", "Key "+dst.APIKey)
	}
	client := notificationHTTPClient(timeout)
	defer client.CloseIdleConnections()
	response, err := postNotificationJSON(
		ctx,
		client,
		"alerta",
		strings.TrimRight(dst.APIURL, "/")+"/alert",
		headers,
		message,
	)
	if err != nil {
		return err
	}
	return readAlertaResponse(response)
}

func readAlertaResponse(response *http.Response) error {
	defer response.Body.Close()
	if response.StatusCode == http.StatusAccepted {
		return errors.New("alerta suppressed the notification (HTTP 202)")
	}
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusCreated {
		return fmt.Errorf("alerta returned HTTP %d", response.StatusCode)
	}
	var result struct {
		Status string `json:"status"`
		ID     string `json:"id"`
	}
	if err := decodeNotificationResponse("alerta", response.Body, &result); err != nil {
		return err
	}
	if result.Status != "ok" || strings.TrimSpace(result.ID) == "" {
		return errors.New("alerta did not acknowledge the notification")
	}
	return nil
}
