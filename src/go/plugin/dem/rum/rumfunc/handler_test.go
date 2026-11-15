// SPDX-License-Identifier: GPL-3.0-or-later

package rumfunc_test

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/dem/internal/rawfunc"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/query"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/rumfunc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordingSource returns one row per read and records which reads ran.
type recordingSource struct{ reads *[]string }

func (s recordingSource) read(name string) { *s.reads = append(*s.reads, name) }

func (s recordingSource) Receiver() query.Receiver { s.read("Receiver"); return query.Receiver{} }
func (s recordingSource) Sites(context.Context, string) ([]query.Site, error) {
	s.read("Sites")
	return []query.Site{{Name: "shop"}}, nil
}
func (s recordingSource) Pages(context.Context, string, string) ([]query.Page, error) {
	s.read("Pages")
	return []query.Page{{Site: "shop"}}, nil
}
func (s recordingSource) Live(context.Context, string, string, int, string) ([]query.LiveEvent, string, error) {
	s.read("Live")
	return []query.LiveEvent{{}}, "", nil
}
func (s recordingSource) Sessions(context.Context, string, string, int64, int64, int, string) ([]query.Session, error) {
	s.read("Sessions")
	return []query.Session{{Site: "shop"}}, nil
}
func (s recordingSource) SessionEvents(context.Context, string, string, string) ([]query.SessionEvent, error) {
	s.read("SessionEvents")
	return []query.SessionEvent{{Site: "shop"}}, nil
}
func (s recordingSource) Errors(context.Context, string, string, int64, int64, string) ([]query.ErrorGroup, error) {
	s.read("Errors")
	return []query.ErrorGroup{{Site: "shop"}}, nil
}

func TestDataRequiresMemberPermissionsAndInfoDoesNot(t *testing.T) {
	for _, declaration := range rumfunc.Declarations() {
		var args []string
		if declaration.ID == "rum-session-events" {
			args = []string{"session_id:visit"}
		}
		t.Run(declaration.ID, func(t *testing.T) {
			for name, permissions := range map[string]string{
				"none":           "",
				"malformed":      "member",
				"without member": "0x17",
			} {
				t.Run(name, func(t *testing.T) {
					var reads []string
					handler := rumfunc.New(recordingSource{&reads}, nil)
					denied := funcapi.ErrorResponse(403, "member permissions are required")
					for _, requestArgs := range [][]string{
						args,
						append([]string{"unsupported:1"}, args...),
						append([]string{"vnode:a", "vnode_guid:"}, args...),
					} {
						response := handler.HandleRaw(context.Background(), funcapi.RawMethodRequest{
							Method:      declaration.ID,
							Args:        requestArgs,
							Permissions: permissions,
						})
						assert.Equal(t, denied, response, "checked before arguments: %v", requestArgs)
					}
					assert.Empty(t, reads, "nothing is read without member permissions")

					info := handler.HandleRaw(context.Background(), funcapi.RawMethodRequest{
						Method:      declaration.ID,
						Info:        true,
						Args:        append([]string{"info"}, args...),
						Permissions: permissions,
					})
					assert.Equal(t, 200, info.Status)
					assert.NotEmpty(t, info.Help)
				})
			}
			for _, permissions := range []string{"0x1b", "0x7b"} {
				var reads []string
				handler := rumfunc.New(recordingSource{&reads}, nil)
				response := handler.HandleRaw(context.Background(), funcapi.RawMethodRequest{
					Method:      declaration.ID,
					Args:        args,
					Permissions: permissions,
				})
				require.NotNil(t, response.RawResponse, permissions)
				assert.Equal(t, 200, response.RawResponse["status"])
				assert.Len(t, response.RawResponse["data"], 1)
				unsupported := handler.HandleRaw(context.Background(), funcapi.RawMethodRequest{
					Method:      declaration.ID,
					Args:        append([]string{"unsupported:1"}, args...),
					Permissions: permissions,
				})
				assert.Equal(t, funcapi.ErrorResponse(400, `unsupported argument "unsupported"`), unsupported)
			}
		})
	}
}

func TestCancellationStatuses(t *testing.T) {
	expired, cancelExpired := context.WithDeadline(context.Background(), time.Unix(0, 0))
	defer cancelExpired()
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	for name, tc := range map[string]struct {
		ctx  context.Context
		want *funcapi.FunctionResponse
	}{
		"deadline": {ctx: expired, want: funcapi.ErrorResponse(504, "request timed out")},
		"cancel":   {ctx: cancelled, want: funcapi.ErrorResponse(499, "request cancelled")},
	} {
		t.Run(name, func(t *testing.T) {
			for _, declaration := range rumfunc.Declarations() {
				var args []string
				if declaration.ID == "rum-session-events" {
					args = []string{"session_id:visit"}
				}
				var reads []string
				handler := rumfunc.New(recordingSource{&reads}, nil)
				response := handler.HandleRaw(tc.ctx, funcapi.RawMethodRequest{
					Method:      declaration.ID,
					Args:        args,
					Permissions: "0x1b",
				})
				assert.Equal(t, tc.want, response, declaration.ID)
				assert.NotContains(t, reads, rowReads[declaration.ID], "%s rows are not read", declaration.ID)
			}
		})
	}
}

// rowReads names the source read that serves each method's rows.
var rowReads = map[string]string{
	"rum-sites":          "Receiver",
	"rum-pages":          "Pages",
	"rum-live":           "Live",
	"rum-sessions":       "Sessions",
	"rum-errors":         "Errors",
	"rum-session-events": "SessionEvents",
}

// sessionsSource holds n sessions and returns at most the requested limit, newest first.
type sessionsSource struct {
	recordingSource
	n int
}

func (s sessionsSource) Sessions(
	_ context.Context,
	_, _ string,
	_, _ int64,
	limit int,
	_ string,
) ([]query.Session, error) {
	out := make([]query.Session, 0, min(s.n, limit))
	for i := range cap(out) {
		out = append(out, query.Session{
			Site:      "shop",
			SessionID: strconv.Itoa(i),
		})
	}
	return out, nil
}

func TestSessionsDiscloseTruncationAtTheRowCap(t *testing.T) {
	for name, tc := range map[string]struct {
		stored    int
		rows      int
		truncated bool
	}{
		"below the cap": {stored: rawfunc.MaxRows - 1, rows: rawfunc.MaxRows - 1},
		"at the cap":    {stored: rawfunc.MaxRows, rows: rawfunc.MaxRows},
		"over the cap":  {stored: rawfunc.MaxRows + 1, rows: rawfunc.MaxRows, truncated: true},
	} {
		t.Run(name, func(t *testing.T) {
			var reads []string
			handler := rumfunc.New(sessionsSource{
				recordingSource: recordingSource{&reads},
				n:               tc.stored,
			}, nil)
			response := handler.HandleRaw(context.Background(), funcapi.RawMethodRequest{
				Method:      "rum-sessions",
				Permissions: "0x1b",
			}).RawResponse
			require.NotNil(t, response)
			rows := response["data"].([][]any)
			require.Len(t, rows, tc.rows)
			assert.Equal(t, "0", rows[0][1], "the newest session stays first")
			assert.Equal(t, tc.truncated, response["truncated"])
			assert.Equal(t, rawfunc.MaxRows, response["limit"])
			assert.Contains(t, response["help"], "capped at "+strconv.Itoa(rawfunc.MaxRows)+" and disclose truncation")
		})
	}
}
