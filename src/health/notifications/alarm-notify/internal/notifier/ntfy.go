// SPDX-License-Identifier: GPL-3.0-or-later

package notifier

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"reflect"
	"strings"
	"time"
	"unicode"

	notifyevent "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/event"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/httpclient"
	notifymsg "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/message"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/secret"
)

type ntfyAction struct {
	Action string `json:"action"`
	Label  string `json:"label"`
	URL    string `json:"url"`
	Clear  bool   `json:"clear"`
}

func (dst Destination) validateNtfy() error {
	if !reflect.DeepEqual(dst, Destination{Type: dst.Type, URL: dst.URL, AccessToken: dst.AccessToken,
		Username: dst.Username, Password: dst.Password}) {
		return errors.New("ntfy destinations support url, access_token, username and password only")
	}
	if (dst.Username == "") != (dst.Password == "") {
		return errors.New("ntfy username and password must be configured together")
	}
	if dst.AccessToken != "" && dst.Username != "" {
		return errors.New("ntfy requires either access_token or username/password, not both")
	}
	for _, field := range []struct{ name, value string }{
		{"url", dst.URL}, {"access_token", dst.AccessToken}, {"username", dst.Username}, {"password", dst.Password},
	} {
		reference, err := secret.IsReference(field.value)
		if err != nil {
			return fmt.Errorf("ntfy %s: %w", field.name, err)
		}
		if !reference {
			if err := validateNtfyField(field.name, field.value); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateNtfyField(name, value string) error {
	if name == "url" {
		if !httpclient.ValidURL(value, false) || strings.Contains(value, "#") {
			return errors.New("ntfy url must be an absolute HTTP(S) topic URL without user information or fragment")
		}
		return nil
	}
	if value == "" {
		return nil // Authentication is optional; references cannot resolve to an empty value.
	}
	if name == "access_token" {
		return validateToken(value, "ntfy", name)
	}
	if strings.Contains(value, "${") || strings.IndexFunc(value, unicode.IsControl) != -1 ||
		(name == "username" && strings.Contains(value, ":")) {
		return fmt.Errorf(
			"ntfy %s must not contain controls or secret interpolation; username must not contain a colon",
			name,
		)
	}
	return nil
}

func sendNtfy(ctx context.Context, dst Destination, event notifyevent.Event, timeout time.Duration) error {
	for _, field := range []struct {
		name  string
		value *string
	}{
		{"url", &dst.URL}, {"access_token", &dst.AccessToken}, {"username", &dst.Username}, {"password", &dst.Password},
	} {
		value, err := secret.Resolve(ctx, *field.value)
		if err != nil {
			return fmt.Errorf("ntfy %s: %w", field.name, err)
		}
		if err := validateNtfyField(field.name, value); err != nil {
			return err
		}
		*field.value = value
	}
	headers := renderNtfyHeaders(event)
	if dst.AccessToken != "" {
		headers.Set("Authorization", "Bearer "+dst.AccessToken)
	} else if dst.Username != "" {
		headers.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(dst.Username+":"+dst.Password)))
	}
	client := httpclient.New(timeout)
	defer client.CloseIdleConnections()
	response, err := httpclient.Post(ctx, client, "ntfy", dst.URL, "text/plain; charset=utf-8", headers,
		strings.NewReader(notifymsg.PlainText(event, false)))
	if err != nil {
		return err
	}
	return readNtfyResponse(response)
}

func renderNtfyHeaders(event notifyevent.Event) http.Header {
	priority, tag := "default", "white_check_mark"
	switch event.Status {
	case "WARNING":
		priority, tag = "high", "warning"
	case "CRITICAL":
		priority, tag = "urgent", "red_circle"
	}
	headers := http.Header{
		"Title":    {encodeNtfyHeader(event.Node + ": " + strings.ReplaceAll(event.Alert, "_", " "))},
		"Priority": {priority}, "Tags": {tag},
	}
	if event.URL != "" {
		// JSON preserves delimiters inside URLs; ntfy also accepts it in the Actions header.
		actions, _ := json.Marshal([]ntfyAction{{Action: "view", Label: "View node", URL: event.URL, Clear: true}})
		headers.Set("Actions", encodeNtfyHeader(string(actions)))
	}
	return headers
}

func encodeNtfyHeader(value string) string {
	encoded := mime.BEncoding.Encode("UTF-8", value)
	if encoded != value || !strings.Contains(value, "=?") {
		return encoded
	}
	// The MIME encoder leaves printable ASCII unchanged, but ntfy decodes literal encoded words too.
	// This remaining ASCII case needs explicit encoding, with each word below the MIME 75-byte limit.
	var words []string
	for len(value) > 0 {
		n := min(len(value), 45)
		words = append(words, "=?UTF-8?B?"+base64.StdEncoding.EncodeToString([]byte(value[:n]))+"?=")
		value = value[n:]
	}
	return strings.Join(words, " ")
}

func readNtfyResponse(response *http.Response) error {
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("ntfy returned HTTP %d", response.StatusCode)
	}
	var result struct {
		ID    string `json:"id"`
		Event string `json:"event"`
	}
	if err := httpclient.DecodeResponse("ntfy", response.Body, &result); err != nil {
		return err
	}
	if strings.TrimSpace(result.ID) == "" || result.Event != "message" {
		return errors.New("invalid ntfy response")
	}
	return nil
}
