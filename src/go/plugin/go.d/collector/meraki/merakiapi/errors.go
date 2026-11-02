// SPDX-License-Identifier: GPL-3.0-or-later

package merakiapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/web"
)

// HTTPError preserves status and retry information without retaining response
// bodies, URLs or headers. A 404 does not establish removal or lack of support.
type HTTPError struct {
	Operation     string
	StatusCode    int
	RetryAfter    time.Duration
	HasRetryAfter bool
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("%s: HTTP status %d", e.Operation, e.StatusCode)
}

func newHTTPError(op string, response *http.Response) *HTTPError {
	delay, known := retryAfter(response.Header.Get("Retry-After"), time.Now())
	return &HTTPError{
		Operation:     op,
		StatusCode:    response.StatusCode,
		RetryAfter:    delay,
		HasRetryAfter: known,
	}
}

func retryAfter(value string, now time.Time) (time.Duration, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	if seconds, err := strconv.ParseUint(value, 10, 64); err == nil {
		const maxSeconds = uint64((1<<63 - 1) / time.Second)
		if seconds > maxSeconds {
			return 0, false
		}
		return time.Duration(seconds) * time.Second, true
	}
	if at, err := http.ParseTime(value); err == nil {
		return max(0, at.Sub(now)), true
	}
	return 0, false
}

// Only known safe causes survive; URL, parser, transport and credential errors
// can contain server-controlled values or credential-file contents.
func safeError(op, message string, cause error) error {
	for _, safe := range []error{context.Canceled, context.DeadlineExceeded, web.ErrResponseTooLarge} {
		if errors.Is(cause, safe) {
			return fmt.Errorf("%s: %s: %w", op, message, safe)
		}
	}
	return fmt.Errorf("%s: %s", op, message)
}
