// SPDX-License-Identifier: GPL-3.0-or-later

package rumfunc_test

import (
	"context"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
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
