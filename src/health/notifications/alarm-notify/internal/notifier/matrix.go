// SPDX-License-Identifier: GPL-3.0-or-later

package notifier

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"time"
	"unicode"
)

func (dst Destination) validateMatrix() error {
	if !reflect.DeepEqual(
		dst,
		Destination{Type: dst.Type, APIURL: dst.APIURL, AccessToken: dst.AccessToken, RoomID: dst.RoomID},
	) {
		return errors.New("matrix destination contains fields for another provider")
	}
	if !validMatrixID(dst.RoomID, '!') || strings.Contains(dst.RoomID, "${") {
		return errors.New("matrix room_id must be a literal !-prefixed room ID without whitespace or controls")
	}
	for _, field := range []struct{ name, value string }{{"api_url", dst.APIURL}, {"access_token", dst.AccessToken}} {
		reference, err := secretReference(field.value)
		if err != nil {
			return fmt.Errorf("matrix %s: %w", field.name, err)
		}
		if !reference {
			if err := validateMatrixField(field.name, field.value); err != nil {
				return err
			}
		}
	}
	return nil
}

func validMatrixID(value string, sigil byte) bool {
	return len(value) > 1 && value[0] == sigil && strings.IndexFunc(value, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsControl(r)
	}) == -1
}

func validateMatrixField(name, value string) error {
	if name == "api_url" {
		if value == "" {
			return errors.New("matrix api_url is required")
		}
		for _, host := range []string{"matrix.org", "matrix-client.matrix.org"} {
			if err := validateAPIBase(value, "matrix", host); err != nil {
				return err
			}
		}
		return nil
	}
	return validateToken(value, "matrix", "access_token")
}

type matrixMessage struct {
	MessageType   string `json:"msgtype"`
	Body          string `json:"body"`
	Format        string `json:"format"`
	FormattedBody string `json:"formatted_body"`
}

func renderMatrix(event Event) matrixMessage {
	icon := "⚠️"
	switch event.Status {
	case "CRITICAL":
		icon = "🔴"
	case "CLEAR":
		icon = "✅"
	}
	text := icon + " " + notificationPlainText(event, false)
	formatted := strings.ReplaceAll(html.EscapeString(text), "\n", "<br>")
	if event.URL != "" {
		text += "\n" + event.URL
		formatted += `<br><a href="` + html.EscapeString(event.URL) + `">` + html.EscapeString(event.URL) + `</a>`
	}
	return matrixMessage{
		MessageType:   "m.notice",
		Body:          text,
		Format:        "org.matrix.custom.html",
		FormattedBody: formatted,
	}
}

func sendMatrix(ctx context.Context, dst Destination, event Event, timeout time.Duration) error {
	for _, field := range []struct {
		name  string
		value *string
	}{{"api_url", &dst.APIURL}, {"access_token", &dst.AccessToken}} {
		value, err := resolveSecret(ctx, *field.value)
		if err != nil {
			return fmt.Errorf("matrix %s: %w", field.name, err)
		}
		if err := validateMatrixField(field.name, value); err != nil {
			return err
		}
		*field.value = value
	}
	// Transaction IDs identify individual sends, not the alert lifecycle.
	endpoint := strings.TrimRight(dst.APIURL, "/") + "/_matrix/client/v3/rooms/" + url.PathEscape(dst.RoomID) +
		"/send/m.room.message/nd_" + rand.Text()
	client := notificationHTTPClient(timeout)
	defer client.CloseIdleConnections()
	response, err := requestNotificationJSON(
		ctx,
		client,
		"matrix",
		http.MethodPut,
		endpoint,
		http.Header{
			"Authorization": {"Bearer " + dst.AccessToken},
			"Accept":        {"application/json"},
		},
		renderMatrix(event),
	)
	if err != nil {
		return err
	}
	return readMatrixResponse(response)
}

func readMatrixResponse(response *http.Response) error {
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("matrix returned HTTP %d", response.StatusCode)
	}
	var result struct {
		EventID   string `json:"event_id"`
		ErrorCode string `json:"errcode"`
		Error     string `json:"error"`
	}
	if err := decodeNotificationResponse("matrix", response.Body, &result); err != nil {
		return err
	}
	if !validMatrixID(result.EventID, '$') || result.ErrorCode != "" || result.Error != "" {
		return errors.New("invalid matrix acknowledgment")
	}
	return nil
}
