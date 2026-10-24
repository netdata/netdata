// SPDX-License-Identifier: GPL-3.0-or-later

package acquisition

import (
	"context"
	"net/http"
	"sync/atomic"

	"github.com/stmcginnis/gofish"
)

// gofish resolves collection members concurrently. Each GET needs its own trace
// for sanitization; a connection-wide trace would mix responses and race.
type logClient struct {
	*gofish.APIClient
	connection   *connection
	ctx          context.Context
	unauthorized atomic.Bool
}

func (c *logClient) Get(raw string) (*http.Response, error) {
	target, err := resolveRedfishURI(c.connection.origin, c.connection.root, raw, uriOpaquePage)
	if err != nil {
		return nil, err
	}
	trace := &requestTrace{}
	resp, err := c.APIClient.WithContext(context.WithValue(c.ctx, requestTraceKey{}, trace)).Get(target.RequestURI())
	if trace.last != nil && trace.last.status == http.StatusUnauthorized {
		c.unauthorized.Store(true)
	}
	return resp, sdkError(err, trace)
}
