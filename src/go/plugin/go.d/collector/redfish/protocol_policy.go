// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync/atomic"
)

const (
	maxCycleRequests     = 100_000
	maxCycleBodyBytes    = 1 << 30
	maxCollectionPages   = 10_000
	maxCollectionMembers = 100_000
)

type operationBudget struct {
	requests atomic.Int64
	bytes    atomic.Int64
	pages    atomic.Int64
	members  atomic.Int64
}

type operationBudgetContextKey struct{}

type logPageBudget struct {
	requests atomic.Int64
	bytes    atomic.Int64
}

type logPageBudgetContextKey struct{}

func withOperationBudget(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if operationBudgetFrom(ctx) != nil {
		return ctx
	}
	return context.WithValue(ctx, operationBudgetContextKey{}, &operationBudget{})
}

func operationBudgetFrom(ctx context.Context) *operationBudget {
	if ctx == nil {
		return nil
	}
	budget, _ := ctx.Value(operationBudgetContextKey{}).(*operationBudget)
	return budget
}

func consumeBudget(value *atomic.Int64, amount, limit int64, subject string) error {
	if value == nil || amount <= 0 {
		return nil
	}
	current := value.Add(amount)
	if current <= limit {
		return nil
	}
	value.Add(-amount)
	return fmt.Errorf("Redfish %s exceeds the internal safety limit", subject)
}

func consumeRequestBudget(ctx context.Context) error {
	if budget := operationBudgetFrom(ctx); budget != nil {
		return consumeBudget(&budget.requests, 1, maxCycleRequests, "request work")
	}
	return nil
}

func consumeBodyBudget(ctx context.Context, size int) error {
	if budget := operationBudgetFrom(ctx); budget != nil {
		return consumeBudget(&budget.bytes, int64(size), maxCycleBodyBytes, "response-body work")
	}
	return nil
}

func consumeCollectionPageBudget(ctx context.Context) error {
	if budget := operationBudgetFrom(ctx); budget != nil {
		return consumeBudget(&budget.pages, 1, maxCollectionPages, "collection page work")
	}
	return nil
}

func consumeCollectionMemberBudget(ctx context.Context, count int) error {
	if budget := operationBudgetFrom(ctx); budget != nil {
		return consumeBudget(&budget.members, int64(count), maxCollectionMembers, "collection member work")
	}
	return nil
}

func withLogPageBudget(ctx context.Context) context.Context {
	return context.WithValue(ctx, logPageBudgetContextKey{}, &logPageBudget{})
}

func consumeLogPageRequestBudget(ctx context.Context) error {
	budget, _ := ctx.Value(logPageBudgetContextKey{}).(*logPageBudget)
	if budget == nil {
		return nil
	}
	return consumeBudget(&budget.requests, 1, maxLogRequestsPerPage, "LogEntry page request work")
}

func consumeLogPageBodyBudget(ctx context.Context, size int) error {
	budget, _ := ctx.Value(logPageBudgetContextKey{}).(*logPageBudget)
	if budget == nil {
		return nil
	}
	return consumeBudget(
		&budget.bytes,
		int64(size),
		maxLogResponseBytesCycle,
		"LogEntry page response-body work",
	)
}

type redfishURIMode uint8

const (
	uriResource redfishURIMode = iota
	uriOpaquePage
	uriProvenance
)

func resolveRedfishURI(
	origin string,
	base *url.URL,
	raw string,
	mode redfishURIMode,
) (*url.URL, error) {
	if base == nil {
		return nil, errors.New("Redfish URI has no base")
	}
	if len(raw) == 0 || len(raw) > maxURIBytes {
		return nil, errors.New("invalid Redfish URI length")
	}
	ref, err := url.Parse(raw)
	if err != nil || ref.Opaque != "" {
		return nil, errors.New("invalid Redfish URI")
	}
	if ref.User != nil {
		return nil, errors.New("Redfish URI contains user-info")
	}
	fragmentOnly := mode == uriProvenance &&
		ref.Path == "" &&
		ref.RawQuery == "" &&
		ref.Fragment != ""
	if !fragmentOnly && !ref.IsAbs() && ref.Host == "" && !strings.HasPrefix(raw, "/") {
		return nil, errors.New("path-relative Redfish URI is unsupported")
	}
	if err := validateEscapedRedfishPath(ref.EscapedPath()); err != nil {
		return nil, err
	}
	switch mode {
	case uriResource:
		if ref.RawQuery != "" {
			return nil, errors.New("unexpected query in Redfish URI")
		}
		if ref.Fragment != "" {
			return nil, errors.New("unexpected fragment in Redfish resource URI")
		}
	case uriOpaquePage:
		if ref.Fragment != "" {
			return nil, errors.New("unexpected fragment in Redfish page URI")
		}
	case uriProvenance:
		if ref.RawQuery != "" {
			return nil, errors.New("unexpected query in Redfish provenance URI")
		}
		if ref.Fragment != "" && !validJSONPointerFragment(ref.Fragment) {
			return nil, errors.New("Redfish provenance fragment is not a JSON Pointer")
		}
	default:
		return nil, errors.New("invalid Redfish URI policy")
	}

	target := base.ResolveReference(ref)
	if err := validateEscapedRedfishPath(target.EscapedPath()); err != nil {
		return nil, err
	}
	host, err := canonicalHost(target, strings.ToLower(target.Scheme))
	if err != nil {
		return nil, err
	}
	target.Scheme = strings.ToLower(target.Scheme)
	target.Host = host
	if (&url.URL{Scheme: target.Scheme, Host: target.Host}).String() != origin {
		return nil, errors.New("Redfish URI crosses the configured origin")
	}
	if !strings.HasPrefix(target.Path, "/redfish/") ||
		!strings.HasPrefix(target.EscapedPath(), "/redfish/") {
		return nil, errors.New("Redfish URI leaves the Redfish path")
	}
	return target, nil
}

func validateEscapedRedfishPath(value string) error {
	if strings.Contains(value, "\\") ||
		strings.Contains(value, "%") {
		return errors.New("Redfish URI contains an ambiguous escaped path")
	}
	return nil
}

func validJSONPointerFragment(fragment string) bool {
	if fragment == "" {
		return true
	}
	if !strings.HasPrefix(fragment, "/") {
		return false
	}
	for i := 0; i < len(fragment); i++ {
		if fragment[i] != '~' {
			continue
		}
		if i+1 >= len(fragment) || (fragment[i+1] != '0' && fragment[i+1] != '1') {
			return false
		}
		i++
	}
	return true
}

func canonicalProvenanceURI(target *url.URL) string {
	if target == nil {
		return ""
	}
	result := canonicalResourceURI(target)
	if target.Fragment != "" {
		result += "#" + target.EscapedFragment()
	}
	return result
}
