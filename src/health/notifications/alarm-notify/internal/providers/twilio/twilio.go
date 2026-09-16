// SPDX-License-Identifier: GPL-3.0-or-later

package twilio

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"unicode"

	notifyevent "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/event"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/httpclient"
	notifymsg "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/message"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/secret"

	configfield "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/config/field"
)

const twilioDefaultAPI = "https://api.twilio.com"

var twilioAccountSID = regexp.MustCompile(`^AC[0-9a-fA-F]{32}$`)

func (dst Config) validate() error {
	for _, field := range []struct{ name, value string }{{"from", dst.From}, {"to", dst.To}} {
		if field.value == "" || strings.TrimSpace(field.value) != field.value || strings.Contains(field.value, "${") ||
			strings.IndexFunc(field.value, unicode.IsControl) != -1 ||
			(field.name == "to" && strings.IndexFunc(field.value, unicode.IsSpace) != -1) {
			return fmt.Errorf(
				"twilio %s must be a nonempty literal sender or recipient without surrounding whitespace or controls",
				field.name,
			)
		}
	}
	for _, field := range []struct{ name, value string }{
		{"account_sid", dst.AccountSID}, {"auth_token", dst.AuthToken}, {"api_url", dst.APIURL},
	} {
		reference, err := secret.IsReference(field.value)
		if err != nil {
			return fmt.Errorf("twilio %s: %w", field.name, err)
		}
		if !reference {
			if err := validateTwilioField(field.name, field.value); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateTwilioField(name, value string) error {
	switch name {
	case "api_url":
		return configfield.APIBase(value, "twilio", "api.twilio.com")
	case "account_sid":
		if !twilioAccountSID.MatchString(value) {
			return errors.New("twilio account_sid must be AC followed by 32 hexadecimal characters")
		}
	case "auth_token":
		return configfield.Token(value, "twilio", "auth_token")
	}
	return nil
}

func (s *Sender) Send(ctx context.Context, event notifyevent.Event) error {
	dst := s.cfg
	client := s.client
	for _, field := range []struct {
		name  string
		value *string
	}{
		{"account_sid", &dst.AccountSID}, {"auth_token", &dst.AuthToken}, {"api_url", &dst.APIURL},
	} {
		value, err := secret.Resolve(ctx, *field.value)
		if err != nil {
			return fmt.Errorf("twilio %s: %w", field.name, err)
		}
		if err := validateTwilioField(field.name, value); err != nil {
			return err
		}
		*field.value = value
	}
	base := dst.APIURL
	if base == "" {
		base = twilioDefaultAPI
	}
	endpoint := strings.TrimRight(base, "/") + "/2010-04-01/Accounts/" + dst.AccountSID + "/Messages.json"
	headers := http.Header{
		"Authorization": {"Basic " + base64.StdEncoding.EncodeToString([]byte(dst.AccountSID+":"+dst.AuthToken))},
	}
	response, err := httpclient.Post(ctx, client, "twilio", endpoint, "application/x-www-form-urlencoded", headers,
		strings.NewReader(renderTwilio(dst, event).Encode()))
	if err != nil {
		return err
	}
	return readTwilioResponse(response)
}

func renderTwilio(dst Config, event notifyevent.Event) url.Values {
	return url.Values{"From": {dst.From}, "To": {dst.To}, "Body": {notifymsg.PlainText(event, true)}}
}

func readTwilioResponse(response *http.Response) error {
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		return fmt.Errorf("twilio returned HTTP %d", response.StatusCode)
	}
	var result struct {
		SID string `json:"sid"`
	}
	if err := httpclient.DecodeResponse("twilio", response.Body, &result); err != nil {
		return err
	}
	if strings.TrimSpace(result.SID) == "" {
		return errors.New("invalid twilio response")
	}
	return nil
}
