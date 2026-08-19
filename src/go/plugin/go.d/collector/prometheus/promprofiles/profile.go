// SPDX-License-Identifier: GPL-3.0-or-later

package promprofiles

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	metrixselector "github.com/netdata/netdata/go/plugins/pkg/metrix/selector"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus/relabel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/profilecatalog"
)

// profileDocument is the single strict top-level profile shape. Its heavy field
// types vary by decode: yaml.Node for the cheap header pass and concrete types
// for one lazy hydration. Decoding the complete raw document each time preserves
// YAML anchors referenced across top-level fields without duplicating this list.
type profileDocument[F, R, T any] struct {
	Match        string   `yaml:"match"`
	App          *string  `yaml:"app,omitempty"`
	Autogen      *autogen `yaml:"autogen,omitempty"`
	FallbackType F        `yaml:"fallback_type,omitempty"`
	Relabeling   R        `yaml:"relabeling,omitempty"`
	Template     T        `yaml:"template"`
}

type autogen struct {
	Selector *metrixselector.Expr `yaml:"selector"`
}

// FallbackType is the shared job/profile policy for classifying untyped scalar
// families. The JSON tags serve the collector's job configuration schema.
type FallbackType struct {
	Gauge   []string `yaml:"gauge,omitempty" json:"gauge"`
	Counter []string `yaml:"counter,omitempty" json:"counter"`
}

// Validate enforces the shared job/profile fallback pattern contract.
func (f FallbackType) Validate() error {
	return errors.Join(
		validateFallbackPatterns("gauge", f.Gauge),
		validateFallbackPatterns("counter", f.Counter),
	)
}

// Profile is a curated, exporter-specific chart profile. Identity is the file
// basename (Name). Match selects the profile by scraped metric names; App, when
// set, is the application identity used as the chart-context "app" segment
// (prometheus.<app>.…) when a job has no `app` set (by the user or service
// discovery). Heavy template and relabeling fields are parsed and validated
// lazily on first use — matching needs only Match, so a large stock library is
// not fully materialized until profiles are selected.
type Profile struct {
	Name  string
	Match string
	App   string

	autogenSelector *metrixselector.Expr
	lazy            *lazyProfile
}

// lazyProfile holds deferred stock-profile fields. It is referenced by pointer so
// Profile value copies share hydration and sync.Once values are never copied.
type lazyProfile struct {
	raw             []byte
	hasApp          bool
	hasFallbackType bool
	hasRelabeling   bool

	fallbackTypeOnce sync.Once
	fallbackType     FallbackType
	fallbackTypeErr  error

	templateOnce sync.Once
	tmpl         charttpl.Group
	templateErr  error

	relabelingOnce sync.Once
	blocks         []relabel.Block
	relabelingErr  error
}

// HasApp reports whether the profile document explicitly contains app.
func (p Profile) HasApp() bool {
	return p.lazy != nil && p.lazy.hasApp
}

// HasFallbackType reports whether the profile document contains a fallback_type
// key without hydrating it. Present null and empty values return true so their
// validation errors cannot silently take the no-policy fast path.
func (p Profile) HasFallbackType() bool {
	return p.lazy != nil && p.lazy.hasFallbackType
}

// FallbackType hydrates and validates profile-owned untyped scalar
// classification on first use. It returns an independent copy so callers cannot
// mutate catalog state.
func (p Profile) FallbackType() (FallbackType, error) {
	if !p.HasFallbackType() {
		return FallbackType{}, nil
	}
	p.lazy.fallbackTypeOnce.Do(func() {
		p.lazy.fallbackType, p.lazy.fallbackTypeErr = parseFallbackType(p.Name, p.lazy.raw)
	})
	if p.lazy.fallbackTypeErr != nil {
		return FallbackType{}, p.lazy.fallbackTypeErr
	}
	return cloneFallbackType(p.lazy.fallbackType), nil
}

// Template parses and validates the chart template on first call and memoizes
// the result. It returns an independent deep copy each call, so callers may
// freely mutate it without corrupting the process-wide catalog. Safe for
// concurrent use.
func (p Profile) Template() (charttpl.Group, error) {
	if p.lazy == nil {
		return charttpl.Group{}, fmt.Errorf("profile %q: no template loaded", p.Name)
	}
	p.lazy.templateOnce.Do(func() {
		p.lazy.tmpl, p.lazy.templateErr = parseTemplate(p.Name, p.lazy.raw)
	})
	if p.lazy.templateErr != nil {
		return charttpl.Group{}, p.lazy.templateErr
	}
	return p.lazy.tmpl.Clone(), nil
}

// HasRelabeling reports whether the profile document contains a relabeling key
// without hydrating it. Present null and empty values return true so their
// validation errors cannot silently take the no-relabel fast path.
func (p Profile) HasRelabeling() bool {
	return p.lazy != nil && p.lazy.hasRelabeling
}

// Relabeling hydrates and validates profile-owned relabel blocks on first use.
// It returns an independent deep copy so callers cannot mutate catalog state.
func (p Profile) Relabeling() ([]relabel.Block, error) {
	if !p.HasRelabeling() {
		return nil, nil
	}
	p.lazy.relabelingOnce.Do(func() {
		p.lazy.blocks, p.lazy.relabelingErr = parseRelabeling(p.Name, p.lazy.raw)
	})
	if p.lazy.relabelingErr != nil {
		return nil, p.lazy.relabelingErr
	}
	return relabel.CloneBlocks(p.lazy.blocks), nil
}

// AutogenSelector returns an independent copy of the profile-scoped fallback
// selector, or nil when the profile does not configure one.
func (p Profile) AutogenSelector() *metrixselector.Expr {
	return cloneSelectorExpr(p.autogenSelector)
}

func (p Profile) clone() Profile {
	out := p
	out.autogenSelector = cloneSelectorExpr(p.autogenSelector)
	return out
}

func cloneSelectorExpr(expr *metrixselector.Expr) *metrixselector.Expr {
	if expr == nil {
		return nil
	}
	out := *expr
	out.Allow = slices.Clone(expr.Allow)
	out.Deny = slices.Clone(expr.Deny)
	return &out
}

func cloneFallbackType(ft FallbackType) FallbackType {
	return FallbackType{
		Gauge:   slices.Clone(ft.Gauge),
		Counter: slices.Clone(ft.Counter),
	}
}

// validateHeader validates the always-loaded fields. Deferred fallback,
// relabeling, and template structure is validated separately at hydration time.
func (p *Profile) validateHeader() error {
	if strings.TrimSpace(p.Match) == "" {
		return fmt.Errorf("profile %q: 'match' must not be empty", p.Name)
	}
	if _, err := matcher.NewSimplePatternsMatcher(p.Match); err != nil {
		return fmt.Errorf("profile %q: 'match': %w", p.Name, err)
	}
	if p.App != "" && !IsValidProfileName(p.App) {
		return fmt.Errorf("profile %q: 'app' %q must match %s", p.Name, p.App, profilecatalog.DefaultValidNamePattern)
	}
	if p.autogenSelector != nil {
		if p.autogenSelector.Empty() {
			return fmt.Errorf("profile %q: 'autogen.selector' must contain at least one allow or deny selector", p.Name)
		}
		for i, item := range p.autogenSelector.Allow {
			if strings.TrimSpace(item) == "" {
				return fmt.Errorf("profile %q: 'autogen.selector.allow[%d]' must not be empty", p.Name, i)
			}
		}
		for i, item := range p.autogenSelector.Deny {
			if strings.TrimSpace(item) == "" {
				return fmt.Errorf("profile %q: 'autogen.selector.deny[%d]' must not be empty", p.Name, i)
			}
		}
		if _, err := p.autogenSelector.Parse(); err != nil {
			return fmt.Errorf("profile %q: 'autogen.selector': %w", p.Name, err)
		}
	}
	return nil
}

// parseTemplate typed-decodes and validates the chart template from the complete
// document so aliases may reference anchors declared in another top-level field.
func parseTemplate(name string, raw []byte) (charttpl.Group, error) {
	var doc profileDocument[yaml.Node, yaml.Node, *charttpl.Group]
	if err := decodeDocumentStrict(raw, &doc); err != nil {
		return charttpl.Group{}, fmt.Errorf("profile %q: unmarshal 'template': %w", name, err)
	}
	if doc.Template == nil {
		return charttpl.Group{}, fmt.Errorf("profile %q: 'template' is required", name)
	}
	group := *doc.Template

	if !groupHasChart(group) {
		return charttpl.Group{}, fmt.Errorf("profile %q: 'template' must contain at least one chart", name)
	}

	spec := charttpl.Spec{
		Version: charttpl.VersionV1,
		Groups:  []charttpl.Group{group},
	}
	if err := spec.Validate(); err != nil {
		return charttpl.Group{}, fmt.Errorf("profile %q: 'template': %w", name, err)
	}

	return group, nil
}

func parseRelabeling(name string, raw []byte) ([]relabel.Block, error) {
	var doc profileDocument[yaml.Node, []relabel.Block, yaml.Node]
	if err := decodeDocumentStrict(raw, &doc); err != nil {
		return nil, fmt.Errorf("profile %q: unmarshal 'relabeling': %w", name, err)
	}
	blocks := doc.Relabeling
	if len(blocks) == 0 {
		return nil, fmt.Errorf("profile %q: 'relabeling' must not be empty", name)
	}
	if _, err := relabel.NewPipeline(blocks); err != nil {
		return nil, fmt.Errorf("profile %q: invalid 'relabeling': %w", name, err)
	}
	return blocks, nil
}

func parseFallbackType(name string, raw []byte) (FallbackType, error) {
	var doc profileDocument[*FallbackType, yaml.Node, yaml.Node]
	if err := decodeDocumentStrict(raw, &doc); err != nil {
		return FallbackType{}, fmt.Errorf("profile %q: unmarshal 'fallback_type': %w", name, err)
	}
	if doc.FallbackType == nil {
		return FallbackType{}, fmt.Errorf("profile %q: 'fallback_type' must not be empty", name)
	}
	ft := *doc.FallbackType
	if len(ft.Gauge) == 0 && len(ft.Counter) == 0 {
		return FallbackType{}, fmt.Errorf(
			"profile %q: 'fallback_type' must contain at least one gauge or counter pattern",
			name,
		)
	}
	if err := ft.Validate(); err != nil {
		return FallbackType{}, fmt.Errorf("profile %q: %w", name, err)
	}
	return ft, nil
}

func validateFallbackPatterns(kind string, patterns []string) error {
	var errs []error
	for i, pattern := range patterns {
		path := fmt.Sprintf("fallback_type.%s[%d]", kind, i)
		if strings.TrimSpace(pattern) == "" {
			errs = append(errs, fmt.Errorf("'%s' must not be empty", path))
			continue
		}
		if pattern != strings.TrimSpace(pattern) {
			errs = append(errs, fmt.Errorf("'%s' must not have leading or trailing whitespace", path))
			continue
		}
		if _, err := matcher.NewGlobMatcher(pattern); err != nil {
			errs = append(errs, fmt.Errorf("'%s' pattern %q: %w", path, pattern, err))
		}
	}
	return errors.Join(errs...)
}

func decodeDocumentStrict(raw []byte, dst any) error {
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	return dec.Decode(dst)
}

func groupHasChart(group charttpl.Group) bool {
	if len(group.Charts) > 0 {
		return true
	}
	return slices.ContainsFunc(group.Groups, groupHasChart)
}
