// SPDX-License-Identifier: GPL-3.0-or-later

package acquisition

import (
	"context"
	"net/http"
	"sync"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/internal/identity"
	"github.com/stmcginnis/gofish"
)

type Client struct {
	connection
	sdk          *gofish.APIClient
	authMode     string
	requestLimit int
	families     map[string]bool

	baseMu          sync.Mutex
	baseMembership  map[string][]baseResourceIdentity
	graphMu         sync.Mutex
	graphMembership map[string]graphMembershipSnapshot

	identities identity.Registry
}

// New owns the Redfish transport policy on the supplied dedicated HTTP client.
// Calls to Check, Acquire and Close are serialized by the collector lifecycle.
func New(cfg Options, client *http.Client) (*Client, error) {
	root, origin, err := NormalizeServiceRoot(cfg.URL)
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
	result := &Client{
		connection: connection{
			config: cfg,
			http:   client,
			root:   root,
			origin: origin,
		},
		baseMembership:  make(map[string][]baseResourceIdentity),
		graphMembership: make(map[string]graphMembershipSnapshot),
		requestLimit:    cfg.MaxConcurrentRequests,
		families:        families,
	}
	transport := &redfishTransport{
		admission: newRequestAdmission(cfg.MaxConcurrentRequests),
		base:      client.Transport,
		root:      root,
		origin:    origin,
	}
	client.Transport = transport
	client.CheckRedirect = transport.checkRedirect
	return result, nil
}

// Check identifies the endpoint without creating a session. Session credentials
// are validated when collection starts.
func (c *Client) Check(ctx context.Context) error {
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
