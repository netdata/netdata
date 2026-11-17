// SPDX-License-Identifier: GPL-3.0-or-later

package browserruntime

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// The same generated release-info file travels through source archives, the
// portable runtime and native payloads. Keep its fixed grammar in sync with
// packaging/release_info.py and NetdataVersion.cmake.
const releaseInfoName = "release-info"

func readPayloadAgent(root *os.Root, m Manifest) (AgentRelease, error) {
	var agent AgentRelease
	info, err := lstatPlain(root, releaseInfoName)
	if err != nil {
		return agent, err
	}
	if !info.Mode().IsRegular() {
		return agent, payloadLayoutError("runtime release-info is not a regular file")
	}
	f, err := root.Open(releaseInfoName)
	if err != nil {
		return agent, err
	}
	defer f.Close()
	// Three validated fields fit within 256 bytes (version is at most 128).
	data, err := io.ReadAll(io.LimitReader(f, 257))
	if err != nil {
		return agent, err
	}
	lines := strings.Split(string(data), "\n")
	if len(data) > 256 || len(lines) != 4 || lines[3] != "" ||
		!strings.HasPrefix(lines[0], "version=") || !strings.HasPrefix(lines[1], "channel=") ||
		!strings.HasPrefix(lines[2], "source=") {
		return agent, payloadLayoutError("invalid runtime release-info")
	}
	agent = AgentRelease{
		Version: strings.TrimPrefix(lines[0], "version="),
		Channel: strings.TrimPrefix(lines[1], "channel="),
		Source:  strings.TrimPrefix(lines[2], "source="),
	}
	if err := agent.Validate(); err != nil {
		return agent, payloadLayoutError("invalid runtime release-info: " + err.Error())
	}
	if agent.Version != m.Version {
		return agent, payloadLayoutError("runtime release-info version differs from its manifest")
	}
	return agent, nil
}

func checkPayloadAgent(root *os.Root, m Manifest, expected *AgentRelease) error {
	if expected != nil {
		if err := expected.Validate(); err != nil {
			return fmt.Errorf("expected Agent release: %w", err)
		}
	}
	agent, err := readPayloadAgent(root, m)
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%w: runtime has no paired Agent identity; install a matching Agent/runtime pair", ErrIncompatible)
	}
	if err != nil {
		return err
	}
	// Execution validates the payload's own provenance. Only installation owners
	// compare it with the Agent release being installed.
	if expected != nil && agent != *expected {
		return fmt.Errorf("%w: runtime version, channel or source differs from the Agent; install a matching Agent/runtime pair", ErrIncompatible)
	}
	return nil
}

// ValidateRelease checks an installation target against this caller's Agent.
// Execution uses Release.Validate; it does not require the caller's release stamp.
func (s *Store) ValidateRelease(release Release) error {
	if s.config.Agent == nil {
		return release.Validate()
	}
	return release.ValidateForAgent(*s.config.Agent)
}

func (s *Store) verifyPreparedAgent(m Manifest, expected *AgentRelease) (result error) {
	if expected == nil {
		return nil
	}
	root, err := s.root.OpenRoot(filepath.Join("generations", m.Archive.SHA256))
	if err != nil {
		return selectedPayloadError(err)
	}
	defer func() { result = errors.Join(result, root.Close()) }()
	agent, err := readPayloadAgent(root, m)
	if err != nil {
		return selectedPayloadError(err)
	}
	if agent != *expected {
		return damaged(errors.New("prepared runtime identity differs from its authenticated release"))
	}
	return nil
}
