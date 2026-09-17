// SPDX-License-Identifier: GPL-3.0-or-later
package signl4

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
	response, err := httpclient.PostJSON(ctx, s.client, "signl4", endpoint, nil, renderSIGNL4(event))
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if !(response.StatusCode == http.StatusOK || response.StatusCode == http.StatusCreated || response.StatusCode == http.StatusAccepted) {
		return fmt.Errorf("signl4 returned HTTP %d", response.StatusCode)
	}
	return nil
}
func (dst Config) validate() error {
	reference, err := secret.IsReference(dst.URL)
	if err != nil {
		return fmt.Errorf("destination.url: %w", err)
	}
	if !reference {
		return configfield.URL(dst.URL)
	}
	return nil
}
