// SPDX-License-Identifier: GPL-3.0-or-later

package bgp

import (
	"fmt"
	"net"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
)

const (
	backendFRR      = "frr"
	backendBIRD     = "bird"
	backendGoBGP    = "gobgp"
	backendOpenBGPD = "openbgpd"
)

const (
	defaultSocketPathFRR  = "/var/run/frr/bgpd.vty"
	defaultSocketPathBIRD = "/var/run/bird.ctl"
	defaultAddressGoBGP   = "127.0.0.1:50051"
)

func (c *Collector) validateConfig() error {
	backend := strings.ToLower(strings.TrimSpace(c.Backend))
	if backend == "" {
		return fmt.Errorf("'backend' is required")
	}
	if backend != backendFRR && backend != backendBIRD && backend != backendGoBGP && backend != backendOpenBGPD {
		return fmt.Errorf("unsupported backend %q", c.Backend)
	}
	socketPath := strings.TrimSpace(c.SocketPath)
	if backend == backendFRR || backend == backendBIRD {
		socketPath = filepath.Clean(socketPath)
		if socketPath == "." {
			socketPath = defaultSocketPathForBackend(backend)
		}
		if backend == backendBIRD && socketPath == defaultSocketPathFRR {
			socketPath = defaultSocketPathBIRD
		}
		if !filepath.IsAbs(socketPath) {
			return fmt.Errorf("'socket_path' must be an absolute path")
		}
	} else {
		socketPath = ""
	}
	address := strings.TrimSpace(c.Address)
	if backend == backendGoBGP {
		if address == "" {
			address = defaultAddressGoBGP
		}
		if _, _, err := net.SplitHostPort(address); err != nil {
			return fmt.Errorf("invalid 'address': %w", err)
		}
	} else {
		address = ""
	}

	zebraSocketPath := strings.TrimSpace(c.ZebraSocketPath)
	if zebraSocketPath == "" {
		zebraSocketPath = filepath.Join(filepath.Dir(socketPath), "zebra.vty")
	}
	if backend == backendFRR {
		zebraSocketPath = filepath.Clean(zebraSocketPath)
		if !filepath.IsAbs(zebraSocketPath) {
			return fmt.Errorf("'zebra_socket_path' must be an absolute path")
		}
	} else {
		zebraSocketPath = ""
	}
	if c.Timeout.Duration() <= 0 {
		return fmt.Errorf("'timeout' must be greater than zero")
	}
	apiURL := strings.TrimRight(strings.TrimSpace(c.APIURL), "/")
	if backend == backendOpenBGPD {
		if apiURL == "" {
			return fmt.Errorf("'api_url' is required for backend %q", backendOpenBGPD)
		}
		u, err := url.Parse(apiURL)
		if err != nil {
			return fmt.Errorf("invalid 'api_url': %w", err)
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			return fmt.Errorf("'api_url' must use http or https")
		}
		if u.Host == "" {
			return fmt.Errorf("'api_url' must include a host")
		}
	}
	if (backend == backendGoBGP || backend == backendOpenBGPD) && c.CollectRIBSummaries && c.RIBSummaryEvery.Duration() <= 0 {
		return fmt.Errorf("'rib_summary_every' must be greater than zero when 'collect_rib_summaries' is enabled")
	}
	if c.MaxFamilies <= 0 {
		return fmt.Errorf("'max_families' must be greater than zero")
	}
	if c.MaxPeers <= 0 {
		return fmt.Errorf("'max_peers' must be greater than zero")
	}
	if c.MaxVNIs <= 0 {
		return fmt.Errorf("'max_vnis' must be greater than zero")
	}
	if c.MaxDeepQueriesPerScrape < 0 {
		return fmt.Errorf("'max_deep_queries_per_scrape' must be zero or greater")
	}

	c.Backend = backend
	c.SocketPath = socketPath
	c.ZebraSocketPath = zebraSocketPath
	c.Address = address
	c.APIURL = apiURL
	return nil
}

func (c *Collector) initSelectMatcher(expr matcher.SimpleExpr) (matcher.Matcher, error) {
	if expr.Empty() {
		return nil, nil
	}
	return expr.Parse()
}

func newBackendClient(cfg Config) (bgpClient, error) {
	switch strings.ToLower(cfg.Backend) {
	case backendFRR:
		return &frrClient{
			socketPath:      cfg.SocketPath,
			zebraSocketPath: cfg.ZebraSocketPath,
			timeout:         cfg.Timeout.Duration(),
		}, nil
	case backendBIRD:
		return &birdClient{
			socketPath: cfg.SocketPath,
			timeout:    cfg.Timeout.Duration(),
		}, nil
	case backendGoBGP:
		return newGoBGPClient(cfg)
	case backendOpenBGPD:
		return newOpenBGPDClient(cfg), nil
	default:
		return nil, fmt.Errorf("unsupported backend %q", cfg.Backend)
	}
}

func defaultSocketPathForBackend(backend string) string {
	switch backend {
	case backendBIRD:
		return defaultSocketPathBIRD
	case backendGoBGP, backendOpenBGPD:
		return ""
	default:
		return defaultSocketPathFRR
	}
}
