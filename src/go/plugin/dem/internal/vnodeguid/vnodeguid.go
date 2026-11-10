// SPDX-License-Identifier: GPL-3.0-or-later

// Package vnodeguid validates captured DEM vnode identities and query selectors.
package vnodeguid

import (
	"errors"
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
)

// Resolve selects the current configured identity by name or a recorded GUID.
// Name lookup is request-local; retained evidence is never renamed or reassigned.
func Resolve(args map[string]string, lookup func(string) (string, bool)) (string, error) {
	name, named := args["vnode"]
	guid, identified := args["vnode_guid"]
	if named && identified {
		return "", errors.New("vnode and vnode_guid are mutually exclusive")
	}
	if named {
		if name == "" {
			return "", errors.New("vnode must be a configured vnode name")
		}
		if lookup == nil {
			return "", errors.New("configured vnode lookup is unavailable")
		}
		var ok bool
		guid, ok = lookup(name)
		if !ok || guid == "" {
			return "", fmt.Errorf("unknown configured vnode %q", name)
		}
	}
	if err := Validate(guid); err != nil {
		return "", err
	}
	return guid, nil
}

// Validate accepts an empty identity for observations without a configured vnode.
func Validate(value string) error {
	if value == "" {
		return nil
	}
	canonical, err := vnodes.ConfiguredGUIDKey(value)
	if err != nil || canonical != value {
		return errors.New("vnode_guid must be empty or a canonical UUID")
	}
	return nil
}
