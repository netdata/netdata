// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"context"
	"net/http"
	"net/url"
	"sync"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
)

type protocolClient struct {
	config      Config
	http        *http.Client
	root        *url.URL
	origin      string
	endpointJob string
	hardwareState
	semMu    sync.RWMutex
	sem      chan struct{}
	families map[string]bool

	authMu          sync.RWMutex
	authMode        string
	token           string
	sessionURI      string
	authInitialized bool
	sessions        []sessionHandle
	refreshMu       sync.Mutex

	baseMu                sync.Mutex
	baseMembership        map[string][]baseResourceIdentity
	graphMu               sync.Mutex
	graphMembership       map[string]graphMembershipSnapshot
	collectionMu          sync.Mutex
	knownCollections      map[string]struct{}
	expansionValue        string
	expansionDisabled     map[string]struct{}
	expansionFallbackSeen bool

	identities identityRegistry

	diagnosticMu             sync.Mutex
	pendingCompatibilityDiag map[string]struct{}
}

func newEndpointClient(cfg Config, client *http.Client) (endpointClient, error) {
	root, origin, err := normalizeServiceRoot(cfg.URL)
	if err != nil {
		return nil, err
	}
	familyMatcher, err := matcher.NewSimplePatternsMatcher(cfg.Collect)
	if err != nil {
		return nil, err
	}
	families := make(map[string]bool, len(collectionFamilies))
	for _, family := range collectionFamilies {
		families[family] = family == "base" || familyMatcher.MatchString(family)
	}
	result := &protocolClient{
		config:            cfg,
		endpointJob:       cfg.Name,
		http:              client,
		root:              root,
		origin:            origin,
		authMode:          cfg.AuthMethod,
		baseMembership:    make(map[string][]baseResourceIdentity),
		graphMembership:   make(map[string]graphMembershipSnapshot),
		knownCollections:  make(map[string]struct{}),
		expansionDisabled: make(map[string]struct{}),
		sem:               make(chan struct{}, cfg.MaxConcurrentRequests),
		families:          families,
	}
	result.hardwareState.initialize()
	return result, nil
}

// Check identifies the endpoint without acquiring remote session state. Session
// credentials are validated when the running collector first authenticates.
func (c *protocolClient) Check(ctx context.Context) error {
	stats := &wireStats{
		failures: make(map[string]int),
	}
	defer c.rememberCompatibilityDiagnostics(stats)
	_, err := c.fetchServiceRoot(ctx, c.config.AuthMethod == "basic", stats)
	return err
}
