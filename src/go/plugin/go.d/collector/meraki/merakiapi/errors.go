// SPDX-License-Identifier: GPL-3.0-or-later

package merakiapi

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/web"
)

// HTTPError reports a non-200 response without its body, URL or headers. A 404
// does not mean the entity is gone: Meraki also answers 404 to some
// authorization failures.
type HTTPError struct {
	Operation  string
	StatusCode int
	// RetryAfter is the parsed Retry-After delay, zero when absent, invalid, past
	// or overflowing.
	RetryAfter time.Duration
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("%s: HTTP status %d", e.Operation, e.StatusCode)
}

func newHTTPError(opName string, resp *http.Response) *HTTPError {
	return &HTTPError{
		Operation:  opName,
		StatusCode: resp.StatusCode,
		RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After"), time.Now()),
	}
}

// parseRetryAfter accepts delta-seconds and HTTP dates. Invalid, past and
// overflowing values return zero.
func parseRetryAfter(value string, now time.Time) time.Duration {
	value = strings.TrimSpace(value)
	if seconds, err := strconv.ParseUint(value, 10, 64); err == nil {
		if seconds > math.MaxInt64/uint64(time.Second) {
			return 0
		}
		return time.Duration(seconds) * time.Second
	}
	if at, err := http.ParseTime(value); err == nil {
		return max(0, at.Sub(now))
	}
	return 0
}

// safeError keeps only causes known to be safe to show. URL, parser, transport
// and credential-file errors can carry server-controlled text or file contents.
func safeError(opName, message string, cause error) error {
	for _, safe := range []error{context.Canceled, context.DeadlineExceeded, web.ErrResponseTooLarge} {
		if errors.Is(cause, safe) {
			return fmt.Errorf("%s: %s: %w", opName, message, safe)
		}
	}
	return fmt.Errorf("%s: %s", opName, message)
}
