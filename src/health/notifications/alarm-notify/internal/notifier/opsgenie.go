// SPDX-License-Identifier: GPL-3.0-or-later

package notifier

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"time"
	"unicode/utf8"
)

const opsgenieDefaultAPI = "https://api.opsgenie.com"

func (dst Destination) validateOpsgenie() error {
	if !reflect.DeepEqual(dst, Destination{Type: dst.Type, APIKey: dst.APIKey, APIURL: dst.APIURL}) {
		return errors.New("opsgenie destination contains fields for another provider")
	}
	for _, field := range []struct{ name, value string }{{"api_url", dst.APIURL}, {"api_key", dst.APIKey}} {
		reference, err := secretReference(field.value)
		if err != nil {
			return fmt.Errorf("opsgenie %s: %w", field.name, err)
		}
		if !reference {
			if err := validateOpsgenieField(field.name, field.value); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateOpsgenieField(name, value string) error {
	if name == "api_url" {
		for _, host := range []string{"api.opsgenie.com", "api.eu.opsgenie.com"} {
			if err := validateAPIBase(value, "opsgenie", host); err != nil {
				return err
			}
		}
		return nil
	}
	return validateToken(value, "opsgenie", "api_key")
}

type opsgenieCreate struct {
	Message     string            `json:"message"`
	Alias       string            `json:"alias"`
	Description string            `json:"description"`
	Source      string            `json:"source"`
	Entity      string            `json:"entity"`
	Priority    string            `json:"priority"`
	User        string            `json:"user"`
	Details     map[string]string `json:"details"`
}

type opsgenieClose struct {
	Source string `json:"source"`
	User   string `json:"user"`
	Note   string `json:"note"`
}

func opsgenieAlias(id string) string {
	// Preserve opaque identity without truncation or URL-sensitive characters.
	return fmt.Sprintf("%x", sha256.Sum256([]byte(id)))
}

func renderOpsgenie(event Event) (any, error) {
	if utf8.RuneCountInString(event.Node) > 100 {
		return nil, errors.New("opsgenie source exceeds the 100-character limit")
	}
	data, err := json.Marshal(event)
	if err != nil {
		return nil, errors.New("could not encode opsgenie event")
	}
	text := notificationPlainText(event, true)
	if event.Status == "CLEAR" {
		message := opsgenieClose{Source: event.Node, User: "Netdata", Note: text + "\n\nEvent: " + string(data)}
		if utf8.RuneCountInString(message.Note) > 25000 {
			return nil, errors.New("opsgenie note exceeds the 25000-character limit")
		}
		return message, nil
	}
	for _, field := range []struct {
		name, value string
		limit       int
	}{{"entity", event.Alert, 512}, {"description", text, 15000}, {"details", "event" + string(data), 8000}} {
		if utf8.RuneCountInString(field.value) > field.limit {
			return nil, fmt.Errorf("opsgenie %s exceeds the %d-character limit", field.name, field.limit)
		}
	}
	title := event.Node + " " + event.Status + ": " + event.Summary
	if utf8.RuneCountInString(title) > 130 {
		title = string([]rune(title)[:127]) + "..."
	}
	priority := "P3"
	if event.Status == "CRITICAL" {
		priority = "P1"
	}
	return opsgenieCreate{
		Message: title, Alias: opsgenieAlias(event.IncidentID), Description: text,
		Source: event.Node, Entity: event.Alert, Priority: priority, User: "Netdata",
		Details: map[string]string{"event": string(data)},
	}, nil
}

func sendOpsgenie(ctx context.Context, dst Destination, event Event, timeout time.Duration) error {
	for _, field := range []struct {
		name  string
		value *string
	}{{"api_url", &dst.APIURL}, {"api_key", &dst.APIKey}} {
		value, err := resolveSecret(ctx, *field.value)
		if err != nil {
			return fmt.Errorf("opsgenie %s: %w", field.name, err)
		}
		if err := validateOpsgenieField(field.name, value); err != nil {
			return err
		}
		*field.value = value
	}
	message, err := renderOpsgenie(event)
	if err != nil {
		return err
	}
	base := dst.APIURL
	if base == "" {
		base = opsgenieDefaultAPI
	}
	endpoint := strings.TrimRight(base, "/") + "/v2/alerts"
	if event.Status == "CLEAR" {
		endpoint += "/" + opsgenieAlias(event.IncidentID) + "/close?identifierType=alias"
	}
	client := notificationHTTPClient(timeout)
	defer client.CloseIdleConnections()
	response, err := postNotificationJSON(ctx, client, "opsgenie", endpoint,
		http.Header{"Authorization": {"GenieKey " + dst.APIKey}, "Accept": {"application/json"}}, message)
	if err != nil {
		return err
	}
	return readOpsgenieResponse(response)
}

func readOpsgenieResponse(response *http.Response) error {
	defer response.Body.Close()
	if response.StatusCode != http.StatusAccepted {
		return fmt.Errorf("opsgenie returned HTTP %d", response.StatusCode)
	}
	var result struct {
		Result    string `json:"result"`
		RequestID string `json:"requestId"`
	}
	if err := decodeNotificationResponse("opsgenie", response.Body, &result); err != nil {
		return err
	}
	if result.Result != "Request will be processed" || validateToken(result.RequestID, "opsgenie", "requestId") != nil {
		return errors.New("invalid opsgenie acknowledgment")
	}
	return nil
}
