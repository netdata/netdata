// SPDX-License-Identifier: GPL-3.0-or-later

package cato_networks

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/pluginconfig"
)

type eventsMarkerStore struct {
	path string
}

func newEventsMarkerStore(configuredPath, accountID, endpoint, vnode string) *eventsMarkerStore {
	path := strings.TrimSpace(configuredPath)
	if path == "" {
		base := strings.TrimSpace(pluginconfig.VarLibDir())
		if base == "" {
			return nil
		}
		path = defaultEventsMarkerPath(base, accountID, endpoint, vnode)
	}
	return &eventsMarkerStore{path: path}
}

func defaultEventsMarkerPath(base, accountID, endpoint, vnode string) string {
	identity := strings.Join([]string{
		strings.TrimSpace(accountID),
		strings.TrimSpace(endpoint),
		strings.TrimSpace(vnode),
	}, "\x00")
	sum := sha256.Sum256([]byte(identity))
	return filepath.Join(base, "cato_networks", hex.EncodeToString(sum[:])[:16]+".events.marker")
}

func (s *eventsMarkerStore) read() (string, error) {
	if s == nil || s.path == "" {
		return "", nil
	}
	bs, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(bs)), nil
}

func (s *eventsMarkerStore) write(marker string) error {
	if s == nil || s.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o750); err != nil {
		return err
	}
	return os.WriteFile(s.path, []byte(strings.TrimSpace(marker)+"\n"), 0o600)
}
