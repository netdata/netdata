// SPDX-License-Identifier: GPL-3.0-or-later

package ilert

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"strings"

	notifyevent "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/event"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/httpclient"
	notifymsg "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/message"

	configfield "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/config/field"
)

const ilertDefaultAPI = "https://api.ilert.com/api"

func (dst Config) validate() error {
	for _, field := range []struct{ name, value string }{
		{"integration_key", dst.IntegrationKey}, {"api_url", dst.APIURL},
	} {
		reference, err := dst.Secrets.IsReference(field.value)
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
		return configfield.APIBase(value, "ilert", "api.ilert.com")
	}
	return configfield.Token(value, "ilert", "integration_key")
}

type ilertEvent struct {
	IntegrationKey string            `json:"integrationKey"`
	EventType      string            `json:"eventType"`
	AlertKey       string            `json:"alertKey"`
	Summary        string            `json:"summary"`
	Details        string            `json:"details"`
	Links          []ilertLink       `json:"links,omitempty"`
	CustomDetails  notifyevent.Event `json:"customDetails"`
}

type ilertLink struct {
	Href string `json:"href"`
	Text string `json:"text"`
}

func renderIlert(dst Config, event notifyevent.Event) ilertEvent {
	message := ilertEvent{
		IntegrationKey: dst.IntegrationKey, EventType: "ALERT",
		// ilert trims keys and compares them case-insensitively; preserve opaque ID distinctions.
		AlertKey: fmt.Sprintf("%x", sha256.Sum256([]byte(event.IncidentID))),
		Summary:  event.Node + " " + event.Status + ": " + event.Summary,
		Details:  notifymsg.PlainText(event, false), CustomDetails: event,
	}
	if event.Status == "CLEAR" {
		message.EventType = "RESOLVE"
	}
	if event.URL != "" {
		message.Links = []ilertLink{{Href: event.URL, Text: "View alert"}}
	}
	return message
}

func (s *Sender) Send(ctx context.Context, event notifyevent.Event) error {
	dst := s.cfg
	client := s.client
	for _, field := range []struct {
		name  string
		value *string
	}{{"integration_key", &dst.IntegrationKey}, {"api_url", &dst.APIURL}} {
		value, err := dst.Secrets.Resolve(ctx, *field.value)
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
	response, err := httpclient.PostJSON(
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
