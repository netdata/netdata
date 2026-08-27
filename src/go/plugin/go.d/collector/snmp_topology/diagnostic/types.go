// SPDX-License-Identifier: GPL-3.0-or-later

package diagnostic

import (
	"cmp"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

const (
	FormatV1           = "netdata.snmp-topology-diagnostic/v1"
	CanonicalJSONV1    = "netdata.canonical-json/v1"
	ContentNamespaceV1 = "netdata.snmp-topology-diagnostic.member/v1"
)

const (
	KindCapabilityRoot = "capability_root"
	SchemaV1           = "1"
)

type TerminalState string

const (
	StateSuccess       TerminalState = "success"
	StateEmpty         TerminalState = "empty"
	StateNotApplicable TerminalState = "not_applicable"
	StateFailed        TerminalState = "failed"
	StateIncomplete    TerminalState = "incomplete"
)

func (s TerminalState) valid() bool {
	switch s {
	case StateSuccess, StateEmpty, StateNotApplicable, StateFailed, StateIncomplete:
		return true
	default:
		return false
	}
}

type SensitivityV1 struct {
	Classification string `json:"classification"`
	Fidelity       string `json:"fidelity"`
	Sanitized      bool   `json:"sanitized"`
}

func ExactRestrictedSensitivity() SensitivityV1 {
	return SensitivityV1{Classification: "restricted", Fidelity: "exact", Sanitized: false}
}

func (s SensitivityV1) Validate() error {
	if s.Classification != "restricted" {
		return fmt.Errorf("classification must be restricted")
	}
	if s.Fidelity != "exact" {
		return fmt.Errorf("fidelity must be exact")
	}
	if s.Sanitized {
		return fmt.Errorf("v1 exact artifacts must not claim sanitization")
	}
	return nil
}

type MemberType struct {
	Kind   string `json:"kind"`
	Schema string `json:"schema"`
}

func (t MemberType) Validate() error {
	if err := validateID("member kind", t.Kind); err != nil {
		return err
	}
	if err := validateRevision("member schema", t.Schema); err != nil {
		return err
	}
	return nil
}

type ContentRef struct {
	Namespace        string `json:"namespace"`
	Kind             string `json:"kind"`
	Schema           string `json:"schema"`
	Canonicalization string `json:"canonicalization"`
	LogicalLength    uint64 `json:"logical_length"`
	SHA256           string `json:"sha256"`
}

func (r ContentRef) Type() MemberType {
	return MemberType{Kind: r.Kind, Schema: r.Schema}
}

func (r ContentRef) Validate() error {
	if r.Namespace != ContentNamespaceV1 {
		return fmt.Errorf("unsupported content namespace %q", r.Namespace)
	}
	if err := r.Type().Validate(); err != nil {
		return err
	}
	if r.Canonicalization != CanonicalJSONV1 {
		return fmt.Errorf("unsupported canonicalization %q", r.Canonicalization)
	}
	if r.LogicalLength == 0 {
		return errors.New("logical length must be nonzero")
	}
	if !sha256Pattern.MatchString(r.SHA256) {
		return errors.New("sha256 must contain 64 lowercase hexadecimal characters")
	}
	return nil
}

func (r ContentRef) Compare(other ContentRef) int {
	if n := cmp.Compare(r.Kind, other.Kind); n != 0 {
		return n
	}
	if n := cmp.Compare(r.Schema, other.Schema); n != 0 {
		return n
	}
	if n := cmp.Compare(r.SHA256, other.SHA256); n != 0 {
		return n
	}
	return cmp.Compare(r.LogicalLength, other.LogicalLength)
}

func (r ContentRef) Key() string {
	return strings.Join([]string{r.Namespace, r.Kind, r.Schema, r.Canonicalization, fmt.Sprint(r.LogicalLength), r.SHA256}, "\x00")
}

type CapabilityKey struct {
	Name     string `json:"name"`
	Revision uint32 `json:"revision"`
}

func (k CapabilityKey) Validate() error {
	if err := validateID("capability", k.Name); err != nil {
		return err
	}
	if k.Revision == 0 {
		return errors.New("capability revision must be nonzero")
	}
	return nil
}

type CapabilityRefV1 struct {
	CapabilityKey
	State TerminalState `json:"state"`
	Root  ContentRef    `json:"root"`
}

func (r CapabilityRefV1) Validate() error {
	if err := r.CapabilityKey.Validate(); err != nil {
		return err
	}
	if !r.State.valid() {
		return fmt.Errorf("invalid capability state %q", r.State)
	}
	if err := r.Root.Validate(); err != nil {
		return fmt.Errorf("root reference: %w", err)
	}
	if r.Root.Kind != KindCapabilityRoot || r.Root.Schema != SchemaV1 {
		return fmt.Errorf("capability root must use %s@%s", KindCapabilityRoot, SchemaV1)
	}
	return nil
}

type ManifestV1 struct {
	Format           string            `json:"format"`
	Canonicalization string            `json:"canonicalization"`
	Sensitivity      SensitivityV1     `json:"sensitivity"`
	Roots            []CapabilityRefV1 `json:"roots"`
	Members          []ContentRef      `json:"members"`
}

func (m ManifestV1) Validate() error {
	if m.Format != FormatV1 {
		return fmt.Errorf("unsupported format %q", m.Format)
	}
	if m.Canonicalization != CanonicalJSONV1 {
		return fmt.Errorf("unsupported manifest canonicalization %q", m.Canonicalization)
	}
	if err := m.Sensitivity.Validate(); err != nil {
		return fmt.Errorf("sensitivity: %w", err)
	}
	if len(m.Roots) == 0 {
		return errors.New("manifest has no capability roots")
	}
	if len(m.Members) == 0 {
		return errors.New("manifest has no members")
	}

	memberKeys := make(map[string]struct{}, len(m.Members))
	for i, ref := range m.Members {
		if err := ref.Validate(); err != nil {
			return fmt.Errorf("members[%d]: %w", i, err)
		}
		if i > 0 && m.Members[i-1].Compare(ref) >= 0 {
			return errors.New("manifest members must be strictly ordered")
		}
		memberKeys[ref.Key()] = struct{}{}
	}

	rootKeys := make(map[CapabilityKey]struct{}, len(m.Roots))
	for i, root := range m.Roots {
		if err := root.Validate(); err != nil {
			return fmt.Errorf("roots[%d]: %w", i, err)
		}
		if i > 0 && compareCapabilityKey(m.Roots[i-1].CapabilityKey, root.CapabilityKey) >= 0 {
			return errors.New("manifest roots must be strictly ordered")
		}
		if _, ok := rootKeys[root.CapabilityKey]; ok {
			return fmt.Errorf("duplicate capability root %s@%d", root.Name, root.Revision)
		}
		rootKeys[root.CapabilityKey] = struct{}{}
		if _, ok := memberKeys[root.Root.Key()]; !ok {
			return fmt.Errorf("root %s@%d is absent from the member inventory", root.Name, root.Revision)
		}
	}
	return nil
}

type SectionInventoryV1 struct {
	Name            string        `json:"name"`
	State           TerminalState `json:"state"`
	ExpectedRecords uint64        `json:"expected_records"`
	Members         []ContentRef  `json:"members"`
}

func (s SectionInventoryV1) Validate() error {
	if err := validateID("section", s.Name); err != nil {
		return err
	}
	if !s.State.valid() {
		return fmt.Errorf("invalid section state %q", s.State)
	}
	if s.State == StateEmpty && s.ExpectedRecords != 0 {
		return errors.New("empty section must expect zero records")
	}
	if s.State == StateSuccess && s.ExpectedRecords == 0 {
		return errors.New("successful section must expect records")
	}
	if s.State == StateSuccess && len(s.Members) == 0 {
		return errors.New("successful section has no members")
	}
	for i, ref := range s.Members {
		if err := ref.Validate(); err != nil {
			return fmt.Errorf("members[%d]: %w", i, err)
		}
	}
	return nil
}

type CapabilityRootV1 struct {
	Capability CapabilityKey        `json:"capability"`
	State      TerminalState        `json:"state"`
	Sections   []SectionInventoryV1 `json:"sections"`
}

func (r CapabilityRootV1) Validate() error {
	if err := r.Capability.Validate(); err != nil {
		return err
	}
	if !r.State.valid() {
		return fmt.Errorf("invalid capability state %q", r.State)
	}
	if len(r.Sections) == 0 {
		return errors.New("capability root has no sections")
	}
	seen := make(map[string]struct{}, len(r.Sections))
	for i, section := range r.Sections {
		if err := section.Validate(); err != nil {
			return fmt.Errorf("sections[%d]: %w", i, err)
		}
		if i > 0 && r.Sections[i-1].Name >= section.Name {
			return errors.New("capability sections must be strictly ordered")
		}
		if _, ok := seen[section.Name]; ok {
			return fmt.Errorf("duplicate section %q", section.Name)
		}
		seen[section.Name] = struct{}{}
	}
	return nil
}

func (r CapabilityRootV1) References() []ContentRef {
	var refs []ContentRef
	for _, section := range r.Sections {
		refs = append(refs, section.Members...)
	}
	return refs
}

func SortContentRefs(refs []ContentRef) {
	slices.SortFunc(refs, func(a, b ContentRef) int { return a.Compare(b) })
}

func SortCapabilityRefs(refs []CapabilityRefV1) {
	slices.SortFunc(refs, func(a, b CapabilityRefV1) int { return compareCapabilityKey(a.CapabilityKey, b.CapabilityKey) })
}

func compareCapabilityKey(a, b CapabilityKey) int {
	if n := cmp.Compare(a.Name, b.Name); n != 0 {
		return n
	}
	return cmp.Compare(a.Revision, b.Revision)
}

var (
	idPattern       = regexp.MustCompile(`^[a-z][a-z0-9_.:-]{0,127}$`)
	revisionPattern = regexp.MustCompile(`^[1-9][0-9]{0,8}$`)
	sha256Pattern   = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

func validateID(label, value string) error {
	if !idPattern.MatchString(value) || strings.Contains(value, "..") || strings.ContainsAny(value, `/\\`) {
		return fmt.Errorf("%s %q is not a portable identifier", label, value)
	}
	return nil
}

func validateRevision(label, value string) error {
	if !revisionPattern.MatchString(value) {
		return fmt.Errorf("%s %q is not a supported revision", label, value)
	}
	return nil
}
