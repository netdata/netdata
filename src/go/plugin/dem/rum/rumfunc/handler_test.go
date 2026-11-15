// SPDX-License-Identifier: GPL-3.0-or-later

package rumfunc_test

import (
	"context"
	"encoding/json"
	"slices"
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

// required holds the parameters a method needs before it reads rows.
var required = map[string]map[string]string{"rum-session-events": {"session_id": "visit"}}

// requiredArgs returns a method's required parameters as key:value arguments.
func requiredArgs(id string) []string {
	var args []string
	for key, value := range required[id] {
		args = append(args, key+":"+value)
	}
	return args
}

// cloudTableBody builds the body the Cloud UI's Function table posts for a data request: the accepted parameters with
// values, an empty search, no facet selections and its transport fields. The table also fills rum-live's after with
// its time window, which rum-live rejects as an invalid cursor; this body leaves it out.
func cloudTableBody(t *testing.T, declaration funcapi.FunctionConfig) []byte {
	body := map[string]any{"query": "", "selections": map[string]any{}, "timeout": 120000, "last": 200}
	for _, param := range declaration.AcceptedParams {
		switch {
		case param == "after" && declaration.ID != "rum-live":
			body["after"] = 1700000000
		case param == "before":
			body["before"] = 1700000900
		}
	}
	for key, value := range required[declaration.ID] {
		body[key] = value
	}
	data, err := json.Marshal(body)
	require.NoError(t, err)
	return data
}

func TestCloudTableRequests(t *testing.T) {
	for _, declaration := range rumfunc.Declarations() {
		t.Run(declaration.ID, func(t *testing.T) {
			var reads []string
			handler := rumfunc.New(recordingSource{&reads}, nil)
			info := handler.HandleRaw(context.Background(), funcapi.RawMethodRequest{
				Method: declaration.ID,
				Info:   true,
				Args: append(
					[]string{"info", "after:1700000000", "before:1700000900"},
					requiredArgs(declaration.ID)...),
			})
			assert.Equal(t, 200, info.Status, info.Message)
			assert.NotEmpty(t, info.Help)
			assert.NotContains(t, reads, rowReads[declaration.ID], "info reads no rows")

			body := cloudTableBody(t, declaration)
			info = handler.HandleRaw(context.Background(), funcapi.RawMethodRequest{
				Method:  declaration.ID,
				Info:    true,
				Args:    []string{"info"},
				Payload: body,
			})
			assert.Equal(t, 200, info.Status, info.Message)

			data := handler.HandleRaw(context.Background(), funcapi.RawMethodRequest{
				Method:      declaration.ID,
				Payload:     body,
				Permissions: "0x1b",
			})
			require.NotNil(t, data.RawResponse, data.Message)
			assert.Equal(t, 200, data.RawResponse["status"])
			assert.Len(t, data.RawResponse["data"], 1)
			assert.Contains(t, reads, rowReads[declaration.ID])
		})
	}
}

// rangeSource records the time range of a sessions read.
type rangeSource struct {
	recordingSource
	after, before *int64
}

func (s rangeSource) Sessions(
	_ context.Context,
	_, _ string,
	after, before int64,
	_ int,
	_ string,
) ([]query.Session, error) {
	*s.after, *s.before = after, before
	return nil, nil
}

func TestCloudTableTimeWindowSelectsTheRange(t *testing.T) {
	var reads []string
	var after, before int64
	handler := rumfunc.New(rangeSource{
		recordingSource: recordingSource{&reads},
		after:           &after,
		before:          &before,
	}, nil)
	response := handler.HandleRaw(context.Background(), funcapi.RawMethodRequest{
		Method: "rum-sessions",
		Payload: []byte(
			`{"after":1700000000,"before":1700000900,"query":"","selections":{},"timeout":120000,"last":200}`,
		),
		Permissions: "0x1b",
	})
	require.NotNil(t, response.RawResponse, response.Message)
	assert.Equal(t, [2]int64{1700000000, 1700000900}, [2]int64{after, before})
}

func TestRejectedRequestsReadNothing(t *testing.T) {
	unknownVnode := func(string) (string, bool) { return "", false }
	for name, tc := range map[string]struct {
		req  funcapi.RawMethodRequest
		want *funcapi.FunctionResponse
	}{
		"data without member access": {
			req: funcapi.RawMethodRequest{
				Permissions: "0x17",
			},
			want: funcapi.ErrorResponse(403, "member permissions are required"),
		},
		"unsupported argument": {
			req: funcapi.RawMethodRequest{
				Args:        []string{"limit:1"},
				Permissions: "0x1b",
			},
			want: funcapi.ErrorResponse(400, `unsupported argument "limit"`),
		},
		"unsupported payload field": {
			req: funcapi.RawMethodRequest{
				Payload:     []byte(`{"limit":1,"timeout":120000}`),
				Permissions: "0x1b",
			},
			want: funcapi.ErrorResponse(400, `unsupported argument "limit"`),
		},
		"unsupported payload field on info": {
			req: funcapi.RawMethodRequest{
				Info:    true,
				Args:    []string{"info"},
				Payload: []byte(`{"limit":1}`),
			},
			want: funcapi.ErrorResponse(400, `unsupported argument "limit"`),
		},
		"search query": {
			req: funcapi.RawMethodRequest{
				Payload:     []byte(`{"query":"checkout"}`),
				Permissions: "0x1b",
			},
			want: funcapi.ErrorResponse(400, `unsupported argument "query"`),
		},
		"payload not an object": {
			req: funcapi.RawMethodRequest{
				Payload:     []byte(`[]`),
				Permissions: "0x1b",
			},
			want: funcapi.ErrorResponse(400, "invalid payload: expected a JSON object"),
		},
		"unknown vnode": {
			req: funcapi.RawMethodRequest{
				Payload:     []byte(`{"vnode":"removed"}`),
				Permissions: "0x1b",
			},
			want: funcapi.ErrorResponse(400, `unknown configured vnode "removed"`),
		},
		"unknown vnode on info": {
			req: funcapi.RawMethodRequest{
				Info: true,
				Args: []string{"info", "vnode:removed"},
			},
			want: funcapi.ErrorResponse(400, `unknown configured vnode "removed"`),
		},
	} {
		t.Run(name, func(t *testing.T) {
			for _, declaration := range rumfunc.Declarations() {
				var reads []string
				handler := rumfunc.New(recordingSource{&reads}, unknownVnode)
				req := tc.req
				req.Method = declaration.ID
				req.Args = append(slices.Clone(req.Args), requiredArgs(declaration.ID)...)
				assert.Equal(t, tc.want, handler.HandleRaw(context.Background(), req), declaration.ID)
				assert.Empty(t, reads, "%s reads nothing", declaration.ID)
			}
		})
	}
}

// The Cloud UI table posts a column filter as a selection named after the column, so a filter must name an
// accepted parameter.
func TestColumnFiltersNameAcceptedParameters(t *testing.T) {
	for _, declaration := range rumfunc.Declarations() {
		var reads []string
		response := rumfunc.New(recordingSource{&reads}, nil).HandleRaw(context.Background(), funcapi.RawMethodRequest{
			Method:      declaration.ID,
			Args:        requiredArgs(declaration.ID),
			Permissions: "0x1b",
		})
		require.NotNil(t, response.RawResponse, response.Message)
		for name, column := range response.RawResponse["columns"].(map[string]any) {
			if filter := column.(map[string]any)["filter"]; filter != "none" {
				assert.Contains(
					t,
					declaration.AcceptedParams,
					name,
					"%s column %s has filter %v",
					declaration.ID,
					name,
					filter,
				)
			}
		}
	}
}
