// SPDX-License-Identifier: GPL-3.0-or-later

package notifier

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	notifyevent "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/event"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/httpclient"
	notifymsg "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/message"
)

const kavenegarDefaultAPI = "https://api.kavenegar.com/v1"

func renderKavenegar(dst Destination, event notifyevent.Event) url.Values {
	return url.Values{
		"sender":   {dst.Sender},
		"receptor": {dst.Recipient},
		"message":  {notifymsg.PlainText(event, true)},
	}
}

func sendKavenegar(ctx context.Context, dst Destination, event notifyevent.Event, timeout time.Duration) error {
	if err := dst.resolveFormProvider(ctx); err != nil {
		return err
	}
	base := dst.APIURL
	if base == "" {
		base = kavenegarDefaultAPI
	}
	endpoint := strings.TrimRight(base, "/") + "/" + url.PathEscape(dst.APIKey) + "/sms/send.json"
	client := httpclient.New(timeout)
	defer client.CloseIdleConnections()
	response, err := httpclient.Post(ctx, client, "kavenegar", endpoint, "application/x-www-form-urlencoded",
		http.Header{"Accept": {"application/json"}}, strings.NewReader(renderKavenegar(dst, event).Encode()))
	if err != nil {
		return err
	}
	return readKavenegarResponse(response)
}

func readKavenegarResponse(response *http.Response) error {
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("kavenegar returned HTTP %d", response.StatusCode)
	}
	var result struct {
		Return struct {
			Status int `json:"status"`
		} `json:"return"`
		Entries []struct {
			MessageID int64 `json:"messageid"`
		} `json:"entries"`
	}
	if err := httpclient.DecodeResponse("kavenegar", response.Body, &result); err != nil {
		return err
	}
	// This acknowledges API acceptance; final SMS delivery is asynchronous.
	if result.Return.Status != 200 || len(result.Entries) != 1 || result.Entries[0].MessageID <= 0 {
		return errors.New("invalid kavenegar response")
	}
	return nil
}
