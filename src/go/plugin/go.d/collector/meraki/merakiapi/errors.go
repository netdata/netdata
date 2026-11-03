// SPDX-License-Identifier: GPL-3.0-or-later

package merakiapi

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
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

// transportError reports a request that got no HTTP response. It keeps the
// cause (DNS, connection, TLS, proxy, refused redirect, timeout) so the failure
// can be diagnosed, but drops the url.Error wrapper: its URL can be a
// server-supplied redirect or page target, and the operation name already
// identifies the request. Credentials travel in headers, never in the URL.
func transportError(opName string, err error) error {
	if urlErr, ok := errors.AsType[*url.Error](err); ok {
		err = urlErr.Err
	}
	// net/http embeds the raw Location in an untyped error before our redirect
	// policy runs. Keep its reason without retaining the server-supplied value.
	if strings.HasPrefix(err.Error(), "failed to parse Location header ") {
		err = errors.New("failed to parse Location header")
	}
	return fmt.Errorf("%s: HTTP request failed: %w", opName, err)
}

// safeError keeps only causes known to be safe to show. Credential-file and
// decoder errors can carry file contents or response data.
func safeError(opName, message string, cause error) error {
	for _, safe := range []error{context.Canceled, context.DeadlineExceeded, web.ErrResponseTooLarge} {
		if errors.Is(cause, safe) {
			return fmt.Errorf("%s: %s: %w", opName, message, safe)
		}
	}
	return fmt.Errorf("%s: %s", opName, message)
}
