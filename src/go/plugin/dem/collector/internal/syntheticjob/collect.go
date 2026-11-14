// SPDX-License-Identifier: GPL-3.0-or-later

package syntheticjob

import (
	"context"
	"errors"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
)

// Collect publishes the session snapshot: attempt metrics always, kind metrics only for a fresh observation.
func (j *Job) Collect(ctx context.Context) error {
	if j.session == nil {
		return errors.New("synthetic job is not active")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	snapshot := j.session.Snapshot(time.Now())
	j.attempt.write(snapshot)
	if snapshot.Fresh {
		j.metrics.Write(snapshot.Latest)
	}
	return nil
}

// SetConfiguredVnode moves the job to another vnode; the session fences observations of the previous one.
func (j *Job) SetConfiguredVnode(v vnodes.VirtualNode) {
	j.vnodeGUID, _ = vnodes.ConfiguredGUIDKey(v.GUID)
	j.request.VnodeGUID = j.vnodeGUID
	if j.session != nil {
		j.session.SetVnodeGUID(j.vnodeGUID)
	}
}
