// SPDX-License-Identifier: GPL-3.0-or-later

package promproof

import (
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"github.com/netdata/netdata/go/plugins/internal/promprofile/input"
	"github.com/netdata/netdata/go/plugins/internal/promprofile/semantics"
	"github.com/netdata/netdata/go/plugins/internal/promprofile/yaml"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/profilecatalog"
	"gopkg.in/yaml.v3"
)

const (
	DescriptorFilename = "proof.yaml"
	ProofRoot          = "src/go/plugin/go.d/collector/prometheus/profile-proofs"
	StockProfileRoot   = "src/go/plugin/go.d/config/go.d/prometheus.profiles/default"
	ProofVersion       = "v1"
)

// Descriptor contains only realizable replay inputs and independently
// authored outcomes. Semantic expectations live in the compiled contract.
type Descriptor struct {
	Version         string                           `yaml:"version"`
	Profile         string                           `yaml:"profile"`
	MetadataExample *prominput.MetadataExample       `yaml:"metadata_example,omitempty"`
	FutureInputs    map[string]prominput.FutureInput `yaml:"future_inputs,omitempty"`
	Cases           map[string]ProofCase             `yaml:"cases"`
}

type ProofCase struct {
	Environment  map[string]map[string]promsemantics.AxisValue `yaml:"environment"`
	Fixture      string                                        `yaml:"fixture,omitempty"`
	Steps        []ProofStep                                   `yaml:"steps,omitempty"`
	Coverage     *bool                                         `yaml:"coverage"`
	Expected     *ExpectedResult                               `yaml:"expected,omitempty"`
	Job          JobReference                                  `yaml:"job,omitempty"`
	FutureInputs map[string]prominput.FutureInput              `yaml:"future_inputs,omitempty"`
}

type ProofStep struct {
	Fixture      string                      `yaml:"fixture"`
	Expected     ExpectedResult              `yaml:"expected"`
	Observations map[string]ProofObservation `yaml:"observations,omitempty"`
}

type ExpectedResult struct {
	Verdict  string   `yaml:"verdict"`
	Findings []string `yaml:"findings,omitempty"`
}

type ProofObservation struct {
	State      string                     `yaml:"state"`
	Predicates ProofObservationPredicates `yaml:"predicates"`
	Limitation string                     `yaml:"limitation,omitempty"`
}

type ProofObservationPredicates struct {
	Membership string `yaml:"membership"`
	Aggregate  string `yaml:"aggregate"`
	Identity   string `yaml:"identity"`
}

type JobReference struct {
	Minimal         bool
	MetadataExample *prominput.MetadataExample
}

func (r *JobReference) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		if node.Tag != "!!str" || node.Value != "minimal" {
			return fmt.Errorf("job must be minimal or a metadata_example mapping")
		}
		r.Minimal = true
		return nil
	case yaml.MappingNode:
		value, err := promyaml.DecodeNode[struct {
			MetadataExample prominput.MetadataExample `yaml:"metadata_example"`
		}]("proof job", node, "metadata_example")
		if err != nil {
			return err
		}
		r.MetadataExample = &value.MetadataExample
		return nil
	default:
		return fmt.Errorf("job must be minimal or a metadata_example mapping")
	}
}

type Bundle struct {
	Path       string
	Descriptor Descriptor
}

func (b Bundle) ProofDirectory() string {
	return filepath.ToSlash(filepath.Dir(filepath.FromSlash(b.Path)))
}

func (b Bundle) ProfilePath() string {
	return StockProfileRoot + "/" + b.Descriptor.Profile + ".yaml"
}

func (b Bundle) ProfileDesignPath() string {
	return b.ProofDirectory() + "/" + promsemantics.ProfileDesignFilename
}

func (b Bundle) OperatorModelPath() string {
	return b.ProofDirectory() + "/OPERATOR-MODEL.md"
}

func (b Bundle) ExternalRoot() string {
	return "prometheus/profiles/" + b.Descriptor.Profile
}

func (b Bundle) SourceSemanticsPath() string {
	return b.ExternalRoot() + "/" + promsemantics.SourceFilename
}

func (b Bundle) SourceRegistryPath() string {
	return b.ExternalRoot() + "/" + promsemantics.SourceRegistryFilename
}

func (b Bundle) SourceRegistryGeneratorPath() string {
	return b.ExternalRoot() + "/" + promsemantics.SourceRegistryGeneratorFilename
}

func (b Bundle) FixturePath(path string) string {
	return b.ExternalRoot() + "/" + path
}

func Load(repoRoot, path string) (Bundle, error) {
	descriptor, err := promyaml.DecodeFile[Descriptor](path, "version", "profile", "cases")
	if err != nil {
		return Bundle{}, fmt.Errorf("proof: %w", err)
	}
	relativePath, err := filepath.Rel(repoRoot, path)
	if err != nil {
		return Bundle{}, fmt.Errorf("proof: resolve descriptor path: %w", err)
	}
	bundle := Bundle{Path: filepath.ToSlash(relativePath), Descriptor: descriptor}
	if err := bundle.validate(); err != nil {
		return Bundle{}, fmt.Errorf("proof: %w", err)
	}
	return bundle, nil
}

func (b Bundle) validate() error {
	descriptor := b.Descriptor
	if descriptor.Version != ProofVersion {
		return fmt.Errorf("version: got %q, want %q", descriptor.Version, ProofVersion)
	}
	proofDirectory := filepath.ToSlash(filepath.Dir(filepath.FromSlash(b.Path)))
	if filepath.ToSlash(filepath.Dir(filepath.FromSlash(proofDirectory))) != ProofRoot {
		return fmt.Errorf("descriptor path %q is not directly below %q", b.Path, ProofRoot)
	}
	if filepath.Base(filepath.FromSlash(b.Path)) != DescriptorFilename {
		return fmt.Errorf("descriptor path %q must end in %q", b.Path, DescriptorFilename)
	}
	if !profilecatalog.DefaultValidName(descriptor.Profile) {
		return fmt.Errorf("profile %q must be lowercase letters, digits, or underscores and start with a letter", descriptor.Profile)
	}
	if descriptor.Profile != filepath.Base(filepath.FromSlash(proofDirectory)) {
		return fmt.Errorf("profile %q must match proof directory %q", descriptor.Profile, proofDirectory)
	}
	if descriptor.MetadataExample != nil {
		if err := validateMetadataExample("metadata_example", *descriptor.MetadataExample); err != nil {
			return err
		}
	}
	if err := validateFutureInputMap("future_inputs", descriptor.FutureInputs); err != nil {
		return err
	}
	if len(descriptor.Cases) == 0 {
		return fmt.Errorf("cases must not be empty")
	}
	for _, name := range sortedMapKeys(descriptor.Cases) {
		if !isValidProofID(name, false) {
			return fmt.Errorf("case name %q must be lower snake case", name)
		}
		if err := validateProofCase("cases."+name, descriptor.Cases[name]); err != nil {
			return err
		}
		if err := validateEffectiveFutureInputs(name, descriptor.FutureInputs, descriptor.Cases[name].FutureInputs); err != nil {
			return err
		}
	}
	return nil
}

func validateProofCase(field string, proofCase ProofCase) error {
	if proofCase.Environment == nil {
		return fmt.Errorf("%s.environment must be present", field)
	}
	for _, profile := range sortedMapKeys(proofCase.Environment) {
		if !profilecatalog.DefaultValidName(profile) {
			return fmt.Errorf("%s.environment profile %q is invalid", field, profile)
		}
		if proofCase.Environment[profile] == nil {
			return fmt.Errorf("%s.environment.%s must be a mapping", field, profile)
		}
		for _, axis := range sortedMapKeys(proofCase.Environment[profile]) {
			if !isValidProofID(axis, false) {
				return fmt.Errorf("%s.environment.%s axis %q must be lower snake case", field, profile, axis)
			}
		}
	}
	fixture := proofCase.Fixture != ""
	steps := len(proofCase.Steps) != 0
	if fixture == steps {
		return fmt.Errorf("%s must declare exactly one of fixture or steps", field)
	}
	if proofCase.Coverage == nil {
		return fmt.Errorf("%s.coverage must be explicit", field)
	}
	if fixture {
		if err := validateFixturePath(field+".fixture", proofCase.Fixture); err != nil {
			return err
		}
		if proofCase.Expected == nil {
			return fmt.Errorf("%s.expected is required with fixture", field)
		}
		if err := validateExpectedResult(field+".expected", *proofCase.Expected); err != nil {
			return err
		}
		if proofCase.Expected.Verdict == "FAIL" && *proofCase.Coverage {
			return fmt.Errorf("%s FAIL result must set coverage: false", field)
		}
	} else {
		if proofCase.Expected != nil {
			return fmt.Errorf("%s.expected belongs to each step when steps are declared", field)
		}
		for index, step := range proofCase.Steps {
			if err := validateProofStep(fmt.Sprintf("%s.steps[%d]", field, index), step); err != nil {
				return err
			}
			if step.Expected.Verdict != "PASS" {
				return fmt.Errorf("%s.steps[%d] lifecycle step must expect PASS", field, index)
			}
		}
	}
	if proofCase.Job.Minimal && proofCase.Job.MetadataExample != nil {
		return fmt.Errorf("%s.job cannot select minimal and metadata_example together", field)
	}
	if proofCase.Job.MetadataExample != nil {
		if err := validateMetadataExample(field+".job.metadata_example", *proofCase.Job.MetadataExample); err != nil {
			return err
		}
	}
	return validateFutureInputMap(field+".future_inputs", proofCase.FutureInputs)
}

func validateProofStep(field string, step ProofStep) error {
	if err := validateFixturePath(field+".fixture", step.Fixture); err != nil {
		return err
	}
	if err := validateExpectedResult(field+".expected", step.Expected); err != nil {
		return err
	}
	for _, target := range sortedMapKeys(step.Observations) {
		if !isValidObservationTarget(target) {
			return fmt.Errorf("%s.observations target %q must be <view>#<input>", field, target)
		}
		if err := validateProofObservation(field+".observations."+target, step.Observations[target]); err != nil {
			return err
		}
	}
	return nil
}

func validateExpectedResult(field string, expected ExpectedResult) error {
	switch expected.Verdict {
	case "PASS":
		if len(expected.Findings) != 0 {
			return fmt.Errorf("%s PASS result must not declare findings", field)
		}
	case "FAIL":
		if len(expected.Findings) == 0 {
			return fmt.Errorf("%s FAIL result must declare findings", field)
		}
	default:
		return fmt.Errorf("%s.verdict %q must be PASS or FAIL", field, expected.Verdict)
	}
	seen := make(map[string]struct{}, len(expected.Findings))
	for index, code := range expected.Findings {
		if !isValidProofID(code, false) {
			return fmt.Errorf("%s.findings[%d] %q must be lower snake case", field, index, code)
		}
		if _, ok := seen[code]; ok {
			return fmt.Errorf("%s.findings contains duplicate code %q", field, code)
		}
		seen[code] = struct{}{}
	}
	return nil
}

func validateProofObservation(field string, observation ProofObservation) error {
	if !slices.Contains([]string{"current", "stale", "absent"}, observation.State) {
		return fmt.Errorf("%s.state %q is invalid", field, observation.State)
	}
	if !slices.Contains([]string{"establish", "unchanged", "added", "removed", "replaced"}, observation.Predicates.Membership) {
		return fmt.Errorf("%s.predicates.membership %q is invalid", field, observation.Predicates.Membership)
	}
	if !slices.Contains([]string{"matches_reducer", "unchanged", "increased", "decreased", "became_gap"}, observation.Predicates.Aggregate) {
		return fmt.Errorf("%s.predicates.aggregate %q is invalid", field, observation.Predicates.Aggregate)
	}
	if !slices.Contains([]string{"establish", "unchanged", "changed", "absent"}, observation.Predicates.Identity) {
		return fmt.Errorf("%s.predicates.identity %q is invalid", field, observation.Predicates.Identity)
	}
	if observation.Limitation != "" && !isValidObservationTarget(observation.Limitation) {
		return fmt.Errorf("%s.limitation %q must be <view>#<input>", field, observation.Limitation)
	}
	return nil
}

func validateMetadataExample(field string, example prominput.MetadataExample) error {
	if example.IntegrationID == "" || example.ExampleName == "" || example.JobName == "" {
		return fmt.Errorf("%s fields must not be empty", field)
	}
	return nil
}

func validateFutureInputMap(field string, inputs map[string]prominput.FutureInput) error {
	if inputs == nil {
		return nil
	}
	ordered := make([]prominput.FutureInput, 0, len(inputs))
	for _, id := range sortedMapKeys(inputs) {
		if !isValidProofID(id, false) {
			return fmt.Errorf("%s key %q must be lower snake case", field, id)
		}
		input := inputs[id]
		if input.Type == "" {
			return fmt.Errorf("%s.%s.type must be explicit", field, id)
		}
		ordered = append(ordered, input)
	}
	return prominput.ValidateFutureInputs(field, ordered)
}

func validateEffectiveFutureInputs(
	caseName string,
	global map[string]prominput.FutureInput,
	local map[string]prominput.FutureInput,
) error {
	effective := make(map[string]prominput.FutureInput, len(global)+len(local))
	maps.Copy(effective, global)
	for id, input := range local {
		if _, ok := effective[id]; ok {
			return fmt.Errorf("cases.%s.future_inputs duplicates profile-level input %q", caseName, id)
		}
		effective[id] = input
	}
	ordered := make([]prominput.FutureInput, 0, len(effective))
	for _, id := range sortedMapKeys(effective) {
		ordered = append(ordered, effective[id])
	}
	return prominput.ValidateFutureInputs("cases."+caseName+" effective future_inputs", ordered)
}

func validateFixturePath(field, path string) error {
	if err := validateRelativePath(field, path); err != nil {
		return err
	}
	if !strings.HasPrefix(path, "fixtures/") || !strings.HasSuffix(path, ".prom") {
		return fmt.Errorf("%s %q must be a fixtures/*.prom path", field, path)
	}
	return nil
}

func isValidObservationTarget(target string) bool {
	parts := strings.Split(target, "#")
	return len(parts) == 2 && isValidProofID(parts[0], true) && isValidProofID(parts[1], false)
}

func isValidProofID(value string, dotted bool) bool {
	if value == "" || value[0] < 'a' || value[0] > 'z' {
		return false
	}
	for _, char := range value[1:] {
		if char >= 'a' && char <= 'z' || char >= '0' && char <= '9' || char == '_' || dotted && char == '.' {
			continue
		}
		return false
	}
	return !strings.Contains(value, "..") && !strings.HasSuffix(value, ".")
}

func validateRelativePath(field, path string) error {
	if path == "" {
		return fmt.Errorf("%s must not be empty", field)
	}
	if strings.Contains(path, `\`) {
		return fmt.Errorf("%s %q must use canonical slash form", field, path)
	}
	native := filepath.FromSlash(path)
	if filepath.IsAbs(native) {
		return fmt.Errorf("%s %q must be relative", field, path)
	}
	clean := filepath.Clean(native)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return fmt.Errorf("%s %q escapes its root", field, path)
	}
	if filepath.ToSlash(clean) != path {
		return fmt.Errorf("%s %q must use canonical slash form", field, path)
	}
	return nil
}

func sortedMapKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}
