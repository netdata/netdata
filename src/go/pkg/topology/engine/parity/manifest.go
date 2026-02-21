// SPDX-License-Identifier: GPL-3.0-or-later

package parity

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// ManifestVersion is the current topology parity manifest schema version.
const ManifestVersion = "v1"

// Manifest defines one or more topology parity scenarios.
type Manifest struct {
	Version   string             `yaml:"version"`
	Source    ManifestSource     `yaml:"source"`
	Scenarios []ManifestScenario `yaml:"scenarios"`
}

// ManifestSource captures provenance for imported parity fixtures.
type ManifestSource struct {
	Repo   string `yaml:"repo"`
	Commit string `yaml:"commit"`
	Path   string `yaml:"path"`
}

// ManifestProtocols declares protocol toggles for a scenario.
type ManifestProtocols struct {
	LLDP   bool `yaml:"lldp"`
	CDP    bool `yaml:"cdp"`
	Bridge bool `yaml:"bridge"`
	ARPND  bool `yaml:"arp_nd"`
}

// ManifestFixture describes one device fixture.
type ManifestFixture struct {
	DeviceID string            `yaml:"device_id"`
	Hostname string            `yaml:"hostname"`
	Address  string            `yaml:"address"`
	WalkFile string            `yaml:"walk_file"`
	Labels   map[string]string `yaml:"labels,omitempty"`
}

// ManifestScenario defines one parity scenario.
type ManifestScenario struct {
	ID          string            `yaml:"id"`
	Description string            `yaml:"description"`
	Protocols   ManifestProtocols `yaml:"protocols"`
	Fixtures    []ManifestFixture `yaml:"fixtures"`
	GoldenYAML  string            `yaml:"golden_yaml"`
	GoldenJSON  string            `yaml:"golden_json"`
}

// ResolvedScenario contains absolute paths resolved from a manifest file.
type ResolvedScenario struct {
	ID          string
	Description string
	Protocols   ManifestProtocols
	Fixtures    []ResolvedFixture
	GoldenYAML  string
	GoldenJSON  string
}

// ResolvedFixture is one resolved fixture path.
type ResolvedFixture struct {
	DeviceID string
	Hostname string
	Address  string
	WalkFile string
	Labels   map[string]string
}

// LoadManifest reads and validates a parity scenario manifest.
func LoadManifest(path string) (Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("read manifest %q: %w", path, err)
	}

	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return Manifest{}, fmt.Errorf("decode manifest %q: %w", path, err)
	}
	if err := m.validate(); err != nil {
		return Manifest{}, fmt.Errorf("validate manifest %q: %w", path, err)
	}
	return m, nil
}

// ResolveScenario resolves one scenario's relative file paths against the
// manifest location and validates that all referenced files exist.
func ResolveScenario(manifestPath string, scenario ManifestScenario) (ResolvedScenario, error) {
	if scenario.ID == "" {
		return ResolvedScenario{}, fmt.Errorf("scenario id is required")
	}

	baseDir := filepath.Dir(manifestPath)
	resolved := ResolvedScenario{
		ID:          scenario.ID,
		Description: scenario.Description,
		Protocols:   scenario.Protocols,
		Fixtures:    make([]ResolvedFixture, 0, len(scenario.Fixtures)),
		GoldenYAML:  resolvePath(baseDir, scenario.GoldenYAML),
		GoldenJSON:  resolvePath(baseDir, scenario.GoldenJSON),
	}

	if err := requireFile(resolved.GoldenYAML); err != nil {
		return ResolvedScenario{}, fmt.Errorf("scenario %q golden_yaml: %w", scenario.ID, err)
	}
	if err := requireFile(resolved.GoldenJSON); err != nil {
		return ResolvedScenario{}, fmt.Errorf("scenario %q golden_json: %w", scenario.ID, err)
	}

	seenDevice := make(map[string]struct{}, len(scenario.Fixtures))
	for _, fixture := range scenario.Fixtures {
		if fixture.DeviceID == "" {
			return ResolvedScenario{}, fmt.Errorf("scenario %q has fixture with empty device_id", scenario.ID)
		}
		if _, ok := seenDevice[fixture.DeviceID]; ok {
			return ResolvedScenario{}, fmt.Errorf("scenario %q has duplicate fixture device_id %q", scenario.ID, fixture.DeviceID)
		}
		seenDevice[fixture.DeviceID] = struct{}{}

		walkPath := resolvePath(baseDir, fixture.WalkFile)
		if err := requireFile(walkPath); err != nil {
			return ResolvedScenario{}, fmt.Errorf("scenario %q fixture %q walk_file: %w", scenario.ID, fixture.DeviceID, err)
		}

		resolved.Fixtures = append(resolved.Fixtures, ResolvedFixture{
			DeviceID: fixture.DeviceID,
			Hostname: fixture.Hostname,
			Address:  fixture.Address,
			WalkFile: walkPath,
			Labels:   copyStringMap(fixture.Labels),
		})
	}

	return resolved, nil
}

// FindScenario finds one scenario by ID.
func (m Manifest) FindScenario(id string) (ManifestScenario, bool) {
	for _, scenario := range m.Scenarios {
		if scenario.ID == id {
			return scenario, true
		}
	}
	return ManifestScenario{}, false
}

func (m Manifest) validate() error {
	if m.Version == "" {
		return fmt.Errorf("version is required")
	}
	if m.Version != ManifestVersion {
		return fmt.Errorf("unsupported version %q (want %q)", m.Version, ManifestVersion)
	}
	if len(m.Scenarios) == 0 {
		return fmt.Errorf("at least one scenario is required")
	}

	seenScenario := make(map[string]struct{}, len(m.Scenarios))
	for _, scenario := range m.Scenarios {
		if scenario.ID == "" {
			return fmt.Errorf("scenario id is required")
		}
		if _, ok := seenScenario[scenario.ID]; ok {
			return fmt.Errorf("duplicate scenario id %q", scenario.ID)
		}
		seenScenario[scenario.ID] = struct{}{}

		if scenario.GoldenYAML == "" || scenario.GoldenJSON == "" {
			return fmt.Errorf("scenario %q requires golden_yaml and golden_json", scenario.ID)
		}
		if len(scenario.Fixtures) == 0 {
			return fmt.Errorf("scenario %q requires at least one fixture", scenario.ID)
		}
		if !scenario.Protocols.LLDP && !scenario.Protocols.CDP && !scenario.Protocols.Bridge && !scenario.Protocols.ARPND {
			return fmt.Errorf("scenario %q must enable at least one protocol", scenario.ID)
		}

		seenDevice := make(map[string]struct{}, len(scenario.Fixtures))
		for _, fixture := range scenario.Fixtures {
			if fixture.DeviceID == "" {
				return fmt.Errorf("scenario %q has fixture with empty device_id", scenario.ID)
			}
			if strings.TrimSpace(fixture.WalkFile) == "" {
				return fmt.Errorf("scenario %q fixture %q requires walk_file", scenario.ID, fixture.DeviceID)
			}
			if _, ok := seenDevice[fixture.DeviceID]; ok {
				return fmt.Errorf("scenario %q has duplicate fixture device_id %q", scenario.ID, fixture.DeviceID)
			}
			seenDevice[fixture.DeviceID] = struct{}{}
		}
	}

	return nil
}

func requireFile(path string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("path is empty")
	}
	st, err := os.Stat(path)
	if err != nil {
		return err
	}
	if st.IsDir() {
		return fmt.Errorf("%q is a directory", path)
	}
	return nil
}

func resolvePath(baseDir, path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Clean(filepath.Join(baseDir, path))
}

func copyStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
