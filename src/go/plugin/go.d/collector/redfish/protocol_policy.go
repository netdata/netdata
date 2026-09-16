// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"errors"
	"net/url"
	"strings"
)

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
