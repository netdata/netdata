// SPDX-License-Identifier: GPL-3.0-or-later
package dem

import (
	"context"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/dem/artifacts"
)

type historyRetention interface {
	EnforceHistoryRetention(context.Context, int, int64) error
}

type artifactRetention interface {
	Enforce(context.Context, int, int64) (artifacts.Stats, error)
}

// Retention is process-owned so disabling every collector cannot stop history
// expiry. It never releases the store: jobs retire after process services join.
type Retention struct {
	store          historyRetention
	policy         HistoryConfig
	artifactStore  artifactRetention
	artifactPolicy HistoryConfig
	loadConfig     func() (Config, error)
	log            *logger.Logger
}

func (r *Retention) Run(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}
		r.reloadPolicy()
		delay := time.Hour
		if err := r.store.EnforceHistoryRetention(ctx, r.policy.Days, r.policy.MaxBytes); err != nil &&
			ctx.Err() == nil {
			r.log.Warningf("DEM history retention failed: %v", err)
			// A partial close/reopen can suspend writes; retry on the history flush cadence.
			delay = 5 * time.Second
		}
		if r.artifactStore != nil {
			stats, err := r.artifactStore.Enforce(ctx, r.artifactPolicy.Days, r.artifactPolicy.MaxBytes)
			if err != nil && ctx.Err() == nil {
				r.log.Warningf("DEM artifact retention failed: %v", err)
				delay = 5 * time.Second
			}
			if stats.ProtectedBytes > r.artifactPolicy.MaxBytes {
				r.log.Warningf("DEM protected run files exceed artifact budget: %d bytes", stats.ProtectedBytes)
			}
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
		if err == nil {
			err = cfg.Artifacts.validate()
		}
	}
	if err != nil {
		r.log.Warningf("reloading DEM retention policies failed; retaining previous policy: %v", err)
		return
	}
	r.policy = cfg.History
	r.artifactPolicy = cfg.Artifacts
}
