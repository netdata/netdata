// SPDX-License-Identifier: GPL-3.0-or-later

package meraki

import (
	"net/http"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/meraki/merakiapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckExportsRetryAfterMinimum(t *testing.T) {
	for _, format := range []string{"seconds", "HTTP date"} {
		t.Run(format, func(t *testing.T) {
			f, server := newFixtureAPI(t)
			c := newTestCollector(t, server)
			now := time.Now()
			wantDeadline := now.Add(10 * time.Minute)
			header := "600"
			if format == "seconds" {
				c.now = func() time.Time { return now }
			} else {
				wantDeadline = wantDeadline.Truncate(time.Second)
				header = wantDeadline.UTC().Format(http.TimeFormat)
			}
			f.set("/organizations/org/devices", fixtureResponse{
				status:     http.StatusTooManyRequests,
				retryAfter: header,
				body:       `{"errors":["rate limit"]}`,
			})

			err := c.Check(t.Context())
			require.ErrorContains(t, err, "HTTP status 429")
			var apiErr *merakiapi.HTTPError
			require.ErrorAs(t, err, &apiErr, "startup metadata must preserve the safe HTTP cause")
			deadline := collectorapi.RetryNotBefore(err)
			require.False(t, deadline.IsZero(), "the failed instance must export its minimum to the framework")
			assert.WithinDuration(t, wantDeadline, deadline, time.Second)

			// Local suppression must retain the original absolute minimum rather
			// than restart the provider's wait on another diagnostic call.
			now = deadline.Add(-time.Second)
			c.now = func() time.Time { return now }
			err = c.Check(t.Context())
			require.ErrorContains(t, err, "Retry-After delay has not elapsed")
			assert.Equal(t, deadline, collectorapi.RetryNotBefore(err))
			require.Len(t, f.calls(), 1)

			now = deadline
			f.set("/organizations/org/devices", fixtureResponse{
				body: `[]`,
			})
			require.NoError(t, c.Check(t.Context()))
			require.Len(t, f.calls(), 2, "the minimum permits requests at its deadline")
		})
	}
}

func TestCheckDoesNotInventRetryAfterMinimum(t *testing.T) {
	for name, response := range map[string]fixtureResponse{
		"missing":        {status: http.StatusTooManyRequests},
		"malformed":      {status: http.StatusTooManyRequests, retryAfter: "later"},
		"explicit zero":  {status: http.StatusTooManyRequests, retryAfter: "0"},
		"past HTTP date": {status: http.StatusTooManyRequests, retryAfter: "Wed, 21 Oct 2015 07:28:00 GMT"},
		"other HTTP":     {status: http.StatusServiceUnavailable, retryAfter: "600"},
	} {
		t.Run(name, func(t *testing.T) {
			f, server := newFixtureAPI(t)
			c := newTestCollector(t, server)
			f.set("/organizations/org/devices", response)
			err := c.Check(t.Context())
			require.Error(t, err)
			assert.True(t, collectorapi.RetryNotBefore(err).IsZero())
		})
	}
}
