// SPDX-License-Identifier: GPL-3.0-or-later

// Package rawfunc is the request and response plumbing shared by the DEM
// raw-request Functions: access checks, strict arguments, vnode selectors,
// cancellation, the table response and its ordered column definition. Each
// handler keeps its own check order; the orders differ observably.
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

// Permitted reports whether the request's access mask holds every required bit.
func Permitted(req funcapi.RawMethodRequest, required uint64) bool {
	mask, err := strconv.ParseUint(req.Permissions, 0, 64)
	return err == nil && mask&required == required
}

// RequireMember returns a 403 response unless the request has member access.
// Every DEM Function requires it for data; info is metadata and needs none.
func RequireMember(req funcapi.RawMethodRequest) *funcapi.FunctionResponse {
	if Permitted(req, MemberAccess) {
		return nil
	}
	return funcapi.ErrorResponse(403, "member permissions are required")
}

// NoPayload rejects a request body sent to a read-only Function.
func NoPayload(req funcapi.RawMethodRequest) *funcapi.FunctionResponse {
	if len(req.Payload) == 0 {
		return nil
	}
	return funcapi.ErrorResponse(400, "this read-only Function does not accept a payload")
}

// ParseArgs reads key:value arguments strictly: every key must be accepted and
// appear once. An info request may also carry the bare word info.
func ParseArgs(req funcapi.RawMethodRequest, accepted []string) (map[string]string, error) {
	args := make(map[string]string, len(req.Args))
	for _, word := range req.Args {
		if req.Info && word == "info" {
			continue
		}
		key, value, ok := strings.Cut(word, ":")
		if !ok {
			return nil, errors.New("expected key:value argument")
		}
		if !slices.Contains(accepted, key) {
			return nil, fmt.Errorf("unsupported argument %q", key)
		}
		if _, exists := args[key]; exists {
			return nil, fmt.Errorf("duplicate argument %q", key)
		}
		args[key] = value
	}
	return args, nil
}

// ResolveVnode sets args["vnode_guid"] from the vnode or vnode_guid selector: a
// configured name resolved by lookup, a recorded GUID, or empty for all. Name
// lookup is request-local; retained evidence is never renamed or reassigned.
func ResolveVnode(args map[string]string, lookup func(string) (string, bool)) error {
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

// Cancelled maps request cancellation or deadline expiry to status 499 and
// returns nil for any other error.
func Cancelled(err error) *funcapi.FunctionResponse {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return funcapi.ErrorResponse(499, "request cancelled")
	}
	return nil
}
