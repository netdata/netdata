// SPDX-License-Identifier: GPL-3.0-or-later

// Package runtime owns DEM's prepared browser runtimes and their lifetime.
package runtime

import (
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"net/url"
	"path"
	"strings"
)

// Manifest is administrator-trusted release metadata. Archive-supplied metadata
// is never a trust anchor. Component versions belong to the release, not the Agent.
type Manifest struct {
	Format   int       `json:"format"`
	Contract int       `json:"contract"`
	Version  string    `json:"version"`
	Platform string    `json:"platform"`
	Archive  Archive   `json:"archive"`
	Layout   Layout    `json:"layout"`
	Bindings []Binding `json:"bindings"`
}

type Archive struct {
	URL           string `json:"url"`
	SHA256        string `json:"sha256"`
	Bytes         int64  `json:"bytes"`
	TarBytes      int64  `json:"tar_bytes"`
	ExpandedBytes int64  `json:"expanded_bytes"`
	Entries       int    `json:"entries"`
}

type Layout struct {
	EntryPoint   string `json:"entrypoint"`
	Node         string `json:"node"`
	Dependencies string `json:"dependencies"`
	Browser      string `json:"browser"`
	Loader       string `json:"loader"`
	Fonts        string `json:"fonts"`
}

// Binding permits one exact ELF interpreter-slot replacement. Expected is the
// full original slot, including padding. No install-time ELF rewriting occurs.
type Binding struct {
	Path     string `json:"path"`
	Offset   int64  `json:"offset"`
	Expected []byte `json:"expected"`
}

func validID(id string) bool {
	if len(id) != 64 || strings.ToLower(id) != id {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil
}

func localName(name string) bool {
	return name != "" && name != "." && path.Clean(name) == name &&
		!path.IsAbs(name) && name != ".." && !strings.HasPrefix(name, "../") &&
		!strings.ContainsAny(name, "\\\x00\r\n")
}

func (m Manifest) Validate() error {
	if m.Format != 1 || m.Version == "" || (m.Platform != "linux/amd64" && m.Platform != "linux/arm64") || !validID(m.Archive.SHA256) {
		return errors.New("invalid runtime identity or platform")
	}
	if m.Contract != 1 {
		return fmt.Errorf("unsupported runtime execution contract %d (Agent supports 1); upgrade the Agent or select a compatible runtime release", m.Contract)
	}
	a := m.Archive
	if a.Bytes <= 0 || a.TarBytes <= 0 || a.ExpandedBytes <= 0 || a.ExpandedBytes > a.TarBytes || a.Entries <= 0 || a.Bytes == math.MaxInt64 || a.TarBytes == math.MaxInt64 {
		return errors.New("invalid runtime archive bounds")
	}
	// Offline-only manifests may omit a URL; they retain the same digest checks.
	if a.URL != "" {
		u, err := url.Parse(a.URL)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Fragment != "" {
			return errors.New("runtime download requires an installed HTTPS URL without credentials")
		}
	}
	for _, name := range []string{m.Layout.EntryPoint, m.Layout.Node, m.Layout.Dependencies, m.Layout.Browser, m.Layout.Loader, m.Layout.Fonts} {
		if !localName(name) || reservedName(name) {
			return errors.New("invalid runtime layout path")
		}
	}
	seen := make(map[string]bool)
	for _, b := range m.Bindings {
		if !localName(b.Path) || reservedName(b.Path) || b.Offset < 0 || len(b.Expected) == 0 || seen[b.Path] {
			return errors.New("invalid or duplicate interpreter binding")
		}
		seen[b.Path] = true
		if b.Expected[len(b.Expected)-1] != 0 {
			return errors.New("interpreter slot must include its terminator")
		}
	}
	if !seen[m.Layout.Node] || !seen[m.Layout.Browser] {
		return fmt.Errorf("node and browser require authenticated interpreter bindings")
	}
	return nil
}

func reservedName(name string) bool {
	for _, reserved := range []string{"prepared.json", "native.json", "chrome", "fonts.conf"} {
		if name == reserved || strings.HasPrefix(name, reserved+"/") {
			return true
		}
	}
	return false
}
