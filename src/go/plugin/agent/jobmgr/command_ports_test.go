// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	functionwire "github.com/netdata/netdata/go/plugins/plugin/framework/functions"
	"github.com/stretchr/testify/assert"
)

func TestRequestValidate(t *testing.T) {
	valid := Request{
		UID:     "request-1",
		LaneKey: "job:a",
		Source:  lifecycle.SourceJobManager,
		Route:   "job/install",
	}

	tests := map[string]struct {
		request Request
		wantErr bool
	}{
		"no payload": {
			request: valid,
		},
		"Function scheduling comes after ingress": {
			request: Request{
				UID: "function-1", Source: lifecycle.SourceFunction, Route: "function",
			},
		},
		"explicit empty payload": {
			request: func() Request {
				request := valid
				request.HasPayload = true
				return request
			}(),
		},
		"payload": {
			request: func() Request {
				request := valid
				request.HasPayload = true
				request.Payload = []byte("{}")
				return request
			}(),
		},
		"missing identity": {
			request: func() Request {
				request := valid
				request.UID = ""
				return request
			}(),
			wantErr: true,
		},
		"missing Job Manager lane": {
			request: func() Request {
				request := valid
				request.LaneKey = ""
				return request
			}(),
			wantErr: true,
		},
		"Function ingress lane": {
			request: Request{
				UID: "function-1", LaneKey: "ingress", Source: lifecycle.SourceFunction, Route: "function",
			},
			wantErr: true,
		},
		"payload without presence marker": {
			request: func() Request {
				request := valid
				request.Payload = []byte("{}")
				return request
			}(),
			wantErr: true,
		},
		"unused payload capacity": {
			request: func() Request {
				request := valid
				request.Payload = make([]byte, 0, 8)
				return request
			}(),
		},
		"unsafe UID": {
			request: func() Request {
				request := valid
				request.UID = "request\nQUIT"
				return request
			}(),
			wantErr: true,
		},
		"overlong UID": {
			request: func() Request {
				request := valid
				request.UID = strings.Repeat("u", lifecycle.MaximumUIDBytes+1)
				return request
			}(),
			wantErr: true,
		},
		"arguments beyond former count limit": {
			request: func() Request {
				request := valid
				request.Args = make([]string, 1_025)
				return request
			}(),
		},
		"oversized arguments": {
			request: func() Request {
				request := valid
				request.Args = []string{strings.Repeat("a", maximumRequestArgumentBytes+1)}
				return request
			}(),
			wantErr: true,
		},
		"oversized lane": {
			request: func() Request {
				request := valid
				request.LaneKey = strings.Repeat("l", maximumRequestMetadataBytes+1)
				return request
			}(),
			wantErr: true,
		},
		"oversized route": {
			request: func() Request {
				request := valid
				request.Route = strings.Repeat("r", maximumRequestMetadataBytes+1)
				return request
			}(),
			wantErr: true,
		},
		"oversized content type": {
			request: func() Request {
				request := valid
				request.ContentType = strings.Repeat("c", maximumRequestMetadataBytes+1)
				return request
			}(),
			wantErr: true,
		},
		"oversized payload": {
			request: func() Request {
				request := valid
				request.HasPayload = true
				request.Payload = make([]byte, functionwire.MaximumInputBodyBytes+1)
				return request
			}(),
			wantErr: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.wantErr, tc.request.Validate() != nil)
		})
	}
}
