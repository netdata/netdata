// SPDX-License-Identifier: GPL-3.0-or-later

package bgp

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	gobgpapi "github.com/osrg/gobgp/v4/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

func TestGoBGPClient(t *testing.T) {
	now := time.Now()
	peers := testGoBGPPeers(now)
	service := &goBGPTestService{
		global: &gobgpapi.Global{
			Asn:      64512,
			RouterId: "192.0.2.254",
		},
		peers: []*gobgpapi.Peer{peers[1].Peer},
		tables: map[string]uint64{
			goBGPRequestKey(gobgpapi.TableType_TABLE_TYPE_VRF, "blue", &gobgpapi.Family{
				Afi:  gobgpapi.Family_AFI_IP6,
				Safi: gobgpapi.Family_SAFI_UNICAST,
			}): 42,
		},
		paths: map[string][]*gobgpapi.Destination{
			goBGPRequestKey(gobgpapi.TableType_TABLE_TYPE_VRF, "blue", &gobgpapi.Family{
				Afi:  gobgpapi.Family_AFI_IP6,
				Safi: gobgpapi.Family_SAFI_UNICAST,
			}): {
				{
					Paths: []*gobgpapi.Path{
						{Best: true, Validation: &gobgpapi.Validation{State: gobgpapi.ValidationState_VALIDATION_STATE_INVALID}},
					},
				},
				{
					Paths: []*gobgpapi.Path{
						{Validation: &gobgpapi.Validation{State: gobgpapi.ValidationState_VALIDATION_STATE_NOT_FOUND}},
					},
				},
				{
					Paths: []*gobgpapi.Path{
						{IsWithdraw: true, Validation: &gobgpapi.Validation{State: gobgpapi.ValidationState_VALIDATION_STATE_VALID}},
					},
				},
			},
		},
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	server := grpc.NewServer()
	gobgpapi.RegisterGoBgpServiceServer(server, service)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() {
		server.Stop()
		_ = listener.Close()
	})

	client, err := newGoBGPClient(Config{
		Address: listener.Addr().String(),
		Timeout: confopt.Duration(time.Second),
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })

	global, err := client.GetBgp()
	require.NoError(t, err)
	assert.Equal(t, int64(64512), global.LocalAS)
	assert.Equal(t, "192.0.2.254", global.RouterID)

	gotPeers, err := client.ListPeers()
	require.NoError(t, err)
	require.Len(t, gotPeers, 1)
	assert.Equal(t, "2001:db8::1", gotPeers[0].Peer.GetState().GetNeighborAddress())

	ref := &gobgpFamilyRef{
		ID:   "blue_ipv6_unicast",
		VRF:  "blue",
		AFI:  "ipv6",
		SAFI: "unicast",
		Family: &gobgpapi.Family{
			Afi:  gobgpapi.Family_AFI_IP6,
			Safi: gobgpapi.Family_SAFI_UNICAST,
		},
	}

	routes, err := client.GetTable(ref)
	require.NoError(t, err)
	assert.Equal(t, uint64(42), routes)

	summary, err := client.ListPathValidation(ref)
	require.NoError(t, err)
	assert.True(t, summary.HasCorrectness)
	assert.Equal(t, int64(0), summary.Valid)
	assert.Equal(t, int64(1), summary.Invalid)
	assert.Equal(t, int64(1), summary.NotFound)

	service.mu.Lock()
	defer service.mu.Unlock()
	require.NotNil(t, service.listPeerReq)
	assert.True(t, service.listPeerReq.GetEnableAdvertised())
	require.NotNil(t, service.getTableReq)
	assert.Equal(t, gobgpapi.TableType_TABLE_TYPE_VRF, service.getTableReq.GetTableType())
	assert.Equal(t, "blue", service.getTableReq.GetName())
	require.NotNil(t, service.listPathReq)
	assert.Equal(t, gobgpapi.TableType_TABLE_TYPE_VRF, service.listPathReq.GetTableType())
	assert.Equal(t, "blue", service.listPathReq.GetName())
	assert.Equal(t, uint64(1024), service.listPathReq.GetBatchSize())
}

type goBGPTestService struct {
	gobgpapi.UnimplementedGoBgpServiceServer

	mu sync.Mutex

	global      *gobgpapi.Global
	peers       []*gobgpapi.Peer
	tables      map[string]uint64
	paths       map[string][]*gobgpapi.Destination
	listPeerReq *gobgpapi.ListPeerRequest
	getTableReq *gobgpapi.GetTableRequest
	listPathReq *gobgpapi.ListPathRequest
}

func (s *goBGPTestService) GetBgp(context.Context, *gobgpapi.GetBgpRequest) (*gobgpapi.GetBgpResponse, error) {
	return &gobgpapi.GetBgpResponse{Global: s.global}, nil
}

func (s *goBGPTestService) ListPeer(req *gobgpapi.ListPeerRequest, stream grpc.ServerStreamingServer[gobgpapi.ListPeerResponse]) error {
	s.mu.Lock()
	s.listPeerReq = req
	s.mu.Unlock()

	for _, peer := range s.peers {
		if err := stream.Send(&gobgpapi.ListPeerResponse{Peer: peer}); err != nil {
			return err
		}
	}
	return nil
}

func (s *goBGPTestService) GetTable(_ context.Context, req *gobgpapi.GetTableRequest) (*gobgpapi.GetTableResponse, error) {
	s.mu.Lock()
	s.getTableReq = req
	s.mu.Unlock()

	return &gobgpapi.GetTableResponse{
		NumDestination: s.tables[goBGPRequestKey(req.GetTableType(), req.GetName(), req.GetFamily())],
	}, nil
}

func (s *goBGPTestService) ListPath(req *gobgpapi.ListPathRequest, stream grpc.ServerStreamingServer[gobgpapi.ListPathResponse]) error {
	s.mu.Lock()
	s.listPathReq = req
	s.mu.Unlock()

	for _, dst := range s.paths[goBGPRequestKey(req.GetTableType(), req.GetName(), req.GetFamily())] {
		if err := stream.Send(&gobgpapi.ListPathResponse{Destination: dst}); err != nil {
			return err
		}
	}
	return nil
}

func goBGPRequestKey(tableType gobgpapi.TableType, name string, family *gobgpapi.Family) string {
	if family == nil {
		return tableType.String() + "|" + name
	}
	return tableType.String() + "|" + name + "|" + family.GetAfi().String() + "/" + family.GetSafi().String()
}
