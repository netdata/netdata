// SPDX-License-Identifier: GPL-3.0-or-later

package pagerduty

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/testutil"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPagerDutyRequestLimit(t *testing.T) {
	for version := range map[int64]struct{}{1: {}, 2: {}} {
		for name, test := range map[string]struct {
			symbol string
			extra  int
		}{"ASCII boundary": {"x", 0}, "ASCII over": {"x", 1}, "escaped boundary": {"<", 0}, "escaped over": {"<", 1}} {
			t.Run(fmt.Sprintf("v%d/%s", version, name), func(t *testing.T) {
				var calls atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					data, err := io.ReadAll(r.Body)
					assert.NoError(t, err)
					assert.Len(t, data, 512*1024)
					calls.Add(1)
					pagerDutyTestAck(w, version, pagerDutyTestIncidentKey)
				}))
				defer server.Close()
				dst := pagerDutyTestConfig(version)
				dst.APIURL = server.URL
				event := testutil.ExpectedEvent()
				event.Info = "x"
				base, err := json.Marshal(renderPagerDuty(dst, event))
				require.NoError(t, err)
				remaining := 512*1024 - len(base) + 1
				width := 1
				if test.symbol == "<" {
					width = 6
				}
				event.Info = strings.Repeat(
					test.symbol,
					remaining/width,
				) + strings.Repeat(
					"x",
					remaining%width+test.extra,
				)
				sender, err := New(dst, server.Client())
				require.NoError(t, err)
				err = sender.Send(context.Background(), event)
				if test.extra == 0 {
					require.NoError(t, err)
					assert.EqualValues(t, 1, calls.Load())
				} else {
					require.ErrorContains(t, err, "512 KiB")
					assert.Zero(t, calls.Load())
				}
			})
		}
	}
}
func pagerDutyTestAck(w http.ResponseWriter, version int64, key string) {
	field, status := "incident_key", 200
	if version == 2 {
		field, status = "dedup_key", 202
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "success", field: key})
}
