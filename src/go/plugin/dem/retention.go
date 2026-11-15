// SPDX-License-Identifier: GPL-3.0-or-later

package dem

import (
	"context"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/artifacts"
)

type historyRetention interface {
	EnforceHistoryRetention(context.Context, int, int64) error
}

type artifactRetention interface {
	Enforce(context.Context, int, int64) (artifacts.Status, error)
}

// Retention is process-owned so disabling every collector cannot stop history
// expiry. It never releases the stores: jobs retire after process services join.
type Retention struct {
	history        historyRetention
	historyPolicy  RetentionConfig
	artifacts      artifactRetention
	artifactPolicy RetentionConfig
	reloadConfig   func() (Config, error)
	log            *logger.Logger
	policyErrorMu  sync.RWMutex
	policyError    string
}

func (r *Retention) Run(ctx context.Context) {
	var historyDue, artifactsDue time.Time
	for {
		if ctx.Err() != nil {
			return
		}
		historyPolicy, artifactPolicy := r.historyPolicy, r.artifactPolicy
		r.reloadPolicy()
		now := time.Now()
		if !now.Before(historyDue) || r.historyPolicy != historyPolicy {
			delay := time.Hour
			if err := r.history.EnforceHistoryRetention(ctx, r.historyPolicy.Days, r.historyPolicy.MaxBytes); err != nil &&
				ctx.Err() == nil {
				r.log.Warningf("DEM history retention failed: %v", err)
				// Retry cleanup promptly; healthy appends remain independent.
				delay = 5 * time.Second
			}
			historyDue = time.Now().Add(delay)
		}
		if !now.Before(artifactsDue) || r.artifactPolicy != artifactPolicy {
			delay := time.Hour
			_, err := r.artifacts.Enforce(ctx, r.artifactPolicy.Days, r.artifactPolicy.MaxBytes)
			if err != nil && ctx.Err() == nil {
				r.log.Warningf("DEM artifact retention failed: %v", err)
				delay = 5 * time.Second
			}
			artifactsDue = time.Now().Add(delay)
		}
		next := historyDue
		if artifactsDue.Before(next) {
			next = artifactsDue
		}
		timer := time.NewTimer(time.Until(next))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

// reloadPolicy runs on the retention goroutine before each sweep. ReloadConfig
// returns validated configuration (LoadConfig); a failed load leaves the last
// policy in force. There is no config watcher.
func (r *Retention) reloadPolicy() {
	if r.reloadConfig == nil {
		return
	}
	cfg, err := r.reloadConfig()
	r.policyErrorMu.Lock()
	r.policyError = ""
	if err != nil {
		r.policyError = err.Error()
	}
	r.policyErrorMu.Unlock()
	if err != nil {
		r.log.Warningf("reloading DEM retention policies failed; retaining previous policy: %v", err)
		return
	}
	r.historyPolicy = cfg.History
	r.artifactPolicy = cfg.Artifacts
}

// PolicyError reports the last reload failure, cleared by a valid reload.
func (r *Retention) PolicyError() string {
	r.policyErrorMu.RLock()
	defer r.policyErrorMu.RUnlock()
	return r.policyError
}
