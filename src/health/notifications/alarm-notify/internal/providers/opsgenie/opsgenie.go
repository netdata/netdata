// SPDX-License-Identifier: GPL-3.0-or-later

package opsgenie

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"

	notifyevent "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/event"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/httpclient"
	notifymsg "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/message"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/secret"

	configfield "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/config/field"
)

const opsgenieDefaultAPI = "https://api.opsgenie.com"

func (dst Config) validate() error {
	for _, field := range []struct{ name, value string }{{"api_url", dst.APIURL}, {"api_key", dst.APIKey}} {
		reference, err := secret.IsReference(field.value)
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
			if err := configfield.APIBase(value, "opsgenie", host); err != nil {
				return err
			}
		}
		return nil
	}
	return configfield.Token(value, "opsgenie", "api_key")
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

func renderOpsgenie(event notifyevent.Event) (any, error) {
	if utf8.RuneCountInString(event.Node) > 100 {
		return nil, errors.New("opsgenie source exceeds the 100-character limit")
	}
	data, err := json.Marshal(event)
	if err != nil {
		return nil, errors.New("could not encode opsgenie event")
	}
	text := notifymsg.PlainText(event, true)
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

func (s *Sender) Send(ctx context.Context, event notifyevent.Event) error {
	dst := s.cfg
	client := s.client
	for _, field := range []struct {
		name  string
		value *string
	}{{"api_url", &dst.APIURL}, {"api_key", &dst.APIKey}} {
		value, err := secret.Resolve(ctx, *field.value)
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
	response, err := httpclient.PostJSON(ctx, client, "opsgenie", endpoint,
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
	if err := httpclient.DecodeResponse("opsgenie", response.Body, &result); err != nil {
		return err
	}
	if result.Result != "Request will be processed" || configfield.Token(result.RequestID, "opsgenie", "requestId") != nil {
		return errors.New("invalid opsgenie acknowledgment")
	}
	return nil
}
