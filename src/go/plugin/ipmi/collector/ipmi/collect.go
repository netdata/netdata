// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux && (amd64 || arm64)

package ipmi

import (
	"context"
	"strings"
	"time"
)

func (c *Collector) collect(ctx context.Context) error {
	snapshot, err := c.reader.Collect(ctx, c.CollectSEL)
	if ctxErr := ctx.Err(); ctxErr != nil {
		err = ctxErr
	}
	if err != nil {
		// The Function must not serve a snapshot older than the failure.
		c.snapshot.Store(nil)
		return err
	}

	c.writeMetrics(snapshot)
	c.snapshot.Store(snapshot)
	if len(snapshot.Warnings) > 0 {
		c.Limit("ipmi:partial-collection", 1, time.Hour).Warning(strings.Join(snapshot.Warnings, "; "))
	}
	return nil
}
