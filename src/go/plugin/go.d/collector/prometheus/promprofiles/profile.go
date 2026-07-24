// SPDX-License-Identifier: GPL-3.0-or-later

package promprofiles

import (
	"bytes"
	"fmt"
	"slices"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	metrixselector "github.com/netdata/netdata/go/plugins/pkg/metrix/selector"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
)

// profileHeader is the eagerly-decoded part of a profile: everything except the
// chart template. Template is captured as an un-decoded yaml.Node so a strict
// decode (KnownFields) still accepts the legitimate `template:` key and rejects
// unknown keys, without typed-decoding or validating the heavy chart tree at
// load time.
type profileHeader struct {
	Match    string    `yaml:"match"`
	App      string    `yaml:"app,omitempty"`
	Autogen  *autogen  `yaml:"autogen,omitempty"`
	Template yaml.Node `yaml:"template"`
}

type autogen struct {
	Selector *metrixselector.Expr `yaml:"selector"`
}

// Profile is a curated, exporter-specific chart profile. Identity is the file
// basename (Name). Match selects the profile by scraped metric names; App, when
// set, is the application identity used as the chart-context "app" segment
// (prometheus.<app>.…) when a job has no `app` set (by the user or service
// discovery). The chart template is parsed and validated lazily on the first
// Template() call — matching needs only Match, so a large stock library is
// neither typed-decoded nor held parsed in memory until a profile is selected.
type Profile struct {
	Name  string
	Match string
	App   string

	autogenSelector *metrixselector.Expr
	lazy            *lazyTemplate
}

// lazyTemplate holds the deferred chart template. It is referenced by pointer so
// Profile value copies (catalog map values, selection slices) share one
// hydration and the sync.Once is never copied.
type lazyTemplate struct {
	raw  []byte
	once sync.Once
	tmpl charttpl.Group
	err  error
}

// Template parses and validates the chart template on first call and memoizes
// the result. It returns an independent deep copy each call, so callers may
// freely mutate it without corrupting the process-wide catalog. Safe for
// concurrent use.
func (p Profile) Template() (charttpl.Group, error) {
	if p.lazy == nil {
		return charttpl.Group{}, fmt.Errorf("profile %q: no template loaded", p.Name)
	}
	p.lazy.once.Do(func() {
		p.lazy.tmpl, p.lazy.err = parseTemplate(p.Name, p.lazy.raw)
	})
	if p.lazy.err != nil {
		return charttpl.Group{}, p.lazy.err
	}
	return p.lazy.tmpl.Clone(), nil
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

// validateHeader validates the always-loaded fields (match + app). Template
// structure is validated separately, at hydrate time.
func (p *Profile) validateHeader() error {
	if strings.TrimSpace(p.Match) == "" {
		return fmt.Errorf("profile %q: 'match' must not be empty", p.Name)
	}
	if _, err := matcher.NewSimplePatternsMatcher(p.Match); err != nil {
		return fmt.Errorf("profile %q: 'match': %w", p.Name, err)
	}
	if p.App != "" && !validProfileName.MatchString(p.App) {
		return fmt.Errorf("profile %q: 'app' %q must match %s", p.Name, p.App, validProfileName.String())
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

// parseTemplate typed-decodes and validates the chart template from the raw
// profile bytes. Strict decoding keeps parity with the header decode (unknown
// keys rejected). It is the deferred half of profile validation.
func parseTemplate(name string, raw []byte) (charttpl.Group, error) {
	var doc struct {
		Match    string         `yaml:"match"`
		App      string         `yaml:"app,omitempty"`
		Autogen  *autogen       `yaml:"autogen,omitempty"`
		Template charttpl.Group `yaml:"template"`
	}
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(&doc); err != nil {
		return charttpl.Group{}, fmt.Errorf("profile %q: unmarshal 'template': %w", name, err)
	}

	if !groupHasChart(doc.Template) {
		return charttpl.Group{}, fmt.Errorf("profile %q: 'template' must contain at least one chart", name)
	}

	spec := charttpl.Spec{
		Version: charttpl.VersionV1,
		Groups:  []charttpl.Group{doc.Template},
	}
	if err := spec.Validate(); err != nil {
		return charttpl.Group{}, fmt.Errorf("profile %q: 'template': %w", name, err)
	}

	return doc.Template, nil
}

func groupHasChart(group charttpl.Group) bool {
	if len(group.Charts) > 0 {
		return true
	}
	return slices.ContainsFunc(group.Groups, groupHasChart)
}
