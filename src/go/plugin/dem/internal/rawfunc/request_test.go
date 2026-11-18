// SPDX-License-Identifier: GPL-3.0-or-later

package rawfunc

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPermitted(t *testing.T) {
	for name, tc := range map[string]struct {
		permissions string
		member      bool
		admin       bool
	}{
		"none":            {permissions: ""},
		"malformed":       {permissions: "member"},
		"partial member":  {permissions: "0x13"},
		"member hex":      {permissions: "0x1b", member: true},
		"member decimal":  {permissions: "27", member: true},
		"member superset": {permissions: "0x1f", member: true},
		"admin":           {permissions: "0x7b", member: true, admin: true},
		"all":             {permissions: "0xffff", member: true, admin: true},
	} {
		t.Run(name, func(t *testing.T) {
			req := funcapi.RawMethodRequest{
				Permissions: tc.permissions,
			}
			assert.Equal(t, tc.member, Permitted(req, MemberAccess))
			assert.Equal(t, tc.admin, Permitted(req, AdminAccess))
		})
	}
}

func TestAdmit(t *testing.T) {
	const guid = "11111111-1111-4111-8111-111111111111"
	accepted := []string{"site", "after", "vnode", "vnode_guid"}
	lookup := func(name string) (string, bool) { return guid, name == "storefront" }
	member := func(req funcapi.RawMethodRequest) funcapi.RawMethodRequest {
		req.Permissions = "0x1b"
		return req
	}
	all := func(args map[string]string) map[string]string {
		args["vnode_guid"] = ""
		return args
	}
	for name, tc := range map[string]struct {
		req  funcapi.RawMethodRequest
		want map[string]string
		deny *funcapi.FunctionResponse
	}{
		"no input": {
			req:  member(funcapi.RawMethodRequest{}),
			want: all(map[string]string{}),
		},
		"arguments": {
			req: member(funcapi.RawMethodRequest{
				Args: []string{"site:a:b", "after:-60"},
			}),
			want: all(map[string]string{"site": "a:b", "after": "-60"}),
		},
		"data without member access": {
			req: funcapi.RawMethodRequest{
				Args:        []string{"unsupported:1"},
				Permissions: "0x17",
			},
			deny: funcapi.ErrorResponse(403, "member permissions are required"),
		},
		"info without permissions": {
			req: funcapi.RawMethodRequest{
				Info: true,
				Args: []string{"info", "site:shop"},
			},
			want: all(map[string]string{"site": "shop"}),
		},
		"Cloud UI table body": {
			req: member(funcapi.RawMethodRequest{
				Payload: []byte(`{"site":"shop","after":1700000000,"query":"","selections":{},"timeout":120000,"last":200,"_auxiliary":{"page":1}}`),
			}),
			want: all(map[string]string{"site": "shop", "after": "1700000000"}),
		},
		"Cloud UI info window on info": {
			req: funcapi.RawMethodRequest{
				Info: true,
				Args: []string{"info", "after:1700000000", "before:1700000900"},
			},
			want: all(map[string]string{"after": "1700000000"}),
		},
		"unaccepted before on data": {
			req: member(funcapi.RawMethodRequest{
				Args: []string{"before:1700000900"},
			}),
			deny: funcapi.ErrorResponse(400, `unsupported argument "before"`),
		},
		"selection": {
			req: member(funcapi.RawMethodRequest{
				Payload: []byte(`{"selections":{"site":["shop"]}}`),
			}),
			want: all(map[string]string{"site": "shop"}),
		},
		"arguments, then selections, then payload": {
			req: member(funcapi.RawMethodRequest{
				Args:    []string{"site:argument"},
				Payload: []byte(`{"site":"payload","after":-60,"selections":{"site":["selection"],"after":["-30"]}}`),
			}),
			want: all(map[string]string{"site": "argument", "after": "-30"}),
		},
		"last duplicate argument wins": {
			req: member(funcapi.RawMethodRequest{
				Args: []string{"site:a", "site=b"},
			}),
			want: all(map[string]string{"site": "b"}),
		},
		"word without a separator": {
			req: member(funcapi.RawMethodRequest{
				Args: []string{"shop"},
			}),
			deny: funcapi.ErrorResponse(400, "expected key:value argument"),
		},
		"word without a separator on info": {
			req: funcapi.RawMethodRequest{
				Info: true,
				Args: []string{"info", "shop"},
			},
			deny: funcapi.ErrorResponse(400, "expected key:value argument"),
		},
		"info word on data": {
			req: member(funcapi.RawMethodRequest{
				Args: []string{"info"},
			}),
			deny: funcapi.ErrorResponse(400, "expected key:value argument"),
		},
		"unsupported argument": {
			req: member(funcapi.RawMethodRequest{
				Args: []string{"limit:1"},
			}),
			deny: funcapi.ErrorResponse(400, `unsupported argument "limit"`),
		},
		"unsupported payload field": {
			req: member(funcapi.RawMethodRequest{
				Payload: []byte(`{"limit":1}`),
			}),
			deny: funcapi.ErrorResponse(400, `unsupported argument "limit"`),
		},
		"unsupported selection": {
			req: member(funcapi.RawMethodRequest{
				Payload: []byte(`{"selections":{"kind":["journey"]}}`),
			}),
			deny: funcapi.ErrorResponse(400, `unsupported argument "kind"`),
		},
		"search query": {
			req: member(funcapi.RawMethodRequest{
				Payload: []byte(`{"query":"checkout"}`),
			}),
			deny: funcapi.ErrorResponse(400, `unsupported argument "query"`),
		},
		"several values": {
			req: member(funcapi.RawMethodRequest{
				Payload: []byte(`{"site":["a","b"]}`),
			}),
			deny: funcapi.ErrorResponse(400, "site requires one string or number value"),
		},
		"payload not an object": {
			req: member(funcapi.RawMethodRequest{
				Payload: []byte(`[]`),
			}),
			deny: funcapi.ErrorResponse(400, "invalid payload: expected a JSON object"),
		},
		"payload on info": {
			req: funcapi.RawMethodRequest{
				Info:    true,
				Args:    []string{"info"},
				Payload: []byte(`{"site":"shop","timeout":120000}`),
			},
			want: all(map[string]string{"site": "shop"}),
		},
		"vnode name": {
			req: member(funcapi.RawMethodRequest{
				Args: []string{"vnode:storefront"},
			}),
			want: map[string]string{"vnode": "storefront", "vnode_guid": guid},
		},
		"unknown vnode": {
			req: member(funcapi.RawMethodRequest{
				Payload: []byte(`{"vnode":"removed"}`),
			}),
			deny: funcapi.ErrorResponse(400, `unknown configured vnode "removed"`),
		},
		"unknown vnode on info": {
			req: funcapi.RawMethodRequest{
				Info: true,
				Args: []string{"info", "vnode:removed"},
			},
			deny: funcapi.ErrorResponse(400, `unknown configured vnode "removed"`),
		},
	} {
		t.Run(name, func(t *testing.T) {
			got, deny := Admit(tc.req, accepted, lookup, false)
			assert.Equal(t, tc.deny, deny)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestResolveVnode(t *testing.T) {
	const guid = "11111111-1111-4111-8111-111111111111"
	for name, tc := range map[string]struct {
		args  map[string]string
		want  string
		err   string
		calls int
	}{
		"all":                        {args: map[string]string{}},
		"empty GUID retains all":     {args: map[string]string{"vnode_guid": ""}},
		"name":                       {args: map[string]string{"vnode": "storefront"}, want: guid, calls: 1},
		"GUID without configuration": {args: map[string]string{"vnode_guid": guid}, want: guid},
		"unknown":                    {args: map[string]string{"vnode": "removed"}, err: `unknown configured vnode "removed"`, calls: 1},
		"empty name":                 {args: map[string]string{"vnode": ""}, err: "vnode must be a configured vnode name"},
		"both":                       {args: map[string]string{"vnode": "storefront", "vnode_guid": guid}, err: "vnode and vnode_guid are mutually exclusive"},
		"both empty":                 {args: map[string]string{"vnode": "", "vnode_guid": ""}, err: "vnode and vnode_guid are mutually exclusive"},
		"invalid GUID":               {args: map[string]string{"vnode_guid": "invalid"}, err: "vnode_guid must be empty or a canonical UUID"},
	} {
		t.Run(name, func(t *testing.T) {
			calls := 0
			err := resolveVnode(tc.args, func(name string) (string, bool) {
				calls++
				return guid, name == "storefront"
			}, false)
			assert.Equal(t, tc.calls, calls)
			if tc.err != "" {
				require.EqualError(t, err, tc.err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, tc.args["vnode_guid"])
		})
	}
	err := resolveVnode(map[string]string{"vnode": "storefront"}, nil, false)
	require.EqualError(t, err, "configured vnode lookup is unavailable")
}

func TestCancelled(t *testing.T) {
	cancelled := funcapi.ErrorResponse(499, "request cancelled")
	timedOut := funcapi.ErrorResponse(504, "request timed out")
	for name, tc := range map[string]struct {
		err  error
		want *funcapi.FunctionResponse
	}{
		"nil":              {},
		"other":            {err: errors.New("disk")},
		"canceled":         {err: context.Canceled, want: cancelled},
		"deadline":         {err: context.DeadlineExceeded, want: timedOut},
		"wrapped canceled": {err: fmt.Errorf("read: %w", context.Canceled), want: cancelled},
		"wrapped deadline": {err: fmt.Errorf("read: %w", context.DeadlineExceeded), want: timedOut},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, Cancelled(tc.err))
		})
	}
}
