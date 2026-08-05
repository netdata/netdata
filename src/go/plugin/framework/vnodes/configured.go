// SPDX-License-Identifier: GPL-3.0-or-later

package vnodes

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/google/uuid"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartemit"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
)

// ValidateConfigured validates a vnode supplied by file configuration or
// DynCfg against every protocol consumer used for configured vnodes.
func ValidateConfigured(vnode *VirtualNode) error {
	_, err := validateConfigured(vnode)
	return err
}

func validateConfigured(vnode *VirtualNode) (string, error) {
	if vnode == nil {
		return "", fmt.Errorf("configured vnode is nil")
	}
	if vnode.Name == "" {
		return "", fmt.Errorf("configured vnode name is required")
	}
	if err := dyncfg.JobNameRuleAllowDots(vnode.Name); err != nil {
		return "", fmt.Errorf("invalid configured vnode name %q: %w", vnode.Name, err)
	}
	if !netdataapi.ValidBareProtocolField(vnode.SourceType) {
		return "", fmt.Errorf("configured vnode source type cannot be represented in the CONFIG protocol")
	}
	if !netdataapi.ValidSingleQuotedProtocolField(vnode.Source) {
		return "", fmt.Errorf("configured vnode source cannot be represented in the CONFIG protocol")
	}
	guidKey, err := ConfiguredGUIDKey(vnode.GUID)
	if err != nil {
		return "", err
	}
	prepared, err := chartemit.PrepareHostInfo(netdataapi.HostInfo{
		GUID:     vnode.GUID,
		Hostname: vnode.Hostname,
		Labels:   vnode.Labels,
	})
	if err != nil {
		return "", fmt.Errorf("invalid configured vnode host metadata: %w", err)
	}
	if prepared.Hostname != vnode.Hostname {
		return "", fmt.Errorf("configured vnode hostname changes during host emission preparation")
	}
	if !netdataapi.ValidSingleQuotedProtocolField(prepared.Hostname) {
		return "", fmt.Errorf("configured vnode hostname cannot be represented in the host protocol")
	}
	for key, value := range vnode.Labels {
		preparedValue, ok := prepared.Labels[key]
		if !ok {
			return "", fmt.Errorf("configured vnode label key %q changes during host emission preparation", key)
		}
		if preparedValue != value {
			return "", fmt.Errorf("configured vnode label value for %q changes during host emission preparation", key)
		}
	}
	for key, value := range prepared.Labels {
		if !netdataapi.ValidSingleQuotedProtocolField(key) ||
			!netdataapi.ValidSingleQuotedProtocolField(value) {
			return "", fmt.Errorf("configured vnode labels cannot be represented in the host protocol")
		}
	}
	return guidKey, nil
}

// ConfiguredGUIDKey returns the canonical comparison key for a configured
// vnode GUID while leaving the caller's spelling unchanged.
func ConfiguredGUIDKey(value string) (string, error) {
	switch len(value) {
	case 32, 36:
	default:
		return "", fmt.Errorf("configured vnode GUID must use canonical or compact UUID syntax")
	}
	parsed, err := uuid.Parse(value)
	if err != nil {
		return "", fmt.Errorf("invalid configured vnode GUID: %w", err)
	}
	canonical := parsed.String()
	switch len(value) {
	case 32:
		if !strings.EqualFold(value, strings.ReplaceAll(canonical, "-", "")) {
			return "", fmt.Errorf("configured vnode GUID must use compact UUID syntax")
		}
	case 36:
		if !strings.EqualFold(value, canonical) {
			return "", fmt.Errorf("configured vnode GUID must use canonical UUID syntax")
		}
	}
	return canonical, nil
}

// ValidateConfiguredSet validates configured vnodes and their set-wide
// hostname and GUID uniqueness deterministically.
func ValidateConfiguredSet(initial map[string]*VirtualNode) error {
	seenHostnames := make(map[string]string)
	seenGUIDs := make(map[string]string)
	for _, id := range slices.Sorted(maps.Keys(initial)) {
		vnode := initial[id]
		if vnode == nil {
			return fmt.Errorf("configured vnode %q is nil", id)
		}
		if vnode.Name != id {
			return fmt.Errorf("configured vnode %q identity differs from its map key", id)
		}
		guidKey, err := validateConfigured(vnode)
		if err != nil {
			return fmt.Errorf("configured vnode %q: %w", id, err)
		}
		if other, ok := seenHostnames[vnode.Hostname]; ok {
			return fmt.Errorf("duplicate configured vnode hostname %q (%s and %s)", vnode.Hostname, other, id)
		}
		if other, ok := seenGUIDs[guidKey]; ok {
			return fmt.Errorf("duplicate configured vnode GUID (%s and %s)", other, id)
		}
		seenHostnames[vnode.Hostname] = id
		seenGUIDs[guidKey] = id
	}
	return nil
}
