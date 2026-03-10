// SPDX-License-Identifier: GPL-3.0-or-later

package secretresolver

import (
	"crypto/tls"
	"net/http"
	"time"
)

// Resolver resolves secret references in config maps.
// It owns all provider runtime dependencies (HTTP clients/endpoints).
type Resolver struct {
	awsHTTPClient     *http.Client
	awsIMDSHTTPClient *http.Client
	awsEndpoint       string

	azureHTTPClient     *http.Client
	azureIMDSHTTPClient *http.Client
	azureLoginEndpoint  string

	gcpHTTPClient            *http.Client
	gcpMetadataHTTPClient    *http.Client
	gcpSecretManagerEndpoint string

	vaultHTTPClient         *http.Client
	vaultHTTPClientInsecure *http.Client

	now func() time.Time
}

// New creates a resolver with secure provider defaults.
func New() *Resolver {
	r := &Resolver{}
	r.ensureDefaults()
	return r
}

func (r *Resolver) ensureDefaults() {
	if r.awsHTTPClient == nil {
		r.awsHTTPClient = &http.Client{Timeout: 10 * time.Second}
	}
	if r.awsIMDSHTTPClient == nil {
		r.awsIMDSHTTPClient = &http.Client{
			Timeout:   2 * time.Second,
			Transport: &http.Transport{Proxy: nil}, // IMDS must never be proxied
		}
	}

	if r.azureHTTPClient == nil {
		r.azureHTTPClient = &http.Client{Timeout: 10 * time.Second}
	}
	if r.azureIMDSHTTPClient == nil {
		r.azureIMDSHTTPClient = &http.Client{
			Timeout:   2 * time.Second,
			Transport: &http.Transport{Proxy: nil}, // IMDS must never be proxied
		}
	}

	if r.gcpHTTPClient == nil {
		r.gcpHTTPClient = &http.Client{Timeout: 10 * time.Second}
	}
	if r.gcpMetadataHTTPClient == nil {
		r.gcpMetadataHTTPClient = &http.Client{
			Timeout:   2 * time.Second,
			Transport: &http.Transport{Proxy: nil}, // metadata server must never be proxied
		}
	}

	if r.vaultHTTPClient == nil {
		r.vaultHTTPClient = &http.Client{
			Timeout:       10 * time.Second,
			CheckRedirect: vaultNoRedirect,
		}
	}
	if r.vaultHTTPClientInsecure == nil {
		r.vaultHTTPClientInsecure = &http.Client{
			Timeout:       10 * time.Second,
			CheckRedirect: vaultNoRedirect,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		}
	}

	if r.now == nil {
		r.now = time.Now
	}
}
