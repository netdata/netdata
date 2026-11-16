// SPDX-License-Identifier: GPL-3.0-or-later

package acquisition

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQueuedBootstrapRequestsUseDiscoveredConcurrency(t *testing.T) {
	for _, multiple := range []bool{true, false} {
		t.Run(fmt.Sprint(multiple), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				first, remaining := make(chan struct{}), make(chan struct{})
				var started atomic.Int64
				client, err := New(testConfig("http://bmc.example", "none").Options, &http.Client{
					Transport: admissionTestTransport(func(req *http.Request) (*http.Response, error) {
						if started.Add(1) == 1 {
							<-first
						} else {
							<-remaining
						}
						return &http.Response{
							StatusCode: http.StatusOK,
							Header:     make(http.Header),
							Request:    req,
							Body: io.NopCloser(strings.NewReader(fmt.Sprintf(
								`{"@odata.id":"/redfish/v1/","RedfishVersion":"1.20.0","ProtocolFeaturesSupported":{"MultipleHTTPRequests":%v}}`,
								multiple,
							))),
						}, nil
					}),
				})
				require.NoError(t, err)
				completed := make(chan error, 3)
				query := func() { _, err := client.Logs().Services(t.Context()); completed <- err }
				go query()
				synctest.Wait()
				go query()
				synctest.Wait()
				assert.EqualValues(t, 1, started.Load(), "bootstrap remains serial until root is read")
				close(first)
				synctest.Wait()
				go query()
				synctest.Wait()
				want := int64(2)
				if multiple {
					want = 3
				}
				assert.Equal(t, want, started.Load(), "queued bootstrap must honor the discovered source policy")
				close(remaining)
				for range 3 {
					require.NoError(t, <-completed)
				}
			})
		})
	}
}

type admissionTestTransport func(*http.Request) (*http.Response, error)

func (f admissionTestTransport) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }
