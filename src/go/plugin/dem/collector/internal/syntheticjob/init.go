// SPDX-License-Identifier: GPL-3.0-or-later

package syntheticjob

import (
	"context"
	"errors"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
)

// Preparation has a fixed budget independent of the public attempt timeout.
const preparationTimeout = 15 * time.Second

// Init validates the shared settings, then build validates the kind options and returns the request for the
// configured vnode GUID. Init reports the first failure in that order.
func (j *Job) Init(
	ctx context.Context,
	checkEvery, updateEvery int,
	build func(vnodeGUID string) (synthetic.Request, error),
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if j.deps.Executor == nil || j.deps.Registry == nil {
		return errors.New("missing synthetic executor or observation registry")
	}
	if checkEvery <= 0 || time.Duration(checkEvery) > time.Duration(1<<63-1)/time.Second {
		return errors.New("check_every must be positive and representable as a duration")
	}
	if updateEvery <= 0 {
		return errors.New("update_every must be positive")
	}
	request, err := build(j.vnodeGUID)
	if err != nil {
		return err
	}
	j.request, j.checkEvery, j.initialized = request, checkEvery, true
	return nil
}

// Check verifies browser preparation without running or registering the job.
func (j *Job) Check(ctx context.Context) error {
	if !j.initialized {
		return errNotInitialized
	}
	ctx, cancel := context.WithTimeout(ctx, preparationTimeout)
	defer cancel()
	return j.deps.Executor.Check(ctx)
}
