// SPDX-License-Identifier: GPL-3.0-or-later

package redisfunc

import (
	"context"

	"github.com/redis/go-redis/v9"
)

// Client is the minimal server surface used by the Function handlers.
// The collector retains ownership of the client lifecycle.
type Client interface {
	SlowLogGet(ctx context.Context, num int64) *redis.SlowLogCmd
}

// Deps defines the dependency surface required by the Function handlers.
type Deps interface {
	// Client returns the job's client, or an error while the job has none.
	Client() (Client, error)
}
