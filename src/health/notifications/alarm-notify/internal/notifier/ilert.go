// SPDX-License-Identifier: GPL-3.0-or-later

package notifier

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const ilertDefaultAPI = "https://api.ilert.com/api"

func (dst Destination) validateIlert() error {
	if dst != (Destination{Type: dst.Type, IntegrationKey: dst.IntegrationKey, APIURL: dst.APIURL}) {
		return errors.New("ilert destinations support integration_key and api_url only")
	}
	for _, field := range []struct{ name, value string }{
		{"integration_key", dst.IntegrationKey}, {"api_url", dst.APIURL},
	} {
		reference, err := secretReference(field.value)
		if err != nil {
			return fmt.Errorf("ilert %s: %w", field.name, err)
		}
		if !reference {
			if err := validateIlertField(field.name, field.value); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateIlertField(name, value string) error {
	if name == "api_url" {
		return validateAPIBase(value, "ilert", "api.ilert.com")
	}
	return validateToken(value, "ilert", "integration_key")
}

type ilertEvent struct {
	IntegrationKey string      `json:"integrationKey"`
	EventType      string      `json:"eventType"`
	AlertKey       string      `json:"alertKey"`
	Summary        string      `json:"summary"`
	Details        string      `json:"details"`
	Links          []ilertLink `json:"links,omitempty"`
	CustomDetails  Event       `json:"customDetails"`
}

type ilertLink struct {
	Href string `json:"href"`
	Text string `json:"text"`
}

func renderIlert(dst Destination, event Event) ilertEvent {
	message := ilertEvent{
		IntegrationKey: dst.IntegrationKey, EventType: "ALERT",
		// ilert trims keys and compares them case-insensitively; preserve opaque ID distinctions.
		AlertKey: fmt.Sprintf("%x", sha256.Sum256([]byte(event.IncidentID))),
		Summary:  event.Node + " " + event.Status + ": " + event.Summary,
		Details:  notificationPlainText(event, false), CustomDetails: event,
	}
	if event.Status == "CLEAR" {
		message.EventType = "RESOLVE"
	}
	if event.URL != "" {
		message.Links = []ilertLink{{Href: event.URL, Text: "View alert"}}
	}
	return message
}

func sendIlert(ctx context.Context, dst Destination, event Event, timeout time.Duration) error {
	for _, field := range []struct {
		name  string
		value *string
	}{{"integration_key", &dst.IntegrationKey}, {"api_url", &dst.APIURL}} {
		value, err := resolveSecret(ctx, *field.value)
		if err != nil {
			return fmt.Errorf("ilert %s: %w", field.name, err)
		}
		if err := validateIlertField(field.name, value); err != nil {
			return err
		}
		*field.value = value
	}
	base := dst.APIURL
	if base == "" {
		base = ilertDefaultAPI
	}
	client := notificationHTTPClient(timeout)
	defer client.CloseIdleConnections()
	response, err := postNotificationJSON(
		ctx,
		client,
		"ilert",
		strings.TrimRight(base, "/")+"/events",
		nil,
		renderIlert(dst, event),
	)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusAccepted {
		return fmt.Errorf("ilert returned HTTP %d", response.StatusCode)
	}
	return nil
}
