// SPDX-License-Identifier: GPL-3.0-or-later
package query

import (
	"context"
	"errors"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/artifacts"
)

type CataloguePage struct {
	artifacts.Page
	PolicyError string `json:"policy_error,omitempty"`
}

// Catalogue exposes current manifest metadata and global storage observations.
func (s *Service) Catalogue(ctx context.Context, filter artifacts.Filter) (CataloguePage, error) {
	if err := ctx.Err(); err != nil {
		return CataloguePage{}, err
	}
	if s.artifacts == nil {
		return CataloguePage{}, errors.New("artifact store unavailable")
	}
	page, err := s.artifacts.Catalogue(ctx, filter)
	result := CataloguePage{
		Page: page,
	}
	if s.policyError != nil {
		result.PolicyError = s.policyError()
	}
	return result, artifactError(err)
}
