// SPDX-License-Identifier: GPL-3.0-or-later

package promsemantics

import (
	"context"
	"fmt"
)

type SemanticContractPaths struct {
	ProfileDesign           string
	SourceSemantics         string
	SourceRegistry          string
	SourceRegistryGenerator string
}

type SemanticContract struct {
	Design   ProfileDesignDocument
	Source   SourceSemanticsDocument
	Registry *SourceRegistryPair
}

func LoadSemanticContract(ctx context.Context, paths SemanticContractPaths) (SemanticContract, error) {
	if err := checkSemanticContext(ctx, "before profile design parsing"); err != nil {
		return SemanticContract{}, err
	}
	if paths.ProfileDesign == "" || paths.SourceSemantics == "" {
		return SemanticContract{}, fmt.Errorf("profile design and source semantics paths must be present")
	}
	if (paths.SourceRegistry == "") != (paths.SourceRegistryGenerator == "") {
		return SemanticContract{}, fmt.Errorf("source registry and generator manifest must be present together")
	}

	design, err := LoadProfileDesign(paths.ProfileDesign)
	if err != nil {
		return SemanticContract{}, err
	}
	if err := checkSemanticContext(ctx, "before source semantics parsing"); err != nil {
		return SemanticContract{}, err
	}
	source, err := LoadSourceSemantics(paths.SourceSemantics)
	if err != nil {
		return SemanticContract{}, err
	}

	contract := SemanticContract{Design: design, Source: source}
	if paths.SourceRegistry != "" {
		if err := checkSemanticContext(ctx, "before source registry parsing"); err != nil {
			return SemanticContract{}, err
		}
		registry, err := LoadSourceRegistryPair(paths.SourceRegistry, paths.SourceRegistryGenerator)
		if err != nil {
			return SemanticContract{}, err
		}
		contract.Registry = &registry
	}
	if err := contract.validateIdentity(); err != nil {
		return SemanticContract{}, fmt.Errorf("semantic contract: %w", err)
	}
	return contract, nil
}

func (c SemanticContract) validateIdentity() error {
	if c.Design.Profile != c.Source.Profile {
		return fmt.Errorf("profile mismatch: design %q, source semantics %q", c.Design.Profile, c.Source.Profile)
	}
	if c.Registry == nil {
		return nil
	}
	if c.Registry.Registry.Profile != c.Design.Profile {
		return fmt.Errorf("profile mismatch: design %q, source registry %q",
			c.Design.Profile, c.Registry.Registry.Profile)
	}
	for id, upstream := range c.Registry.Generator.Upstreams {
		sourceUpstream, ok := c.Source.Upstreams[id]
		if !ok {
			return fmt.Errorf("source registry upstream %q is absent from source semantics", id)
		}
		if sourceUpstream.Repository != upstream.Repository || sourceUpstream.Commit != upstream.Commit {
			return fmt.Errorf("source registry upstream %q disagrees with source semantics repository/commit", id)
		}
	}
	return nil
}

func checkSemanticContext(ctx context.Context, phase string) error {
	if ctx == nil {
		return fmt.Errorf("semantic contract: nil context")
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("semantic contract canceled %s: %w", phase, err)
	}
	return nil
}
