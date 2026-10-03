// SPDX-License-Identifier: GPL-3.0-or-later
package dem

import (
	"context"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
)

type historyRetention interface {
	EnforceRumHistoryRetention(context.Context, int, int64) error
}

// Retention is process-owned so disabling every collector cannot stop history
// expiry. It never releases the store: jobs retire after process services join.
type Retention struct {
	store      historyRetention
	policy     HistoryConfig
	loadConfig func() (Config, error)
	log        *logger.Logger
}

func (r *Retention) Run(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}
		r.reloadPolicy()
		delay := time.Hour
		if err := r.store.EnforceRumHistoryRetention(ctx, r.policy.Days, r.policy.MaxBytes); err != nil &&
			ctx.Err() == nil {
			r.log.Warningf("RUM history retention failed: %v", err)
			// A partial close/reopen can suspend writes; retry on the history flush cadence.
			delay = 5 * time.Second
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

// reloadPolicy runs on the retention goroutine before each sweep. Invalid
// edits leave the last validated policy in force; there is no config watcher.
func (r *Retention) reloadPolicy() {
	if r.loadConfig == nil {
		return
	}
	cfg, err := r.loadConfig()
	if err == nil {
		err = cfg.History.validate()
	}
	if err != nil {
		r.log.Warningf("reloading RUM history policy failed; retaining previous policy: %v", err)
		return
	}
	r.policy = cfg.History
}
