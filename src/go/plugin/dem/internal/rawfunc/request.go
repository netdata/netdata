// SPDX-License-Identifier: GPL-3.0-or-later

// Package rawfunc is the request and response plumbing shared by the DEM
// raw-request Functions: request admission (access, strict parameters, vnode
// selectors), cancellation, the table response and its ordered column
// definition.
package rawfunc

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/dem/internal/vnodeguid"
)

// Access masks from src/libnetdata/user-auth/http-access.h.
const (
	// MemberAccess is HTTP_ACCESS_MAP_OLD_MEMBER: signed in, same Space,
	// anonymous and sensitive data.
	MemberAccess uint64 = 0x1b
	// AdminAccess is HTTP_ACCESS_MAP_OLD_ADMIN: member access plus viewing and
	// editing Agent configuration.
	AdminAccess uint64 = 0x7b
)

// MaxRows is the row bound of the DEM tables that cap their rows. The synthetic
// limit argument accepts 1 to MaxRows and its help states the cap; RUM live
// pages and session summaries use the same bound.
const MaxRows = 2000

// VnodeHelp states the vnode selector rule that resolveVnode enforces. It ends
// the help of every method that accepts vnode and vnode_guid.
const VnodeHelp = "For one virtual node, pass vnode (its configured name) or vnode_guid (its GUID, also one no " +
	"longer configured), not both."

// Permitted reports whether the request's access mask holds every required bit.
func Permitted(req funcapi.RawMethodRequest, required uint64) bool {
	mask, err := strconv.ParseUint(req.Permissions, 0, 64)
	return err == nil && mask&required == required
}

// Admit runs the request checks every DEM Function shares, in this order:
//  1. member access, unless the request is info (metadata needs none);
//  2. parameters, strictly: names from the key:value arguments, the payload
//     selections and the top-level payload fields (funcapi.ParseRawParams
//     precedence), each an accepted name with one string or number value; an
//     argument without a separator is rejected, except the word info on info;
//  3. the vnode or vnode_guid selector, resolved into args["vnode_guid"].
//
// Strict names allow what the Cloud UI's Function table sends with every
// request: its transport fields, an empty query, and on info an after or before
// the method does not accept (the table's time window).
func Admit(
	req funcapi.RawMethodRequest,
	accepted []string,
	lookup func(string) (string, bool),
) (map[string]string, *funcapi.FunctionResponse) {
	if !req.Info && !Permitted(req, MemberAccess) {
		return nil, funcapi.ErrorResponse(403, "member permissions are required")
	}
	args, err := parameters(req, accepted)
	if err != nil {
		return nil, funcapi.ErrorResponse(400, "%v", err)
	}
	if err := resolveVnode(args, lookup); err != nil {
		return nil, funcapi.ErrorResponse(400, "%v", err)
	}
	return args, nil
}

// transportFields are the Cloud UI table's request fields that carry no input.
var transportFields = []string{"selections", "timeout", "last", "_auxiliary"}

func parameters(req funcapi.RawMethodRequest, accepted []string) (map[string]string, error) {
	for _, arg := range req.Args {
		if !strings.ContainsAny(arg, ":=") && !(req.Info && arg == "info") {
			return nil, errors.New("expected key:value argument")
		}
	}
	params, err := funcapi.ParseRawParams(req)
	if err != nil {
		return nil, fmt.Errorf("invalid payload: %w", err)
	}
	args := make(map[string]string)
	for _, name := range params.Names() {
		value, err := params.Text(name)
		switch {
		case slices.Contains(accepted, name):
			if err != nil {
				return nil, err
			}
			args[name] = value
		case slices.Contains(transportFields, name),
			name == funcapi.ParamQuery && err == nil && strings.TrimSpace(value) == "",
			req.Info && (name == funcapi.ParamAfter || name == funcapi.ParamBefore):
		default:
			return nil, fmt.Errorf("unsupported argument %q", name)
		}
	}
	return args, nil
}

// resolveVnode sets args["vnode_guid"] from the vnode or vnode_guid selector: a
// configured name resolved by lookup, a recorded GUID, or empty for all. Name
// lookup is request-local; retained evidence is never renamed or reassigned.
func resolveVnode(args map[string]string, lookup func(string) (string, bool)) error {
	name, named := args["vnode"]
	guid, identified := args["vnode_guid"]
	if named && identified {
		return errors.New("vnode and vnode_guid are mutually exclusive")
	}
	if named {
		if name == "" {
			return errors.New("vnode must be a configured vnode name")
		}
		if lookup == nil {
			return errors.New("configured vnode lookup is unavailable")
		}
		var ok bool
		guid, ok = lookup(name)
		if !ok || guid == "" {
			return fmt.Errorf("unknown configured vnode %q", name)
		}
	}
	if err := vnodeguid.Validate(guid); err != nil {
		return err
	}
	args["vnode_guid"] = guid
	return nil
}

// Cancelled maps an expired deadline to status 504 and a cancellation to 499,
// and returns nil for any other error. When the Agent cancels a request or its
// deadline expires, the Function framework answers it with the same statuses
// and discards the handler's response.
func Cancelled(err error) *funcapi.FunctionResponse {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return funcapi.ErrorResponse(504, "request timed out")
	case errors.Is(err, context.Canceled):
		return funcapi.ErrorResponse(499, "request cancelled")
	default:
		return nil
	}
}
