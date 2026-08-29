// SPDX-License-Identifier: GPL-3.0-or-later

package redfishsd

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish"
)

const maxServiceRootBytes = 1 << 20

type endpointCandidate struct {
	address netip.Addr
	port    int
	profile ProfileConfig
	url     string
	key     string
}

func makeCandidate(address netip.Addr, port int, profile ProfileConfig) (endpointCandidate, error) {
	host := net.JoinHostPort(address.String(), strconv.Itoa(port))
	rawURL := (&url.URL{Scheme: profile.Scheme, Host: host, Path: "/redfish/v1/"}).String()
	canonical, key, err := redfish.DiscoveryEndpointIdentity(rawURL)
	if err != nil {
		return endpointCandidate{}, err
	}
	return endpointCandidate{
		address: address, port: port, profile: profile, url: canonical, key: key,
	}, nil
}

func probeCandidate(ctx context.Context, candidate endpointCandidate) error {
	client, err := redfish.NewDiscoveryHTTPClient(candidate.profile.ProbeConfig, candidate.url)
	if err != nil {
		return fmt.Errorf("create discovery HTTP client: %w", err)
	}
	defer client.CloseIdleConnections()
	current, err := url.Parse(candidate.url)
	if err != nil {
		return err
	}
	seen := make(map[string]struct{}, 4)
	for redirects := 0; redirects <= 3; redirects++ {
		if _, ok := seen[current.String()]; ok {
			return errors.New("Redfish discovery redirect loop")
		}
		seen[current.String()] = struct{}{}
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, current.String(), nil)
		if err != nil {
			return err
		}
		request.Header.Set("Accept", "application/json")
		request.Header.Set("OData-Version", "4.0")
		response, err := client.Do(request)
		if err != nil {
			return sanitizeProbeTransportError(err)
		}
		if response.StatusCode >= 300 && response.StatusCode <= 399 {
			location := response.Header.Get("Location")
			_ = response.Body.Close()
			if location == "" || redirects == 3 {
				return probeHTTPStatusError("unsupported redirect response", response)
			}
			next, err := redfish.ResolveDiscoveryRedirect(candidate.url, current, location)
			if err != nil {
				return fmt.Errorf("reject Redfish discovery redirect: %w", err)
			}
			current = next
			continue
		}
		if response.StatusCode != http.StatusOK {
			_ = response.Body.Close()
			return probeHTTPStatusError("ServiceRoot", response)
		}
		payload, err := io.ReadAll(io.LimitReader(response.Body, maxServiceRootBytes+1))
		closeErr := response.Body.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		if len(payload) > maxServiceRootBytes {
			return errors.New("ServiceRoot response exceeds discovery limit")
		}
		return redfish.ValidateDiscoveryServiceRoot(payload, current, candidate.url)
	}
	return errors.New("unreachable redirect state")
}

func probeHTTPStatusError(subject string, response *http.Response) error {
	return fmt.Errorf("%s returned HTTP %d", subject, response.StatusCode)
}

func sanitizeProbeTransportError(err error) error {
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	var verificationError *tls.CertificateVerificationError
	var unknownAuthority x509.UnknownAuthorityError
	var hostnameError x509.HostnameError
	var recordHeader tls.RecordHeaderError
	switch {
	case errors.As(err, &verificationError),
		errors.As(err, &unknownAuthority),
		errors.As(err, &hostnameError):
		return errors.New("Redfish discovery TLS certificate verification failed")
	case errors.As(err, &recordHeader):
		return errors.New("Redfish discovery TLS protocol error")
	}
	var networkError net.Error
	if errors.As(err, &networkError) && networkError.Timeout() {
		return errors.New("Redfish discovery transport timed out")
	}
	return errors.New("Redfish discovery transport error")
}
