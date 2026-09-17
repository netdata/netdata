// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"context"
	"net/http"
	"net/url"
	"sync"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/stmcginnis/gofish"
)

type protocolClient struct {
	config      Config
	http        *http.Client
	root        *url.URL
	origin      string
	endpointJob string
	hardwareState
	sdk          *gofish.APIClient
	authMode     string
	requestLimit int
	families     map[string]bool

	baseMu          sync.Mutex
	baseMembership  map[string][]baseResourceIdentity
	graphMu         sync.Mutex
	graphMembership map[string]graphMembershipSnapshot

	identities identityRegistry
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
		config:          cfg,
		endpointJob:     cfg.Name,
		http:            client,
		root:            root,
		origin:          origin,
		baseMembership:  make(map[string][]baseResourceIdentity),
		graphMembership: make(map[string]graphMembershipSnapshot),
		requestLimit:    cfg.MaxConcurrentRequests,
		families:        families,
	}
	result.hardwareState.initialize()
	return result, nil
}

// Check identifies the endpoint without creating a session. Session credentials
// are validated when collection starts.
func (c *protocolClient) Check(ctx context.Context) error {
	cfg := c.sdkConfig()
	if c.config.AuthMethod == "basic" {
		cfg.Username, cfg.Password, cfg.BasicAuth = c.config.Username, c.config.Password, true
	}
	_, root, err := c.connectSDK(ctx, cfg, nil)
	if err != nil {
		return err
	}
	_, err = c.decodeServiceRoot(root)
	return err
}
