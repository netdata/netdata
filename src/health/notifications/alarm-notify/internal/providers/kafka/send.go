// SPDX-License-Identifier: GPL-3.0-or-later
package kafka

import (
	"context"
	"fmt"
	notifyevent "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/event"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/httpclient"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/secret"
	"net/http"

	configfield "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/config/field"
)

func (s *Sender) Send(ctx context.Context, event notifyevent.Event) error {
	endpoint, err := secret.Resolve(ctx, s.cfg.URL)
	if err != nil {
		return fmt.Errorf("destination.url: %w", err)
	}
	if err := configfield.URL(endpoint); err != nil {
		return err
	}
	response, err := httpclient.PostJSON(ctx, s.client, "kafka", endpoint, nil, renderKafka(s.cfg, event))
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if !(response.StatusCode == http.StatusNoContent) {
		return fmt.Errorf("kafka returned HTTP %d", response.StatusCode)
	}
	return nil
}
