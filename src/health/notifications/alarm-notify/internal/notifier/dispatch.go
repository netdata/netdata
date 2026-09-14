// SPDX-License-Identifier: GPL-3.0-or-later

package notifier

import (
	"context"
	"errors"
	"time"
)

type deliveryResult struct {
	destination string
	err         error
}

func dispatch(
	ctx context.Context,
	cfg Config,
	destinations []string,
	event Event,
	timeout time.Duration,
	report func(deliveryResult),
) error {
	succeeded := 0
	for _, name := range destinations {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := sendWebhook(ctx, cfg.Destinations[name], event, timeout)
		report(deliveryResult{destination: name, err: err})
		if err == nil {
			succeeded++
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(destinations) > 0 && succeeded == 0 {
		return errors.New("all selected destinations failed")
	}
	return nil
}
