// SPDX-License-Identifier: GPL-3.0-or-later

package meraki

import (
	"errors"
	"net/http"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/meraki/merakiapi"
)

// Meraki answers a rate-limited request with 429 and a Retry-After delay. The
// collector sends nothing until that delay ends, without sleeping: Check and
// Collect fail fast in the meantime. The deadline also travels on the error
// (collectorapi.WithRetryNotBefore), so the job manager does not retry a failed
// startup earlier, even though it discards this collector instance.

// checkRetryAfter fails while the last requested Retry-After wait is running.
func (c *Collector) checkRetryAfter() error {
	if c.now().Before(c.retryNotBefore) {
		err := errors.New("Meraki API Retry-After delay has not elapsed")
		return collectorapi.WithRetryNotBefore(err, c.retryNotBefore)
	}
	return nil
}

// rememberRetryAfter records the wait requested by a 429 response and attaches
// its deadline to err. Other errors, and nil, pass through unchanged.
func (c *Collector) rememberRetryAfter(err error) error {
	httpErr, ok := errors.AsType[*merakiapi.HTTPError](err)
	if !ok || httpErr.StatusCode != http.StatusTooManyRequests || httpErr.RetryAfter <= 0 {
		return err
	}
	c.retryNotBefore = c.now().Add(httpErr.RetryAfter)
	return collectorapi.WithRetryNotBefore(err, c.retryNotBefore)
}
