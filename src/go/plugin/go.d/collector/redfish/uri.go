// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strconv"
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
	if (&url.URL{
		Scheme: target.Scheme,
		Host:   target.Host,
	}).String() != origin {
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

func normalizeServiceRoot(raw string) (*url.URL, string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, "", errors.New("is required")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, "", errors.New("invalid URL syntax")
	}
	if u.Opaque != "" || u.Scheme == "" || u.Host == "" {
		return nil, "", errors.New("must be an absolute HTTP or HTTPS URL")
	}
	if u.User != nil {
		return nil, "", errors.New("must not contain user-info")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return nil, "", errors.New("must not contain a query or fragment")
	}
	if u.RawPath != "" {
		return nil, "", errors.New("must not contain an encoded path")
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return nil, "", errors.New("scheme must be http or https")
	}
	switch u.EscapedPath() {
	case "", "/", "/redfish/v1", "/redfish/v1/":
	default:
		return nil, "", errors.New("path must be empty, /, /redfish/v1, or /redfish/v1/")
	}

	host, err := canonicalHost(u, scheme)
	if err != nil {
		return nil, "", err
	}
	origin := (&url.URL{
		Scheme: scheme,
		Host:   host,
	}).String()
	return &url.URL{
		Scheme: scheme,
		Host:   host,
		Path:   "/redfish/v1/",
	}, origin, nil
}

func canonicalHost(u *url.URL, scheme string) (string, error) {
	hostname := u.Hostname()
	if hostname == "" {
		return "", errors.New("host is required")
	}
	port := u.Port()
	if port != "" {
		value, err := strconv.Atoi(port)
		if err != nil || value < 1 || value > 65535 {
			return "", fmt.Errorf("invalid port %q", port)
		}
		port = strconv.Itoa(value)
	}

	addressHost := hostname
	if addr, err := netip.ParseAddr(hostname); err == nil {
		zone := addr.Zone()
		addr = addr.Unmap()
		if addr.Is4() && zone != "" {
			return "", errors.New("IPv4 host must not contain an interface zone")
		}
		if addr.Is6() && addr.IsLinkLocalUnicast() && addr.Zone() == "" {
			return "", errors.New("link-local IPv6 host requires an interface zone")
		}
		addressHost = addr.String()
	} else {
		if strings.Contains(hostname, "%") {
			return "", errors.New("DNS host must not contain a percent escape")
		}
		addressHost = strings.TrimSuffix(strings.ToLower(addressHost), ".")
		if addressHost == "" {
			return "", errors.New("host is required")
		}
	}

	if port == "80" && scheme == "http" || port == "443" && scheme == "https" {
		port = ""
	}
	if strings.Contains(addressHost, ":") {
		if port == "" {
			return "[" + addressHost + "]", nil
		}
		return net.JoinHostPort(addressHost, port), nil
	}
	if port != "" {
		return net.JoinHostPort(addressHost, port), nil
	}
	return addressHost, nil
}

func sameResourceIdentity(left, right string) bool {
	if left == right {
		return true
	}
	return strings.TrimSuffix(left, "/") == strings.TrimSuffix(right, "/") &&
		strings.TrimSuffix(left, "/") == "/redfish/v1"
}

func (c *protocolClient) resolveURI(base *url.URL, raw string, allowQuery bool) (*url.URL, error) {
	mode := uriResource
	if allowQuery {
		mode = uriOpaquePage
	}
	return resolveRedfishURI(c.origin, base, raw, mode)
}

func canonicalResourceURI(target *url.URL) string {
	copy := *target
	copy.RawQuery = ""
	copy.Fragment = ""
	return copy.EscapedPath()
}

const maxURIBytes = 8192
