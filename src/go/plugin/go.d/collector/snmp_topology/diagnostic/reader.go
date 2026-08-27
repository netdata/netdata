// SPDX-License-Identifier: GPL-3.0-or-later

package diagnostic

import (
	"bytes"
	"errors"
	"fmt"
	"io"
)

type MemberSource interface {
	Open(ContentRef) (io.ReadCloser, error)
}

type DecodeMemberFunc func([]byte, ReaderLimits) ([]ContentRef, error)

type Closure struct {
	RootType      MemberType
	Decode        map[MemberType]DecodeMemberFunc
	ValidateGraph func(CapabilityRootV1, MemberSource, ReaderLimits) error
}

type Registry struct {
	closures map[CapabilityKey]Closure
}

func NewRegistry() *Registry {
	return &Registry{closures: make(map[CapabilityKey]Closure)}
}

func (r *Registry) Register(key CapabilityKey, closure Closure) error {
	if r == nil {
		return errors.New("nil registry")
	}
	if err := key.Validate(); err != nil {
		return err
	}
	if err := closure.RootType.Validate(); err != nil {
		return fmt.Errorf("root type: %w", err)
	}
	if closure.RootType != (MemberType{Kind: KindCapabilityRoot, Schema: SchemaV1}) {
		return fmt.Errorf("root type must be %s@%s", KindCapabilityRoot, SchemaV1)
	}
	if len(closure.Decode) == 0 {
		return errors.New("closure has no member decoders")
	}
	if _, ok := closure.Decode[closure.RootType]; !ok {
		return errors.New("closure has no root decoder")
	}
	if _, exists := r.closures[key]; exists {
		return fmt.Errorf("capability closure %s@%d is already registered", key.Name, key.Revision)
	}
	decoders := make(map[MemberType]DecodeMemberFunc, len(closure.Decode))
	for memberType, decode := range closure.Decode {
		if err := memberType.Validate(); err != nil {
			return err
		}
		if decode == nil {
			return fmt.Errorf("nil decoder for %s@%s", memberType.Kind, memberType.Schema)
		}
		decoders[memberType] = decode
	}
	closure.Decode = decoders
	r.closures[key] = closure
	return nil
}

type ValidationReport struct {
	Integrity    bool
	Schema       bool
	Completeness bool
	Replayable   bool
	Preserved    bool
	Authenticity TrustState
	Capability   CapabilityKey
	State        TerminalState
	Members      uint64
	LogicalBytes uint64
}

func (r *Registry) ValidateCapability(
	manifest ManifestV1,
	source MemberSource,
	key CapabilityKey,
	limits ReaderLimits,
) (ValidationReport, error) {
	report := ValidationReport{Capability: key, Authenticity: manifest.Authenticity.State}
	if r == nil {
		return report, errors.New("nil registry")
	}
	if source == nil {
		return report, errors.New("nil member source")
	}
	if err := limits.Validate(); err != nil {
		return report, err
	}
	if uint64(len(manifest.Members)) > limits.MaxMembers {
		return report, fmt.Errorf("manifest member count %d exceeds limit %d", len(manifest.Members), limits.MaxMembers)
	}
	if err := manifest.Validate(); err != nil {
		return report, fmt.Errorf("manifest: %w", err)
	}
	closure, ok := r.closures[key]
	if !ok {
		return report, fmt.Errorf("unsupported capability closure %s@%d", key.Name, key.Revision)
	}
	root, ok := manifestRoot(manifest, key)
	if !ok {
		return report, fmt.Errorf("manifest does not contain capability %s@%d", key.Name, key.Revision)
	}
	report.State = root.State

	graphBudget, err := newBudget(limits)
	if err != nil {
		return report, err
	}
	memberInventory := make(map[string]ContentRef, len(manifest.Members))
	for _, ref := range manifest.Members {
		if err := graphBudget.addMember(ref); err != nil {
			return report, err
		}
		memberInventory[ref.Key()] = ref
		data, err := readMember(source, ref, limits)
		if err != nil {
			return report, fmt.Errorf("verify %s@%s %s: %w", ref.Kind, ref.Schema, ref.SHA256, err)
		}
		if err := VerifyContent(ref, data); err != nil {
			return report, fmt.Errorf("verify %s@%s %s: %w", ref.Kind, ref.Schema, ref.SHA256, err)
		}
	}
	report.Integrity = true
	report.Members = graphBudget.members
	report.LogicalBytes = graphBudget.logicalBytes

	if root.Root.Type() != closure.RootType {
		return report, fmt.Errorf("capability root has type %s@%s, expected %s@%s",
			root.Root.Kind, root.Root.Schema, closure.RootType.Kind, closure.RootType.Schema)
	}
	rootData, err := readMember(source, root.Root, limits)
	if err != nil {
		return report, err
	}
	var rootDocument CapabilityRootV1
	if err := DecodeCanonical(rootData, limits, &rootDocument); err != nil {
		return report, fmt.Errorf("decode capability root: %w", err)
	}
	if err := rootDocument.Validate(); err != nil {
		return report, fmt.Errorf("validate capability root: %w", err)
	}
	if rootDocument.Capability != key {
		return report, fmt.Errorf("root declares capability %s@%d, expected %s@%d",
			rootDocument.Capability.Name, rootDocument.Capability.Revision, key.Name, key.Revision)
	}
	if rootDocument.State != root.State {
		return report, fmt.Errorf("capability state mismatch: manifest=%s root=%s", root.State, rootDocument.State)
	}

	visited := make(map[string]bool)
	visiting := make(map[string]bool)
	var visit func(ContentRef) error
	visit = func(ref ContentRef) error {
		key := ref.Key()
		if visiting[key] {
			return fmt.Errorf("capability closure contains a cycle at %s", ref.SHA256)
		}
		if visited[key] {
			return nil
		}
		inventoryRef, exists := memberInventory[key]
		if !exists || inventoryRef != ref {
			return fmt.Errorf("referenced member %s@%s %s is absent from the manifest inventory",
				ref.Kind, ref.Schema, ref.SHA256)
		}
		decode, exists := closure.Decode[ref.Type()]
		if !exists {
			return fmt.Errorf("unsupported member %s@%s inside requested capability %s@%d",
				ref.Kind, ref.Schema, root.Name, root.Revision)
		}
		visiting[key] = true
		data, err := readMember(source, ref, limits)
		if err != nil {
			return err
		}
		children, err := decode(data, limits)
		if err != nil {
			return fmt.Errorf("decode %s@%s %s: %w", ref.Kind, ref.Schema, ref.SHA256, err)
		}
		if err := graphBudget.addReferences(uint64(len(children))); err != nil {
			return err
		}
		for _, child := range children {
			if err := child.Validate(); err != nil {
				return err
			}
			if err := visit(child); err != nil {
				return err
			}
		}
		delete(visiting, key)
		visited[key] = true
		return nil
	}
	if err := visit(root.Root); err != nil {
		return report, err
	}
	if closure.ValidateGraph != nil {
		if err := closure.ValidateGraph(rootDocument, source, limits); err != nil {
			return report, fmt.Errorf("validate capability graph: %w", err)
		}
	}
	report.Schema = true
	report.Completeness = root.State == StateSuccess || root.State == StateEmpty || root.State == StateNotApplicable
	report.Replayable = report.Completeness
	// Exact historical output preservation is a separate later capability.
	report.Preserved = false
	return report, nil
}

func DecodeCapabilityRoot(key CapabilityKey) DecodeMemberFunc {
	return func(data []byte, limits ReaderLimits) ([]ContentRef, error) {
		var root CapabilityRootV1
		if err := DecodeCanonical(data, limits, &root); err != nil {
			return nil, err
		}
		if err := root.Validate(); err != nil {
			return nil, err
		}
		if root.Capability != key {
			return nil, fmt.Errorf("root declares capability %s@%d, expected %s@%d",
				root.Capability.Name, root.Capability.Revision, key.Name, key.Revision)
		}
		return root.References(), nil
	}
}

func DecodeLeaf[T interface{ Validate() error }]() DecodeMemberFunc {
	return func(data []byte, limits ReaderLimits) ([]ContentRef, error) {
		var value T
		if err := DecodeCanonical(data, limits, &value); err != nil {
			return nil, err
		}
		if err := value.Validate(); err != nil {
			return nil, err
		}
		return nil, nil
	}
}

func manifestRoot(manifest ManifestV1, key CapabilityKey) (CapabilityRefV1, bool) {
	for _, root := range manifest.Roots {
		if root.CapabilityKey == key {
			return root, true
		}
	}
	return CapabilityRefV1{}, false
}

func readMember(source MemberSource, ref ContentRef, limits ReaderLimits) ([]byte, error) {
	if ref.LogicalLength > limits.MaxMemberBytes {
		return nil, fmt.Errorf("member logical length %d exceeds limit %d", ref.LogicalLength, limits.MaxMemberBytes)
	}
	reader, err := source.Open(ref)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	max := int64(ref.LogicalLength)
	limited := io.LimitReader(reader, max+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if uint64(len(data)) != ref.LogicalLength {
		return nil, fmt.Errorf("member length mismatch: reference=%d actual=%d", ref.LogicalLength, len(data))
	}
	return data, nil
}

// DecodeReferenced reads and strictly decodes one referenced canonical member.
// Capability validation should run first when closure completeness matters.
func DecodeReferenced(source MemberSource, ref ContentRef, limits ReaderLimits, dst any) error {
	data, err := readMember(source, ref, limits)
	if err != nil {
		return err
	}
	if err := VerifyContent(ref, data); err != nil {
		return err
	}
	return DecodeCanonical(data, limits, dst)
}

type MemorySource map[string][]byte

func (s MemorySource) Open(ref ContentRef) (io.ReadCloser, error) {
	data, ok := s[ref.Key()]
	if !ok {
		return nil, errors.New("member not found")
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}
