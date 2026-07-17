// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
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
		"explicit empty payload": {
			request: func() Request {
				request := valid
				request.HasPayload = true
				return request
			}(),
		},
		"reserved payload": {
			request: func() Request {
				request := valid
				request.HasPayload = true
				request.InputBodyToken = 1
				request.Payload = make([]byte, 2, 8)
				request.PayloadCapacity = 8
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
		"payload without reservation": {
			request: func() Request {
				request := valid
				request.Payload = []byte("{}")
				return request
			}(),
			wantErr: true,
		},
		"retained payload capacity without reservation": {
			request: func() Request {
				request := valid
				request.Payload = make([]byte, 0, 8)
				return request
			}(),
			wantErr: true,
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
		"reservation capacity mismatch": {
			request: func() Request {
				request := valid
				request.HasPayload = true
				request.InputBodyToken = 1
				request.Payload = make([]byte, 2, 8)
				request.PayloadCapacity = 7
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
