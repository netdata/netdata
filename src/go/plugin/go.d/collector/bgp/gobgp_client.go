// SPDX-License-Identifier: GPL-3.0-or-later

package bgp

import (
	"context"
	"crypto/tls"
	"io"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/tlscfg"
	gobgpapi "github.com/osrg/gobgp/v4/api"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

type goBGPClient struct {
	conn    *grpc.ClientConn
	client  gobgpapi.GoBgpServiceClient
	timeout time.Duration
}

func newGoBGPClient(cfg Config) (*goBGPClient, error) {
	transportCreds, err := newGoBGPTransportCredentials(cfg)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout.Duration())
	defer cancel()

	conn, err := grpc.DialContext(
		ctx,
		cfg.Address,
		grpc.WithTransportCredentials(transportCreds),
		grpc.WithBlock(),
	)
	if err != nil {
		return nil, err
	}

	return &goBGPClient{
		conn:    conn,
		client:  gobgpapi.NewGoBgpServiceClient(conn),
		timeout: cfg.Timeout.Duration(),
	}, nil
}

func (c *goBGPClient) Close() error {
	if c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

func (c *goBGPClient) GetBgp() (*gobgpGlobalInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	resp, err := c.client.GetBgp(ctx, &gobgpapi.GetBgpRequest{})
	if err != nil {
		return nil, err
	}

	return &gobgpGlobalInfo{
		LocalAS:  int64(resp.GetGlobal().GetAsn()),
		RouterID: resp.GetGlobal().GetRouterId(),
	}, nil
}

func (c *goBGPClient) ListPeers() ([]*gobgpPeerInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	stream, err := c.client.ListPeer(ctx, &gobgpapi.ListPeerRequest{EnableAdvertised: true})
	if err != nil {
		return nil, err
	}

	var peers []*gobgpPeerInfo
	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		peers = append(peers, &gobgpPeerInfo{Peer: resp.GetPeer()})
	}

	return peers, nil
}

func (c *goBGPClient) ListRpki() ([]*gobgpRpkiInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	stream, err := c.client.ListRpki(ctx, &gobgpapi.ListRpkiRequest{})
	if err != nil {
		return nil, err
	}

	var servers []*gobgpRpkiInfo
	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		servers = append(servers, &gobgpRpkiInfo{Server: resp.GetServer()})
	}

	return servers, nil
}

func (c *goBGPClient) GetTable(ref *gobgpFamilyRef) (uint64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	tableType, name := gobgpTableRequest(ref)
	resp, err := c.client.GetTable(ctx, &gobgpapi.GetTableRequest{
		TableType: tableType,
		Family:    ref.Family,
		Name:      name,
	})
	if err != nil {
		return 0, err
	}

	return resp.GetNumDestination(), nil
}

func (c *goBGPClient) ListPathValidation(ref *gobgpFamilyRef) (gobgpValidationSummary, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	tableType, name := gobgpTableRequest(ref)
	stream, err := c.client.ListPath(ctx, &gobgpapi.ListPathRequest{
		TableType: tableType,
		Name:      name,
		Family:    ref.Family,
		BatchSize: 1024,
	})
	if err != nil {
		return gobgpValidationSummary{}, err
	}

	var summary gobgpValidationSummary
	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return gobgpValidationSummary{}, err
		}

		path := selectGoBGPValidationPath(resp.GetDestination())
		if path == nil {
			continue
		}

		switch path.GetValidation().GetState() {
		case gobgpapi.ValidationState_VALIDATION_STATE_VALID:
			summary.Valid++
			summary.HasCorrectness = true
		case gobgpapi.ValidationState_VALIDATION_STATE_INVALID:
			summary.Invalid++
			summary.HasCorrectness = true
		case gobgpapi.ValidationState_VALIDATION_STATE_NOT_FOUND:
			summary.NotFound++
			summary.HasCorrectness = true
		}
	}

	return summary, nil
}

func gobgpTableRequest(ref *gobgpFamilyRef) (gobgpapi.TableType, string) {
	if ref != nil && ref.VRF != "" && ref.VRF != "default" {
		return gobgpapi.TableType_TABLE_TYPE_VRF, ref.VRF
	}
	return gobgpapi.TableType_TABLE_TYPE_GLOBAL, ""
}

func newGoBGPTransportCredentials(cfg Config) (credentials.TransportCredentials, error) {
	tlsCfg, err := newGoBGPTLSConfig(cfg)
	if err != nil {
		return nil, err
	}
	if tlsCfg == nil {
		return insecure.NewCredentials(), nil
	}
	return credentials.NewTLS(tlsCfg), nil
}

func newGoBGPTLSConfig(cfg Config) (*tls.Config, error) {
	if cfg.ServerName == "" && cfg.TLSCA == "" && cfg.TLSCert == "" && cfg.TLSKey == "" && !cfg.InsecureSkipVerify {
		return nil, nil
	}

	tlsCfg, err := tlscfg.NewTLSConfig(cfg.TLSConfig)
	if err != nil {
		return nil, err
	}
	if tlsCfg == nil {
		tlsCfg = &tls.Config{Renegotiation: tls.RenegotiateNever}
	}
	if cfg.ServerName != "" {
		tlsCfg.ServerName = cfg.ServerName
	}
	return tlsCfg, nil
}

func selectGoBGPValidationPath(dst *gobgpapi.Destination) *gobgpapi.Path {
	if dst == nil {
		return nil
	}

	var fallback *gobgpapi.Path
	for _, path := range dst.GetPaths() {
		if path == nil || path.GetIsWithdraw() {
			continue
		}
		if path.GetBest() {
			return path
		}
		if fallback == nil {
			fallback = path
		}
	}
	return fallback
}
