// SPDX-License-Identifier: GPL-3.0-or-later

package notifier

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	notifyevent "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/event"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/httpclient"
	notifymsg "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/message"
)

const prowlDefaultAPI = "https://api.prowlapp.com/publicapi"

func renderProwl(dst Destination, event notifyevent.Event) (url.Values, error) {
	priority := "0"
	switch event.Status {
	case "WARNING":
		priority = "1"
	case "CRITICAL":
		priority = "2"
	}
	form := url.Values{
		"apikey": {dst.APIKey}, "application": {"Netdata"}, "priority": {priority},
		"event":       {event.Node + " " + event.Status + ": " + event.Summary},
		"description": {notifymsg.PlainText(event, false)}, "url": {event.URL},
	}
	// Prowl documents UTF-8 byte limits, not character counts.
	for _, field := range []struct {
		name  string
		limit int
	}{{"event", 1024}, {"description", 10000}, {"url", 512}} {
		if len(form.Get(field.name)) > field.limit {
			return nil, fmt.Errorf("prowl %s exceeds the %d-byte limit", field.name, field.limit)
		}
	}
	return form, nil
}

func sendProwl(ctx context.Context, dst Destination, event notifyevent.Event, timeout time.Duration) error {
	if err := dst.resolveFormProvider(ctx); err != nil {
		return err
	}
	message, err := renderProwl(dst, event)
	if err != nil {
		return err
	}
	base := dst.APIURL
	if base == "" {
		base = prowlDefaultAPI
	}
	client := httpclient.New(timeout)
	defer client.CloseIdleConnections()
	response, err := httpclient.Post(
		ctx,
		client,
		"prowl",
		strings.TrimRight(base, "/")+"/add",
		"application/x-www-form-urlencoded",
		http.Header{"Accept": {"application/xml"}},
		strings.NewReader(message.Encode()),
	)
	if err != nil {
		return err
	}
	return readProwlResponse(response)
}

func readProwlResponse(response *http.Response) error {
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("prowl returned HTTP %d", response.StatusCode)
	}
	data, err := httpclient.ReadResponse("prowl", response.Body)
	if err != nil {
		return err
	}
	var result struct {
		XMLName xml.Name `xml:"prowl"`
		Success []struct {
			Code int `xml:"code,attr"`
		} `xml:"success"`
		Errors []struct{} `xml:"error"`
	}
	decoder := xml.NewDecoder(bytes.NewReader(data))
	found := false
	invalid := errors.New("invalid prowl response")
	// Require exactly one document; Decode alone ignores trailing documents and text.
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return invalid
		}
		switch token := token.(type) {
		case xml.StartElement:
			if found || decoder.DecodeElement(&result, &token) != nil {
				return invalid
			}
			found = true
		case xml.CharData:
			if len(bytes.TrimSpace(token)) != 0 {
				return invalid
			}
		case xml.Comment, xml.ProcInst:
		default:
			return invalid
		}
	}
	if !found || len(result.Success) != 1 || result.Success[0].Code != 200 || len(result.Errors) != 0 {
		return invalid
	}
	return nil
}
