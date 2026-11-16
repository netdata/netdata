// SPDX-License-Identifier: GPL-3.0-or-later

package merakiapi_test

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/meraki/merakiapi"
)

func TestHTTPStatusAndRetryAfter(t *testing.T) {
	tests := map[string]struct {
		status int
		header string
		delay  time.Duration
	}{
		"bad request":                          {status: http.StatusBadRequest},
		"unauthorized":                         {status: http.StatusUnauthorized},
		"forbidden":                            {status: http.StatusForbidden},
		"not found is a status error":          {status: http.StatusNotFound},
		"no content is not an empty inventory": {status: http.StatusNoContent},
		"server failure":                       {status: http.StatusServiceUnavailable},
		"rate limit with seconds": {
			status: http.StatusTooManyRequests,
			header: "120",
			delay:  120 * time.Second,
		},
		"explicit zero": {status: http.StatusTooManyRequests, header: "0"},
		"past HTTP date": {
			status: http.StatusTooManyRequests,
			header: "Wed, 21 Oct 2015 07:28:00 GMT",
		},
		"missing":           {status: http.StatusTooManyRequests},
		"negative":          {status: http.StatusTooManyRequests, header: "-1"},
		"overflow":          {status: http.StatusTooManyRequests, header: "18446744073709551615"},
		"duration overflow": {status: http.StatusTooManyRequests, header: "9223372037"},
		"garbage":           {status: http.StatusTooManyRequests, header: "SYNTHETIC_PRIVATE_BODY"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var calls int
			client, err := merakiapi.New(merakiapi.Config{
				APIKey: syntheticKey,
			}, &http.Client{
				Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
					calls++
					return response(
						r,
						test.status,
						http.Header{
							"Retry-After": {test.header},
						},
						"SYNTHETIC_PRIVATE_BODY",
					), nil
				}),
			})
			require.NoError(t, err)
			got, err := client.ListOrganizations(t.Context())
			require.Error(t, err)
			assert.Nil(t, got)
			var status *merakiapi.HTTPError
			require.ErrorAs(t, err, &status)
			assert.Equal(t, &merakiapi.HTTPError{
				Operation:  "getOrganizations",
				StatusCode: test.status,
				RetryAfter: test.delay,
			}, status)
			assert.NotContains(t, err.Error(), "SYNTHETIC_PRIVATE_BODY")
			assert.Nil(t, errors.Unwrap(status))
			assert.Equal(t, 1, calls, "status errors must not trigger automatic retries")
		})
	}
}

func TestHTTPDateRetryAfter(t *testing.T) {
	at := time.Now().UTC().Add(2 * time.Minute).Truncate(time.Second)
	client, err := merakiapi.New(merakiapi.Config{
		APIKey: syntheticKey,
	}, &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return response(
				r,
				http.StatusTooManyRequests,
				http.Header{
					"Retry-After": {at.Format(http.TimeFormat)},
				},
				"",
			), nil
		}),
	})
	require.NoError(t, err)
	_, err = client.ListOrganizations(t.Context())
	var status *merakiapi.HTTPError
	require.ErrorAs(t, err, &status)
	assert.InDelta(t, time.Until(at).Seconds(), status.RetryAfter.Seconds(), 1)
}
