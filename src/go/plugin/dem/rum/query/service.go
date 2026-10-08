// SPDX-License-Identifier: GPL-3.0-or-later

// Package query provides copied RUM observations over admitted sites and retained history.
package query

import (
	"context"
	"sort"

	"github.com/netdata/netdata/go/plugins/plugin/dem/internal/redact"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/history"
	rumregistry "github.com/netdata/netdata/go/plugins/plugin/dem/rum/registry"
)

type Service struct {
	registry *rumregistry.Registry
	history  *history.Store
}

func New(registry *rumregistry.Registry, history *history.Store) *Service {
	return &Service{
		registry: registry,
		history:  history,
	}
}

func (s *Service) Receiver() Receiver {
	state := s.registry.Availability()
	return Receiver{
		Serving:   state.Serving,
		PublicURL: state.PublicURL,
		GeoIP:     state.GeoIP,
	}
}

// InvalidArgument identifies caller input rejected by a domain query.
type InvalidArgument struct{ Message string }

func (e InvalidArgument) Error() string { return e.Message }

// visit holds a lease only while copying domain snapshots. Retirement can
// revoke future admission without exposing a collector or desired config.
func (s *Service) visit(ctx context.Context, filter string, fn func(string, *rumregistry.Site)) error {
	keys := s.registry.Keys()
	sort.Strings(keys)
	for _, key := range keys {
		if err := ctx.Err(); err != nil {
			return err
		}
		if filter != "" && key != filter {
			continue
		}
		site, leaseCtx, release, ok := s.registry.AcquireSite(key)
		if !ok {
			continue
		}
		if leaseCtx.Err() == nil {
			fn(key, site)
		}
		release()
	}
	return ctx.Err()
}
func siteRedactor(site *rumregistry.Site) *redact.Redactor {
	if site.Redactor != nil {
		return site.Redactor
	}
	return redact.NewRedactor()
}
