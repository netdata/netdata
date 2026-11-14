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
			req := funcapi.RawMethodRequest{Permissions: tc.permissions}
			assert.Equal(t, tc.member, Permitted(req, MemberAccess))
			assert.Equal(t, tc.admin, Permitted(req, AdminAccess))
			if tc.member {
				assert.Nil(t, RequireMember(req))
			} else {
				assert.Equal(t, funcapi.ErrorResponse(403, "member permissions are required"), RequireMember(req))
			}
		})
	}
}

func TestNoPayload(t *testing.T) {
	assert.Nil(t, NoPayload(funcapi.RawMethodRequest{}))
	assert.Equal(
		t,
		funcapi.ErrorResponse(400, "this read-only Function does not accept a payload"),
		NoPayload(funcapi.RawMethodRequest{Payload: []byte(`{}`)}),
	)
}

func TestParseArgs(t *testing.T) {
	accepted := []string{"site", "after"}
	for name, tc := range map[string]struct {
		req  funcapi.RawMethodRequest
		want map[string]string
		err  string
	}{
		"none":              {want: map[string]string{}},
		"accepted":          {req: funcapi.RawMethodRequest{Args: []string{"site:shop", "after:-60"}}, want: map[string]string{"site": "shop", "after": "-60"}},
		"empty value":       {req: funcapi.RawMethodRequest{Args: []string{"site:"}}, want: map[string]string{"site": ""}},
		"value with colon":  {req: funcapi.RawMethodRequest{Args: []string{"site:a:b"}}, want: map[string]string{"site": "a:b"}},
		"info word on info": {req: funcapi.RawMethodRequest{Info: true, Args: []string{"info", "site:shop"}}, want: map[string]string{"site": "shop"}},
		"info word on data": {req: funcapi.RawMethodRequest{Args: []string{"info"}}, err: "expected key:value argument"},
		"bare word":         {req: funcapi.RawMethodRequest{Args: []string{"shop"}}, err: "expected key:value argument"},
		"unsupported":       {req: funcapi.RawMethodRequest{Args: []string{"limit:1"}}, err: `unsupported argument "limit"`},
		"duplicate":         {req: funcapi.RawMethodRequest{Args: []string{"site:a", "site:b"}}, err: `duplicate argument "site"`},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := ParseArgs(tc.req, accepted)
			if tc.err != "" {
				require.EqualError(t, err, tc.err)
				return
			}
			require.NoError(t, err)
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
			err := ResolveVnode(tc.args, func(name string) (string, bool) {
				calls++
				return guid, name == "storefront"
			})
			assert.Equal(t, tc.calls, calls)
			if tc.err != "" {
				require.EqualError(t, err, tc.err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, tc.args["vnode_guid"])
		})
	}
	err := ResolveVnode(map[string]string{"vnode": "storefront"}, nil)
	require.EqualError(t, err, "configured vnode lookup is unavailable")
}

func TestCancelled(t *testing.T) {
	cancelled := funcapi.ErrorResponse(499, "request cancelled")
	for name, tc := range map[string]struct {
		err  error
		want *funcapi.FunctionResponse
	}{
		"nil":      {},
		"other":    {err: errors.New("disk")},
		"canceled": {err: context.Canceled, want: cancelled},
		"deadline": {err: context.DeadlineExceeded, want: cancelled},
		"wrapped":  {err: fmt.Errorf("read: %w", context.Canceled), want: cancelled},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, Cancelled(tc.err))
		})
	}
}
