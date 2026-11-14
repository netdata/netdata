// SPDX-License-Identifier: GPL-3.0-or-later

// Package runtime owns DEM's prepared browser runtimes and their lifetime.
package runtime

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"path"
	goruntime "runtime"
	"slices"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
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

// Identity names this release in run results, qualification and status.
func (m Manifest) Identity() synthetic.RuntimeIdentity {
	return synthetic.RuntimeIdentity{Version: m.Version, SHA256: m.Archive.SHA256, Contract: m.Contract}
}

func (m Manifest) Validate() error {
	if m.Format != 1 || m.Version == "" || !supportedPlatform(m.Platform) || !validID(m.Archive.SHA256) {
		return errors.New("invalid runtime identity or platform")
	}
	if m.Contract != synthetic.ExecutionContract {
		return fmt.Errorf("unsupported runtime execution contract %d (Agent supports %d); upgrade the Agent or select a compatible runtime release", m.Contract, synthetic.ExecutionContract)
	}
	a := m.Archive
	if a.Bytes <= 0 || a.TarBytes <= 0 || a.ExpandedBytes <= 0 || a.ExpandedBytes > a.TarBytes || a.Entries <= 0 || a.Bytes == math.MaxInt64 || a.TarBytes == math.MaxInt64 {
		return errors.New("invalid runtime archive bounds")
	}
	// Offline-only manifests may omit a URL; they retain the same digest checks.
	if a.URL != "" && !ValidHTTPSURL(a.URL) {
		return errors.New("runtime download requires an installed HTTPS URL without credentials")
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

// ValidHTTPSURL is the rule for runtime archive, source and release discovery
// URLs: absolute HTTPS with a host, without credentials or a fragment.
func ValidHTTPSURL(address string) bool {
	u, err := url.Parse(address)
	return err == nil && u.Scheme == "https" && u.Host != "" && u.User == nil && u.Fragment == ""
}

// SupportedHost reports whether this process can run browser runtimes at all.
func SupportedHost() bool {
	return goruntime.GOOS == "linux" && supportedPlatform(hostPlatform())
}

// MatchesHost reports whether m was built for this process's architecture.
func (m Manifest) MatchesHost() bool { return m.Platform == hostPlatform() }

// hostPlatform names the release platform for this architecture; runtime
// releases exist only for Linux.
func hostPlatform() string { return "linux/" + goruntime.GOARCH }

func supportedPlatform(platform string) bool {
	return platform == "linux/amd64" || platform == "linux/arm64"
}

// clone returns a copy that shares no binding bytes with m. Accessors hand out
// clones, so a caller cannot change a manifest that a candidate later selects.
func (m Manifest) clone() Manifest {
	m.Bindings = slices.Clone(m.Bindings)
	for i := range m.Bindings {
		m.Bindings[i].Expected = bytes.Clone(m.Bindings[i].Expected)
	}
	return m
}

// sameManifest compares every signed field, including the archive URL.
func sameManifest(a, b Manifest) bool { return sameJSON(a, b) }

// sameExecutable ignores the archive URL: mirrors may change transport without
// changing executable identity.
func sameExecutable(a, b Manifest) bool {
	a.Archive.URL, b.Archive.URL = "", ""
	return sameJSON(a, b)
}

// sameJSON compares values in the JSON form in which manifests and releases are
// signed and persisted. A value that cannot be encoded never compares equal.
func sameJSON(a, b any) bool {
	left, errA := json.Marshal(a)
	right, errB := json.Marshal(b)
	return errA == nil && errB == nil && bytes.Equal(left, right)
}
